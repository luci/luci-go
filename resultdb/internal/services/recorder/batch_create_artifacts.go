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

	"cloud.google.com/go/spanner"
	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TODO(crbug.com/1177213) - make this configurable.
const MaxBatchCreateArtifactSize = 10 * 1024 * 1024

type artifactCreationRequest struct {
	testID      string
	resultID    string
	artifactID  string
	contentType string

	hash   string
	size   int64
	data   []byte
	gcsURI string
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

func parseCreateArtifactRequest(req *pb.CreateArtifactRequest) (invocations.ID, *artifactCreationRequest, error) {
	if req.GetArtifact() == nil {
		return "", nil, errors.Reason("artifact: unspecified").Err()
	}
	if err := pbutil.ValidateArtifactID(req.Artifact.ArtifactId); err != nil {
		return "", nil, errors.Annotate(err, "artifact_id").Err()
	}
	if req.Artifact.ContentType != "" {
		if _, _, err := mime.ParseMediaType(req.Artifact.ContentType); err != nil {
			return "", nil, errors.Annotate(err, "content_type").Err()
		}
	}

	// parent
	if req.Parent == "" {
		return "", nil, errors.Reason("parent: unspecified").Err()
	}
	invIDStr, testID, resultID, err := pbutil.ParseTestResultName(req.Parent)
	if err != nil {
		if invIDStr, err = pbutil.ParseInvocationName(req.Parent); err != nil {
			return "", nil, errors.Reason("parent: neither valid invocation name nor valid test result name").Err()
		}
	}

	sizeBytes := int64(len(req.Artifact.Contents))

	if sizeBytes != 0 && req.Artifact.SizeBytes != 0 && sizeBytes != req.Artifact.SizeBytes {
		return "", nil, errors.Reason("sizeBytes and contents are specified but don't match").Err()
	}

	// If contents field is empty, try to set size from the request instead.
	if sizeBytes == 0 {
		if req.Artifact.SizeBytes != 0 {
			sizeBytes = req.Artifact.SizeBytes
		}
	}

	return invocations.ID(invIDStr), &artifactCreationRequest{
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
func parseBatchCreateArtifactsRequest(in *pb.BatchCreateArtifactsRequest) (invocations.ID, []*artifactCreationRequest, error) {
	var tSize int64
	var invID invocations.ID

	if err := pbutil.ValidateBatchRequestCount(len(in.Requests)); err != nil {
		return "", nil, err
	}
	arts := make([]*artifactCreationRequest, len(in.Requests))
	for i, req := range in.Requests {
		inv, art, err := parseCreateArtifactRequest(req)
		if err != nil {
			return "", nil, errors.Annotate(err, "requests[%d]", i).Err()
		}
		switch {
		case invID == "":
			invID = inv
		case invID != inv:
			return "", nil, errors.Reason("requests[%d]: only one invocation is allowed: %q, %q", i, invID, inv).Err()
		}

		// TODO(ddoman): limit the max request body size in prpc level.
		tSize += art.size
		if tSize > MaxBatchCreateArtifactSize {
			return "", nil, errors.Reason("the total size of artifact contents exceeded %d", MaxBatchCreateArtifactSize).Err()
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
		hash string
		size int64
	}
	var states map[string]state
	ks := spanner.KeySets()
	for _, a := range arts {
		ks = spanner.KeySets(invID.Key(a.parentID(), a.artifactID), ks)
	}
	var b spanutil.Buffer
	err := span.Read(ctx, "Artifacts", ks, []string{"ParentId", "ArtifactId", "RBECASHash", "Size"}).Do(
		func(row *spanner.Row) (err error) {
			var pid, aid string
			var hash string
			var size int64
			if err = b.FromSpanner(row, &pid, &aid, &hash, &size); err != nil {
				return
			}
			if states == nil {
				states = make(map[string]state)
			}
			// The artifact exists.
			states[invID.Key(pid, aid).String()] = state{hash, size}
			return
		},
	)
	if err != nil {
		return nil, appstatus.Errorf(codes.Internal, "%s", err)
	}

	newArts := make([]*artifactCreationRequest, 0, len(arts)-len(states))
	for _, a := range arts {
		st, ok := states[invID.Key(a.parentID(), a.artifactID).String()]
		if ok && a.size != st.size {
			return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different size: %d != %d`, a.name(invID), a.size, st.size)
		}

		// Save the hash, so that it can be reused in the post-verification
		// after rbecase.UpdateBlob().
		if a.hash == "" {
			h := sha256.Sum256(a.data)
			a.hash = artifacts.AddHashPrefix(hex.EncodeToString(h[:]))
		}

		switch {
		case !ok:
			newArts = append(newArts, a)
		case a.hash != st.hash:
			return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different hash`, a.name(invID))
		default:
			// artifact exists
		}
	}
	return newArts, nil
}

// checkArtStates checks if the states of the associated invocation and artifacts are
// compatible with creation of the artifacts. On success, it returns a list of
// the artifactCreationRequests of which artifact don't have states in Spanner yet.
func checkArtStates(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) (reqs []*artifactCreationRequest, realm string, err error) {
	var invState pb.Invocation_State

	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return invocations.ReadColumns(ctx, invID, map[string]interface{}{
			"State": &invState, "Realm": &realm,
		})
	})

	eg.Go(func() (err error) {
		reqs, err = findNewArtifacts(ctx, invID, arts)
		return
	})

	switch err := eg.Wait(); {
	case err != nil:
		return nil, "", err
	case invState != pb.Invocation_ACTIVE:
		return nil, "", appstatus.Errorf(codes.FailedPrecondition, "%s is not active", invID.Name())
	}
	return reqs, realm, nil
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
			span.BufferWrite(ctx, spanutil.InsertMap("Artifacts", map[string]interface{}{
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
		return errors.Annotate(err, "failed to write artifact to Spanner").Err()
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
	resp, err := casClient.BatchUpdateBlobs(ctx, casReq, &grpc.MaxSendMsgSizeCallOption{MaxBatchCreateArtifactSize})
	if err != nil {
		// If BatchUpdateBlobs() returns INVALID_ARGUMENT, it means that
		// the total size of the artifact contents was bigger than the max size that
		// BatchUpdateBlobs() can accept.
		return errors.Annotate(err, "cas.BatchUpdateBlobs failed").Err()
	}
	for i, r := range resp.GetResponses() {
		cd := codes.Code(r.Status.Code)
		if cd != codes.OK {
			// Each individual error can be due to resource exhausted or unmatched digest.
			// If unmatched digest, this RPC has a bug and needs to be fixed.
			// If resource exhausted, the RBE server quota needs to be adjusted.
			//
			// Either case, it's a server-error, and an internal error will be returned.
			return errors.Reason("artifact %q: cas.BatchUpdateBlobs failed", arts[i].name(invID)).Err()
		}
	}
	return nil
}

// BatchCreateArtifacts implements pb.RecorderServer.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}
	if len(in.Requests) == 0 {
		logging.Debugf(ctx, "Received a BatchCreateArtifactsRequest with 0 requests; returning")
		return &pb.BatchCreateArtifactsResponse{}, nil
	}
	invID, arts, err := parseBatchCreateArtifactsRequest(in)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if err := validateInvocationToken(ctx, token, invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	var artsToCreate []*artifactCreationRequest
	var realm string
	func() {
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()
		artsToCreate, realm, err = checkArtStates(ctx, invID, arts)
	}()
	if err != nil {
		return nil, err
	}
	if len(artsToCreate) == 0 {
		logging.Debugf(ctx, "Found no artifacts to create")
		return &pb.BatchCreateArtifactsResponse{}, nil
	}

	artsToUpload := make([]*artifactCreationRequest, 0, len(artsToCreate))
	for _, a := range artsToCreate {
		// Only upload to RBE CAS the ones that are not in GCS
		if a.gcsURI == "" {
			artsToUpload = append(artsToUpload, a)
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
		ret.Artifacts[i] = &pb.Artifact{
			Name:        a.name(invID),
			ArtifactId:  a.artifactID,
			ContentType: a.contentType,
			SizeBytes:   a.size,
		}
	}
	return ret, nil
}
