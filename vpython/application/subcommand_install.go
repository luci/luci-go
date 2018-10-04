// Copyright 2017 The LUCI Authors.
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

package application

import (
	"context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/vpython/venv"
)

var subcommandInstall = &subcommands.Command{
	UsageLine: "install",
	ShortDesc: "installs the configured VirtualEnv",
	LongDesc:  "installs the configured VirtualEnv, but doesn't run any associated commands",
	Advanced:  false,
	CommandRun: func() subcommands.CommandRun {
		var cr installCommandRun

		fs := cr.GetFlags()
		fs.StringVar(&cr.name, "name", cr.name,
			"Use this VirtualEnv name, instead of generating one via hash. This will force specification "+
				"validation, causing any existing VirtualEnv with this name to be deleted and reprovisioned "+
				"if it doesn't match.")

		return &cr
	},
}

type installCommandRun struct {
	subcommands.CommandRunBase

	name string
}

func (cr *installCommandRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	c := cli.GetContext(app, cr, env)
	a := getApplication(c, args)
	a.opts.EnvConfig.PruneThreshold = 0 // Don't prune on install.
	a.opts.EnvConfig.OverrideName = cr.name
	a.opts.EnvConfig.FailIfLocked = true

	return run(c, func(c context.Context) error {
		err := venv.With(c, a.opts.EnvConfig, func(context.Context, *venv.Env) error {
			return nil
		})
		if err != nil {
			return errors.Annotate(err, "failed to setup the environment").Err()
		}

		logging.Infof(c, "Successfully setup the environment.")
		return nil
	})
}
