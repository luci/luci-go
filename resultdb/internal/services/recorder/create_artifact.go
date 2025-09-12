// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package recorder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Headers that can be used with the streaming artifact creation endpoint.
const (
	// The hash of the uploaded artifact, of the form:
	// sha256:<hexadecimal string>
	// Required.
	artifactContentHashHeaderKey = "Content-Hash"
	// The length of the uploaded artifact. Required.
	artifactContentSizeHeaderKey = "Content-Length"
	// The MIME type of the uploaded artifact. Optional.
	artifactContentTypeHeaderKey = "Content-Type"
	// The work unit or invocation update token. Required.
	updateTokenHeaderKey = "Update-Token"
	// The artifact type of the uploaded artifact. Optional.
	artifactTypeHeaderKey = "Artifact-Type"

	// The following header values are URL encoded, even if for some it is not
	// strictly necessary, to allow expanding the accepted character
	// alphabet for these fields in future without breaking API compatibility.

	// The test module name.
	// See test_id_structured.module_name in `CreateArtifactRequest`.
	testModuleNameHeaderKey = "Test-Module-Name" // URL encoded value

	// The test module scheme.
	// See test_id_structured.module_scheme in `CreateArtifactRequest`.
	testModuleSchemeHeaderKey = "Test-Module-Scheme" // URL encoded value

	// The test module variant.
	// Each header captures one or more URL-encoded test variant key-value pairs,
	// each such pair separated by a comma. For example, the variant:
	// {
	//    "key": "value",
	//    "key2": "value2",
	//    "key3": "special value containing symbols :,@$",
	// }
	// can be represented as:
	//
	// Test-Module-Variant: key%3Avalue, key2%3Avalue2, special%20value%20containing%20symbols%20%3A%2C%40%24
	//
	// or
	//
	// Test-Module-Variant: key%3Avalue, key2%3Avalue2
	// Test-Module-Variant: special%20value%20containing%20symbols%20%3A%2C%40%24
	//
	// or some other combination to remain within header length and count limits. See also HTTP "Accept"
	// header as a canonical example of a HTTP header that accepts multiple values.
	testVariantHeaderKey = "Test-Module-Variant"

	// The test coarse name.
	testCoarseNameHeaderKey = "Test-Coarse-Name" // URL encoded value

	// The test fine name.
	testFineNameHeaderKey = "Test-Fine-Name" // URL encoded value

	// The test case name.
	testCaseNameHeaderKey = "Test-Case-Name" // URL encoded value

	// The test result ID, see also result_id in `CreateArtifactRequest`.
	resultIDHeaderKey = "Result-ID" // URL encoded value

	// The artifact ID, see also artifact_id in `CreateArtifactRequest`.
	artifactIDHeaderKey = "Artifact-ID" // URL encoded value
)

// artifactCreationHandler can handle artifact creation requests.
//
// Request:
//   - Router parameter "artifact" MUST be a valid artifact name.
//   - The request body MUST be the artifact contents.
//   - The request MUST include an Update-Token header with the value of
//     invocation's update token.
//   - The request MUST include a Content-Length header. It must be <= MaxArtifactContentSize.
//   - The request MUST include a Content-Hash header with value "sha256:{hash}"
//     where {hash} is a lower-case hex-encoded SHA256 hash of the artifact
//     contents.
//   - The request SHOULD have a Content-Type header.
type artifactCreationHandler struct {
	// RBEInstance is the full name of the RBE instance used for artifact storage.
	// Format: projects/{project}/instances/{instance}.
	RBEInstance                    string
	NewCASWriter                   func(context.Context) (bytestream.ByteStream_WriteClient, error)
	MaxArtifactContentStreamLength int64
	bufSize                        int
}

