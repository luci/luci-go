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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TODO(crbug.com/1177213) - make this less-fragile.
const MaxBatchUploadArtifactSize = 10 * 1024 * 1024

// batchArtifactCreator creates many artifacts at once.
type batchArtifactCreator struct {
	// RBEInstance is the full name of the RBE instance used for artifact storage.
	// Format: projects/{project}/instances/{instance}.
	RBEInstance string
}

type artifactCreationRequest struct {
	invID       invocations.ID
	parentID    string
	artifactID  string
	contentType string

	hash string
	size int64
	data []byte
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
// from the BatchCreationArtifactsRequest w/o hash. Use compHashAndCheckState() for
// hash computation. It returns an error, if
//   - any of the artifact IDs are invalid,
//   - the total size exceeds MaxBatchUploadArtifactSize, or
//   - there are more than one invocations associated with the artifacts.
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
		if tSize > MaxBatchUploadArtifactSize {
			return nil, errors.Reason("the total size of artifact contents exceeded %d", MaxBatchUploadArtifactSize).Err()
		}
		ret = append(ret, &artifactCreationRequest{
			invID:      invocations.ID(invID),
			artifactID: req.Artifact.ArtifactId,
			parentID:   artifacts.ParentID(testID, resultID),

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
				panic(fmt.Sprintf("data %q hash %q", a.data, a.hash))
				exists, err := artifacts.Exist(ctx, inv, a.parentID, a.artifactID, a.hash, a.size)
				if err == nil && !exists {
					ret = append(ret, a)
				}
				return errors.Annotate(err, "artifact (%q, %q, %q)", inv, a.parentID, a.artifactID).Err()
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
	artsToSave, err := compHashAndCheckState(ctx, arts)
	if err != nil {
		return nil, err
	}
	if len(artsToSave) == 0 {
		logging.Debugf(ctx, "Found no artifacts to upload")
		return nil, nil
	}

	return nil, nil
}
