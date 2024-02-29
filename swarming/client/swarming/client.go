// Copyright 2023 The LUCI Authors.
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

package swarming

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/cipd/version"
	"go.chromium.org/luci/client/casclient"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/grpc/prpc"

	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"
)

const (
	// ServerEnvVar is Swarming server host to which a client connects.
	ServerEnvVar = "SWARMING_SERVER"

	// TaskIDEnvVar is a Swarming task ID in which this task is running.
	//
	// The `swarming` command line tool uses this to populate `ParentTaskId`
	// when being used to trigger new tasks from within a swarming task.
	TaskIDEnvVar = "SWARMING_TASK_ID"

	// UserEnvVar is the OS user name (not Swarming specific).
	//
	// The `swarming` command line tool uses this to populate `User`
	// when being used to trigger new tasks.
	UserEnvVar = "USER"
)

// UserAgent identifies the version of the client.
//
// It is sent in all RPCs.
var UserAgent = "swarming 0.4.2"

func init() {
	ver, err := version.GetStartupVersion()
	if err != nil || ver.InstanceID == "" {
		return
	}
	UserAgent += fmt.Sprintf(" (%s@%s)", ver.PackageName, ver.InstanceID)
}

// Client can make requests to Swarming, in particular launch tasks and wait
// for their execution to finish.
//
// A client must be closed with Close when done working with it to avoid leaking
// goroutines.
type Client interface {
	Close(ctx context.Context)

	NewTask(ctx context.Context, req *swarmingv2.NewTaskRequest) (*swarmingv2.TaskRequestMetadataResponse, error)
	CountTasks(ctx context.Context, start float64, state swarmingv2.StateQuery, tags []string) (*swarmingv2.TasksCount, error)
	ListTasks(ctx context.Context, limit int32, start float64, state swarmingv2.StateQuery, tags []string) ([]*swarmingv2.TaskResultResponse, error)
	CancelTask(ctx context.Context, taskID string, killRunning bool) (*swarmingv2.CancelResponse, error)
	CancelTasks(ctx context.Context, limit int32, tags []string, killRunning bool, start, end time.Time) (*swarmingv2.TasksCancelResponse, error)

	TaskRequest(ctx context.Context, taskID string) (*swarmingv2.TaskRequestResponse, error)
	TaskOutput(ctx context.Context, taskID string, out io.Writer) (swarmingv2.TaskState, error)
	TaskResult(ctx context.Context, taskID string, fields *TaskResultFields) (*swarmingv2.TaskResultResponse, error)
	TaskResults(ctx context.Context, taskIDs []string, fields *TaskResultFields) ([]ResultOrErr, error)

	CountBots(ctx context.Context, dimensions []*swarmingv2.StringPair) (*swarmingv2.BotsCount, error)
	ListBots(ctx context.Context, dimensions []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error)
	DeleteBot(ctx context.Context, botID string) (*swarmingv2.DeleteResponse, error)
	TerminateBot(ctx context.Context, botID string, reason string) (*swarmingv2.TerminateResponse, error)
	ListBotTasks(ctx context.Context, botID string, limit int32, start float64, state swarmingv2.StateQuery) ([]*swarmingv2.TaskResultResponse, error)

	FilesFromCAS(ctx context.Context, outdir string, casRef *swarmingv2.CASReference) ([]string, error)
}

// TaskResultFields defines what optional parts of TaskResultResponse to get.
//
// Swarming doesn't support generic field masks yet, so this struct is kind of
// ad-hoc right now.
//
// A nil value means to fetch the default set of fields.
type TaskResultFields struct {
	WithPerf bool // if true, fetch internal performance stats
}

// ResultOrErr is returned by TaskResults. It either carries a task result or
// an error if it could not be obtained.
type ResultOrErr struct {
	Result *swarmingv2.TaskResultResponse
	Err    error
}

