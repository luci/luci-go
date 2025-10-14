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
	"mime"
	"strings"

	"cloud.google.com/go/spanner"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/gsutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TODO(crbug.com/1177213) - make this configurable.
const MaxBatchCreateArtifactSize = 10 * 1024 * 1024

// MaxShardContentSize is the maximum content size in BQ row.
// Artifacts content bigger than this size needs to be sharded.
// Leave 10 KB for other fields, the rest is content.
const MaxShardContentSize = bq.RowMaxBytes - 10*1024

// LookbackWindow is used when chunking. It specifies how many bytes we should
// look back to find new line/white space characters to split the chunks.
const LookbackWindow = 1024

// Represents an artifact creation request.
// This is used both by BatchCreateArtifacts and the (single) streaming artifact upload.
type artifactCreationRequest struct {
	// the work unit ID in which to create the artifact, if any.
	// Either this or invocationID will be set to a non-emtpy value, but not both.
	workUnitID workunits.ID
	// the invocation ID in which to create the artifact, if any.
	invocationID invocations.ID

	// the flat test id.
	testID       string
	resultID     string
	artifactID   string
	contentType  string
	artifactType string

	// rbeCASHash is a hash of the artifact data that will be stored in RBE-CAS.
	// It is not supplied or calculated for client-stored GCS or RBE artifacts (where gcsURI or rbeURI supplied).
	rbeCASHash string
	// size is the size of the artifact data in bytes. In the case of a GCS artifact it is user-specified, optional and not verified.
	size int64
	// data is the artifact contents data that will be stored in RBE-CAS. If gcsURI or rbeURI is provided, this must be empty.
	// This field is only set when used from BatchCreateArtifacts; in the single artifact creation endpoint the
	// artifact contents is streamed.
	data []byte
	// gcsURI is the location of the artifact content if it is stored in GCS.
	// If this is provided, data must be empty.
	gcsURI string
	// rbeURI is the location of the artifact content if it is stored in external RBE.
	// If this is provided, data must be empty.
	rbeURI string

	// The structured test ID corresponding to `testID` and `moduleVariant`.
	// This may be missing the ModuleVariant (legacy clients may not be setting this).
	testIDStructured *pb.TestIdentifier
}

// name returns the artifact name.
func (a *artifactCreationRequest) name() string {
	if a.invocationID != "" {
		if a.testID == "" {
			return pbutil.LegacyInvocationArtifactName(string(a.invocationID), a.artifactID)
		}
		return pbutil.LegacyTestResultArtifactName(string(a.invocationID), a.testID, a.resultID, a.artifactID)
	}
	if a.workUnitID != (workunits.ID{}) {
		if a.testID == "" {
			return pbutil.WorkUnitArtifactName(string(a.workUnitID.RootInvocationID), string(a.workUnitID.WorkUnitID), a.artifactID)
		}
		return pbutil.TestResultArtifactName(string(a.workUnitID.RootInvocationID), string(a.workUnitID.WorkUnitID), a.testID, a.resultID, a.artifactID)
	}
	panic("logic error: artifact should have either invocationID or workUnitID")
}

// parentID returns the local parent ID of the artifact.
func (a *artifactCreationRequest) parentID() string {
	return artifacts.ParentID(a.testID, a.resultID)
}

