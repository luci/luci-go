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

package lib

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"github.com/maruel/subcommands"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/client/downloader"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/system/signals"
)

const (
	// Define environment variables used in Swarming client.

	// ServerEnvVar is Swarming server host to which a client connect.
	// Example: "chromium-swarm.appspot.com"
	ServerEnvVar = "SWARMING_SERVER"

	// TaskIDEnvVar is Swarming task ID in which this task is running.
	// The `swarming` command line tool uses this to populate `ParentTaskId`
	// when being used to trigger new tasks from within a swarming task.
	TaskIDEnvVar = "SWARMING_TASK_ID"

	// UserEnvVar is user name.
	// The `swarming` command line tool uses this to populate `User`
	// when being used to trigger new tasks.
	UserEnvVar = "USER"
)

// TriggerResults is a set of results from using the trigger subcommand,
// describing all of the tasks that were triggered successfully.
type TriggerResults struct {
	// Tasks is a list of successfully triggered tasks represented as
	// TriggerResult values.
	Tasks []*swarming.SwarmingRpcsTaskRequestMetadata `json:"tasks"`
}

// The swarming server has an internal 60-second deadline for responding to
// requests, so 90 seconds shouldn't cause any requests to fail that would
// otherwise succeed.
const swarmingRPCRequestTimeout = 90 * time.Second

const swarmingAPISuffix = "/_ah/api/swarming/v1/"

// swarmingService is an interface intended to stub out the swarming API
// bindings for testing.
type swarmingService interface {
	NewTask(ctx context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error)
	CountTasks(ctx context.Context, start float64, state string, tags ...string) (*swarming.SwarmingRpcsTasksCount, error)
	ListTasks(ctx context.Context, limit int64, state string, tags []string, fields []googleapi.Field) ([]*swarming.SwarmingRpcsTaskResult, error)
	CancelTask(ctx context.Context, taskID string, req *swarming.SwarmingRpcsTaskCancelRequest) (*swarming.SwarmingRpcsCancelResponse, error)
	GetTaskRequest(ctx context.Context, taskID string) (*swarming.SwarmingRpcsTaskRequest, error)
	GetTaskResult(ctx context.Context, taskID string, perf bool) (*swarming.SwarmingRpcsTaskResult, error)
	GetTaskOutput(ctx context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error)
	GetFilesFromIsolate(ctx context.Context, outdir string, isolateRef *swarming.SwarmingRpcsFilesRef) ([]string, error)
	GetFilesFromCAS(ctx context.Context, outdir string, cascli *rbeclient.Client, casRef *swarming.SwarmingRpcsCASReference) ([]string, error)
	CountBots(ctx context.Context, dimensions ...string) (*swarming.SwarmingRpcsBotsCount, error)
	ListBots(ctx context.Context, dimensions []string, fields []googleapi.Field) ([]*swarming.SwarmingRpcsBotInfo, error)
	DeleteBot(ctx context.Context, botID string) (*swarming.SwarmingRpcsDeletedResponse, error)
	TerminateBot(ctx context.Context, botID string) (*swarming.SwarmingRpcsTerminateResponse, error)
}

type swarmingServiceImpl struct {
	client  *http.Client
	service *swarming.Service
	worker  int
}