// HandlePUT implements router.Handler.
// Invocation artifact uploads happen via this path. It expects the full artifact
// resource name in the URL and implements idempotent artifact creation semantics
// (create or replace).
func (h *artifactCreationHandler) HandlePUT(c *router.Context) {
	ac := &artifactCreator{artifactCreationHandler: h}
	mw := artifactcontent.NewMetricsWriter(c)
	defer func() {
		mw.Upload(c.Request.Context(), ac.size)
	}()

	const isLegacyEndpoint = true
	err := ac.handle(c, isLegacyEndpoint)
	st, ok := appstatus.Get(err)
	switch {
	case ok:
		logging.Warningf(c.Request.Context(), "Responding with %s: %s", st.Code(), err)
		http.Error(c.Writer, st.Message(), grpcutil.CodeStatus(st.Code()))
	case err != nil:
		logging.Errorf(c.Request.Context(), "Internal server error: %s", err)
		http.Error(c.Writer, "Internal server error", http.StatusInternalServerError)
	default:
		// For PUT requests, status 204 (no content) is the normal success code.
		c.Writer.WriteHeader(http.StatusNoContent)
	}
}

// HandlePOST implements router.Handler.
// Work unit artifact uploads happen via this path. It expects the work unit resource name
// in the URL only. The test identifier, result id (if any) and artifact id are passed
// as HTTP headers, leaving the server to assign the resource name. (This saves the
// client encoding the structured test ID into a flat test ID, which can be non-trivial.)
//
// Like HandlePUT, ths method is idempotent. However, the verb POST is used as the
// final URL (artifact resource name) of the uploaded artifact is not the work unit
// resource name in the request, making the PUT verb an inappropriate choice.
func (h *artifactCreationHandler) HandlePOST(c *router.Context) {
	ac := &artifactCreator{artifactCreationHandler: h}
	mw := artifactcontent.NewMetricsWriter(c)
	defer func() {
		mw.Upload(c.Request.Context(), ac.size)
	}()

	const isLegacyEndpoint = false
	err := ac.handle(c, isLegacyEndpoint)
	st, ok := appstatus.Get(err)
	switch {
	case ok:
		logging.Warningf(c.Request.Context(), "Responding with %s: %s", st.Code(), err)
		http.Error(c.Writer, st.Message(), grpcutil.CodeStatus(st.Code()))
	case err != nil:
		logging.Errorf(c.Request.Context(), "Internal server error: %s", err)
		http.Error(c.Writer, "Internal server error", http.StatusInternalServerError)
	default:
		// For POST requests, status 201 (created) is the normal success code.
		c.Writer.WriteHeader(http.StatusCreated)
	}
}

// artifactCreator handles one artifact creation request.
type artifactCreator struct {
	*artifactCreationHandler
	size int64
}

func (ac *artifactCreator) handle(c *router.Context, isLegacyEndpoint bool) error {
	ctx := c.Request.Context()

	// Parse and validate the request.
	req, err := parseStreamingArtifactUploadRequest(c, isLegacyEndpoint, ac.MaxArtifactContentStreamLength)
	if err != nil {
		return err
	}
	ac.size = req.size

	// Read and verify the current state.
	sameExists, err := verifyArtifactStateBeforeWriting(ctx, req)
	if err != nil {
		return err
	}
	if sameExists {
		// Exit early. We don't need to see the artifact contents.
		return nil
	}

	// Read the request body through a digest verifying proxy.
	// This is mandatory because RBE-CAS does not guarantee digest verification in
	// all cases.
	ver := &digestVerifier{
		r:            c.Request.Body,
		expectedHash: artifacts.TrimHashPrefix(req.rbeCASHash),
		expectedSize: req.size,
		actualHash:   sha256.New(),
	}

	// Forward the request body to RBE-CAS.
	if err := ac.writeToCAS(ctx, req, ver); err != nil {
		return errors.Fmt("failed to write to CAS: %w", err)
	}

	if err := ver.ReadVerify(ctx); err != nil {
		return err
	}

	reqs := []*artifactCreationRequest{req}
	_, err = createArtifactStates(ctx, reqs)
	if err != nil {
		return removeRequestNumberFromAppStatusError(err)
	}
	return nil
}

