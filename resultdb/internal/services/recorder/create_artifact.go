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
	"net/http"
	"strconv"
	"strings"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifactcontent"
	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	artifactContentHashHeaderKey = "Content-Hash"
	artifactContentSizeHeaderKey = "Content-Length"
	artifactContentTypeHeaderKey = "Content-Type"
	updateTokenHeaderKey         = "Update-Token"
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

// Handle implements router.Handler.
func (h *artifactCreationHandler) Handle(c *router.Context) {
	ac := &artifactCreator{artifactCreationHandler: h}
	mw := artifactcontent.NewMetricsWriter(c)
	defer func() {
		mw.Upload(c.Request.Context(), ac.size)
	}()

	err := ac.handle(c)
	st, ok := appstatus.Get(err)
	switch {
	case ok:
		logging.Warningf(c.Request.Context(), "Responding with %s: %s", st.Code(), err)
		http.Error(c.Writer, st.Message(), grpcutil.CodeStatus(st.Code()))
	case err != nil:
		logging.Errorf(c.Request.Context(), "Internal server error: %s", err)
		http.Error(c.Writer, "Internal server error", http.StatusInternalServerError)
	default:
		c.Writer.WriteHeader(http.StatusNoContent)
	}
}

// artifactCreator handles one artifact creation request.
type artifactCreator struct {
	*artifactCreationHandler

	artifactName  string
	invID         invocations.ID
	testID        string
	resultID      string
	artifactID    string
	localParentID string
	contentType   string

	hash string
	size int64
}

func (ac *artifactCreator) handle(c *router.Context) error {
	ctx := c.Request.Context()

	// Parse and validate the request.
	if err := ac.parseRequest(c); err != nil {
		return err
	}

	// Read and verify the current state.
	switch sameExists, err := ac.verifyStateBeforeWriting(ctx); {
	case err != nil:
		return err
	case sameExists:
		return nil
	}

	// Read the request body through a digest verifying proxy.
	// This is mandatory because RBE-CAS does not guarantee digest verification in
	// all cases.
	ver := &digestVerifier{
		r:            c.Request.Body,
		expectedHash: artifacts.TrimHashPrefix(ac.hash),
		expectedSize: ac.size,
		actualHash:   sha256.New(),
	}

	// Forward the request body to RBE-CAS.
	if err := ac.writeToCAS(ctx, ver); err != nil {
		return errors.Annotate(err, "failed to write to CAS").Err()
	}

	if err := ver.ReadVerify(ctx); err != nil {
		return err
	}

	// Record the artifact in Spanner.
	var realm string
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) (err error) {
		// Verify the state again.
		var sameExists bool
		realm, sameExists, err = ac.verifyState(ctx)
		switch {
		case err != nil:
			return err
		case sameExists:
			return nil
		}

		span.BufferWrite(ctx, spanutil.InsertMap("Artifacts", map[string]any{
			"InvocationId": ac.invID,
			"ParentId":     ac.localParentID,
			"ArtifactId":   ac.artifactID,
			"ContentType":  ac.contentType,
			"Size":         ac.size,
			"RBECASHash":   ac.hash,
		}))
		return nil
	})
	if err != nil {
		return err
	}
	spanutil.IncRowCount(ctx, 1, spanutil.Artifacts, spanutil.Inserted, realm)
	return nil
}