func parseCreateArtifactRequest(req *pb.CreateArtifactRequest, batchLevelParent string, cfg *config.CompiledServiceConfig) (*artifactCreationRequest, error) {
	if req.GetArtifact() == nil {
		return nil, errors.New("artifact: unspecified")
	}
	var wuID workunits.ID
	var invID invocations.ID
	var testID, resultID string

	if req.Parent == "" {
		// Parent field is required if there is no batch-level parent, but
		// no value was specified.
		if batchLevelParent == "" {
			return nil, errors.New("parent: unspecified")
		}

		// Inherit invocation/work unit from batch-level.
		isLegacyName := strings.HasPrefix(batchLevelParent, "invocations/")
		if !isLegacyName {
			wuID = workunits.MustParseName(batchLevelParent)
		} else {
			invID = invocations.MustParseName(batchLevelParent)
		}
	} else { // req.Parent != ""
		if batchLevelParent != "" && req.Parent != batchLevelParent {
			return nil, errors.Fmt("parent: must be empty or equal to the batch-level parent; got %q, want %q", req.Parent, batchLevelParent)
		}
		// Use a heuristic to determine how we should parse the parent name.
		isLegacyName := strings.HasPrefix(req.Parent, "invocations/")
		if !isLegacyName {
			var err error
			wuID, err = workunits.ParseName(req.Parent)
			if err != nil {
				return nil, errors.Fmt("parent: %w", err)
			}
		} else {
			// Parsing the parent string as an invocation name first. If this fails, attempt to parse it as a test result name.
			// This order ensures that `testID` and `resultID` are only assigned if a valid test result parent is identified,
			if invocationID, ok := pbutil.TryParseInvocationName(req.Parent); ok {
				invID = invocations.ID(invocationID)
			} else {
				var invIDStr string
				var err error
				invIDStr, testID, resultID, err = pbutil.ParseLegacyTestResultName(req.Parent)
				if err != nil {
					return nil, errors.Fmt("parent: not a valid work unit name, invocation name or test result name; got %q", req.Parent)
				}
				invID = invocations.ID(invIDStr)
			}
		}
	}

	var testIDStructured *pb.TestIdentifier
	if testID != "" {
		testIDBase, err := pbutil.ParseAndValidateTestID(testID)
		if err != nil {
			return nil, errors.Fmt("parent: encoded test id: %w", err)
		}
		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateTestIDToScheme(cfg, testIDBase); err != nil {
			return nil, errors.Fmt("parent: encoded test id: %w", err)
		}
		testIDStructured = &pb.TestIdentifier{
			ModuleName:    testIDBase.ModuleName,
			ModuleScheme:  testIDBase.ModuleScheme,
			ModuleVariant: nil,
			CoarseName:    testIDBase.CoarseName,
			FineName:      testIDBase.FineName,
			CaseName:      testIDBase.CaseName,
		}
	}
	if req.Artifact.TestIdStructured != nil {
		testIDToValidate := proto.Clone(req.Artifact.TestIdStructured).(*pb.TestIdentifier)
		if testIDToValidate.ModuleVariant == nil && testIDToValidate.ModuleVariantHash == "" {
			// Historically, this RPC accepted a structured test ID without module variant.
			// For compatibility reasons, we still want such variant-less to pass validation.
			testIDToValidate.ModuleVariant = &pb.Variant{}
		}

		testIDBase := pbutil.ExtractBaseTestIdentifier(testIDToValidate)
		if err := pbutil.ValidateStructuredTestIdentifierForStorage(testIDToValidate); err != nil {
			return nil, errors.Fmt("artifact: test_id_structured: %w", err)
		}
		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateTestIDToScheme(cfg, testIDBase); err != nil {
			return nil, errors.Fmt("artifact: test_id_structured: %w", err)
		}
		if req.Artifact.ResultId == "" {
			return nil, errors.New("artifact: result_id is required if test_id_structured is specified")
		}
		if err := pbutil.ValidateResultID(req.Artifact.ResultId); err != nil {
			return nil, errors.Fmt("artifact: result_id: %w", err)
		}
		if testID != "" || resultID != "" {
			return nil, errors.New("artifact: test_id_structured must not be specified if parent is a test result name (legacy format)")
		}
		testID = pbutil.EncodeTestID(testIDBase)
		resultID = req.Artifact.ResultId
		testIDStructured = req.Artifact.TestIdStructured
	}

	if req.Artifact.ResultId != "" && req.Artifact.TestIdStructured == nil {
		return nil, errors.New("artifact: test_id_structured is required if result_id is specified")
	}

	if err := pbutil.ValidateArtifactID(req.Artifact.ArtifactId); err != nil {
		return nil, errors.Fmt("artifact: artifact_id: %w", err)
	}

	if req.Artifact.ArtifactType != "" {
		if err := pbutil.ValidateArtifactType(req.Artifact.ArtifactType); err != nil {
			return nil, errors.Fmt("artifact: artifact_type: %w", err)
		}
	}

	if req.Artifact.ContentType != "" {
		if _, _, err := mime.ParseMediaType(req.Artifact.ContentType); err != nil {
			return nil, errors.Fmt("artifact: content_type: %w", err)
		}
	}

	if req.Artifact.SizeBytes < 0 {
		return nil, errors.New("artifact: size_bytes: must be non-negative")
	}

	var sizeBytes int64
	var rbeCASHash string
	if (req.Artifact.GcsUri != "" && len(req.Artifact.Contents) > 0) ||
		(req.Artifact.GcsUri != "" && req.Artifact.RbeUri != "") ||
		(len(req.Artifact.Contents) > 0 && req.Artifact.RbeUri != "") {
		return nil, errors.New("artifact: only one of contents, gcs_uri and rbe_uri can be given")
	}
	if req.Artifact.GcsUri != "" {
		bucket, objectName := gs.Path(req.Artifact.GcsUri).Split()
		// From https://cloud.google.com/storage/quotas.
		const gcsBucketMaxLength = 222    // bytes (owing to only ASCII characters being allowed in names)
		const gcsFileNameMaxLength = 1024 // bytes
		if bucket == "" {
			return nil, errors.Fmt("artifact: gcs_uri: missing bucket name; got %q", req.Artifact.GcsUri)
		}
		if objectName == "" {
			return nil, errors.Fmt("artifact: gcs_uri: missing object name; got %q", req.Artifact.GcsUri)
		}
		if len(bucket) > gcsBucketMaxLength {
			return nil, errors.Fmt("artifact: gcs_uri: bucket name component exceeds %d bytes", gcsBucketMaxLength)
		}
		if len(objectName) > gcsFileNameMaxLength {
			return nil, errors.Fmt("artifact: gcs_uri: object name component exceeds %d bytes", gcsFileNameMaxLength)
		}
		// Try to set size from the request.
		sizeBytes = req.Artifact.SizeBytes
	} else if req.Artifact.RbeUri != "" {
		_, _, _, size, err := pbutil.ParseRbeURI(req.Artifact.RbeUri)
		if err != nil {
			return nil, errors.Fmt("artifact: rbe_uri: %w", err)
		}
		if req.Artifact.SizeBytes != 0 && size != req.Artifact.SizeBytes {
			return nil, errors.Fmt("artifact: size_bytes: does not match the size of external RBE artifact; got %v, want %v", req.Artifact.SizeBytes, size)
		}
		sizeBytes = size
	} else {
		// Internal RBE-CAS.
		data := req.Artifact.Contents

		// Take size from the contents uploaded.
		sizeBytes = int64(len(data))
		if req.Artifact.SizeBytes != 0 && sizeBytes != req.Artifact.SizeBytes {
			return nil, errors.New("artifact: size_bytes: does not match the size of contents (and the artifact is not a GCS or RBE reference)")
		}

		h := sha256.Sum256(data)
		rbeCASHash = artifacts.AddHashPrefix(hex.EncodeToString(h[:]))
	}

	return &artifactCreationRequest{
		workUnitID:       wuID,
		invocationID:     invID,
		testID:           testID,
		resultID:         resultID,
		artifactID:       req.Artifact.ArtifactId,
		contentType:      req.Artifact.ContentType,
		artifactType:     req.Artifact.ArtifactType,
		rbeCASHash:       rbeCASHash,
		data:             req.Artifact.Contents,
		size:             sizeBytes,
		gcsURI:           req.Artifact.GcsUri,
		rbeURI:           req.Artifact.RbeUri,
		testIDStructured: testIDStructured,
	}, nil
}