// writeToCAS writes contents in r to RBE-CAS.
// ac.hash and ac.size must match the contents.
func (ac *artifactCreator) writeToCAS(ctx context.Context, a *artifactCreationRequest, r io.Reader) (err error) {
	ctx, overallSpan := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.artifactCreator.writeToCAS")
	defer func() { tracing.End(overallSpan, err) }()

	// Protocol:
	// https://github.com/bazelbuild/remote-apis/blob/7802003e00901b4e740fe0ebec1243c221e02ae2/build/bazel/remote/execution/v2/remote_execution.proto#L193-L205
	// https://github.com/googleapis/googleapis/blob/c8e291e6a4d60771219205b653715d5aeec3e96b/google/bytestream/bytestream.proto#L55

	w, err := ac.NewCASWriter(ctx)
	if err != nil {
		return errors.Fmt("failed to create a CAS writer: %w", err)
	}
	defer w.CloseSend()

	bufSize := ac.bufSize
	if bufSize == 0 {
		bufSize = 1024 * 1024
		if bufSize > int(a.size) {
			bufSize = int(a.size)
		}
	}
	buf := make([]byte, bufSize)

	// Copy data from r to w using buffer buf.
	// Include the resource name only in the first request.
	first := true
	bytesSent := 0
	for {
		_, readSpan := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.artifactCreator.writeToCAS.readChunk")
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			tracing.End(readSpan, err)
			if err != io.ErrUnexpectedEOF {
				return errors.Fmt("failed to read artifact contents: %w", err)
			}
			return appstatus.BadRequest(errors.Fmt("failed to read artifact contents: %w", err))
		}
		tracing.End(readSpan, nil, attribute.Int("size", n))
		last := err == io.EOF

		// Prepare the request.
		// WriteRequest message: https://github.com/googleapis/googleapis/blob/c8e291e6a4d60771219205b653715d5aeec3e96b/google/bytestream/bytestream.proto#L128
		req := &bytestream.WriteRequest{
			Data:        buf[:n],
			FinishWrite: last,
			WriteOffset: int64(bytesSent),
		}

		// Include the resource name only in the first request.
		if first {
			first = false
			req.ResourceName = ac.genWriteResourceName(ctx, a)
		}

		// Send the request.
		_, writeSpan := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.artifactCreator.writeToCAS.writeChunk",
			attribute.Int("size", n),
		)
		// Do not shadow err! It is checked below again.
		if err = w.Send(req); err != nil && err != io.EOF {
			tracing.End(writeSpan, err)
			return errors.Fmt("failed to write data to RBE-CAS: %w", err)
		}
		tracing.End(writeSpan, nil)
		bytesSent += n
		if last || err == io.EOF {
			// Either this was the last chunk, or server closed the stream.
			break
		}
	}

	// Read and interpret the response.
	switch res, err := w.CloseAndRecv(); {
	case status.Code(err) == codes.InvalidArgument:
		logging.Warningf(ctx, "RBE-CAS responded with %s", err)
		return appstatus.Errorf(codes.InvalidArgument, "Content-Hash and/or Content-Length do not match the request body")
	case err != nil:
		return errors.Fmt("failed to read RBE-CAS write response: %w", err)
	case res.CommittedSize == a.size:
		return nil
	default:
		return errors.Fmt("unexpected blob commit size %d, expected %d", res.CommittedSize, a.size)
	}
}

// genWriteResourceName generates a random resource name that can be used
// to write the blob to RBE-CAS.
func (ac *artifactCreator) genWriteResourceName(ctx context.Context, req *artifactCreationRequest) string {
	uuidBytes := make([]byte, 16)
	if _, err := mathrand.Read(ctx, uuidBytes); err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		"%s/uploads/%s/blobs/%s/%d",
		ac.RBEInstance,
		uuid.Must(uuid.FromBytes(uuidBytes)),
		artifacts.TrimHashPrefix(req.rbeCASHash),
		req.size)
}