// ClientOptions is passed to NewClient.
type ClientOptions struct {
	// ServiceURL is root URL of the Swarming service.
	//
	// Required.
	ServiceURL string

	// RBEAddr is "host:port" of the RBE-CAS service to use.
	//
	// Default is the prod service.
	RBEAddr string

	// UserAgent is put into User-Agent HTTP header with each request.
	//
	// Default is UserAgent const.
	UserAgent string

	// Auth contains options for constructing authenticating clients.
	//
	// It is used only when AuthenticatedClient or RBEClientFactory are omitted.
	Auth auth.Options

	// AuthenticatedClient is http.Client that attaches authentication headers.
	//
	// Will be used when talking to the Swarming backend.
	//
	// Default is a client constructed using go.chromium.org/luci/auth based on
	// the given Auth options.
	AuthenticatedClient *http.Client

	// RBEClientFactory can create RBE clients on demand.
	//
	// Will be used to fetch files from RBE-CAS.
	//
	// Default constructs a client using go.chromium.org/luci/auth based on
	// the given Auth options. It calls LUCI Token Server to get per-instance RBE
	// authentication tokens. This works only with LUCI RBE instances.
	RBEClientFactory func(ctx context.Context, addr, instance string) (*rbeclient.Client, error)
}

// NewClient initializes Swarming client using given options.
//
// The passed context will become the root context for RBE client background
// goroutines.
func NewClient(ctx context.Context, opts ClientOptions) (Client, error) {
	if opts.ServiceURL == "" {
		return nil, errors.Reason("service URL is required").Err()
	}
	if opts.RBEAddr == "" {
		opts.RBEAddr = casclient.AddrProd
	}
	if opts.UserAgent == "" {
		opts.UserAgent = UserAgent
	}
	if opts.AuthenticatedClient == nil {
		cl, err := auth.NewAuthenticator(ctx, auth.SilentLogin, opts.Auth).Client()
		if err != nil {
			return nil, err
		}
		opts.AuthenticatedClient = cl
	}
	if opts.RBEClientFactory == nil {
		opts.RBEClientFactory = func(ctx context.Context, addr, instance string) (*rbeclient.Client, error) {
			return casclient.NewLegacy(ctx, addr, instance, opts.Auth, true)
		}
	}

	serverURL, err := lhttp.ParseHostURL(opts.ServiceURL)
	if err != nil {
		return nil, errors.Annotate(err, "bad service URL %q", opts.ServiceURL).Err()
	}

	prpcClient := prpc.Client{
		C:    opts.AuthenticatedClient,
		Host: serverURL.Host,
		Options: &prpc.Options{
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					MaxDelay: time.Minute,
					Limited: retry.Limited{
						Delay:   time.Second,
						Retries: 10,
					},
				}
			},
			// The swarming server has an internal 60-second deadline for responding to
			// requests, so 90 seconds shouldn't cause any requests to fail that would
			// otherwise succeed.
			PerRPCTimeout: 90 * time.Second,
			UserAgent:     opts.UserAgent,
			Insecure:      serverURL.Scheme == "http",
		},
	}

	return &swarmingServiceImpl{
		ctx:         ctx,
		opts:        opts,
		botsClient:  swarmingv2.NewBotsClient(&prpcClient),
		tasksClient: swarmingv2.NewTasksClient(&prpcClient),
		rbe:         map[string]*rbeclient.Client{},
	}, nil
}

////////////////////////////////////////////////////////////////////////////////

type swarmingServiceImpl struct {
	ctx         context.Context
	opts        ClientOptions
	botsClient  swarmingv2.BotsClient
	tasksClient swarmingv2.TasksClient

	m   sync.Mutex
	rbe map[string]*rbeclient.Client // instance name => RBE client
}

// rbeClient constructs a new RBE client or returns the existing one.
func (s *swarmingServiceImpl) rbeClient(inst string) (*rbeclient.Client, error) {
	if inst == "" {
		return nil, errors.Reason("no RBE instance name set").Err()
	}
	s.m.Lock()
	defer s.m.Unlock()
	if cl := s.rbe[inst]; cl != nil {
		return cl, nil
	}
	cl, err := s.opts.RBEClientFactory(s.ctx, s.opts.RBEAddr, inst)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create RBE client for %s", inst).Err()
	}
	s.rbe[inst] = cl
	return cl, nil
}

