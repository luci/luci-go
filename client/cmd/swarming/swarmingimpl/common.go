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
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	rbeclient "github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/filemetadata"
	"github.com/maruel/subcommands"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/client/internal/common"
	swarmingv1 "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/prpc"
	swarmingv2 "go.chromium.org/luci/swarming/proto/api_v2"

	"google.golang.org/protobuf/encoding/protojson"
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

	// Number of tasks and bots to grab in List* requests
	Limit = 1000
)

// TriggerResults is a set of results from using the trigger subcommand,
// describing all of the tasks that were triggered successfully.
type TriggerResults struct {
	// Tasks is a list of successfully triggered tasks represented as
	// TriggerResult values.
	Tasks []*swarmingv1.SwarmingRpcsTaskRequestMetadata `json:"tasks"`
}

// The swarming server has an internal 60-second deadline for responding to
// requests, so 90 seconds shouldn't cause any requests to fail that would
// otherwise succeed.
const swarmingRPCRequestTimeout = 90 * time.Second

const swarmingAPISuffix = "/_ah/api/swarming/v1/"

// swarmingService is an interface intended to stub out the swarming API
// bindings for testing.
type swarmingService interface {
	NewTask(ctx context.Context, req *swarmingv1.SwarmingRpcsNewTaskRequest) (*swarmingv1.SwarmingRpcsTaskRequestMetadata, error)
	CountTasks(ctx context.Context, start float64, state string, tags ...string) (*swarmingv1.SwarmingRpcsTasksCount, error)
	ListTasks(ctx context.Context, limit int64, start float64, state string, tags []string, fields []googleapi.Field) ([]*swarmingv1.SwarmingRpcsTaskResult, error)
	CancelTask(ctx context.Context, taskID string, killRunning bool) (*swarmingv2.CancelResponse, error)
	TaskRequest(ctx context.Context, taskID string) (*swarmingv2.TaskRequestResponse, error)
	TaskOutput(ctx context.Context, taskID string) (*swarmingv2.TaskOutputResponse, error)
	TaskResult(ctx context.Context, taskID string, perf bool) (*swarmingv2.TaskResultResponse, error)
	FilesFromCAS(ctx context.Context, outdir string, cascli *rbeclient.Client, casRef *swarmingv2.CASReference) ([]string, error)
	CountBots(ctx context.Context, dimensions []*swarmingv2.StringPair) (*swarmingv2.BotsCount, error)
	ListBots(ctx context.Context, dimensions []*swarmingv2.StringPair) ([]*swarmingv2.BotInfo, error)
	DeleteBot(ctx context.Context, botID string) (*swarmingv2.DeleteResponse, error)
	TerminateBot(ctx context.Context, botID string, reason string) (*swarmingv2.TerminateResponse, error)
	ListBotTasks(ctx context.Context, botID string, limit int64, start float64, state string, fields []googleapi.Field) ([]*swarmingv1.SwarmingRpcsTaskResult, error)
}

type swarmingServiceImpl struct {
	client      *http.Client
	service     *swarmingv1.Service
	botsClient  swarmingv2.BotsClient
	tasksClient swarmingv2.TasksClient
}

