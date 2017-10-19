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
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/client/authcli"
	"go.chromium.org/luci/client/internal/common"
	"go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/common/logging/gologger"
)

var swarmingAPISuffix = "/api/swarming/v1/"

// swarmingService is an interface intended to stub out the swarming API
// bindings for testing.
type swarmingService interface {
	// TODO: Add support for trigger-related mocks.
	GetTaskResult(c context.Context, taskID string, perf bool) (*swarming.SwarmingRpcsTaskResult, error)
	GetTaskOutput(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error)
}

type swarmingServiceImpl struct {
	*swarming.Service
}

func (s *swarmingServiceImpl) GetTaskResult(c context.Context, taskID string, perf bool) (*swarming.SwarmingRpcsTaskResult, error) {
	return s.Service.Task.Result(taskID).IncludePerformanceStats(perf).Context(c).Do()
}

func (s *swarmingServiceImpl) GetTaskOutput(c context.Context, taskID string) (*swarming.SwarmingRpcsTaskOutput, error) {
	return s.Service.Task.Stdout(taskID).Context(c).Do()
}

type taskState int32

const (
	maskAlive                = 1
	stateBotDied   taskState = 1 << 1
	stateCancelled taskState = 1 << 2
	stateCompleted taskState = 1 << 3
	stateExpired   taskState = 1 << 4
	statePending   taskState = 1<<5 | maskAlive
	stateRunning   taskState = 1<<6 | maskAlive
	stateTimedOut  taskState = 1 << 7
	stateUnknown   taskState = -1
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
	default:
		return stateUnknown, fmt.Errorf("unrecognized state %q", state)
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
}

// Init initializes common flags.
func (c *commonFlags) Init(authOpts auth.Options) {
	c.defaultFlags.Init(&c.Flags)
	c.authFlags.Register(&c.Flags, authOpts)
	c.Flags.StringVar(&c.serverURL, "server", os.Getenv("SWARMING_SERVER"), "Server URL; required. Set $SWARMING_SERVER to set a default.")
}

// Parse parses the common flags.
func (c *commonFlags) Parse() error {
	if err := c.defaultFlags.Parse(); err != nil {
		return err
	}
	if c.serverURL == "" {
		return errors.New("must provide -server")
	}
	s, err := lhttp.CheckURL(c.serverURL)
	if err != nil {
		return err
	}
	c.serverURL = s
	c.parsedAuthOpts, err = c.authFlags.Options()
	return err
}

func (c *commonFlags) createAuthClient() (*http.Client, error) {
	// Don't enforce authentication by using OptionalLogin mode. This is needed
	// for IP whitelisted bots: they have NO credentials to send.
	ctx := gologger.StdConfig.Use(context.Background())
	return auth.NewAuthenticator(ctx, auth.OptionalLogin, c.parsedAuthOpts).Client()
}

func printError(a subcommands.Application, err error) {
	fmt.Fprintf(a.GetErr(), "%s: %s\n", a.GetName(), err)
}

type jsonDump struct {
	TaskID  string
	ViewURL string
	Request swarming.SwarmingRpcsNewTaskRequest
}
