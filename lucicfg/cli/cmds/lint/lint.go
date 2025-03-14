// Copyright 2020 The LUCI Authors.
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

// Package lint implements 'lint' subcommand.
package lint

import (
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	luciflag "go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/cli/base"
	"go.chromium.org/luci/lucicfg/pkg"
)

// Cmd is 'lint' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "lint [options] [files...]",
		ShortDesc: "applies linter checks to *.star files",
		LongDesc:  `Applies linter checks to the given Starlark files.`,
		CommandRun: func() subcommands.CommandRun {
			lr := &lintRun{}
			lr.Init(params)
			lr.Flags.Var(luciflag.CommaList(&lr.checks), "checks", "Apply these lint checks instead of the ones defined in the PACKAGE.star.")
			return lr
		},
	}
}

type lintRun struct {
	base.Subcommand

	checks []string
}

type lintResult struct {
	// LinterFindings is all discovered linter warnings.
	LinterFindings []*buildifier.Finding `json:"linter_findings,omitempty"`
}

func (lr *lintRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !lr.CheckArgs(args, 0, -1) {
		return 1
	}
	ctx := cli.GetContext(a, lr, env)
	return lr.Done(lr.run(ctx, args))
}

func (lr *lintRun) run(ctx context.Context, inputs []string) (res *lintResult, err error) {
	roots, err := pkg.ScanForRoots(inputs)
	if err != nil {
		return nil, err
	}

	type findingsAndErr struct {
		findings []*buildifier.Finding
		err      error
	}
	out := make([]findingsAndErr, len(roots))

	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(5) // note that buildifier.Visit is itself parallelized
	for idx, root := range roots {
		eg.Go(func() error {
			local, err := pkg.PackageOnDisk(ctx, root.Root)
			if err != nil {
				out[idx] = findingsAndErr{nil, err}
				return nil
			}
			checks := lr.checks
			if len(lr.checks) == 0 {
				checks = local.Definition.LintChecks
			}
			findings, err := buildifier.Lint(ctx,
				local.Code,
				root.RelFiles(),
				checks,
				local.DiskPath,
				local.Formatter,
			)
			out[idx] = findingsAndErr{findings, err}
			return nil
		})
	}
	_ = eg.Wait()

	// Merge the result in the deterministic order.
	var findings []*buildifier.Finding
	var merr errors.MultiError
	for _, pair := range out {
		findings = append(findings, pair.findings...)
		var asMerr errors.MultiError
		if errors.As(pair.err, &asMerr) {
			merr = append(merr, asMerr...)
		} else if pair.err != nil {
			merr = append(merr, pair.err)
		}
	}

	for _, f := range findings {
		if text := f.Format(); text != "" {
			fmt.Fprintf(os.Stderr, "%s", text)
		}
	}
	return &lintResult{LinterFindings: findings}, merr.AsError()
}