// parseStreamingArtifactUploadRequest parses an artifactCreationRequest based on the HTTP request.
// The request payload is not parsed so that it can be handled in streaming fashion; as such
// the `data` field is not populated.
//
// isLegacyEndpoint indicates whether the endpoint being handled is the legacy PUT endpoint that
// accepts the full artifact resource name as the URL instead of the newer POST endpoint that
// uses the parent invocation / work unit name as the URL.
func parseStreamingArtifactUploadRequest(c *router.Context, isLegacyEndpoint bool, maxStreamLength int64) (*artifactCreationRequest, error) {
	cfg, err := config.Service(c.Request.Context())
	if err != nil {
		return nil, err
	}

	var workUnitID workunits.ID
	var invocationID invocations.ID
	var testID string
	var resultID string
	var artifactID string

	// The structured test ID corresponding to `testID`.
	// This may be missing the ModuleVariant.
	var testIDStructured *pb.TestIdentifier

	if isLegacyEndpoint {
		// Read the artifact name.
		// We must use EscapedPath(), not Path, to preserve test ID's own encoding.
		artifactName := strings.TrimPrefix(c.Request.URL.EscapedPath(), "/")

		// Parse and validate the artifact name.
		var invIDString string
		invIDString, testID, resultID, artifactID, err = pbutil.ParseLegacyArtifactName(artifactName)
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "URL: bad artifact name: %s", err)
		}
		invocationID = invocations.ID(invIDString)

		// Validate the test ID with respect to the configured test schemes.
		if testID != "" {
			testIDBase, err := pbutil.ParseAndValidateTestID(testID)
			if err != nil {
				panic("logic error: ParseLegacyArtifactName did not validate the TestID")
			}
			if err := validateTestIDToScheme(cfg, testIDBase); err != nil {
				return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: test_id_structured: %s", err)
			}
			testIDStructured = &pb.TestIdentifier{
				ModuleName:   testIDBase.ModuleName,
				ModuleScheme: testIDBase.ModuleScheme,
				// Use nil instead of &pb.Variant{} to indicate we don't know the variant,
				// as opposed to the variant being the empty variant.
				ModuleVariant: nil,
				CoarseName:    testIDBase.CoarseName,
				FineName:      testIDBase.FineName,
				CaseName:      testIDBase.CaseName,
			}
		}
	} else {
		// The endpoint should be:
		// - invocations/{INVOCATION_ID}/artifacts
		// - rootInvocations/{ROOT_INVOCATION_ID}/workUnits/{WORK_UNIT_ID}/artifacts
		// To stick closely to aip.dev/133.
		collectionName := strings.TrimPrefix(c.Request.URL.EscapedPath(), "/")
		workUnitOrInvocationName, found := strings.CutSuffix(collectionName, "/artifacts")
		if !found {
			return nil, appstatus.Errorf(codes.InvalidArgument, "URL: expected suffix '/artifacts' for artifacts upload endpoint")
		}

		// Read the work unit or invocation name from the URL.
		if strings.HasPrefix(workUnitOrInvocationName, "invocations/") {
			var invIDString string
			invIDString, err = pbutil.ParseInvocationName(workUnitOrInvocationName)
			if err != nil {
				return nil, appstatus.Errorf(codes.InvalidArgument, "URL: bad invocation name: %s", err)
			}
			invocationID = invocations.ID(invIDString)
		} else {
			workUnitID, err = workunits.ParseName(workUnitOrInvocationName)
			if err != nil {
				return nil, appstatus.Errorf(codes.InvalidArgument, "URL: bad work unit name: %s", err)
			}
		}

		// Read test ID, result ID and artifact ID from request headers.
		testIDStructured, err = parseOptionalTestIDHeaders(c.Request.Header)
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "%s", err)
		}

		resultID, err = readRequestHeaderUnescaped(c.Request.Header, resultIDHeaderKey)
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "%s header: %s", resultIDHeaderKey, err)
		}

		// Validate the test ID and result ID.
		if testIDStructured != nil {
			if err := pbutil.ValidateStructuredTestIdentifierForStorage(testIDStructured); err != nil {
				return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: test_id_structured: %s", err)
			}
			testIDBase := pbutil.ExtractBaseTestIdentifier(testIDStructured)
			if err := validateTestIDToScheme(cfg, testIDBase); err != nil {
				return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: test_id_structured: %s", err)
			}
			testID = pbutil.EncodeTestID(testIDBase)

			if resultID == "" {
				return nil, appstatus.Errorf(codes.InvalidArgument, "%s header is missing", resultIDHeaderKey)
			}
			if err := pbutil.ValidateResultID(resultID); err != nil {
				return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: result_id: %s", err)
			}
		} else {
			if resultID != "" {
				return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: result_id: specified, but no test ID is specified; did you forget to set %s and other Test- headers?", testModuleNameHeaderKey)
			}
		}

		artifactID, err = readRequestHeaderUnescaped(c.Request.Header, artifactIDHeaderKey)
		if err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "%s header: %s", artifactIDHeaderKey, err)
		}
		if artifactID == "" {
			return nil, appstatus.Errorf(codes.InvalidArgument, "%s header is missing", artifactIDHeaderKey)
		}
		if err := pbutil.ValidateArtifactID(artifactID); err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: artifact_id: %s", err)
		}
	}

	// Parse and validate the update token.
	updateToken := c.Request.Header.Get(updateTokenHeaderKey)
	if updateToken == "" {
		return nil, appstatus.Errorf(codes.Unauthenticated, "%s header is missing", updateTokenHeaderKey)
	}
	if invocationID != "" {
		if err := validateInvocationToken(c.Request.Context(), updateToken, invocationID); err != nil {
			return nil, appstatus.Errorf(codes.PermissionDenied, "invalid %s header value", updateTokenHeaderKey)
		}
	} else {
		if err := validateWorkUnitUpdateToken(c.Request.Context(), updateToken, workUnitID); err != nil {
			return nil, appstatus.Errorf(codes.PermissionDenied, "invalid %s header value", updateTokenHeaderKey)
		}
	}

	// Parse and validate the hash.
	var rbeCASHash string
	switch rbeCASHash = c.Request.Header.Get(artifactContentHashHeaderKey); {
	case rbeCASHash == "":
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s header is missing", artifactContentHashHeaderKey)
	case !artifacts.ContentHashRe.MatchString(rbeCASHash):
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s header value does not match %s", artifactContentHashHeaderKey, artifacts.ContentHashRe)
	}

	// Parse and validate the size.
	sizeHeader := c.Request.Header.Get(artifactContentSizeHeaderKey)
	if sizeHeader == "" {
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s header is missing", artifactContentSizeHeaderKey)
	}

	var size int64
	switch size, err = strconv.ParseInt(sizeHeader, 10, 64); {
	case err != nil:
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s header is malformed: %s", artifactContentSizeHeaderKey, err)
	case size < 0 || size > maxStreamLength:
		return nil, appstatus.Errorf(codes.InvalidArgument, "%s header must be a value between 0 and %d", artifactContentSizeHeaderKey, maxStreamLength)
	}

	contentType := c.Request.Header.Get(artifactContentTypeHeaderKey)
	if contentType != "" {
		if _, _, err := mime.ParseMediaType(contentType); err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: content_type: %s", err)
		}
	}

	artifactType := c.Request.Header.Get(artifactTypeHeaderKey)

	if artifactType != "" {
		if err := pbutil.ValidateArtifactType(artifactType); err != nil {
			return nil, appstatus.Errorf(codes.InvalidArgument, "artifact: artifact_type: %s", err)
		}
	}

	return &artifactCreationRequest{
		workUnitID:       workUnitID,
		invocationID:     invocationID,
		testID:           testID,
		resultID:         resultID,
		artifactID:       artifactID,
		contentType:      contentType,
		artifactType:     artifactType,
		rbeCASHash:       rbeCASHash,
		size:             size,
		testIDStructured: testIDStructured,
	}, nil
}

