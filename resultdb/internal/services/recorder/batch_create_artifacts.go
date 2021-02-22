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

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
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

var maxReqSizeOpt = &grpc.MaxSendMsgSizeCallOption{MaxBatchCreateArtifactSize}

type artifactCreationRequest struct {
	invID       invocations.ID
	testID      string
	resultID    string
	artifactID  string
	contentType string

	hash string
	size int64
	data []byte
}

// name returns the artifact name.
func (a *artifactCreationRequest) name() string {
	if a.testID == "" {
		return pbutil.InvocationArtifactName(string(a.invID), a.artifactID)
	}
	return pbutil.TestResultArtifactName(string(a.invID), a.testID, a.resultID, a.artifactID)
}

// parentID returns the local parent ID of the artifact.
func (a *artifactCreationRequest) parentID() string {
	return artifacts.ParentID(a.testID, a.resultID)
}

func parseArtifactParent(parent string) (invID, testID, resultID string, err error) {
	invID, testID, resultID, err = pbutil.ParseTestResultName(parent)
	if err != nil {
		if invID, err = pbutil.ParseInvocationName(parent); err != nil {
			err = errors.Reason("parent: neither valid invocation name nor valid test result name").Err()
			return
		}
	}
	return
}

// newArtifactCreationRequestsFromProto creates artifactionCreationRequests from
// from the BatchCreationArtifactsRequest w/o hash. The hash of each request is not set.
// Use compHashAndCheckState() for hash computation. It returns an error, if
// - any of the artifact IDs are invalid,
// - the total size exceeds MaxBatchCreateArtifactSize, or
// - there are more than one invocations associated with the artifacts.
func newArtifactCreationRequestsFromProto(in *pb.BatchCreateArtifactsRequest) ([]*artifactCreationRequest, error) {
	var tSize int64
	ret := make([]*artifactCreationRequest, 0, len(in.Requests))
	for i, req := range in.Requests {
		if req.Artifact == nil {
			continue
		}
		if err := pbutil.ValidateArtifactID(req.Artifact.ArtifactId); err != nil {
			return nil, errors.Annotate(err, "requests[%d]: artifact_id", i).Err()
		}

		invID, testID, resultID, err := parseArtifactParent(req.Parent)
		switch {
		case err != nil:
			return nil, errors.Annotate(err, "requests[%d]", i).Err()
		case i > 0 && invocations.ID(invID) != ret[i-1].invID:
			msg := "requests[%d]: parent: the invocation ID(%q) is different to the previous invocation ID(%q)"
			return nil, errors.Reason(msg, i, invID, ret[i-1].invID).Err()
		}

		cSize := int64(len(req.Artifact.Contents))
		tSize += cSize
		if tSize > MaxBatchCreateArtifactSize {
			return nil, errors.Reason("the total size of artifact contents exceeded %d", MaxBatchCreateArtifactSize).Err()
		}
		ret = append(ret, &artifactCreationRequest{
			invID:      invocations.ID(invID),
			testID:     testID,
			resultID:   resultID,
			artifactID: req.Artifact.ArtifactId,

			contentType: req.Artifact.ContentType,
			size:        cSize,
			data:        req.Artifact.Contents,
		})
	}
	return ret, nil
}

// compHashAndCheckState computes and sets the hash of arts. It returns an error if
// the associated invocation is not ACTIVE, or any of the artifacts already exists w/
// different hash or size. On success, it returns a list of the artifactCreationRequests
// of which artifact don't exist yet.
func compHashAndCheckState(ctx context.Context, arts []*artifactCreationRequest) ([]*artifactCreationRequest, error) {
	if len(arts) == 0 {
		return nil, nil
	}
	inv := arts[0].invID
	ret := make([]*artifactCreationRequest, 0, len(arts))

	// Read the state concurrently.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	err := parallel.FanOutIn(func(work chan<- func() error) {
		work <- func() (err error) {
			invState, err := invocations.ReadState(ctx, inv)
			if err == nil && invState != pb.Invocation_ACTIVE {
				return appstatus.Errorf(codes.FailedPrecondition, "%s is not active", inv.Name())
			}
			return
		}

		for _, a := range arts {
			work <- func() (err error) {
				h := sha256.Sum256(a.data)
				a.hash = fmt.Sprintf("sha256:%s", hex.EncodeToString(h[:]))
				exists, err := artifacts.Exist(ctx, inv, a.parentID(), a.artifactID, a.hash, a.size)
				if err == nil && !exists {
					ret = append(ret, a)
				}
				return errors.Annotate(err, "artifact %q", a.name()).Err()
			}
		}
	})

	if err != nil {
		return nil, err
	}
	return ret, nil
}

