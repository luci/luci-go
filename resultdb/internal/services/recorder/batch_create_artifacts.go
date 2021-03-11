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
	"golang.org/x/sync/errgroup"
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

	hash string
	size int64
	data []byte
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
	return invocations.ID(invIDStr), &artifactCreationRequest{
		artifactID:  req.Artifact.ArtifactId,
		contentType: req.Artifact.ContentType,
		data:        req.Artifact.Contents,
		size:        int64(len(req.Artifact.Contents)),
		testID:      testID,
		resultID:    resultID,
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

		h := sha256.Sum256(a.data)
		newHash := artifacts.AddHashPrefix(hex.EncodeToString(h[:]))
		switch {
		case !ok:
			// New artifact; save the hash, as it will be checked in the post-verification
			// after rbecase.UpdateBlob().
			a.hash = newHash
			newArts = append(newArts, a)
		case newHash != st.hash:
			return nil, appstatus.Errorf(codes.AlreadyExists, `%q: exists w/ different hash`, a.name(invID))
		default:
			// artifact exists
		}
	}
	return newArts, nil
}

// checkArtStates checks if the states of the associated invocation and artifacts are valid.
// On success, it returns a list of the artifactCreationRequests of which artifact don't
// have states in Spanner yet.
func checkArtStates(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) ([]*artifactCreationRequest, error) {
	eg, ctx := errgroup.WithContext(ctx)
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	eg.Go(func() error {
		invState, err := invocations.ReadState(ctx, invID)
		if invState != pb.Invocation_ACTIVE {
			return appstatus.Errorf(codes.FailedPrecondition, "%s is not active", invID.Name())
		}
		return err
	})

	var newArts []*artifactCreationRequest
	eg.Go(func() (err error) {
		newArts, err = findNewArtifacts(ctx, invID, arts)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return newArts, nil
}

// BatchCreateArtifacts implements pb.RecorderServer.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}
	if len(in.Requests) == 0 {
		logging.Debugf(ctx, "Received a BatchCreateArtifactsRequest with 0 requests; returning")
		return nil, nil
	}
	invID, arts, err := parseBatchCreateArtifactsRequest(in)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if err := validateInvocationToken(ctx, token, invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	artsToCreate, err := checkArtStates(ctx, invID, arts)
	if err != nil {
		return nil, err
	}
	if len(artsToCreate) == 0 {
		logging.Debugf(ctx, "Found no artifacts to create")
		return nil, nil
	}

	// TODO(ddoman): upload the artifacts to RBE-CAS.
	return nil, nil
}
