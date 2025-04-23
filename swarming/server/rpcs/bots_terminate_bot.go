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

package rpcs

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

const (
	maxReasonLength           = 1000
	terminationTaskExpiration = 5 * 24 * time.Hour
)

// TerminateBot implements the corresponding RPC method.
func (srv *BotsServer) TerminateBot(ctx context.Context, req *apipb.TerminateRequest) (*apipb.TerminateResponse, error) {
	if req.BotId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bot_id is required")
	}
	reason := req.Reason
	if req.Reason != "" {
		reason := strings.ReplaceAll(req.Reason, "\n", " ")
		reason = strings.ReplaceAll(reason, "\r", " ")
		if len(reason) > maxReasonLength {
			return nil, status.Errorf(codes.InvalidArgument, "reason is too long: %d > %d", len(reason), maxReasonLength)
		}
	}
	res := State(ctx).ACL.CheckBotPerm(ctx, req.BotId, acls.PermPoolsTerminateBot)
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}

	// Skip submitting a new task if there's already a pending termination task.
	// This matters when GCE Provider terminate bots that run for days. It calls
	// TerminateBot every hour, resulting in a big backlog of pending termination
	// tasks.
	//
	// Note that we do not care about transactions or race conditions here, since
	// it is OK to submit a duplicate termination task once in a while if the
	// existing one is being processed right now. Such duplicate task will just
	// naturally eventually expire.
	taskID, err := findExistingTerminationTask(ctx, req.BotId)
	if err != nil {
		return nil, err
	}
	if taskID != "" {
		return &apipb.TerminateResponse{
			TaskId: taskID,
		}, nil
	}

	// Create a new Termination task.
	task, err := srv.newTerminationTask(ctx, req.BotId, reason)
	if err != nil {
		return nil, err
	}
	return &apipb.TerminateResponse{
		TaskId: model.RequestKeyToTaskID(task.Result.TaskRequestKey(), model.AsRequest),
	}, nil
}

func findExistingTerminationTask(ctx context.Context, botID string) (string, error) {
	tags := []string{
		fmt.Sprintf("id:%s", botID),
		"swarming.terminate:1",
	}

	filters, err := model.NewFilterFromTags(tags)
	if err != nil {
		return "", err
	}
	qs, _ := model.FilterTasksByState(
		model.TaskResultSummaryQuery(),
		apipb.StateQuery_QUERY_PENDING,
		model.SplitOptimally)
	if len(qs) != 1 {
		panic(fmt.Sprintf("expecting one query for pending tasks, got %d.", len(qs)))
	}
	qs = model.FilterTasksByTags(qs[0], model.SplitOptimally, filters)
	if len(qs) != 1 {
		panic(fmt.Sprintf("expecting one query for pending tasks with tags, got %d.", len(qs)))
	}
	q := qs[0]
	q = q.Limit(1).KeysOnly(true)

	var existing []*datastore.Key
	if err = datastore.GetAll(ctx, q, &existing); err != nil {
		logging.Errorf(ctx, "Error querying TaskResultSummary: %s", err)
		return "", status.Errorf(codes.Internal, "datastore error fetching tasks")
	}
	if len(existing) == 0 {
		return "", nil
	}
	return model.RequestKeyToTaskID(existing[0].Parent(), model.AsRequest), nil
}

func (srv *BotsServer) newTerminationTask(ctx context.Context, botID, reason string) (*tasks.CreatedTask, error) {
	req, err := newTerminateRequest(ctx, botID, reason)
	if err != nil {
		return nil, err
	}

	creationOp := &tasks.CreationOp{
		Request: req,
		Config:  State(ctx).Config,
	}

	return createTaskWithRetry(ctx, creationOp, srv.TasksManager)
}

func newTerminateRequest(ctx context.Context, botID, reason string) (*model.TaskRequest, error) {
	rbe, err := State(ctx).Config.RBEConfig(botID)
	switch {
	case err != nil:
		logging.Errorf(ctx, "Error fetching RBE config: %s", err)
		return nil, status.Errorf(codes.Internal, "error fetching RBE config")
	case rbe.Instance == "":
		return nil, status.Errorf(codes.Internal, "RBE instance is required")
	}

	name := fmt.Sprintf("Terminate %s", botID)
	if reason != "" {
		name = fmt.Sprintf("%s: %s", name, reason)
	}
	now := clock.Now(ctx)
	tr := &model.TaskRequest{
		Created:     now,
		Name:        name,
		Priority:    0,
		Expiration:  now.Add(terminationTaskExpiration),
		RBEInstance: rbe.Instance,
		ManualTags: []string{
			"swarming.terminate:1",
			fmt.Sprintf("rbe:%s", rbe.Instance),
		},
		ServiceAccount: "none",
		Authenticated:  State(ctx).ACL.Caller(),
		TaskSlices: []model.TaskSlice{
			{
				ExpirationSecs: int64(terminationTaskExpiration.Seconds()),
				Properties: model.TaskProperties{
					Dimensions: model.TaskDimensions{
						"id": []string{botID},
					},
				},
			},
		},
	}
	autoTags := genAutoTags(tr, "none")
	tr.Tags = append(tr.ManualTags, autoTags...)
	tr.Tags = append(tr.Tags, fmt.Sprintf("id:%s", botID))
	tr.Tags = append(tr.Tags, "swarming.pool.template:no_pool")
	sort.Strings(tr.Tags)
	if !tr.IsTerminate() {
		panic(fmt.Sprintf("generated termination task %+v is not recognized as a termination task", tr))
	}
	return tr, nil
}
