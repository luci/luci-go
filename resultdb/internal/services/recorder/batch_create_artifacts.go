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
	"time"

	"cloud.google.com/go/spanner"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
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

type artifactCreationRequest struct {
	// the flat test id.
	testID      string
	resultID    string
	artifactID  string
	contentType string

	// hash is a hash of the artifact data.  It is not supplied or calculated for GCS artifacts.
	hash string
	// size is the size of the artifact data in bytes.  In the case of a GCS artifact it is user-specified, optional and not verified.
	size int64
	// data is the artifact contents data that will be stored in RBE-CAS.  If gcsURI is provided, this must be empty.
	data []byte
	// gcsURI is the location of the artifact content if it is stored in GCS.  If this is provided, data must be empty.
	gcsURI string
}

type invocationInfo struct {
	id         string
	realm      string
	createTime time.Time
}

// name returns the artifact name.
func (a *artifactCreationRequest) name(invID invocations.ID) string {
	if a.testID == "" {
		return pbutil.InvocationArtifactName(string(invID), a.artifactID)
	}
	return pbutil.TestResultArtifactName(string(invID), a.testID, a.resultID, a.artifactID)
}

// parentID returns the local parent ID of the artifact.
func (a *artifactCreationRequest) parentID() string {
	return artifacts.ParentID(a.testID, a.resultID)
}

