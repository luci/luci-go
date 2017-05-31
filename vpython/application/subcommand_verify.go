// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package application

import (
	"strings"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/vpython/api/vpython"
	"github.com/luci/luci-go/vpython/spec"

	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
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
	a := getApplication(c, args)

	return run(c, func(c context.Context) error {
		// Make sure that we can resolve the referenced specifiction.
		if err := a.opts.ResolveSpec(c); err != nil {
			return errors.Annotate(err).Reason("failed to resolve specification").Err()
		}
		if a.opts.EnvConfig.Spec.Virtualenv == nil {
			a.opts.EnvConfig.Spec.Virtualenv = &a.opts.EnvConfig.Package
		}
		if err := spec.NormalizeSpec(a.opts.EnvConfig.Spec); err != nil {
			return errors.Annotate(err).Reason("failed to normalize specification").Err()
		}
		s := a.opts.EnvConfig.Spec
		renderedSpec := spec.Render(s)
		logging.Infof(c, "Successfully verified specification:\n%s", renderedSpec)

		// Run our Verification generator and verify each generated environment.
		if a.WithVerificationConfig != nil {
			err := a.WithVerificationConfig(c, func(cfg Config, verificationScenarios []*vpython.Pep425Tag) error {
				if len(s.VerifyPep425Tag) > 0 {
					verificationScenarios = s.VerifyPep425Tag
				}
				if len(verificationScenarios) == 0 {
					return nil
				}

				var failures []string
				for _, vs := range verificationScenarios {
					// Create a verification environment to pass to our package loader.
					e := vpython.Environment{
						Spec:      s.Clone(),
						Pep425Tag: []*vpython.Pep425Tag{vs},
					}

					if err := cfg.PackageLoader.Resolve(c, &e); err != nil {
						logging.Errorf(c, "Failed to resolve against %q: %s", vs.TagString(), err)
						failures = append(failures, vs.TagString())
					}
				}

				if len(failures) > 0 {
					logging.Errorf(c, "Verification failed for %d scenario(s): %s\n%s",
						len(failures), strings.Join(failures, ", "), renderedSpec)
					return errors.New("verification failed")
				}

				logging.Infof(c, "Verified %d scenario(s).", len(verificationScenarios))
				return nil
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}
