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

package cli

import (
	"context"
	"fmt"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
)

// TODO(vadimsh): If the config set is not provided, try to guess it from the
// git repo and location of files within the repo (by comparing them to output
// of GetConfigSets() listing). Present the guess to the end user, so they can
// confirm it a put it into the flag/config.

func cmdValidate(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "validate",
		ShortDesc: "sends *.cfg files to LUCI Config service for validation",
		CommandRun: func() subcommands.CommandRun {
			c := &validateRun{}
			c.init(params, true)
			c.Flags.StringVar(&c.configSet, "config-set", "<name>", "Name of the config set to validate against.")
			return c
		},
	}
}

type validateRun struct {
	subcommand

	configSet string
}

type validateResult struct {
	// Note: this is just an example.
	Files []string `json:"files"`
}

func (c *validateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(c.run(ctx))
}

func (c *validateRun) run(ctx context.Context) (*validateResult, error) {
	svc, err := c.configService(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: implement. For now it just lists files in an existing config set
	// as an example of how to make RPCs.

	resp, err := svc.GetConfigSets().
		ConfigSet(c.configSet).
		IncludeFiles(true).
		Context(ctx).Do()
	if err != nil {
		return nil, err
	}

	result := validateResult{}
	for _, cs := range resp.ConfigSets {
		for _, f := range cs.Files {
			fmt.Println(f.Path)
			result.Files = append(result.Files, f.Path)
		}
	}

	return &result, nil
}