// parseBatchCreateArtifactsRequest parses a batch request and returns
// artifactCreationRequests for each of the artifacts w/o hash computation.
// It returns an error, if
// - any of the artifact IDs or contentTypes are invalid,
// - the total size exceeds MaxBatchCreateArtifactSize, or
// - there are more than one invocations associated with the artifacts.
// - both data and a GCS URI are supplied
// - mix use of the legacy format of requests[i].parent field and the new schema, including
//   - different top-level parent and requests[i].parent.
//   - use of test result name as requests[i].parent with test_id_structured, result_id specified.
func parseBatchCreateArtifactsRequest(ctx context.Context, in *pb.BatchCreateArtifactsRequest, cfg *config.CompiledServiceConfig) (arts []*artifactCreationRequest, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.parseBatchCreateArtifactsRequest",
		attribute.Int("cr.dev.count", len(in.Requests)),
	)
	defer func() { tracing.End(ts, err) }()

	var tSize int64

	if err := pbutil.ValidateBatchRequestCountAndSize(in.Requests); err != nil {
		return nil, errors.Fmt("requests: %w", err)
	}

	// The invocation ID common to all requests, if this request is indeed using invocation IDs.
	// This is used to validate a request using invocation IDs, only uses one invocation ID.
	var commonInvID invocations.ID
	// If this request is using work units. If it is, it cannot use invocation IDs.
	// It may reference multiple work units in the same batch.
	var usingWorkUnits bool

	if in.Parent != "" {
		isLegacyParent := strings.HasPrefix(in.Parent, "invocations/")
		if !isLegacyParent {
			if err := pbutil.ValidateWorkUnitName(in.Parent); err != nil {
				return nil, errors.Fmt("parent: %w", err)
			}
		} else {
			// For legacy compatibility: also accept invocation IDs.
			if err := pbutil.ValidateInvocationName(in.Parent); err != nil {
				return nil, errors.Fmt("parent: %w", err)
			}
			commonInvID = invocations.MustParseName(in.Parent)
		}
	}

	nameToRequestIndex := make(map[string]int)

	arts = make([]*artifactCreationRequest, len(in.Requests))
	for i, req := range in.Requests {
		art, err := parseCreateArtifactRequest(req, in.Parent, cfg)
		if err != nil {
			return nil, errors.Fmt("requests[%d]: %w", i, err)
		}

		if (art.workUnitID != workunits.ID{}) {
			if commonInvID != "" {
				// Work unit ID specified on artifact, but invocation ID has been used previously.
				return nil, errors.Fmt("requests[%d]: parent: cannot create artifacts in mix of invocations and work units in the same request, expected %q", i, commonInvID.Name())
			}
			usingWorkUnits = true
		} else {
			if art.invocationID == "" {
				panic("logic error: parseCreateArtifactRequest should have validated either a workUnitID or invocationID is set")
			}

			if usingWorkUnits {
				// An invocation ID specified on the artifact, but work unit ID used previously.
				return nil, errors.Fmt("requests[%d]: parent: cannot create artifacts in mix of invocations and work units in the same request, expected another work unit", i)
			}
			if commonInvID == "" {
				commonInvID = art.invocationID
			} else if commonInvID != art.invocationID {
				return nil, errors.Fmt("requests[%d]: parent: only one invocation is allowed: got %q, want %q", i, art.invocationID.Name(), commonInvID.Name())
			}
		}

		// Count the size of artifacts going to RBE-CAS as it has limits on the size of its Batch upload.
		// N.B. As at writing, this limit is impossible to exceed because the total request size limit is the
		// same as the content limit imposed here, but if limits change this could be enforced again.
		if art.gcsURI == "" && art.rbeURI == "" {
			tSize += art.size
		}
		if tSize > MaxBatchCreateArtifactSize {
			return nil, errors.Fmt("the total size of artifact contents to be stored exceeded %d", MaxBatchCreateArtifactSize)
		}

		name := art.name()
		if previousIndex, ok := nameToRequestIndex[name]; ok {
			return nil, errors.Fmt("requests[%d]: same parent, test_id_structured, result_id and artifact_id as requests[%d]", i, previousIndex)
		}
		nameToRequestIndex[name] = i

		arts[i] = art
	}

	// Validate request IDs.
	if usingWorkUnits && in.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return nil, errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(in.RequestId); err != nil {
		return nil, errors.Fmt("request_id: %w", err)
	}
	for i, r := range in.Requests {
		if err := emptyOrEqual("request_id", r.RequestId, in.RequestId); err != nil {
			return nil, errors.Fmt("requests[%d]: %w", i, err)
		}
	}
	return arts, nil
}

