// Copyright 2018 The LUCI Authors.
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

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/gce/api/instances/v1"
)

// connectCmd is the command to connect to a Swarming server.
type connectCmd struct {
	cmdRunBase
	// dir is the path to use as the Swarming bot directory.
	dir string
	// server is the Swarming server URL to connect to.
	server string
	// provider is the Provider server URL to retrieve the Swarming server URL from.
	provider string
	// user is the name of the local user to start the Swarming bot process as.
	user string
}

// validateFlags validates parsed command line flags.
func (cmd *connectCmd) validateFlags(c context.Context) error {
	switch {
	case cmd.dir == "":
		return errors.New("-dir is required")
	// TODO(crbug/945063): Remove -server.
	case cmd.provider == "" && cmd.server == "":
		return errors.New("-provider or -server is required")
	case cmd.user == "":
		return errors.New("-user is required")
	}
	return nil
}

// Run runs the command to connect to a Swarming server.
func (cmd *connectCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	c := cli.GetContext(app, cmd, env)
	if err := cmd.validateFlags(c); err != nil {
		logging.Errorf(c, "%s", err.Error())
		return 1
	}
	if cmd.server == ":metadata" {
		meta := getMetadata(c)
		srv, err := meta.Get("instance/attributes/swarming-server")
		if err != nil {
			logging.Errorf(c, "%s", err.Error())
			return 1
		}
		cmd.server = srv
	}
	if cmd.provider != "" {
		meta := getMetadata(c)
		name, err := meta.InstanceName()
		if err != nil {
			logging.Errorf(c, "%s", err.Error())
			return 1
		}
		if cmd.provider == ":metadata" {
			cmd.provider, err = meta.Get("instance/attributes/provider")
			if err != nil {
				logging.Errorf(c, "%s", err.Error())
				return 1
			}
		}
		prov := newInstances(c, cmd.serviceAccount, cmd.provider)
		inst, err := prov.Get(c, &instances.GetRequest{
			Hostname: name,
		})
		if err != nil {
			logging.Errorf(c, "%s", err.Error())
			return 1
		}
		cmd.server = inst.Swarming
	}

	swr := getSwarming(c)
	swr.server = cmd.server
	if err := swr.Configure(c, cmd.dir, cmd.user); err != nil {
		logging.Errorf(c, "%s", err.Error())
		return 1
	}
	return 0
}

// newConnectCmd returns a new command to connect to a Swarming server.
func newConnectCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "connect -dir <path> -server <server> -user <name>",
		ShortDesc: "connects to a swarming server",
		LongDesc:  "Connects to a Swarming server.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &connectCmd{}
			cmd.Initialize()
			cmd.Flags.StringVar(&cmd.dir, "dir", "", "Path to use as the Swarming bot directory.")
			cmd.Flags.StringVar(&cmd.provider, "provider", "", "Provider server URL to retrieve Swarming server URL from.")
			cmd.Flags.StringVar(&cmd.server, "server", "", "Deprecated. Use -provider.")
			cmd.Flags.StringVar(&cmd.user, "user", "", "Name of the local user to start the Swarming bot process as.")
			return cmd
		},
	}
}