func parseCreateArtifactRequest(req *pb.CreateArtifactRequest, requireParent bool, cfg *config.CompiledServiceConfig) (invocations.ID, *artifactCreationRequest, error) {
	if req.GetArtifact() == nil {
		return "", nil, errors.New("artifact: unspecified")
	}
	var invID, testID, resultID string
	var err error

	if requireParent && req.Parent == "" {
		return "", nil, errors.New("parent: unspecified")
	}

	if req.Parent != "" {
		// Parsing the parent string as an invocation name first. If this fails, attempt to parse it as a test result name.
		// This order ensures that `testID` and `resultID` are only assigned if a valid test result parent is identified,
		if invocationID, ok := pbutil.TryParseInvocationName(req.Parent); ok {
			invID = invocationID
		} else {
			if invID, testID, resultID, err = pbutil.ParseTestResultName(req.Parent); err != nil {
				return "", nil, errors.New("parent: neither valid invocation name nor valid test result name")
			}
		}
	}

	if req.Artifact.TestIdStructured != nil {
		testIDBase := pbutil.BaseTestIdentifier{
			ModuleName:   req.Artifact.TestIdStructured.ModuleName,
			ModuleScheme: req.Artifact.TestIdStructured.ModuleScheme,
			CoarseName:   req.Artifact.TestIdStructured.CoarseName,
			FineName:     req.Artifact.TestIdStructured.FineName,
			CaseName:     req.Artifact.TestIdStructured.CaseName,
		}
		if err := pbutil.ValidateBaseTestIdentifier(testIDBase); err != nil {
			return "", nil, errors.Fmt("test_id_structured: %w", err)
		}
		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateTestIDToScheme(cfg, testIDBase); err != nil {
			return "", nil, errors.Fmt("test_id_structured: %w", err)
		}
		if req.Artifact.ResultId == "" {
			return "", nil, errors.New("result_id is required if test_id_structured is specified")
		}
		if err := pbutil.ValidateResultID(req.Artifact.ResultId); err != nil {
			return "", nil, errors.Fmt("result_id: %w", err)
		}
		if testID != "" || resultID != "" {
			return "", nil, errors.New("test_id_structured must not be specified if parent is a test result name (legacy format)")
		}
		testID = pbutil.EncodeTestID(testIDBase)
		resultID = req.Artifact.ResultId
	}

	if req.Artifact.ResultId != "" && req.Artifact.TestIdStructured == nil {
		return "", nil, errors.New("test_id_structured is required if result_id is specified")
	}

	if err := pbutil.ValidateArtifactID(req.Artifact.ArtifactId); err != nil {
		return "", nil, errors.Fmt("artifact_id: %w", err)
	}

	if req.Artifact.ContentType != "" {
		if _, _, err := mime.ParseMediaType(req.Artifact.ContentType); err != nil {
			return "", nil, errors.Fmt("content_type: %w", err)
		}
	}

	if len(req.Artifact.Contents) != 0 && req.Artifact.GcsUri != "" {
		return "", nil, errors.New("only one of contents and gcs_uri can be given")
	}

	sizeBytes := int64(len(req.Artifact.Contents))

	if sizeBytes != 0 && req.Artifact.SizeBytes != 0 && sizeBytes != req.Artifact.SizeBytes {
		return "", nil, errors.New("sizeBytes and contents are specified but don't match")
	}

	// If contents field is empty, try to set size from the request instead.
	if sizeBytes == 0 {
		if req.Artifact.SizeBytes != 0 {
			sizeBytes = req.Artifact.SizeBytes
		}
	}

	return invocations.ID(invID), &artifactCreationRequest{
		artifactID:  req.Artifact.ArtifactId,
		contentType: req.Artifact.ContentType,
		data:        req.Artifact.Contents,
		size:        sizeBytes,
		testID:      testID,
		resultID:    resultID,
		gcsURI:      req.Artifact.GcsUri,
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
func parseBatchCreateArtifactsRequest(in *pb.BatchCreateArtifactsRequest, cfg *config.CompiledServiceConfig) (invocations.ID, []*artifactCreationRequest, error) {
	var tSize int64
	var invID invocations.ID

	// TODO: Try to get rid of this case if we can and always expect in.Requests has at least one entry.
	if len(in.Requests) > 0 {
		if err := pbutil.ValidateBatchRequestCountAndSize(in.Requests); err != nil {
			return "", nil, errors.Fmt("requests: %w", err)
		}
	}

	if in.Parent != "" {
		if err := pbutil.ValidateInvocationName(in.Parent); err != nil {
			return "", nil, errors.Fmt("invocation: %w", err)
		}
		invID = invocations.MustParseName(in.Parent)
	}

	requireParent := in.Parent == ""
	arts := make([]*artifactCreationRequest, len(in.Requests))
	for i, req := range in.Requests {
		inv, art, err := parseCreateArtifactRequest(req, requireParent, cfg)
		if err != nil {
			return "", nil, errors.Fmt("requests[%d]: %w", i, err)
		}

		if in.Parent == "" {
			// Legacy uploader check:
			//    * testIDStructured and resultID are not set.
			//    * all parents belong to the same invocation.
			if req.Artifact.TestIdStructured != nil || req.Artifact.ResultId != "" {
				return "", nil, errors.Fmt("requests[%d]: test_id_structured or result_id must not be specified if top-level invocation is not set (legacy uploader)", i)
			}
			if invID == "" {
				invID = inv
			} else if inv != invID {
				return "", nil, errors.Fmt("requests[%d]: only one invocation is allowed: %q, %q", i, invID, inv)
			}
		}
		if in.Parent != "" && req.Parent != "" && in.Parent != req.Parent {
			return "", nil, errors.Fmt("requests[%d]: only one parent is allowed: %q, %q", i, in.Parent, req.Parent)
		}

		// TODO(ddoman): limit the max request body size in prpc level.
		tSize += art.size
		if tSize > MaxBatchCreateArtifactSize {
			return "", nil, errors.Fmt("the total size of artifact contents exceeded %d", MaxBatchCreateArtifactSize)
		}
		arts[i] = art
	}
	return invID, arts, nil
}

// findNewArtifacts returns a list of the artifacts that don't have states yet.
// If one exists w/ different hash/size, this returns an error.
func findNewArtifacts(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) ([]*artifactCreationRequest, error) {
	// artifacts are not expected to exist in most cases, and this map would likely
	// be empty.
	type state struct {
		hash   string
		size   int64
		gcsURI string
	}
	var states map[string]state
	ks := spanner.KeySets()
	for _, a := range arts {
		ks = spanner.KeySets(invID.Key(a.parentID(), a.artifactID), ks)
	}
	var b spanutil.Buffer
	err := span.Read(ctx, "Artifacts", ks, []string{"ParentId", "ArtifactId", "RBECASHash", "Size", "GcsURI"}).Do(
		func(row *spanner.Row) (err error) {
			var pid, aid string
			var hash string
			var size = new(int64)
			var gcsURI string
			if err = b.FromSpanner(row, &pid, &aid, &hash, &size, &gcsURI); err != nil {
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
			states[invID.Key(pid, aid).String()] = state{hash, *size, gcsURI}
			return
		},
	)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "%s", err)
	}

	newArts := make([]*artifactCreationRequest, 0, len(arts)-len(states))
	for _, a := range arts {
		// Save the hash, so that it can be reused in the post-verification
		// after rbecase.UpdateBlob().
		if a.gcsURI == "" && a.hash == "" {
			h := sha256.Sum256(a.data)
			a.hash = artifacts.AddHashPrefix(hex.EncodeToString(h[:]))
		}
		st, ok := states[invID.Key(a.parentID(), a.artifactID).String()]
		if !ok {
			newArts = append(newArts, a)
			continue
		}
		if (a.gcsURI == "") != (st.gcsURI == "") {
			// Can't change from GCS to non-GCS and vice-versa
			return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different storage scheme`, a.name(invID))
		}
		if a.size != st.size {
			return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different size: %d != %d`, a.name(invID), a.size, st.size)
		}
		if a.gcsURI != "" {
			if a.gcsURI != st.gcsURI {
				return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different GCS URI: %s != %s`, a.name(invID), a.gcsURI, st.gcsURI)
			}
		} else {
			if a.hash != st.hash {
				return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different hash`, a.name(invID))
			}
		}
	}
	return newArts, nil
}