// findNewArtifacts returns a list of the artifacts that don't have states yet.
// If one exists w/ different hash/size, this returns an error.
func findNewArtifacts(ctx context.Context, arts []*artifactCreationRequest) ([]*artifactCreationRequest, error) {
	// artifacts are not expected to exist in most cases, and this map would likely
	// be empty.
	type state struct {
		hash   string
		size   int64
		gcsURI string
		rbeURI string
	}
	var states map[string]state
	ks := spanner.KeySets()
	for _, a := range arts {
		invID := a.invocationID
		if invID == "" {
			invID = a.workUnitID.LegacyInvocationID()
		}
		ks = spanner.KeySets(invID.Key(a.parentID(), a.artifactID), ks)
	}
	var b spanutil.Buffer
	err := span.Read(ctx, "Artifacts", ks, []string{"InvocationId", "ParentId", "ArtifactId", "RBECASHash", "Size", "GcsURI", "RbeURI"}).Do(
		func(row *spanner.Row) (err error) {
			var invID invocations.ID
			var pid, aid string
			var hash string
			var size = new(int64)
			var gcsURI string
			var rbeURI string
			if err = b.FromSpanner(row, &invID, &pid, &aid, &hash, &size, &gcsURI, &rbeURI); err != nil {
				return
			}
			if states == nil {
				states = make(map[string]state)
			}
			// treat non-existing size as 0.
			if size == nil {
				size = new(int64)
			}
			// The artifact exists.
			states[invID.Key(pid, aid).String()] = state{hash, *size, gcsURI, rbeURI}
			return
		},
	)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "%s", err)
	}

	newArts := make([]*artifactCreationRequest, 0, len(arts)-len(states))
	for i, a := range arts {
		aCopy := *a
		a = &aCopy
		invID := a.invocationID
		if invID == "" {
			invID = a.workUnitID.LegacyInvocationID()
		}
		st, ok := states[invID.Key(a.parentID(), a.artifactID).String()]
		if !ok {
			newArts = append(newArts, a)
			continue
		}
		if (a.gcsURI == "") != (st.gcsURI == "") {
			// Can't change from GCS to non-GCS and vice-versa
			return nil, appstatus.Errorf(codes.AlreadyExists, `requests[%d]: artifact %q already exists with a different storage scheme (GCS vs non-GCS)`, i, a.name())
		}
		if (a.rbeURI == "") != (st.rbeURI == "") {
			// Can't change from RBE to non-RBE and vice-versa
			return nil, appstatus.Errorf(codes.AlreadyExists, `requests[%d]: artifact %q already exists with a different storage scheme (RBE vs non-RBE)`, i, a.name())
		}
		if a.gcsURI != "" {
			if a.gcsURI != st.gcsURI {
				return nil, appstatus.Errorf(codes.AlreadyExists, `requests[%d]: artifact %q already exists with a different GCS URI: %q != %q`, i, a.name(), a.gcsURI, st.gcsURI)
			}
		} else if a.rbeURI != "" {
			if a.rbeURI != st.rbeURI {
				return nil, appstatus.Errorf(codes.AlreadyExists, `requests[%d]: artifact %q already exists with a different RBE URI: %q != %q`, i, a.name(), a.rbeURI, st.rbeURI)
			}
		} else {
			if a.rbeCASHash != st.hash {
				return nil, appstatus.Errorf(codes.AlreadyExists, `requests[%d]: artifact %q already exists with a different hash`, i, a.name())
			}
		}
		if a.size != st.size {
			return nil, appstatus.Errorf(codes.AlreadyExists, `requests[%d]: artifact %q already exists with a different size: %d != %d`, i, a.name(), a.size, st.size)
		}
	}
	return newArts, nil
}