func parseOptionalTestIDHeaders(h http.Header) (testID *pb.TestIdentifier, err error) {
	moduleName, err := readRequestHeaderUnescaped(h, testModuleNameHeaderKey)
	if err != nil {
		return nil, fmt.Errorf("%s header: %s", testModuleNameHeaderKey, err)
	}
	moduleScheme, err := readRequestHeaderUnescaped(h, testModuleSchemeHeaderKey)
	if err != nil {
		return nil, fmt.Errorf("%s header: %s", testModuleSchemeHeaderKey, err)
	}
	coarseName, err := readRequestHeaderUnescaped(h, testCoarseNameHeaderKey)
	if err != nil {
		return nil, fmt.Errorf("%s header: %s", testCoarseNameHeaderKey, err)
	}
	fineName, err := readRequestHeaderUnescaped(h, testFineNameHeaderKey)
	if err != nil {
		return nil, fmt.Errorf("%s header: %s", testFineNameHeaderKey, err)
	}
	caseName, err := readRequestHeaderUnescaped(h, testCaseNameHeaderKey)
	if err != nil {
		return nil, fmt.Errorf("%s header: %s", testCaseNameHeaderKey, err)
	}
	variant, err := parseTestVariantHeaders(h)
	if err != nil {
		return nil, fmt.Errorf("%s header: %s", testVariantHeaderKey, err)
	}

	// If any part is present, then all parts must be present.
	hasTest := moduleName != "" || moduleScheme != "" || coarseName != "" || fineName != "" || caseName != "" || len(variant.Def) > 0
	if hasTest {
		// If the test ID is set, validate it.
		if moduleName == "" {
			return nil, fmt.Errorf("%s header is missing (only part of a test ID is specified)", testModuleNameHeaderKey)
		}
		if moduleScheme == "" {
			return nil, fmt.Errorf("%s header is missing (only part of a test ID is specified)", testModuleSchemeHeaderKey)
		}
		// Coarse and Fine name are not validated because they are optional fields.
		// They are not required for all test schemes.
		if caseName == "" {
			return nil, fmt.Errorf("%s header is missing (only part of a test ID is specified)", testCaseNameHeaderKey)
		}
		// A variant with no keys is valid.

		return &pb.TestIdentifier{
			ModuleName:    moduleName,
			ModuleScheme:  moduleScheme,
			ModuleVariant: variant,
			CoarseName:    coarseName,
			FineName:      fineName,
			CaseName:      caseName,
		}, nil
	}
	return nil, nil
}

