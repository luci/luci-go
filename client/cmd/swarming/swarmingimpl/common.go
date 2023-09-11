// Copyright 2015 The LUCI Authors.
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

package swarmingimpl

import (
	"bytes"
	"context"
	"net/url"
	"sort"
	"strings"
	"time"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"

	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/base"
	"go.chromium.org/luci/client/cmd/swarming/swarmingimpl/swarming"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

const (
	// Number of tasks and bots to grab in List* requests
	DefaultNumberToFetch = 1000
)

type swarmingServiceImpl struct {
	botsClient  swarmingv2.BotsClient
	tasksClient swarmingv2.TasksClient
}

func (s *swarmingServiceImpl) NewTask(ctx context.Context, req *swarmingv2.NewTaskRequest) (res *swarmingv2.TaskRequestMetadataResponse, err error) {
	return s.tasksClient.NewTask(ctx, req)
}

func (s *swarmingServiceImpl) CountTasks(ctx context.Context, start float64, state swarmingv2.StateQuery, tags ...string) (res *swarmingv2.TasksCount, err error) {
	return s.tasksClient.CountTasks(ctx, &swarmingv2.TasksCountRequest{
		Start: &timestamppb.Timestamp{
			Seconds: int64(start),
		},
		Tags:  tags,
		State: state,
	})
}

func (s *swarmingServiceImpl) ListTasks(ctx context.Context, limit int32, start float64, state swarmingv2.StateQuery, tags []string) ([]*swarmingv2.TaskResultResponse, error) {
	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	tasks := make([]*swarmingv2.TaskResultResponse, 0, limit)
	cursor := ""
	// Keep calling as long as there's a cursor indicating more tasks to list.
	for {
		var numberToFetch int32
		if limit < DefaultNumberToFetch {
			numberToFetch = limit
		} else {
			numberToFetch = DefaultNumberToFetch
		}
		req := &swarmingv2.TasksWithPerfRequest{
			Cursor:                  cursor,
			Limit:                   numberToFetch,
			Tags: tags,
			IncludePerformanceStats: false,
		}
		if start > 0 {
			req.Start = &timestamppb.Timestamp{
				Seconds: int64(start),
			}
		}
		tl, err := s.tasksClient.ListTasks(ctx, req)
		if err != nil {
			return tasks, err
		}
		limit -= int32(len(tl.Items))
		tasks = append(tasks, tl.Items...)
		cursor = tl.Cursor
		if cursor == "" || limit <= 0 {
			break
		}
	}

	return tasks, nil
}

func (s *swarmingServiceImpl) CancelTask(ctx context.Context, taskID string, killRunning bool) (res *swarmingv2.CancelResponse, err error) {
	return s.tasksClient.CancelTask(ctx, &swarmingv2.TaskCancelRequest{
		KillRunning: killRunning,
		TaskId:      taskID,
	})
}

func (s *swarmingServiceImpl) TaskRequest(ctx context.Context, taskID string) (res *swarmingv2.TaskRequestResponse, err error) {
	return s.tasksClient.GetRequest(ctx, &swarmingv2.TaskIdRequest{TaskId: taskID})
}

func (s *swarmingServiceImpl) TaskResult(ctx context.Context, taskID string, perf bool) (res *swarmingv2.TaskResultResponse, err error) {
	return s.tasksClient.GetResult(ctx, &swarmingv2.TaskIdWithPerfRequest{
		IncludePerformanceStats: perf,
		TaskId:                  taskID,
	})
}

func (s *swarmingServiceImpl) TaskOutput(ctx context.Context, taskID string) (res *swarmingv2.TaskOutputResponse, err error) {
	// We fetch 160 chunks every time which amounts to a max of 16mb each time.
	// Each chunk is 100kbs.
	// See https://chromium.googlesource.com/infra/luci/luci-py/+/b517353c0df0b52b4bdda4231ff37e749dc627af/appengine/swarming/api_common.py#343
	const outputLength = 160 * 100 * 1024

	var output bytes.Buffer
	for {
		resp, err := s.tasksClient.GetStdout(ctx, &swarmingv2.TaskIdWithOffsetRequest{
			Offset: int64(output.Len()),
			Length: outputLength,
			TaskId: taskID,
		})
		if err != nil {
			return nil, err
		}
		output.Write(resp.Output)
		// If there is less output bytes than length then we have reached the
		// final output chunk and can stop looking for new data.
		if len(resp.Output) < outputLength {
			// Pass the final state we saw as the current output
			return &swarmingv2.TaskOutputResponse{
				State:  resp.State,
				Output: output.Bytes(),
			}, nil
		}
	}
}

// FilesFromCAS downloads outputs from CAS.
func (s *swarmingServiceImpl) FilesFromCAS(ctx context.Context, outdir string, cascli *rbeclient.Client, casRef *swarmingv2.CASReference) ([]string, error) {
	d := digest.Digest{
		Hash: casRef.Digest.Hash,
		Size: casRef.Digest.SizeBytes,
	}
	outputs, _, err := cascli.DownloadDirectory(ctx, d, outdir, filemetadata.NewNoopCache())
	if err != nil {
		return nil, errors.Annotate(err, "failed to download directory").Err()
	}
	files := make([]string, 0, len(outputs))
	for path := range outputs {
		files = append(files, path)
	}
	sort.Strings(files)
	return files, nil
}

func (s *swarmingServiceImpl) CountBots(ctx context.Context, dimensions []*swarmingv2.StringPair) (res *swarmingv2.BotsCount, err error) {
	return s.botsClient.CountBots(ctx, &swarmingv2.BotsCountRequest{
		Dimensions: dimensions,
	})
}

func (s *swarmingServiceImpl) ListBots(ctx context.Context, dimensions []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error) {
	// TODO: Allow increasing the Limit past 1000. Ideally the server should treat
	// a missing Limit as "as much as will fit within the RPC response" (e.g.
	// 32MB). At the time of adding this Limit(1000) parameter, the server has
	// a hard-coded maximum page size of 1000, and a default Limit of 200.
	cursor := ""
	// Keep calling as long as there's a cursor indicating more bots to list.
	bots := make([]*swarmingv2.BotInfo, 0, DefaultNumberToFetch)
	for {
		resp, err := s.botsClient.ListBots(ctx, &swarmingv2.BotsRequest{
			Limit:      DefaultNumberToFetch,
			Cursor:     cursor,
			Dimensions: dimensions,
		})
		bots = append(bots, resp.Items...)
		if err != nil {
			return bots, err
		}

		cursor = resp.Cursor
		if cursor == "" {
			break
		}
	}
	return bots, nil
}

func (s *swarmingServiceImpl) DeleteBot(ctx context.Context, botID string) (res *swarmingv2.DeleteResponse, err error) {
	return s.botsClient.DeleteBot(ctx, &swarmingv2.BotRequest{
		BotId: botID,
	})
}

func (s *swarmingServiceImpl) TerminateBot(ctx context.Context, botID string, reason string) (res *swarmingv2.TerminateResponse, err error) {
	return s.botsClient.TerminateBot(ctx, &swarmingv2.TerminateRequest{
		BotId:  botID,
		Reason: reason,
	})
}

// TODO(vadimsh): Convert into flag.Value.
func stateMap(value string) (state swarmingv2.StateQuery, err error) {
	value = strings.ToLower(value)
	switch value {
	case "pending":
		state = swarmingv2.StateQuery_QUERY_PENDING
	case "running":
		state = swarmingv2.StateQuery_QUERY_RUNNING
	case "pending_running":
		state = swarmingv2.StateQuery_QUERY_PENDING_RUNNING
	case "completed":
		state = swarmingv2.StateQuery_QUERY_COMPLETED
	case "completed_success":
		state = swarmingv2.StateQuery_QUERY_COMPLETED_SUCCESS
	case "completed_failure":
		state = swarmingv2.StateQuery_QUERY_COMPLETED_FAILURE
	case "expired":
		state = swarmingv2.StateQuery_QUERY_EXPIRED
	case "timed_out":
		state = swarmingv2.StateQuery_QUERY_TIMED_OUT
	case "bot_died":
		state = swarmingv2.StateQuery_QUERY_BOT_DIED
	case "canceled":
		state = swarmingv2.StateQuery_QUERY_CANCELED
	case "":
	case "all":
		state = swarmingv2.StateQuery_QUERY_ALL
	case "deduped":
		state = swarmingv2.StateQuery_QUERY_DEDUPED
	case "killed":
		state = swarmingv2.StateQuery_QUERY_KILLED
	case "no_resource":
		state = swarmingv2.StateQuery_QUERY_NO_RESOURCE
	case "client_error":
		state = swarmingv2.StateQuery_QUERY_CLIENT_ERROR
	default:
		err = errors.Reason("Invalid state %s", value).Err()
	}
	return state, err
}

func (s *swarmingServiceImpl) ListBotTasks(ctx context.Context, botID string, limit int32, start float64, state swarmingv2.StateQuery) (res []*swarmingv2.TaskResultResponse, err error) {
	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	tasks := make([]*swarmingv2.TaskResultResponse, 0, limit)
	cursor := ""

	// Keep calling as long as there's a cursor indicating more tasks to list.
	for {
		var numberToFetch int32
		if limit < DefaultNumberToFetch {
			numberToFetch = limit
		} else {
			numberToFetch = DefaultNumberToFetch
		}
		req := &swarmingv2.BotTasksRequest{
			BotId:                   botID,
			Cursor:                  cursor,
			Limit:                   numberToFetch,
			State:                   state,
			IncludePerformanceStats: false,
		}
		if start > 0 {
			req.Start = &timestamppb.Timestamp{
				Seconds: int64(start),
			}
		}
		lbt, err := s.botsClient.ListBotTasks(ctx, req)
		if err != nil {
			return tasks, err
		}
		limit -= int32(len(lbt.Items))
		tasks = append(tasks, lbt.Items...)
		cursor = lbt.Cursor
		if cursor == "" || limit <= 0 {
			break
		}
	}

	return tasks, nil
}

// TaskIsAlive is true if the task is pending or running.
func TaskIsAlive(t swarmingv2.TaskState) bool {
	return t == swarmingv2.TaskState_PENDING || t == swarmingv2.TaskState_RUNNING
}

// TaskIsCompleted is true if the task has completed (successfully or not).
func TaskIsCompleted(t swarmingv2.TaskState) bool {
	return t == swarmingv2.TaskState_COMPLETED
}

func init() {
	// TODO(vadimsh): Move swarmingServiceImpl{} to a dedicate "swarming" package.
	base.SwarmingFactory = func(ctx context.Context, serverURL *url.URL, auth base.AuthFlags) (swarming.Swarming, error) {
		httpClient, err := auth.NewHTTPClient(ctx)
		if err != nil {
			return nil, err
		}
		prpcOpts := prpc.DefaultOptions()
		// The swarming server has an internal 60-second deadline for responding to
		// requests, so 90 seconds shouldn't cause any requests to fail that would
		// otherwise succeed.
		prpcOpts.PerRPCTimeout = 90 * time.Second
		prpcOpts.UserAgent = SwarmingUserAgent
		prpcOpts.Insecure = serverURL.Scheme == "http"
		prpcClient := prpc.Client{
			C:       httpClient,
			Options: prpcOpts,
			Host:    serverURL.Host,
		}
		return &swarmingServiceImpl{
			botsClient:  swarmingv2.NewBotsClient(&prpcClient),
			tasksClient: swarmingv2.NewTasksClient(&prpcClient),
		}, nil
	}
}
