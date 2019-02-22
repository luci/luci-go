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

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/maruel/subcommands"

	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/client/authcli"
	"go.chromium.org/luci/client/downloader"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
)

// triggerResults is a set of results from using the trigger subcommand,
// describing all of the tasks that were triggered successfully.
type triggerResults struct {
	// Tasks is a list of successfully triggered tasks represented as
	// TriggerResult values.
	Tasks []*swarming.SwarmingRpcsTaskRequestMetadata `json:"tasks"`
}

var swarmingAPISuffix = "/_ah/api/swarming/v1/"

// swarmingService is an interface intended to stub out the swarming API
// bindings for testing.
type swarmingService interface {
	NewTask(ctx context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (*swarming.SwarmingRpcsTaskRequestMetadata, error)
	GetTaskResult(ctx context.Context, taskID string, perf bool) (*swarming.SwarmingRpcsTaskResult, error)
	GetTaskOutput(ctx context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error)
	GetTaskOutputs(ctx context.Context, taskID, outputDir string, ref *swarming.SwarmingRpcsFilesRef) ([]string, error)
}

type swarmingServiceImpl struct {
	*http.Client
	*swarming.Service

	worker int
}

func (s *swarmingServiceImpl) NewTask(ctx context.Context, req *swarming.SwarmingRpcsNewTaskRequest) (res *swarming.SwarmingRpcsTaskRequestMetadata, err error) {
	err = retryGoogleRPC(ctx, "NewTask", func() (ierr error) {
		res, ierr = s.Service.Tasks.New(req).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) GetTaskResult(ctx context.Context, taskID string, perf bool) (res *swarming.SwarmingRpcsTaskResult, err error) {
	err = retryGoogleRPC(ctx, "GetTaskResult", func() (ierr error) {
		res, ierr = s.Service.Task.Result(taskID).IncludePerformanceStats(perf).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) GetTaskOutput(ctx context.Context, taskID string) (res *swarming.SwarmingRpcsTaskOutput, err error) {
	err = retryGoogleRPC(ctx, "GetTaskOutput", func() (ierr error) {
		res, ierr = s.Service.Task.Stdout(taskID).Context(ctx).Do()
		return
	})
	return
}

func (s *swarmingServiceImpl) GetTaskOutputs(ctx context.Context, taskID, outputDir string, ref *swarming.SwarmingRpcsFilesRef) ([]string, error) {
	// Create a task-id-based subdirectory to house the outputs.
	dir := filepath.Join(filepath.Clean(outputDir), taskID)

	// This function can be retried when the RPC returned an HTTP 500. In this case,
	// the directory will already exist and may contain partial results. Take no chance
	// and restart from scratch.
	if err := os.RemoveAll(dir); err != nil {
		return nil, errors.Annotate(err, "failed to remove directory: %s", dir).Err()
	}

	if err := os.Mkdir(dir, os.ModePerm); err != nil {
		return nil, err
	}
	isolatedClient := isolatedclient.New(nil, s.Client, ref.Isolatedserver, ref.Namespace, nil, nil)

	var filesMu sync.Mutex
	var files []string
	ctx = common.CancelOnCtrlC(ctx)
	opts := &downloader.Options{
		MaxConcurrentJobs: s.worker,
		FileCallback: func(name string, _ *isolated.File) {
			filesMu.Lock()
			files = append(files, name)
			filesMu.Unlock()
		},
	}
	dl := downloader.New(ctx, isolatedClient, isolated.HexDigest(ref.Isolated), dir, opts)
	return files, dl.Wait()
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

type commonFlags struct {
	subcommands.CommandRunBase
	defaultFlags common.Flags
	authFlags    authcli.Flags
	serverURL    string

	parsedAuthOpts auth.Options
	worker         int
}

// Init initializes common flags.
func (c *commonFlags) Init(authOpts auth.Options) {
	c.defaultFlags.Init(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
	c.Flags.StringVar(&c.serverURL, "server", os.Getenv("SWARMING_SERVER"), "Server URL; required. Set $SWARMING_SERVER to set a default.")
	c.Flags.IntVar(&c.worker, "worker", 8, "Number of workers used to download isolated files.")
}

// Parse parses the common flags.
func (c *commonFlags) Parse() error {
	if err := c.defaultFlags.Parse(); err != nil {
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
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonFlags) createAuthClient(ctx context.Context) (*http.Client, error) {
	// Don't enforce authentication by using OptionalLogin mode. This is needed
	// for IP whitelisted bots: they have NO credentials to send.
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, c.parsedAuthOpts).Client()
}

func (c *commonFlags) createSwarmingClient(ctx context.Context) (swarmingService, error) {
	client, err := c.createAuthClient(ctx)
	if err != nil {
		return nil, err
	}
	s, err := swarming.New(client)
	if err != nil {
		return nil, err
	}
	s.BasePath = c.serverURL + swarmingAPISuffix
	return &swarmingServiceImpl{client, s, c.worker}, nil
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
			err = transient.Tag.Apply(err)
		}
		return err
	}, retry.LogCallback(ctx, rpcName))
}
