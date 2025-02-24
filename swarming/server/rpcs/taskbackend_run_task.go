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
	"maps"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

// RunTask implements bbpb.TaskBackendServer.
func (srv *TaskBackend) RunTask(ctx context.Context, req *bbpb.RunTaskRequest) (*bbpb.RunTaskResponse, error) {
	if err := srv.CheckBuildbucket(ctx); err != nil {
		return nil, err
	}

	cfg, err := ingestBackendConfigWithDefaults(req.GetBackendConfig())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad backend config in request: %s", err)
	}

	if err := srv.validateRunTaskRequest(ctx, req, cfg); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "bad request: %s", err)
	}

	newTaskReq, err := computeNewTaskRequest(ctx, req, cfg)
	if err != nil {
		return nil, err
	}

	bt := &model.BuildTask{
		BuildID:          req.BuildId,
		BuildbucketHost:  req.BuildbucketHost,
		PubSubTopic:      req.PubsubTopic,
		LatestTaskStatus: apipb.TaskState_PENDING,
		UpdateID:         clock.Now(ctx).UnixNano(),
	}
	res, err := srv.TasksServer.newTask(ctx, newTaskReq, bt)
	if err != nil {
		return nil, err
	}

	return srv.convertNewTaskToRunTaskResponse(ctx, res), nil
}

func (srv *TaskBackend) validateRunTaskRequest(ctx context.Context, req *bbpb.RunTaskRequest, cfg *apipb.SwarmingTaskBackendConfig) error {
	var err error
	switch {
	case req.BuildId == "":
		return errors.New("build_id is required")
	case req.PubsubTopic == "":
		return errors.New("pubsub_topic is required")
	case teeErr(srv.validateTarget(req.Target), &err) != nil:
		return errors.Annotate(err, "target").Err()
	case req.StartDeadline.AsTime().Before(clock.Now(ctx)):
		return errors.New("start_deadline must be in the future")
	case cfg.GetAgentBinaryCipdFilename() == "":
		return errors.New("agent_binary_cipd_filename: required")
	default:
		return nil
	}
}

// computeNewTaskRequest converts *bbpb.RunTaskRequest to apipb.NewTaskRequest.
//
// If there's an issue, returns a grpc error.
func computeNewTaskRequest(ctx context.Context, req *bbpb.RunTaskRequest, cfg *apipb.SwarmingTaskBackendConfig) (*apipb.NewTaskRequest, error) {
	name := cfg.TaskName
	if name == "" {
		name = fmt.Sprintf("bb-%s", req.BuildId)
	}

	slices, err := computeTaskSlices(ctx, req, cfg)
	if err != nil {
		return nil, err
	}

	return &apipb.NewTaskRequest{
		Name:                 name,
		ParentTaskId:         cfg.ParentRunId,
		Priority:             cfg.Priority,
		ServiceAccount:       cfg.ServiceAccount,
		BotPingToleranceSecs: int32(cfg.BotPingTolerance),
		Tags:                 cfg.Tags,
		Realm:                req.Realm,
		TaskSlices:           slices,
		RequestUuid:          req.RequestId,
	}, nil
}

