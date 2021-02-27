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

func parseArtifactParent(parent string) (invIDStr, testID, resultID string, err error) {
	invIDStr, testID, resultID, err = pbutil.ParseTestResultName(parent)
	if err != nil {
		if invIDStr, err = pbutil.ParseInvocationName(parent); err != nil {
			err = errors.Reason("parent: neither valid invocation name nor valid test result name").Err()
			return
		}
	}
	return
}

// newArtifactCreationRequestsFromProto creates artifactCreationRequests from
// the BatchCreationArtifactsRequest w/o hash. The hash of each request is not set.
// Use compHashAndCheckState() for hash computation. It returns an error, if
// - any of the artifact IDs are invalid,
// - the total size exceeds MaxBatchCreateArtifactSize, or
// - there are more than one invocations associated with the artifacts.
func newArtifactCreationRequestsFromProto(in *pb.BatchCreateArtifactsRequest) (invID invocations.ID, arts []*artifactCreationRequest, err error) {
	var tSize int64
	arts = make([]*artifactCreationRequest, 0, len(in.Requests))
	for i, req := range in.Requests {
		art, parent := req.Artifact, req.Parent

		if art == nil {
			return "", nil, errors.Reason("requests[%d]: artifact: unspecified", i).Err()
		}
		if parent == "" {
			return "", nil, errors.Reason("requests[%d]: parent: unspecified", i).Err()
		}

		if err := pbutil.ValidateArtifactID(art.ArtifactId); err != nil {
			return "", nil, errors.Annotate(err, "requests[%d]: artifact_id", i).Err()
		}
		invIDStr, testID, resultID, err := parseArtifactParent(parent)
		switch {
		case err != nil:
			return "", nil, errors.Annotate(err, "requests[%d]", i).Err()
		case i == 0:
			invID = invocations.ID(invIDStr)
		case i > 0 && invocations.ID(invIDStr) != invID:
			msg := "requests[%d]: the invocation ID(%q) is different to the previous invocation ID(%q)"
			return "", nil, errors.Reason(msg, i, invIDStr, invID).Err()
		}

		if art.ContentType != "" {
			_, _, err := mime.ParseMediaType(art.ContentType)
			if err != nil {
				return "", nil, errors.Reason("requests[%d]: content_type: invalid %q", i, art.ContentType).Err()
			}
		}

		cSize := int64(len(art.Contents))
		tSize += cSize
		// TODO(ddoman): limit the max request body size in prpc level.
		if tSize > MaxBatchCreateArtifactSize {
			return "", nil, errors.Reason("the total size of artifact contents exceeded %d", MaxBatchCreateArtifactSize).Err()
		}

		arts = append(arts, &artifactCreationRequest{
			testID:     testID,
			resultID:   resultID,
			artifactID: art.ArtifactId,

			contentType: art.ContentType,
			size:        cSize,
			data:        art.Contents,
		})
	}
	return
}

// readArtifactStates returns a map of artifact states for the given artifactCreationRequests.
func readArtifactStates(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) (map[string]artifactCreationRequest, error) {
	var b spanutil.Buffer
	// most artifacts are not expected to exist, and this map will likely be empty.
	ret := map[string]artifactCreationRequest{}
	ks := spanner.KeySets()
	for _, a := range arts {
		ks = spanner.KeySets(invID.Key(a.parentID(), a.artifactID), ks)
	}
	err := span.Read(ctx, "Artifacts", ks, []string{"ParentId", "ArtifactId", "RBECASHash", "Size"}).Do(
		func(row *spanner.Row) (err error) {
			var pid, aid string
			var hash string
			var size int64

			if err = b.FromSpanner(row, &pid, &aid, &hash, &size); err != nil {
				return
			}
			ret[invID.Key(pid, aid).String()] = artifactCreationRequest{hash: hash, size: size}
			return
		},
	)
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// compHashAndCheckState computes and sets the hash of arts. It returns an error if
// the associated invocation is not ACTIVE, or any of the artifacts already exists w/
// different hash or size. On success, it returns a list of the artifactCreationRequests
// of which artifact don't exist yet.
func compHashAndCheckState(ctx context.Context, invID invocations.ID, arts []*artifactCreationRequest) ([]*artifactCreationRequest, error) {
	if len(arts) == 0 {
		return nil, nil
	}

	// read the states of inv and artifacts.
	var invState pb.Invocation_State
	var artStates map[string]artifactCreationRequest
	eg, ctx := errgroup.WithContext(ctx)
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()
	eg.Go(func() (err error) {
		invState, err = invocations.ReadState(ctx, invID)
		return err
	})
	eg.Go(func() (err error) {
		artStates, err = readArtifactStates(ctx, invID, arts)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	// early cancel the transaction.
	cancel()

	if invState != pb.Invocation_ACTIVE {
		return nil, appstatus.Errorf(codes.FailedPrecondition, "%s is not active", invID.Name())
	}
	newArts := make([]*artifactCreationRequest, 0, len(arts)-len(artStates))
	for _, a := range arts {
		st, ok := artStates[invID.Key(a.parentID(), a.artifactID).String()]
		if !ok {
			// New artifact. Calculate and save the hash.
			// The hash will be checked in the post-verification after rbecas.UpdateBlob().
			h := sha256.Sum256(a.data)
			a.hash = artifacts.AddHashPrefix(hex.EncodeToString(h[:]))
			newArts = append(newArts, a)
			continue
		}

		if a.size != st.size {
			return nil, appstatus.Errorf(codes.AlreadyExists, `exists w/ different size: %d != %d`, a.size, st.size)
		}

		h := sha256.Sum256(a.data)
		hStr := artifacts.AddHashPrefix(hex.EncodeToString(h[:]))
		if hStr != st.hash {
			return nil, appstatus.Errorf(codes.AlreadyExists, `exists w/ different hash`)
		}
	}

	return newArts, nil
}

// BatchCreateArtifacts implements pb.RecorderServer.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}
	invID, arts, err := newArtifactCreationRequestsFromProto(in)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if len(arts) == 0 {
		logging.Debugf(ctx, "Received a request with 0 artifacts; returning")
		return nil, nil
	}
	if err := validateInvocationToken(ctx, token, invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}
	artsToCreate, err := compHashAndCheckState(ctx, invID, arts)
	if err != nil {
		return nil, err
	}
	if len(artsToCreate) == 0 {
		logging.Debugf(ctx, "Found no artifacts to create")
		return nil, nil
	}

	return nil, nil
}
