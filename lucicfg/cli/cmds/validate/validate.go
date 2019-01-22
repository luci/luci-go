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

// Package validate implements 'validate' subcommand.
package validate

import (
	"context"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/lucicfg"

	"go.chromium.org/luci/lucicfg/cli/base"
)

// TODO(vadimsh): If the config set is not provided, try to guess it from the
// git repo and location of files within the repo (by comparing them to output
// of GetConfigSets() listing). Present the guess to the end user, so they can
// confirm it a put it into the flag/config.

// Cmd is 'validate' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "validate -config-set CONFIG_SET [CONFIG_DIR]",
		ShortDesc: "sends files under CONFIG_DIR (or CWD if not set) to LUCI Config service for validation",
		CommandRun: func() subcommands.CommandRun {
			vr := &validateRun{}
			vr.Init(params, true)
			vr.Flags.StringVar(&vr.configSet, "config-set", "<name>", "Name of the config set to validate against.")
			vr.Flags.BoolVar(&vr.failOnWarnings, "fail-on-warnings", false, "Treat validation warnings as errors.")
			return vr
		},
	}
}

type validateRun struct {
	base.Subcommand

	configSet      string
	configDir      string
	failOnWarnings bool
}

func (vr *validateRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !vr.CheckArgs(args, 0, 1) {
		return 1
	}

	if len(args) == 1 {
		vr.configDir = args[0]
	} else {
		configDir, err := os.Getwd()
		if err != nil {
			return vr.Done(nil, err)
		}
		vr.configDir = configDir
	}

	return vr.Done(vr.run(cli.GetContext(a, vr, env)))
}

func (vr *validateRun) run(ctx context.Context) (*lucicfg.ValidationResult, error) {
	svc, err := vr.ConfigService(ctx)
	if err != nil {
		return nil, err
	}
	configSet, err := lucicfg.ReadConfigSet(vr.configDir)
	if err != nil {
		return nil, err
	}
	result, err := configSet.Validate(ctx, vr.configSet, lucicfg.RemoteValidator(svc))
	if err != nil {
		return nil, err
	}
	result.Log(ctx)
	return result, result.OverallError(vr.failOnWarnings)
}