func (s *swarmingServiceImpl) NewTask(ctx context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (res *swarming.SwarmingRpcsTaskRequestMetadata, err error) {
	err = retryGoogleRPC(ctx, "NewTask", func() (ierr error) {
		res, ierr = s.service.Tasks.New(req).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) CountTasks(ctx context.Context, start float64, state string, tags ...string) (res *swarming.SwarmingRpcsTasksCount, err error) {
	err = retryGoogleRPC(ctx, "CountTasks", func() (ierr error) {
		res, ierr = s.service.Tasks.Count().Context(ctx).Start(start).State(state).Tags(tags...).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) ListTasks(ctx context.Context, limit int64, state string, tags []string, fields []googleapi.Field) ([]*swarming.SwarmingRpcsTaskResult, error) {
	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	tasks := []*swarming.SwarmingRpcsTaskResult{}
	// If no fields are specified, all fields will be returned. If any fields are
	// specified, ensure the cursor is specified so we can get subsequent pages.
	if len(fields) > 0 {
		fields = append(fields, "cursor")
	}
	call := s.service.Tasks.List().Context(ctx).Limit(limit).State(state).Tags(tags...).Fields(fields...)
	// Keep calling as long as there's a cursor indicating more bots to list.
	for {
		var res *swarming.SwarmingRpcsTaskList
		err := retryGoogleRPC(ctx, "ListTasks", func() (ierr error) {
			res, ierr = call.Do()
			return
		})
		if err != nil {
			return tasks, err
		}

		tasks = append(tasks, res.Items...)
		if res.Cursor == "" || int64(len(tasks)) >= limit || len(res.Items) == 0 {
			break
		}
		call.Cursor(res.Cursor)
	}

	if int64(len(tasks)) > limit {
		tasks = tasks[0:limit]
	}

	return tasks, nil
}

func (s *swarmingServiceImpl) CancelTask(ctx context.Context, taskID string, req *swarming.SwarmingRpcsTaskCancelRequest) (res *swarming.SwarmingRpcsCancelResponse, err error) {
	err = retryGoogleRPC(ctx, "CancelTask", func() (ierr error) {
		res, ierr = s.service.Task.Cancel(taskID, req).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) GetTaskRequest(ctx context.Context, taskID string) (res *swarming.SwarmingRpcsTaskRequest, err error) {
	err = retryGoogleRPC(ctx, "GetTaskRequest", func() (ierr error) {
		res, ierr = s.service.Task.Request(taskID).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) GetTaskResult(ctx context.Context, taskID string, perf bool) (res *swarming.SwarmingRpcsTaskResult, err error) {
	err = retryGoogleRPC(ctx, "GetTaskResult", func() (ierr error) {
		res, ierr = s.service.Task.Result(taskID).IncludePerformanceStats(perf).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) GetTaskOutput(ctx context.Context, taskID string) (res *swarming.SwarmingRpcsTaskOutput, err error) {
	err = retryGoogleRPC(ctx, "GetTaskOutput", func() (ierr error) {
		res, ierr = s.service.Task.Stdout(taskID).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) GetFilesFromIsolate(ctx context.Context, outdir string, isolateRef *swarming.SwarmingRpcsFilesRef) ([]string, error) {
	isolatedOpts := []isolatedclient.Option{
		isolatedclient.WithAuthClient(s.client),
		isolatedclient.WithNamespace(isolateRef.Namespace),
		isolatedclient.WithUserAgent(SwarmingUserAgent),
	}
	isolatedClient := isolatedclient.NewClient(isolateRef.Isolatedserver, isolatedOpts...)

	var filesMu sync.Mutex
	var files []string
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer signals.HandleInterrupt(cancel)()
	opts := &downloader.Options{
		MaxConcurrentJobs: s.worker,
		FileCallback: func(name string, _ *isolated.File) {
			filesMu.Lock()
			files = append(files, name)
			filesMu.Unlock()
		},
		FileStatsCallback: func(fileStats downloader.FileStats, _ time.Duration) {
			logging.Debugf(ctx, "Downloaded %d of %d bytes in %d of %d files to %s",
				fileStats.BytesCompleted, fileStats.BytesScheduled,
				fileStats.CountCompleted, fileStats.CountScheduled,
				outdir)
		},
	}
	dl := downloader.New(ctx, isolatedClient, isolated.HexDigest(isolateRef.Isolated), outdir, opts)
	return files, dl.Wait()
}

// GetFilesFromCAS downloads outputs from CAS.
func (s *swarmingServiceImpl) GetFilesFromCAS(ctx context.Context, outdir string, cascli *rbeclient.Client, casRef *swarming.SwarmingRpcsCASReference) ([]string, error) {
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
	return files, nil
}

func (s *swarmingServiceImpl) CountBots(ctx context.Context, dimensions ...string) (res *swarming.SwarmingRpcsBotsCount, err error) {
	err = retryGoogleRPC(ctx, "CountBots", func() (ierr error) {
		res, ierr = s.service.Bots.Count().Context(ctx).Dimensions(dimensions...).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) ListBots(ctx context.Context, dimensions []string, fields []googleapi.Field) ([]*swarming.SwarmingRpcsBotInfo, error) {
	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	bots := []*swarming.SwarmingRpcsBotInfo{}
	// If no fields are specified, all fields will be returned. If any fields are
	// specified, ensure the cursor is specified so we can get subsequent pages.
	if len(fields) > 0 {
		fields = append(fields, "cursor")
	}
	call := s.service.Bots.List().Context(ctx).Dimensions(dimensions...).Fields(fields...)
	// Keep calling as long as there's a cursor indicating more bots to list.
	for {
		var res *swarming.SwarmingRpcsBotList
		err := retryGoogleRPC(ctx, "ListBots", func() (ierr error) {
			res, ierr = call.Do()
			return
		})
		if err != nil {
			return bots, err
		}

		bots = append(bots, res.Items...)
		if res.Cursor == "" {
			break
		}
		call.Cursor(res.Cursor)
	}
	return bots, nil
}

func (s *swarmingServiceImpl) DeleteBot(ctx context.Context, botID string) (res *swarming.SwarmingRpcsDeletedResponse, err error) {
	err = retryGoogleRPC(ctx, "DeleteBot", func() (ierr error) {
		res, ierr = s.service.Bot.Delete(botID).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) TerminateBot(ctx context.Context, botID string) (res *swarming.SwarmingRpcsTerminateResponse, err error) {
	err = retryGoogleRPC(ctx, "TerminateBot", func() (ierr error) {
		res, ierr = s.service.Bot.Terminate(botID).Context(ctx).Do()
		return
	})
	return
}

type taskState int32

const (
	maskAlive                 = 1
	stateBotDied    taskState = 1 << 1
	stateCancelled  taskState = 1 << 2
	stateCompleted  taskState = 1 << 3
	stateExpired    taskState = 1 << 4
	statePending    taskState = 1<<5 | maskAlive
	stateRunning    taskState = 1<<6 | maskAlive
	stateTimedOut   taskState = 1 << 7
	stateNoResource taskState = 1 << 8
	stateKilled     taskState = 1 << 9
	stateUnknown    taskState = -1
)

func parseTaskState(state string) (taskState, error) {
	switch state {
	case "BOT_DIED":
		return stateBotDied, nil
	case "CANCELED":
		return stateCancelled, nil
	case "COMPLETED":
		return stateCompleted, nil
	case "EXPIRED":
		return stateExpired, nil
	case "PENDING":
		return statePending, nil
	case "RUNNING":
		return stateRunning, nil
	case "TIMED_OUT":
		return stateTimedOut, nil
	case "NO_RESOURCE":
		return stateNoResource, nil
	case "KILLED":
		return stateKilled, nil
	default:
		return stateUnknown, errors.Reason("unrecognized state: %q", state).Err()
	}
}

func (t taskState) Alive() bool {
	return (t & maskAlive) != 0
}

// AuthFlags is an interface to register auth flags and create http.Client and CAS Client.
type AuthFlags interface {
	// Register registers auth flags to the given flag set. e.g. -service-account-json.
	Register(f *flag.FlagSet)

	// Parse parses auth flags.
	Parse() error

	// NewHTTPClient creates an authroised http.Client.
	NewHTTPClient(ctx context.Context) (*http.Client, error)

	// NewCASClient creates an authroised RBE Client.
	NewCASClient(ctx context.Context, instance string) (*rbeclient.Client, error)
}

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags common.Flags
	authFlags    AuthFlags
	serverURL    string
	worker       int
}

// Init initializes common flags.
func (c *commonFlags) Init(authFlags AuthFlags) {
	c.defaultFlags.Init(&c.Flags)
	c.authFlags = authFlags
	c.authFlags.Register(&c.Flags)
	c.Flags.StringVar(&c.serverURL, "server", os.Getenv(ServerEnvVar), fmt.Sprintf("Server URL; required. Set $%s to set a default.", ServerEnvVar))
	c.Flags.StringVar(&c.serverURL, "S", os.Getenv(ServerEnvVar), "Alias for -server.")
	c.Flags.IntVar(&c.worker, "worker", 8, "Number of workers used to download isolated files.")
}

// Parse parses the common flags.
func (c *commonFlags) Parse() error {
	if err := c.defaultFlags.Parse(); err != nil {
		return err
	}
	if err := c.authFlags.Parse(); err != nil {
		return err
	}
	if c.serverURL == "" {
		return errors.Reason("must provide -server").Err()
	}
	s, err := lhttp.CheckURL(c.serverURL)
	if err != nil {
		return err
	}
	c.serverURL = s
	return nil
}

func (c *commonFlags) createSwarmingClient(ctx context.Context) (swarmingService, error) {
	authcli, err := c.authFlags.NewHTTPClient(ctx)
	if err != nil {
		return nil, err
	}
	// Create a copy of the client so that the timeout only applies to Swarming
	// RPC requests, not to Isolate requests made by this service. A shallow
	// copy is ok because only the timeout needs to be different.
	rpcClient := *authcli
	rpcClient.Timeout = swarmingRPCRequestTimeout
	s, err := swarming.NewService(ctx, option.WithHTTPClient(&rpcClient))
	if err != nil {
		return nil, err
	}
	s.BasePath = c.serverURL + swarmingAPISuffix
	s.UserAgent = SwarmingUserAgent
	return &swarmingServiceImpl{
		client:  authcli,
		service: s,
		worker:  c.worker}, nil
}

func tagTransientGoogleAPIError(err error) error {
	// Responses with HTTP codes < 500, if we got them, indicate fatal errors.
	if gerr, _ := err.(*googleapi.Error); gerr != nil && gerr.Code < 500 {
		return err
	}
	// Everything else (HTTP code >= 500, timeouts, DNS issues, etc) is considered
	// a transient error.
	return transient.Tag.Apply(err)
}

func printError(a subcommands.Application, err error) {
	fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
}

// retryGoogleRPC retries an RPC on transient errors, such as HTTP 500.
func retryGoogleRPC(ctx context.Context, rpcName string, rpc func() error) error {
	return retry.Retry(ctx, transient.Only(retry.Default), func() error {
		err := rpc()
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code >= 500 {
			return transient.Tag.Apply(err)
		}
		if err != nil {
			return errors.Annotate(err, "failed to call %s", rpcName).Err()
		}
		return nil
	}, retry.LogCallback(ctx, rpcName))
}