func (s *swarmingServiceImpl) NewTask(ctx context.Context, req *swarmingv1.SwarmingRpcsNewTaskRequest) (res *swarmingv1.SwarmingRpcsTaskRequestMetadata, err error) {
	err = retryGoogleRPC(ctx, "NewTask", func() (ierr error) {
		res, ierr = s.service.Tasks.New(req).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) CountTasks(ctx context.Context, start float64, state string, tags ...string) (res *swarmingv1.SwarmingRpcsTasksCount, err error) {
	err = retryGoogleRPC(ctx, "CountTasks", func() (ierr error) {
		res, ierr = s.service.Tasks.Count().Context(ctx).Start(start).State(state).Tags(tags...).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) ListTasks(ctx context.Context, limit int64, start float64, state string, tags []string, fields []googleapi.Field) ([]*swarmingv1.SwarmingRpcsTaskResult, error) {
	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	tasks := []*swarmingv1.SwarmingRpcsTaskResult{}
	// If no fields are specified, all fields will be returned. If any fields are
	// specified, ensure the cursor is specified so we can get subsequent pages.
	if len(fields) > 0 {
		fields = append(fields, "cursor")
	}
	call := s.service.Tasks.List().Context(ctx).Limit(limit).Start(start).State(state).Tags(tags...).Fields(fields...)
	// Keep calling as long as there's a cursor indicating more tasks to list.
	for {
		var res *swarmingv1.SwarmingRpcsTaskList
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
	bots := make([]*swarmingv2.BotInfo, 0, Limit)
	for {
		resp, err := s.botsClient.ListBots(ctx, &swarmingv2.BotsRequest{
			Limit:      Limit,
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

func (s *swarmingServiceImpl) ListBotTasks(ctx context.Context, botID string, limit int64, start float64, state string, fields []googleapi.Field) (res []*swarmingv1.SwarmingRpcsTaskResult, err error) {
	// Create an empty array so that if serialized to JSON it's an empty list,
	// not null.
	tasks := []*swarmingv1.SwarmingRpcsTaskResult{}
	// If no fields are specified, all fields will be returned. If any fields are
	// specified, ensure the cursor is specified so we can get subsequent pages.
	if len(fields) > 0 {
		fields = append(fields, "cursor")
	}

	call := s.service.Bot.Tasks(botID).Context(ctx).Limit(limit).Start(start).Fields(fields...)
	if state != "" {
		call = call.State(state)
	}
	// Keep calling as long as there's a cursor indicating more tasks to list.
	for {
		var res *swarmingv1.SwarmingRpcsBotTasks
		err := retryGoogleRPC(ctx, "ListBotTasks", func() (ierr error) {
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

const DefaultIndent = " "

func DefaultProtoMarshalOpts() protojson.MarshalOptions {
	return protojson.MarshalOptions{
		UseProtoNames: true,
		Multiline:     true,
		Indent:        DefaultIndent,
	}
}

func Alive(t swarmingv2.TaskState) bool {
	return t == swarmingv2.TaskState_PENDING || t == swarmingv2.TaskState_RUNNING
}

func Completed(t swarmingv2.TaskState) bool {
	return t == swarmingv2.TaskState_COMPLETED
}

// AuthFlags is an interface to register auth flags and create http.Client and CAS Client.
type AuthFlags interface {
	// Register registers auth flags to the given flag set. e.g. -service-account-json.
	Register(f *flag.FlagSet)

	// Parse parses auth flags.
	Parse() error

	// NewHTTPClient creates an authroised http.Client.
	NewHTTPClient(ctx context.Context) (*http.Client, error)

	// NewRBEClient creates an authroised RBE Client.
	NewRBEClient(ctx context.Context, addr string, instance string) (*rbeclient.Client, error)
}

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags common.Flags
	authFlags    AuthFlags
	serverURL    string
}

// Init initializes common flags.
func (c *commonFlags) Init(authFlags AuthFlags) {
	c.defaultFlags.Init(&c.Flags)
	c.authFlags = authFlags
	c.authFlags.Register(&c.Flags)
	c.Flags.StringVar(&c.serverURL, "server", os.Getenv(ServerEnvVar), fmt.Sprintf("Server URL; required. Set $%s to set a default.", ServerEnvVar))
	c.Flags.StringVar(&c.serverURL, "S", os.Getenv(ServerEnvVar), "Alias for -server.")
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
	s, err := swarmingv1.NewService(ctx, option.WithHTTPClient(&rpcClient))
	if err != nil {
		return nil, err
	}
	s.BasePath = c.serverURL + swarmingAPISuffix
	s.UserAgent = SwarmingUserAgent

	prpcOpts := prpc.DefaultOptions()
	prpcOpts.UserAgent = SwarmingUserAgent
	prpcOpts.PerRPCTimeout = swarmingRPCRequestTimeout
	prpcClient := prpc.Client{
		C:       authcli,
		Options: prpcOpts,
	}
	switch {
	case strings.HasPrefix(c.serverURL, "https://"):
		prpcClient.Host = strings.TrimPrefix(c.serverURL, "https://")
	case strings.HasPrefix(c.serverURL, "http://"):
		prpcClient.Host = strings.TrimPrefix(c.serverURL, "http://")
		prpcClient.Options.Insecure = true
		if !lhttp.IsLocalHost(prpcClient.Host) {
			return nil, errors.Reason("http url for -server may only be used with localhost").Err()
		}
	default:
		prpcClient.Host = c.serverURL
	}
	return &swarmingServiceImpl{
		client:      authcli,
		service:     s,
		botsClient:  swarmingv2.NewBotsClient(&prpcClient),
		tasksClient: swarmingv2.NewTasksClient(&prpcClient),
	}, nil
}

func tagTransientGoogleAPIError(err error) error {
	// Responses with HTTP codes < 500, if we got them, indicate fatal errors.
	if gerr, _ := err.(*googleapi.Error); gerr != nil && gerr.Code < 500 {
		return err
	}

	// HTTP error already has transient.Tag if it is retryable.
	if _, ok := lhttp.IsHTTPError(err); ok {
		return err
	}

	// Everything else (timeouts, DNS issues, etc) is considered
	// a transient error.
	return transient.Tag.Apply(err)
}

func printError(a subcommands.Application, err error) {
	fmt.Fprintf(a.GetErr(), "%s: %s\n%s\n", a.GetName(), err, strings.Join(errors.RenderStack(err), "\n"))
}

// retryGoogleRPC retries an RPC on transient errors, such as HTTP 500.
func retryGoogleRPC(ctx context.Context, rpcName string, rpc func() error) error {
	return retry.Retry(ctx, transient.Only(retry.Default), func() error {
		err := rpc()
		if gerr, ok := err.(*googleapi.Error); ok && gerr.Code >= 500 {
			return transient.Tag.Apply(err)
		}

		if errors.Contains(err, context.DeadlineExceeded) {
			return transient.Tag.Apply(err)
		}

		var temporary bool
		errors.Walk(err, func(err error) bool {
			if terr, ok := err.(interface{ Temporary() bool }); ok && terr.Temporary() {
				temporary = true
				return false
			}
			return true
		})

		if temporary {
			return transient.Tag.Apply(err)
		}

		if err != nil {
			return errors.Annotate(err, "failed to call %s", rpcName).Err()
		}
		return nil
	}, retry.LogCallback(ctx, rpcName))
}
