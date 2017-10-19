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

const (
	maskAlive      TaskState = 1
	StateBotDied             = 1 << 1
	StateCancelled           = 1 << 2
	StateCompleted           = 1 << 3
	StateExpired             = 1 << 4
	StatePending             = 1<<5 | maskAlive
	StateRunning             = 1<<6 | maskAlive
	StateTimedOut            = 1 << 7
	StateUnknown             = 1 << 31
)

type TaskState int64

func ParseTaskState(state string) (TaskState, error) {
	switch state {
	case "BOT_DIED":
		return StateBotDied, nil
	case "CANCELED":
		return StateCancelled, nil
	case "COMPLETED":
		return StateCompleted, nil
	case "EXPIRED":
		return StateExpired, nil
	case "PENDING":
		return StatePending, nil
	case "RUNNING":
		return StateRunning, nil
	case "TIMED_OUT":
		return StateTimedOut, nil
	default:
		return StateUnknown, fmt.Errorf("unrecognized state %q", state)
	}
}

func (t TaskState) Alive() bool {
	return (t & maskAlive) == 1
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
	TaskID   string
	ViewURL  string
	Request  swarming.SwarmingRpcsNewTaskRequest
}
