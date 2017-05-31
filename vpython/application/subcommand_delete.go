// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package application

import (
	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/vpython/venv"

	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
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
			return errors.Reason("failed to delete %(count)d environment(s)").
				D("count", failures).
				Err()
		}
		return nil
	})
}