// checkArtStates checks if the states of the associated invocation and artifacts are
// compatible with creation of the artifacts. On success, it returns a list of
// the artifactCreationRequests of which artifact don't have states in Spanner yet.
func checkArtStates(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) (reqs []*artifactCreationRequest, invInfo *invocationInfo, err error) {
	var invState pb.Invocation_State
	var createTime time.Time
	var realm string

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return invocations.ReadColumns(ctx, invID, map[string]any{
			"State": &invState, "Realm": &realm, "CreateTime": &createTime,
		})
	})

	eg.Go(func() (err error) {
		reqs, err = findNewArtifacts(ctx, invID, arts)
		return
	})

	switch err := eg.Wait(); {
	case err != nil:
		return nil, nil, err
	case invState != pb.Invocation_ACTIVE:
		return nil, nil, appstatus.Errorf(codes.FailedPrecondition, "%s is not active", invID.Name())
	}
	return reqs, &invocationInfo{
		id:         string(invID),
		realm:      realm,
		createTime: createTime,
	}, nil
}

// createArtifactStates creates the states of given artifacts in Spanner.
func createArtifactStates(ctx context.Context, realm string, invID invocations.ID, arts []*artifactCreationRequest) error {
	var noStateArts []*artifactCreationRequest
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) (err error) {
		// Verify all the states again.
		noStateArts, _, err = checkArtStates(ctx, invID, arts)
		if err != nil {
			return err
		}
		if len(noStateArts) == 0 {
			logging.Warningf(ctx, "The states of all the artifacts already exist.")
		}
		for _, a := range noStateArts {
			span.BufferWrite(ctx, spanutil.InsertMap("Artifacts", map[string]any{
				"InvocationId": invID,
				"ParentId":     a.parentID(),
				"ArtifactId":   a.artifactID,
				"ContentType":  a.contentType,
				"Size":         a.size,
				"RBECASHash":   a.hash,
				"GcsURI":       a.gcsURI,
			}))
		}
		return nil
	})
	if err != nil {
		return errors.Fmt("failed to write artifact to Spanner: %w", err)
	}
	spanutil.IncRowCount(ctx, len(noStateArts), spanutil.Artifacts, spanutil.Inserted, realm)
	return nil
}

