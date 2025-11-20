// Copyright 2025 The LUCI Authors.
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

package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/turboci/id"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket/appengine/model"
)

// getParentViaStage gets the parent build from stage.CreatedBy.
//
// Always returns an appstatus error when something is wrong.
func getParentViaStage(ctx context.Context, stage *orchestratorpb.Stage) (*model.Build, error) {
	stageAttemptID := stage.GetCreatedBy().GetStageAttempt()
	if stageAttemptID == nil {
		return nil, nil
	}

	stageAttemptIDStr := id.ToString(stageAttemptID)
	q := datastore.NewQuery(model.BuildKind).Eq("stage_attempt_id", stageAttemptIDStr)
	var blds []*model.Build
	if err := datastore.GetAll(ctx, q, &blds); err != nil {
		logging.Errorf(ctx, "Failed to query builds by stage_attempt_id %q: %s", stageAttemptID, err)
		return nil, appstatus.Error(codes.Internal, "failed to query builds by stage_attempt_id")
	}
	if len(blds) != 1 {
		buildIDs := make([]int64, len(blds))
		for i, bld := range blds {
			buildIDs[i] = bld.ID
		}
		logging.Errorf(ctx, "Expect 1 build by stage_attempt_id %q, but got %d: %v", stageAttemptIDStr, len(blds), buildIDs)
		return nil, appstatus.Errorf(codes.Internal, "expect 1 build by stage_attempt_id %q, but got %d", stageAttemptIDStr, len(blds))
	}
	return blds[0], nil
}