func parseTestVariantHeaders(h http.Header) (*pb.Variant, error) {
	rawValues := h.Values(testVariantHeaderKey)
	var keyValuePairs []string
	for _, rawValue := range rawValues {
		// Golang may not split headers on a comma, such as if there is not a space that follows.
		// Manually split the values.
		for _, part := range strings.Split(rawValue, ",") {
			escaped := strings.TrimSpace(part)
			unescaped, err := url.PathUnescape(escaped)
			if err != nil {
				return nil, fmt.Errorf("invalid key-value pair: %q", escaped)
			}
			keyValuePairs = append(keyValuePairs, unescaped)
		}
	}

	v, err := pbutil.VariantFromStrings(keyValuePairs)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func readRequestHeaderUnescaped(h http.Header, key string) (string, error) {
	escaped := h.Get(key)
	if escaped == "" {
		return "", nil
	}
	unescaped, err := url.PathUnescape(escaped)
	if err != nil {
		return "", errors.Fmt("invalid URL-escaped value %q: %w", escaped, err)
	}
	return unescaped, nil
}

// verifyArtifactStateBeforeWriting checks if the Spanner state is compatible with creation of the
// artifact. If an identical artifact already exists, sameAlreadyExists is true.
func verifyArtifactStateBeforeWriting(ctx context.Context, art *artifactCreationRequest) (sameAlreadyExists bool, err error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Reuse the implementation of BatchCreateArtifacts here to save on
	// code duplication.
	arts := []*artifactCreationRequest{art}

	// Verify work unit/invocation state.
	wuInfos, invInfo, err := checkInvocationOrWorkUnitState(ctx, arts)
	if err != nil {
		return false, removeRequestNumberFromAppStatusError(err) // NotFound, FailedPrecondition or internal error.
	}

	// Verify the request is valid for the current system state.
	err = validateBatchCreateArtifactsRequestForSystemState(ctx, wuInfos, invInfo, arts)
	if err != nil {
		// Strip any references to "requests[0]: " in the returned error.
		// This is a single artifact create method, not the batch create method.
		return false, removeRequestNumberFromAppStatusError(err)
	}

	// Find the new artifacts.
	noStateArts, err := findNewArtifacts(ctx, arts)
	if err != nil {
		// Strip any references to "requests[0]: " in the returned error.
		// This is a single artifact create method, not the batch create method.
		return false, removeRequestNumberFromAppStatusError(err)
	}
	sameAlreadyExists = len(noStateArts) == 0

	return sameAlreadyExists, nil
}

// removeRequestNumberFromAppStatusError removes the annotation "requests[0]: "
// at the start of an appstatus error. This is useful where batch validation
// methods are called from the single version of the same RPC.
func removeRequestNumberFromAppStatusError(err error) error {
	st, ok := appstatus.Get(err)
	if ok {
		msg := st.Message()
		if strings.HasPrefix(msg, "requests[0]: ") || strings.HasPrefix(msg, "bad request: requests[0]: ") {
			msg = strings.Replace(msg, "requests[0]: ", "", 1)
		}
		return appstatus.Error(st.Code(), msg)
	}
	return err
}

// digestVerifier is an io.Reader that also verifies the digest.
type digestVerifier struct {
	r            io.Reader
	expectedSize int64
	expectedHash string

	actualSize int64
	actualHash hash.Hash
}

func (v *digestVerifier) Read(p []byte) (n int, err error) {
	n, err = v.r.Read(p)
	v.actualSize += int64(n)
	v.actualHash.Write(p[:n])
	return n, err
}

// ReadVerify reads through the rest of the v.r
// and returns a non-nil error if the content have unexpected hash or size.
// The error may be annotated with appstatus.
func (v *digestVerifier) ReadVerify(ctx context.Context) (err error) {
	_, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.digestVerifier.ReadVerify")
	defer func() { tracing.End(ts, err) }()

	// Read until the end.
	if _, err := io.Copy(io.Discard, v); err != nil {
		return err
	}

	// Verify size.
	if v.actualSize != v.expectedSize {
		return appstatus.Errorf(
			codes.InvalidArgument,
			"Content-Length header value %d does not match the length of the request body, %d",
			v.expectedSize,
			v.actualSize,
		)
	}

	// Verify hash.
	hashFromBody := hex.EncodeToString(v.actualHash.Sum(nil))
	hashFromHeader := v.expectedHash
	if hashFromBody != hashFromHeader {
		return appstatus.Errorf(
			codes.InvalidArgument,
			`Content-Hash header value "%s" does not match the hash of the request body, "%s"`,
			artifacts.AddHashPrefix(hashFromHeader),
			artifacts.AddHashPrefix(hashFromBody),
		)
	}

	return nil
}