// BatchCreateArtifacts implements pb.RecorderServer.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}
	arts, err := newArtifactCreationRequestsFromProto(in)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if len(arts) == 0 {
		logging.Debugf(ctx, "Received a request with 0 artifacts; returning")
		return nil, nil
	}
	if err := validateInvocationToken(ctx, token, arts[0].invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	artsToCreate, err := compHashAndCheckState(ctx, arts)
	if err != nil {
		return nil, err
	}
	if len(artsToCreate) == 0 {
		logging.Debugf(ctx, "Found no artifacts to create")
		return nil, nil
	}

	// send
	casReq := &repb.BatchUpdateBlobsRequest{
		InstanceName: s.Options.ArtifactRBEInstance,
	}
	for _, a := range artsToCreate {
		casReq.Requests = append(casReq.Requests, &repb.BatchUpdateBlobsRequest_Request{
			Digest: &repb.Digest{Hash: a.hash, SizeBytes: a.size},
			Data:   a.data,
		})
	}
	resp, err := s.casClient.BatchUpdateBlobs(ctx, casReq, maxReqSizeOpt)
	if err != nil {
		// The only possible error here is INVALID_ARGUMENT, which indicates that
		// the client attempted to upload more than the server supported limit.
		logging.Errorf(ctx, "RBE-CAS.BatchUpdateBlobs() failed w/ %s", err)
		return nil, appstatus.Errorf(codes.Internal, "Exceeded the maximum size limit")
	}

	for _, r := range resp.Responses {
		cd := codes.Code(r.Status.Code)
		if cd != codes.OK {
			logging.Errorf(ctx, "artifact %q: RBE-CAS.BatchUploadBlobs() responded %s", cd)
		}
		err = appstatus.Errorf(codes.Internal, "Failed to upload the artifact contents to the storage backend.")
	}
	if err != nil {
		return nil, err
	}

	// Save them in Spanner, if all the above succeeded.
	err = createArtifactStates(ctx, artsToCreate)
	if err != nil {
		return nil, err
	}

	// Duplicates are skipped silently and not considered as an error.
	// Therefore, this returns all the input artifacts, including both skipped and created
	// ones.
	return &pb.BatchCreateArtifactsResponse{Artifacts: artifactCreationRequestsToProto(arts)}, nil
}

func artifactCreationRequestsToProto(arts []*artifactCreationRequest) []*pb.Artifact {
	ret := make([]*pb.Artifact, len(arts))
	for i, a := range arts {
		ret[i] = &pb.Artifact{Name: a.name()}
	}
	return ret
}

// createArtifactStates creates the states of given artifacts in Spanner.
func createArtifactStates(ctx context.Context, arts []*artifactCreationRequest) error {
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		inv := arts[0].invID
		return parallel.FanOutIn(func(work chan<- func() error) {
			work <- func() (err error) {
				invState, err := invocations.ReadState(ctx, inv)
				if err == nil && invState != pb.Invocation_ACTIVE {
					return appstatus.Errorf(codes.FailedPrecondition, "%s is not active", inv.Name())
				}
				return
			}

			for _, a := range arts {
				work <- func() (err error) {
					// Verify the state again.
					exists, err := artifacts.Exist(ctx, inv, a.parentID(), a.artifactID, a.hash, a.size)
					if err != nil || exists {
						return errors.Annotate(err, "artifact %q", a.name()).Err()
					}
					span.BufferWrite(ctx, spanutil.InsertMap("Artifacts", map[string]interface{}{
						"InvocationId": a.invID,
						"ParentId":     a.parentID(),
						"ArtifactId":   a.artifactID,
						"ContentType":  a.contentType,
						"Size":         a.size,
						"RBECASHash":   a.hash,
					}))
					return
				}
			}
		})
	})
	return err
}