// writeToCAS writes contents in r to RBE-CAS.
// ac.hash and ac.size must match the contents.
func (ac *artifactCreator) writeToCAS(ctx context.Context, r io.Reader) (err error) {
	ctx, overallSpan := tracing.Start(ctx, "resultdb.writeToCAS")
	defer func() { tracing.End(overallSpan, err) }()

	// Protocol:
	// https://github.com/bazelbuild/remote-apis/blob/7802003e00901b4e740fe0ebec1243c221e02ae2/build/bazel/remote/execution/v2/remote_execution.proto#L193-L205
	// https://github.com/googleapis/googleapis/blob/c8e291e6a4d60771219205b653715d5aeec3e96b/google/bytestream/bytestream.proto#L55

	w, err := ac.NewCASWriter(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to create a CAS writer").Err()
	}
	defer w.CloseSend()

	bufSize := ac.bufSize
	if bufSize == 0 {
		bufSize = 1024 * 1024
		if bufSize > int(ac.size) {
			bufSize = int(ac.size)
		}
	}
	buf := make([]byte, bufSize)

	// Copy data from r to w using buffer buf.
	// Include the resource name only in the first request.
	first := true
	bytesSent := 0
	for {
		_, readSpan := tracing.Start(ctx, "resultdb.readChunk")
		n, err := r.Read(buf)
		if err != nil && err != io.EOF {
			tracing.End(readSpan, err)
			if err != io.ErrUnexpectedEOF {
				return errors.Annotate(err, "failed to read artifact contents").Err()
			}
			return appstatus.BadRequest(errors.Annotate(err, "failed to read artifact contents").Err())
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
		defer setSizeMetric(ctx, buf[:n], ac.contentType)

		// Include the resource name only in the first request.
		if first {
			first = false
			req.ResourceName = ac.genWriteResourceName(ctx)
		}

		// Send the request.
		_, writeSpan := tracing.Start(ctx, "resultdb.writeChunk",
			attribute.Int("size", n),
		)
		// Do not shadow err! It is checked below again.
		if err = w.Send(req); err != nil && err != io.EOF {
			tracing.End(writeSpan, err)
			return errors.Annotate(err, "failed to write data to RBE-CAS").Err()
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
		return errors.Annotate(err, "failed to read RBE-CAS write response").Err()
	case res.CommittedSize == ac.size:
		return nil
	default:
		return errors.Reason("unexpected blob commit size %d, expected %d", res.CommittedSize, ac.size).Err()
	}
}

// genWriteResourceName generates a random resource name that can be used
// to write the blob to RBE-CAS.
func (ac *artifactCreator) genWriteResourceName(ctx context.Context) string {
	uuidBytes := make([]byte, 16)
	if _, err := mathrand.Read(ctx, uuidBytes); err != nil {
		panic(err)
	}
	return fmt.Sprintf(
		"%s/uploads/%s/blobs/%s/%d",
		ac.RBEInstance,
		uuid.Must(uuid.FromBytes(uuidBytes)),
		artifacts.TrimHashPrefix(ac.hash),
		ac.size)
}

// parseRequest populates ac fields based on the HTTP request.
func (ac *artifactCreator) parseRequest(c *router.Context) error {
	// Read the artifact name.
	// We must use EscapedPath(), not Path, to preserve test ID's own encoding.
	ac.artifactName = strings.TrimPrefix(c.Request.URL.EscapedPath(), "/")

	// Parse and validate the artifact name.
	var invIDString string
	var err error
	invIDString, ac.testID, ac.resultID, ac.artifactID, err = pbutil.ParseArtifactName(ac.artifactName)
	if err != nil {
		return appstatus.Errorf(codes.InvalidArgument, "bad artifact name: %s", err)
	}
	ac.invID = invocations.ID(invIDString)
	ac.localParentID = artifacts.ParentID(ac.testID, ac.resultID)

	// Parse and validate the hash.
	switch ac.hash = c.Request.Header.Get(artifactContentHashHeaderKey); {
	case ac.hash == "":
		return appstatus.Errorf(codes.InvalidArgument, "%s header is missing", artifactContentHashHeaderKey)
	case !artifacts.ContentHashRe.MatchString(ac.hash):
		return appstatus.Errorf(codes.InvalidArgument, "%s header value does not match %s", artifactContentHashHeaderKey, artifacts.ContentHashRe)
	}

	// Parse and validate the size.
	sizeHeader := c.Request.Header.Get(artifactContentSizeHeaderKey)
	if sizeHeader == "" {
		return appstatus.Errorf(codes.InvalidArgument, "%s header is missing", artifactContentSizeHeaderKey)
	}
	switch ac.size, err = strconv.ParseInt(sizeHeader, 10, 64); {
	case err != nil:
		return appstatus.Errorf(codes.InvalidArgument, "%s header is malformed: %s", artifactContentSizeHeaderKey, err)
	case ac.size < 0 || ac.size > ac.MaxArtifactContentStreamLength:
		return appstatus.Errorf(codes.InvalidArgument, "%s header must be a value between 0 and %d", artifactContentSizeHeaderKey, ac.MaxArtifactContentStreamLength)
	}

	// Parse and validate the update token.
	updateToken := c.Request.Header.Get(updateTokenHeaderKey)
	if updateToken == "" {
		return appstatus.Errorf(codes.Unauthenticated, "%s header is missing", updateTokenHeaderKey)
	}
	if err := validateInvocationToken(c.Request.Context(), updateToken, ac.invID); err != nil {
		return appstatus.Errorf(codes.PermissionDenied, "invalid %s header value", updateTokenHeaderKey)
	}

	ac.contentType = c.Request.Header.Get(artifactContentTypeHeaderKey)

	return nil
}

// verifyStateBeforeWriting checks Spanner state in a read-only transaction,
// see verifyState comment.
func (ac *artifactCreator) verifyStateBeforeWriting(ctx context.Context) (sameAlreadyExists bool, err error) {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	_, sameAlreadyExists, err = ac.verifyState(ctx)
	return
}

// verifyState checks if the Spanner state is compatible with creation of the
// artifact. If an identical artifact already exists, sameAlreadyExists is true.
func (ac *artifactCreator) verifyState(ctx context.Context) (realm string, sameAlreadyExists bool, err error) {
	var (
		invState       pb.Invocation_State
		hash           spanner.NullString
		size           spanner.NullInt64
		artifactExists bool
	)

	// Read the state concurrently.
	err = parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() (err error) {
			return invocations.ReadColumns(ctx, ac.invID, map[string]any{
				"State": &invState, "Realm": &realm,
			})
		}

		work <- func() error {
			key := ac.invID.Key(ac.localParentID, ac.artifactID)
			err := spanutil.ReadRow(ctx, "Artifacts", key, map[string]any{
				"RBECASHash": &hash,
				"Size":       &size,
			})
			artifactExists = err == nil
			if spanner.ErrCode(err) == codes.NotFound {
				// This is expected.
				return nil
			}
			return err
		}
	})

	// Interpret the state.
	switch {
	case err != nil:
		return

	case invState != pb.Invocation_ACTIVE:
		err = appstatus.Errorf(codes.FailedPrecondition, "%s is not active", ac.invID.Name())
		return

	case hash.Valid && hash.StringVal == ac.hash && size.Valid && size.Int64 == ac.size:
		// The same artifact already exists.
		sameAlreadyExists = true
		return

	case artifactExists:
		// A different artifact already exists.
		err = appstatus.Errorf(codes.AlreadyExists, "artifact %q already exists", ac.artifactName)
		return
	}
	return
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
	_, ts := tracing.Start(ctx, "resultdb.digestVerifier.ReadVerify")
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