func computeTaskSlices(ctx context.Context, req *bbpb.RunTaskRequest, cfg *apipb.SwarmingTaskBackendConfig) ([]*apipb.TaskSlice, error) {
	cmd := []string{cfg.AgentBinaryCipdFilename}
	cmd = append(cmd, req.AgentArgs...)

	caches := make([]*apipb.CacheEntry, 0, len(req.Caches))
	for _, c := range req.Caches {
		caches = append(caches, &apipb.CacheEntry{
			Name: c.Name,
			Path: c.Path,
		})
	}

	dimsByExp, exps, err := computeDimsByExp(req)
	if err != nil {
		return nil, err
	}
	if len(exps) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "dimensions are required")
	}

	secretBytes, err := proto.Marshal(req.Secrets)
	if err != nil {
		logging.Errorf(ctx, "failed to marshal secrets: %s", err)
		return nil, status.Errorf(codes.Internal, "failed to marshal secrets")
	}

	genSlice := func(dims []*apipb.StringPair, exp time.Duration) *apipb.TaskSlice {
		return &apipb.TaskSlice{
			ExpirationSecs: int32(exp.Seconds()),
			Properties: &apipb.TaskProperties{
				GracePeriodSecs:      int32(req.GracePeriod.Seconds),
				ExecutionTimeoutSecs: int32(req.ExecutionTimeout.Seconds),
				Command:              cmd,
				Caches:               caches,
				Dimensions:           dims,
				CipdInput: &apipb.CipdInput{
					Packages: []*apipb.CipdPackage{
						{
							PackageName: cfg.AgentBinaryCipdPkg,
							Version:     cfg.AgentBinaryCipdVers,
							Path:        ".",
						},
					},
					Server: cfg.AgentBinaryCipdServer,
				},
				SecretBytes: secretBytes,
			},
		}
	}

	baseExp := req.StartDeadline.AsTime().Sub(clock.Now(ctx))

	var lastExp time.Duration
	slices := make([]*apipb.TaskSlice, 0, len(exps))
	for i := 1; i < len(exps); i++ {
		curExp := exps[i] - lastExp
		slices = append(slices, genSlice(dimsByExp[exps[i]], curExp))
		lastExp = exps[i]
	}

	baseExp = max(baseExp-lastExp, 60*time.Second)
	slices = append(slices, genSlice(dimsByExp[exps[0]], baseExp))
	return slices, nil
}

// computeDimsByExp returns dimensions grouped by expirations and a sorted list
// of expirations.
// Dimensions in each group have been sorted by key and value.
//
// If there's an issue, returns a grpc error.
func computeDimsByExp(req *bbpb.RunTaskRequest) (map[time.Duration][]*apipb.StringPair, []time.Duration, error) {
	dimsByExp := make(map[time.Duration][]*apipb.StringPair)
	for i, cache := range req.Caches {
		exp := cache.WaitForWarmCache.AsDuration()
		if exp == 0 {
			continue
		}
		if exp%time.Minute != 0 {
			return nil, nil, status.Errorf(
				codes.InvalidArgument,
				"cache %d: wait_for_warm_cache must be a multiple of a minute", i)
		}
		dimsByExp[exp] = append(dimsByExp[exp], &apipb.StringPair{
			Key:   "caches",
			Value: cache.Name,
		})
	}

	for _, dim := range req.Dimensions {
		exp := dim.Expiration.AsDuration()
		dimsByExp[exp] = append(dimsByExp[exp], &apipb.StringPair{
			Key:   dim.Key,
			Value: dim.Value,
		})
	}
	exps := slices.Sorted(maps.Keys(dimsByExp))

	for i, exp := range exps {
		if exp == 0 {
			continue
		}
		// Add base dimensions if any.
		dimsByExp[exp] = append(dimsByExp[exp], dimsByExp[0]...)
		// Add dimensions with longer expirations.
		for j := i + 1; j < len(exps); j++ {
			dimsByExp[exp] = append(dimsByExp[exp], dimsByExp[exps[j]]...)
		}
		slices.SortFunc(dimsByExp[exp], func(a, b *apipb.StringPair) int {
			if n := strings.Compare(a.Key, b.Key); n != 0 {
				return n
			}
			return strings.Compare(a.Value, b.Value)
		})
	}

	return dimsByExp, exps, nil
}

func (srv *TaskBackend) convertNewTaskToRunTaskResponse(ctx context.Context, res *tasks.CreatedTask) *bbpb.RunTaskResponse {
	taskID := model.RequestKeyToTaskID(res.Result.TaskRequestKey(), model.AsRequest)

	var task *bbpb.Task
	if res.BuildTask != nil {
		task = res.BuildTask.ToProto(res.Result, srv.BuildbucketTarget)
	} else {
		// res.BuildTask can be nil when the task is dedupped by properties hash.
		// As of Feb 2025, we don't have plan to dedup build tasks by properties
		// hash, so this code should never be reached.
		task = &bbpb.Task{
			Id: &bbpb.TaskID{
				Id:     taskID,
				Target: srv.BuildbucketTarget,
			},
		}
	}
	task.Link = fmt.Sprintf("https://%s.appspot.com/task?id=%s", srv.TasksServer.SwarmingProject, taskID)
	return &bbpb.RunTaskResponse{
		Task: task,
	}
}