func uploadArtifactBlobs(ctx context.Context, rbeIns string, casClient repb.ContentAddressableStorageClient, invID invocations.ID, arts []*artifactCreationRequest) error {
	casReq := &repb.BatchUpdateBlobsRequest{InstanceName: rbeIns}
	for _, a := range arts {
		casReq.Requests = append(casReq.Requests, &repb.BatchUpdateBlobsRequest_Request{
			Digest: &repb.Digest{Hash: artifacts.TrimHashPrefix(a.hash), SizeBytes: a.size},
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
			return errors.Fmt("artifact %q: cas.BatchUpdateBlobs failed", arts[i].name(invID))
		}
	}
	return nil
}

// allowedBucketsForUser returns the GCS buckets a user is allowed to reference by reading
// the project config.
// If no config exists for the user, an empty map will be returned, rather than an error.
func allowedBucketsForUser(ctx context.Context, project, user string) (allowedBuckets map[string]bool, err error) {
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

// BatchCreateArtifacts implements pb.RecorderServer.
// This functions uploads the artifacts to RBE-CAS.
// If the artifact is a text-based artifact, it will also get uploaded to BigQuery.
// We have a percentage control to determine how many percent of artifacts got
// uploaded to BigQuery.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}
	if len(in.Requests) == 0 {
		logging.Debugf(ctx, "Received a BatchCreateArtifactsRequest with 0 requests; returning")
		return &pb.BatchCreateArtifactsResponse{}, nil
	}
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}

	invID, arts, err := parseBatchCreateArtifactsRequest(in, cfg)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if err := validateInvocationToken(ctx, token, invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	var artsToCreate []*artifactCreationRequest
	var invInfo *invocationInfo
	func() {
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()
		artsToCreate, invInfo, err = checkArtStates(ctx, invID, arts)
	}()
	if err != nil {
		return nil, err
	}
	if len(artsToCreate) == 0 {
		logging.Debugf(ctx, "Found no artifacts to create")
		return &pb.BatchCreateArtifactsResponse{}, nil
	}
	realm := invInfo.realm
	project, _ := realms.Split(realm)
	user := auth.CurrentUser(ctx).Identity

	var allowedBuckets map[string]bool = nil
	artsToUpload := make([]*artifactCreationRequest, 0, len(artsToCreate))
	for _, a := range artsToCreate {
		// Only upload to RBE CAS the ones that are not in GCS
		if a.gcsURI == "" {
			artsToUpload = append(artsToUpload, a)
		} else {
			// Check this GCS reference is allowed by the project config.
			// Delay construction of the checker (which may occasionally involve an RPC) until we know we
			// actually need it.
			if allowedBuckets == nil {
				allowedBuckets, err = allowedBucketsForUser(ctx, project, string(user))
				if err != nil {
					return nil, errors.Fmt("fetch allowed buckets for user %s: %w", string(user), err)
				}
			}
			bucket, _ := gsutil.Split(a.gcsURI)
			if _, ok := allowedBuckets[bucket]; !ok {
				return nil, appstatus.Errorf(codes.PermissionDenied, "the user %s does not have permission to reference GCS objects in bucket %s in project %s", string(user), bucket, project)
			}
		}
	}

	if err := uploadArtifactBlobs(ctx, s.ArtifactRBEInstance, s.casClient, invID, artsToUpload); err != nil {
		return nil, err
	}
	if err := createArtifactStates(ctx, realm, invID, artsToCreate); err != nil {
		return nil, err
	}

	// Return all the artifacts to indicate that they were created.
	ret := &pb.BatchCreateArtifactsResponse{Artifacts: make([]*pb.Artifact, len(arts))}
	for i, a := range arts {
		var structuredTestIDProto *pb.TestIdentifierBase
		if a.testID != "" {
			structuredTestID, err := pbutil.ParseAndValidateTestID(a.testID)
			if err != nil {
				// Should never happen.
				return nil, errors.Fmt("parse test id: %w", err)
			}
			structuredTestIDProto = &pb.TestIdentifierBase{
				ModuleName:   structuredTestID.ModuleName,
				ModuleScheme: structuredTestID.ModuleScheme,
				CoarseName:   structuredTestID.CoarseName,
				FineName:     structuredTestID.FineName,
				CaseName:     structuredTestID.CaseName,
			}
		}
		ret.Artifacts[i] = &pb.Artifact{
			Name:             a.name(invID),
			TestIdStructured: structuredTestIDProto,
			TestId:           a.testID,
			ResultId:         a.resultID,
			ArtifactId:       a.artifactID,
			ContentType:      a.contentType,
			SizeBytes:        a.size,
		}
	}
	return ret, nil
}
