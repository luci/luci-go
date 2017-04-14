// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package application

import (
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/spec"
	"github.com/luci/luci-go/vpython/venv"
)

var subcommandVerify = &subcommands.Command{
	UsageLine: "verify",
	ShortDesc: "verifies that a spec and its wheels are valid",
	LongDesc: "verifies that a spec file is valid, and that all of the wheels listed resolve for " +
		"all of the configured verification architectures.",
	Advanced: false,
	CommandRun: func() subcommands.CommandRun {
		return &verifyCommandRun{}
	},
}

type verifyCommandRun struct {
	subcommands.CommandRunBase
}

func (cr *verifyCommandRun) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	c := cli.GetContext(app, cr, env)
	cfg := getConfig(c, args)

	return run(c, func(c context.Context) error {
		// Make sure that we can resolve the referenced specifiction.
		if err := cfg.opts.ResolveSpec(c); err != nil {
			return errors.Annotate(err).Reason("failed to resolve specification").Err()
		}
		if err := spec.Normalize(cfg.opts.EnvConfig.Spec, &cfg.opts.EnvConfig.Package); err != nil {
			return errors.Annotate(err).Reason("failed to normalize specification").Err()
		}
		s := cfg.opts.EnvConfig.Spec
		renderedSpec := spec.Render(s)
		logging.Infof(c, "Successfully verified specification:\n%s", renderedSpec)

		// Run our Verification generator and verify each generated environment.
		if cfg.Verification != nil {
			total := 0
			var failures []string
			cfg.Verification(c, func(c context.Context, title string, pl venv.PackageLoader, e *vpython.Environment) {
				total++

				// Augment this base environment with the current specification to
				// resolve. We clone becauase Resolve mutates the supplied Environment,
				// and we don't want to destroy our base data state.
				e = e.Clone()
				e.Spec = s.Clone()

				if err := pl.Resolve(c, e); err != nil {
					logging.Errorf(c, "Failed to resolve against %q: %s", title, err)
					failures = append(failures, title)
				}
			})

			if len(failures) > 0 {
				logging.Errorf(c, "Verification failed for %d scenario(s): %s\n%s",
					len(failures), strings.Join(failures, ", "), renderedSpec)
				return errors.New("verification failed")
			}

			logging.Infof(c, "Verified %d scenario(s).", total)
		}

		return nil
	})
}