func checkInvocationOrWorkUnitState(ctx context.Context, arts []*artifactCreationRequest) (workUnitsInfo map[workunits.ID]workunits.TestResultInfo, invInfo *invocations.TestResultInfo, err error) {
	workUnitIDs := workunits.NewIDSet()
	for _, a := range arts {
		if a.workUnitID != (workunits.ID{}) {
			workUnitIDs.Add(a.workUnitID)
		}
	}
	if len(workUnitIDs) > 0 {
		parentInfos, err := workunits.ReadTestResultInfos(ctx, workUnitIDs.SortedByRowID())
		if err != nil {
			return nil, nil, err // NotFound or internal error.
		}
		for _, wuID := range workUnitIDs.SortedByID() {
			if parentInfos[wuID].FinalizationState != pb.WorkUnit_ACTIVE {
				return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "%q is not active", wuID.Name())
			}
		}
		workUnitsInfo = parentInfos
	} else {
		// We are using invocation IDs. The invocation ID on all requests should be the same.
		invID := arts[0].invocationID
		parentInfo, err := invocations.ReadTestResultInfo(ctx, arts[0].invocationID)
		if err != nil {
			return nil, nil, err // NotFound or internal error.
		}
		if parentInfo.State != pb.Invocation_ACTIVE {
			return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "%q is not active", invID.Name())
		}
		invInfo = &parentInfo
	}

	return workUnitsInfo, invInfo, nil
}

func validateBatchCreateArtifactsRequestForSystemState(ctx context.Context, workUnitsInfo map[workunits.ID]workunits.TestResultInfo, invInfo *invocations.TestResultInfo, arts []*artifactCreationRequest) error {
	allowedBucketsByProject := make(map[string]map[string]bool)
	allowedRBEInstancesByProject := make(map[string]map[string]bool)
	user := auth.CurrentUser(ctx).Identity

	for i, a := range arts {
		// Test result-artifact. Validate the module matches the work unit.
		if a.testIDStructured != nil {
			var expectedModule *pb.ModuleIdentifier
			strictValidation := true
			if a.workUnitID != (workunits.ID{}) {
				// Using work units.
				expectedModule = workUnitsInfo[a.workUnitID].ModuleID
			} else {
				// Using invocations.
				expectedModule = invInfo.ModuleID
				strictValidation = false
			}

			testIDToValidate := a.testIDStructured
			if testIDToValidate.ModuleVariant == nil && expectedModule != nil {
				// Legacy callers may not specify the ModuleVariant. If it is not
				// specified (indicated by nil as opposed to &pb.Variant{} with no defs),
				// let it pass validation. We will backfill the ModuleVariant from the
				// parent work unit or invocation later.
				//
				// Clone to avoid having changes to the test ID made here (to let it pass
				// validation) propagate backwards to the caller.
				testIDToValidate = proto.Clone(testIDToValidate).(*pb.TestIdentifier)
				testIDToValidate.ModuleVariant = expectedModule.ModuleVariant
			}
			if err := validateUploadAgainstWorkUnitModule(testIDToValidate, expectedModule, strictValidation); err != nil {
				return appstatus.Errorf(codes.FailedPrecondition, "requests[%d]: artifact: %s", i, err)
			}
		}

		var realm string
		if a.workUnitID != (workunits.ID{}) {
			realm = workUnitsInfo[a.workUnitID].Realm
		} else {
			realm = invInfo.Realm
		}
		project, _ := realms.Split(realm)
		if a.gcsURI != "" {
			// Check this GCS reference is allowed by the project config.
			// Delay construction of the checker (which may occasionally involve an RPC) until we know we
			// actually need it.
			if allowedBucketsByProject[project] == nil {
				allowedBuckets, err := allowedGCSBucketsForUser(ctx, project, string(user))
				if err != nil {
					// Internal error.
					return errors.Fmt("fetch allowed GCS buckets for project %s: %w", project, err)
				}
				allowedBucketsByProject[project] = allowedBuckets
			}
			bucket, _ := gsutil.Split(a.gcsURI)
			if _, ok := allowedBucketsByProject[project][bucket]; !ok {
				return appstatus.Errorf(codes.PermissionDenied, "requests[%d]: the user does not have permission to reference GCS objects in bucket %q in project %q", i, bucket, project)
			}
		}
		if a.rbeURI != "" {
			// Check this RBE reference is allowed by the project config.
			if allowedRBEInstancesByProject[project] == nil {
				allowedRbeInstances, err := allowedRbeInstancesForUser(ctx, project, string(user))
				if err != nil {
					return errors.Fmt("fetch allowed RBE instances for project %s: %w", project, err)
				}
				allowedRBEInstancesByProject[project] = allowedRbeInstances
			}
			rbeProject, rbeInstance, _, _, err := pbutil.ParseRbeURI(a.rbeURI)
			if err != nil {
				return appstatus.Errorf(codes.InvalidArgument, "requests[%d]: artifact: rbe_uri: invalid RBE URI: %q", i, a.rbeURI)
			}
			rbeInstancePath := pbutil.RbeInstancePath(rbeProject, rbeInstance)
			if _, ok := allowedRBEInstancesByProject[project][rbeInstancePath]; !ok {
				return appstatus.Errorf(codes.PermissionDenied, "requests[%d]: the user does not have permission to reference RBE objects in RBE instance %q", i, rbeInstancePath)
			}
		}
	}
	return nil
}

