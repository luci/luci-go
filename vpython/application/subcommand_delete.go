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

	"go.chromium.org/luci/vpython/venv"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

var subcommandDelete = &subcommands.Command{
	UsageLine: "delete",
	ShortDesc: "deletes existing VirtualEnv",
	LongDesc:  "offers deletion options for existing vpython VirtualEnv installatioins",
	Advanced:  false,
	CommandRun: func() subcommands.CommandRun {
		var cr deleteCommandRun

		fs := cr.GetFlags()
		fs.BoolVar(&cr.all, "all", cr.all, "Delete all VirtualEnv environments, rather than the current ones.")

		return &cr
	},
}

type deleteCommandRun struct {
	subcommands.CommandRunBase

	all bool
}

func (cr *deleteCommandRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	c := cli.GetContext(app, cr, env)
	a := getApplication(c, args)

	return run(c, func(c context.Context) error {
		if !cr.all {
			if err := venv.Delete(c, a.opts.EnvConfig); err != nil {
				return err
			}
			logging.Infof(c, "Successfully deleted environment.")
			return nil
		}

		it := venv.Iterator{
			OnlyComplete: true,
		}
		failures := 0
		err := it.ForEach(c, &a.opts.EnvConfig, func(c context.Context, e *venv.Env) error {
			logging.Debugf(c, "Attempting to delete VirtualEnv [%s]...", e.Name)

			if err := e.Delete(c); err != nil {
				logging.WithError(err).Errorf(c, "Failed to delete environment: %s", e.Name)
				failures++
				return nil
			}

			logging.Infof(c, "Deleted VirtualEnv: %s", e.Name)
			return nil
		})
		if err != nil {
			return err
		}

		if failures > 0 {
			return errors.Reason("failed to delete %d environment(s)", failures).Err()
		}
		return nil
	})
}
