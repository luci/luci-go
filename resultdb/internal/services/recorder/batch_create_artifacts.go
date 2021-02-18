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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/artifacts"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TODO(crbug.com/1177213) - make this less-fragile.
const MaxBatchUploadArtifactSize = 10 * 1024 * 1024

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

func parseBatchCreateArtifactsRequest(in *pb.BatchCreateArtifactsRequest) ([]*artifactCreator, error) {
	var ret []*artifactCreator
	var tSize int64

	for i, req := range in.Requests {
		if req == nil || req.Artifact == nil {
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
			return nil, errors.Reason("requests[%d]: parent: the invocation ID(%q) is different to the previous invocation ID(%q)", i, invID, ret[i-1].invID).Err()
		}

		cSize := int64(len(req.Artifact.Contents))
		tSize += cSize
		if tSize > MaxBatchUploadArtifactSize {
			return nil, errors.Reason("the total size of artifact contents exceeded %d", MaxBatchUploadArtifactSize).Err()
		}
		ret = append(ret, &artifactCreator{
			invID:         invocations.ID(invID),
			artifactID:    req.Artifact.ArtifactId,
			localParentID: artifacts.ParentID(testID, resultID),
			contentType:   req.Artifact.ContentType,
			size:          cSize,
		})
	}
	return ret, nil
}

// BatchCreateArtifacts implements pb.RecorderServer.
func (s *recorderServer) BatchCreateArtifacts(ctx context.Context, in *pb.BatchCreateArtifactsRequest) (*pb.BatchCreateArtifactsResponse, error) {
	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}

	arts, err := parseBatchCreateArtifactsRequest(in)
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}
	if len(arts) == 0 {
		return nil, nil
	}

	if err := validateInvocationToken(ctx, token, arts[0].invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	return nil, nil
}