// createArtifactStates creates the states of given artifacts in Spanner.
func createArtifactStates(ctx context.Context, arts []*artifactCreationRequest) (results []*pb.Artifact, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.createArtifactStates")
	defer func() { tracing.End(ts, err) }()

	var insertsByRealm map[string]int
	var wuInfos map[workunits.ID]workunits.TestResultInfo
	var invInfo *invocations.TestResultInfo

	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		// New transaction attempt, reset the read values to avoid cross-contamination
		// by previous attempts.
		insertsByRealm = make(map[string]int)
		wuInfos = nil
		invInfo = nil

		// Verify work unit/invocation state.
		var err error
		wuInfos, invInfo, err = checkInvocationOrWorkUnitState(ctx, arts)
		if err != nil {
			return err // NotFound, FailedPrecondition or internal error.
		}

		// Verify the request is valid for the current system state.
		err = validateBatchCreateArtifactsRequestForSystemState(ctx, wuInfos, invInfo, arts)
		if err != nil {
			return err // FailedPrecondition or PermissionDenied error.
		}

		// Find the new artifacts.
		noStateArts, err := findNewArtifacts(ctx, arts)
		if err != nil {
			return err
		}
		if len(noStateArts) == 0 {
			logging.Warningf(ctx, "The states of all the artifacts already exist.")
		}
		if len(noStateArts) != 0 && len(noStateArts) != len(arts) {
			logging.Warningf(ctx, "Some of the artifacts already exist, but not all.")
		}

		// Prepare the mutations.
		for _, a := range noStateArts {
			var invID invocations.ID
			if a.invocationID != "" {
				invID = a.invocationID
			} else {
				invID = a.workUnitID.LegacyInvocationID()
			}
			var moduleVariant *pb.Variant
			if a.testID != "" {
				// Pull the module variant from the invocation/work unit.
				// This is needed as for legacy compatibility reasons the variant
				// may not always be set in the request.
				// Moreover, if the variant is set on the request, it is validated
				// to be consistent with the values on the work unit/invocation.
				if a.invocationID != "" {
					moduleVariant = invInfo.ModuleID.GetModuleVariant()
				} else {
					moduleVariant = wuInfos[a.workUnitID].ModuleID.GetModuleVariant()
				}
			}

			span.BufferWrite(ctx, spanutil.InsertMap("Artifacts", map[string]any{
				"InvocationId":  invID,
				"ParentId":      a.parentID(),
				"ArtifactId":    a.artifactID,
				"ContentType":   a.contentType,
				"ArtifactType":  a.artifactType,
				"Size":          a.size,
				"RBECASHash":    a.rbeCASHash,
				"GcsURI":        a.gcsURI,
				"RbeURI":        a.rbeURI,
				"ModuleVariant": moduleVariant,
			}))

			if a.invocationID != "" {
				insertsByRealm[invInfo.Realm]++
			} else {
				insertsByRealm[wuInfos[a.workUnitID].Realm]++
			}
		}

		return nil
	})
	if err != nil {
		return nil, errors.Fmt("write artifacts to Spanner: %w", err)
	}
	for realm, count := range insertsByRealm {
		spanutil.IncRowCount(ctx, count, spanutil.Artifacts, spanutil.Inserted, realm)
	}

	// Construct the results.
	results = make([]*pb.Artifact, 0, len(arts))
	for _, a := range arts {
		var testIDStructured *pb.TestIdentifier
		if a.testID != "" {
			// Pull the module variant from the invocation/work unit.
			// This is needed as for legacy compatibility reasons the variant
			// may not always be set in the request.
			testIDStructured = proto.Clone(a.testIDStructured).(*pb.TestIdentifier)
			if a.invocationID != "" {
				testIDStructured.ModuleVariant = invInfo.ModuleID.GetModuleVariant()
				testIDStructured.ModuleVariantHash = invInfo.ModuleID.GetModuleVariantHash()
			} else {
				// The ModuleID should always be set as it is enforced by validation.
				testIDStructured.ModuleVariant = wuInfos[a.workUnitID].ModuleID.ModuleVariant
				testIDStructured.ModuleVariantHash = wuInfos[a.workUnitID].ModuleID.ModuleVariantHash
			}
		}

		results = append(results, &pb.Artifact{
			Name:             a.name(),
			TestIdStructured: testIDStructured,
			TestId:           a.testID,
			ResultId:         a.resultID,
			ArtifactId:       a.artifactID,
			ContentType:      a.contentType,
			ArtifactType:     a.artifactType,
			SizeBytes:        a.size,
			GcsUri:           a.gcsURI,
			RbeUri:           a.rbeURI,
			HasLines:         artifacts.IsLogSupportedArtifact(a.artifactID, a.contentType),
		})
	}

	return results, nil
}