func (s *swarmingServiceImpl) Close(ctx context.Context) {
	s.m.Lock()
	defer s.m.Unlock()
	if len(s.rbe) != 0 {
		for inst, rbe := range s.rbe {
			logging.Debugf(ctx, "Closing RBE client for %s", inst)
			if err := rbe.Close(); err != nil {
				logging.Errorf(ctx, "Error closing RBE client for %s: %s", inst, err)
			}
		}
		logging.Debugf(ctx, "All RBE clients closed")
	}
}

func (s *swarmingServiceImpl) NewTask(ctx context.Context, req *swarmingv2.NewTaskRequest) (res *swarmingv2.TaskRequestMetadataResponse, err error) {
	return s.tasksClient.NewTask(ctx, req)
}

func (s *swarmingServiceImpl) CountTasks(ctx context.Context, start float64, state swarmingv2.StateQuery, tags []string) (res *swarmingv2.TasksCount, err error) {
	return s.tasksClient.CountTasks(ctx, &swarmingv2.TasksCountRequest{
		Start: &timestamppb.Timestamp{
			Seconds: int64(start),
		},
		Tags:  tags,
		State: state,
	})
}

func (s *swarmingServiceImpl) ListTasks(ctx context.Context, limit int32, start float64, state swarmingv2.StateQuery, tags []string) ([]*swarmingv2.TaskResultResponse, error) {
	const defaultPageSize = 1000

	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	tasks := make([]*swarmingv2.TaskResultResponse, 0, limit)
	cursor := ""
	// Keep calling as long as there's a cursor indicating more tasks to list.
	for {
		var pageSize int32
		if limit < defaultPageSize {
			pageSize = limit
		} else {
			pageSize = defaultPageSize
		}
		req := &swarmingv2.TasksWithPerfRequest{
			Cursor:                  cursor,
			Limit:                   pageSize,
			State:                   state,
			Tags:                    tags,
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

func (s *swarmingServiceImpl) CancelTasks(ctx context.Context, limit int32, tags []string, killRunning bool, start, end time.Time) (*swarmingv2.TasksCancelResponse, error) {
	requestLimit := int32(1000)
	cursor := ""
	// Keep calling as long as there's a cursor indicating more tasks to cancel.
	for {
		if limit < requestLimit {
			requestLimit = limit
		}
		cancelRequest := &swarmingv2.TasksCancelRequest{
			Limit:       requestLimit,
			Cursor:      cursor,
			Tags:        tags,
			KillRunning: killRunning,
		}
		if !start.IsZero() {
			cancelRequest.Start = timestamppb.New(start)
		}
		if !end.IsZero() {
			cancelRequest.End = timestamppb.New(end)
		}
		resp, err := s.tasksClient.CancelTasks(ctx, cancelRequest)
		if err != nil {
			return resp, err
		}

		limit -= resp.Matched
		cursor = resp.Cursor
		if cursor == "" || limit <= 0 {
			return resp, nil
		}
	}
}

func (s *swarmingServiceImpl) TaskRequest(ctx context.Context, taskID string) (res *swarmingv2.TaskRequestResponse, err error) {
	return s.tasksClient.GetRequest(ctx, &swarmingv2.TaskIdRequest{TaskId: taskID})
}

func (s *swarmingServiceImpl) TaskResult(ctx context.Context, taskID string, fields *TaskResultFields) (res *swarmingv2.TaskResultResponse, err error) {
	perf := false
	if fields != nil {
		perf = fields.WithPerf
	}
	return s.tasksClient.GetResult(ctx, &swarmingv2.TaskIdWithPerfRequest{
		IncludePerformanceStats: perf,
		TaskId:                  taskID,
	})
}

func (s *swarmingServiceImpl) TaskResults(ctx context.Context, taskIDs []string, fields *TaskResultFields) ([]ResultOrErr, error) {
	// TODO(vadimsh): Split large batches into multiple concurrent RPCs.
	perf := false
	if fields != nil {
		perf = fields.WithPerf
	}
	res, err := s.tasksClient.BatchGetResult(ctx, &swarmingv2.BatchGetResultRequest{
		TaskIds:                 taskIDs,
		IncludePerformanceStats: perf,
	})
	if err != nil {
		return nil, err
	}
	if len(res.Results) != len(taskIDs) {
		return nil, status.Errorf(codes.FailedPrecondition, "expecting %d items in the result, got %d", len(taskIDs), len(res.Results))
	}
	out := make([]ResultOrErr, len(taskIDs))
	for i, taskID := range taskIDs {
		if res.Results[i].TaskId != taskID {
			return nil, status.Errorf(codes.FailedPrecondition, "unexpected response format: expecting outcome of task %q, but got %q", taskID, res.Results[i].TaskId)
		}
		switch x := res.Results[i].Outcome.(type) {
		case *swarmingv2.BatchGetResultResponse_ResultOrError_Result:
			out[i].Result = x.Result
		case *swarmingv2.BatchGetResultResponse_ResultOrError_Error:
			out[i].Err = status.FromProto(x.Error).Err()
		default:
			return nil, status.Errorf(codes.FailedPrecondition, "unexpected response format: unexpected outcome of task %q", taskID)
		}
	}
	return out, nil
}

func (s *swarmingServiceImpl) TaskOutput(ctx context.Context, taskID string, out io.Writer) (state swarmingv2.TaskState, err error) {
	// We fetch 160 chunks every time which amounts to a max of 16mb each time.
	// Each chunk is 100kbs.
	// See https://chromium.googlesource.com/infra/luci/luci-py/+/b517353c0df0b52b4bdda4231ff37e749dc627af/appengine/swarming/api_common.py#343
	const perRequestLength = 160 * 100 * 1024

	var offset int64
	for {
		resp, err := s.tasksClient.GetStdout(ctx, &swarmingv2.TaskIdWithOffsetRequest{
			Offset: offset,
			Length: perRequestLength,
			TaskId: taskID,
		})
		if err != nil {
			return state, err
		}
		state = resp.State
		offset += int64(len(resp.Output))
		if len(resp.Output) != 0 {
			if _, err := out.Write(resp.Output); err != nil {
				return state, err
			}
		}
		// If there is less output bytes than what we requested, then we have
		// reached the final output chunk and can stop looking for new data.
		if len(resp.Output) < perRequestLength {
			return state, nil
		}
	}
}

// FilesFromCAS downloads outputs from CAS.
func (s *swarmingServiceImpl) FilesFromCAS(ctx context.Context, outdir string, casRef *swarmingv2.CASReference) ([]string, error) {
	cascli, err := s.rbeClient(casRef.CasInstance)
	if err != nil {
		return nil, err
	}
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
	const defaultPageSize = 1000

	cursor := ""
	// Keep calling as long as there's a cursor indicating more bots to list.
	bots := make([]*swarmingv2.BotInfo, 0, defaultPageSize)
	for {
		resp, err := s.botsClient.ListBots(ctx, &swarmingv2.BotsRequest{
			Limit:      defaultPageSize,
			Cursor:     cursor,
			Dimensions: dimensions,
		})
		if err != nil {
			return bots, err
		}
		bots = append(bots, resp.Items...)

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

func (s *swarmingServiceImpl) ListBotTasks(ctx context.Context, botID string, limit int32, start float64, state swarmingv2.StateQuery) (res []*swarmingv2.TaskResultResponse, err error) {
	const defaultPageSize = 1000

	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	tasks := make([]*swarmingv2.TaskResultResponse, 0, limit)
	cursor := ""

	// Keep calling as long as there's a cursor indicating more tasks to list.
	for {
		var pageSize int32
		if limit < defaultPageSize {
			pageSize = limit
		} else {
			pageSize = defaultPageSize
		}
		req := &swarmingv2.BotTasksRequest{
			BotId:                   botID,
			Cursor:                  cursor,
			Limit:                   pageSize,
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