func uploadArtifactBlobs(ctx context.Context, rbeIns string, casClient repb.ContentAddressableStorageClient, arts []*artifactCreationRequest) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.uploadArtifactBlobs")
	defer func() { tracing.End(ts, err) }()

	casReq := &repb.BatchUpdateBlobsRequest{InstanceName: rbeIns}
	for _, a := range arts {
		casReq.Requests = append(casReq.Requests, &repb.BatchUpdateBlobsRequest_Request{
			Digest: &repb.Digest{Hash: artifacts.TrimHashPrefix(a.rbeCASHash), SizeBytes: a.size},
			Data:   a.data,
		})
	}
	resp, err := casClient.BatchUpdateBlobs(ctx, casReq, &grpc.MaxSendMsgSizeCallOption{MaxSendMsgSize: MaxBatchCreateArtifactSize})
	if err != nil {
		// If BatchUpdateBlobs() returns INVALID_ARGUMENT, it means that
		// the total size of the artifact contents was bigger than the max size that
		// BatchUpdateBlobs() can accept.
		return errors.Fmt("cas.BatchUpdateBlobs failed: %w", err)
	}
	for i, r := range resp.GetResponses() {
		cd := codes.Code(r.Status.Code)
		if cd != codes.OK {
			// Each individual error can be due to resource exhausted or unmatched digest.
			// If unmatched digest, this RPC has a bug and needs to be fixed.
			// If resource exhausted, the RBE server quota needs to be adjusted.
			//
			// Either case, it's a server-error, and an internal error will be returned.
			return errors.Fmt("artifact %q: cas.BatchUpdateBlobs failed", arts[i].name())
		}
	}
	return nil
}

// allowedGCSBucketsForUser returns the GCS buckets a user is allowed to reference
// by reading the project config. If no config exists for the user, an empty map
// will be returned, rather than an error.
func allowedGCSBucketsForUser(ctx context.Context, project, user string) (allowedBuckets map[string]bool, err error) {
	allowedBuckets = map[string]bool{}
	// This is cached for 1 minute, so no need to re-optimize here.
	cfg, err := config.Project(ctx, project)
	if err != nil {
		if errors.Is(err, config.ErrNotFoundProjectConfig) {
			return allowedBuckets, nil
		}
		return nil, err
	}

	for _, list := range cfg.GcsAllowList {
		for _, listUser := range list.Users {
			if listUser == user {
				for _, bucket := range list.Buckets {
					allowedBuckets[bucket] = true
				}
				return allowedBuckets, nil
			}
		}
	}
	return allowedBuckets, nil
}

// allowedRbeInstancesForUser returns the RBE instances a user is allowed to
// reference by reading the project config. If no config exists for the user, an
// empty map will be returned, rather than an error.
func allowedRbeInstancesForUser(ctx context.Context, project, user string) (allowedInstances map[string]bool, err error) {
	allowedInstances = map[string]bool{}
	// This is cached for 1 minute, so no need to re-optimize here.
	cfg, err := config.Project(ctx, project)
	if err != nil {
		if errors.Is(err, config.ErrNotFoundProjectConfig) {
			return allowedInstances, nil
		}
		return nil, err
	}

	for _, list := range cfg.RbeAllowList {
		for _, listUser := range list.Users {
			if listUser == user {
				for _, instance := range list.Instances {
					allowedInstances[instance] = true
				}
				return allowedInstances, nil
			}
		}
	}
	return allowedInstances, nil
}

func verifyBatchCreateArtifactsPermissions(ctx context.Context, arts []*artifactCreationRequest) (err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/services/recorder.verifyBatchCreateArtifactsPermissions")
	defer func() { tracing.End(ts, err) }()

	// Validate the update token.
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}
	var commonInvID invocations.ID
	var commonWorkUnitState string
	for i, a := range arts {
		if a.invocationID != "" {
			if commonInvID == "" {
				commonInvID = a.invocationID
			}
			if commonInvID != "" && commonInvID != a.invocationID {
				panic("logic error: request with multiple different invocations, this should have been caught in parseBatchCreateArtifactsRequest")
			}
		} else {
			tokenState := workUnitUpdateTokenState(a.workUnitID)
			if commonWorkUnitState == "" {
				commonWorkUnitState = tokenState
			} else if commonWorkUnitState != tokenState {
				return appstatus.BadRequest(errors.Fmt("requests[%d]: parent: work unit %q requires a different update token to request[0]'s %q, but this RPC only accepts one update token", i, a.workUnitID.Name(), arts[0].workUnitID.Name()))
			}
		}
	}

	if commonInvID == "" && commonWorkUnitState == "" {
		panic("logic error: neither work unit and invocation IDs specified, this should have been caught in parseBatchCreateArtifactsRequest")
	}
	if commonInvID != "" {
		if err := validateInvocationToken(ctx, token, commonInvID); err != nil {
			return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
		}
	}
	if commonWorkUnitState != "" {
		if err := validateWorkUnitUpdateTokenForState(ctx, token, commonWorkUnitState); err != nil {
			return appstatus.Errorf(codes.PermissionDenied, "invalid update token")
		}
	}

	return nil
}

// BatchCreateArtifacts implements pb.RecorderServer.
// This functions uploads the artifacts to RBE-CAS.
// If the artifact is a text-based artifact, it will also get uploaded to BigQuery.
// We have a percentage control to determine how many percent of artifacts got
// uploaded to BigQuery.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	arts, err := parseBatchCreateArtifactsRequest(ctx, in, cfg)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if err := verifyBatchCreateArtifactsPermissions(ctx, arts); err != nil {
		return nil, err
	}

	requiresRBECASUpload := false
	for _, a := range arts {
		if a.gcsURI == "" && a.rbeURI == "" {
			requiresRBECASUpload = true
			break
		}
	}

	if requiresRBECASUpload {
		// We cannot upload to RBE-CAS atomically with writing to Spanner.
		// So we will validate the request early w.r.t. current state of invocations
		// work units in Spanner, and if this succeeds, speculatively upload
		// the artifacts to RBE-CAS. We will then apply all validation again when
		// we commit the artifacts to Spanner.
		// Worst case, if this fails, we have a few orphaned artifacts in RBE-CAS.

		var artsToCreate []*artifactCreationRequest
		var wuInfos map[workunits.ID]workunits.TestResultInfo
		var invInfo *invocations.TestResultInfo
		func() {
			ctx, cancel := span.ReadOnlyTransaction(ctx)
			defer cancel()
			wuInfos, invInfo, err = checkInvocationOrWorkUnitState(ctx, arts)
			if err != nil {
				return // NotFound, FailedPrecondition or internal error.
			}
			artsToCreate, err = findNewArtifacts(ctx, arts)
		}()
		if err != nil {
			return nil, err
		}

		if err := validateBatchCreateArtifactsRequestForSystemState(ctx, wuInfos, invInfo, arts); err != nil {
			return nil, err
		}

		artsToUpload := make([]*artifactCreationRequest, 0, len(artsToCreate))
		for _, a := range artsToCreate {
			// Only upload to RBE CAS the ones that are not in GCS or external RBE.
			if a.gcsURI == "" && a.rbeURI == "" {
				artsToUpload = append(artsToUpload, a)
			}
		}

		if len(artsToUpload) > 0 {
			if err := uploadArtifactBlobs(ctx, s.ArtifactRBEInstance, s.casClient, artsToUpload); err != nil {
				return nil, err
			}
		}
	}

	results, err := createArtifactStates(ctx, arts)
	if err != nil {
		return nil, err
	}

	// Return all the artifacts to indicate that they were created.
	ret := &pb.BatchCreateArtifactsResponse{Artifacts: results}
	return ret, nil
}
