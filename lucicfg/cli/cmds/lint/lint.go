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

	"go.chromium.org/luci/common/cli"
	luciflag "go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/cli/base"
)

// Cmd is 'lint' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "lint [options] [files...]",
		ShortDesc: "applies linter checks to *.star files",
		LongDesc:  `Applies linter checks to the given Starlark files.`,
		CommandRun: func() subcommands.CommandRun {
			lr := &lintRun{checks: []string{"default"}}
			lr.Init(params)
			lr.Flags.Var(luciflag.CommaList(&lr.checks), "checks", "Apply these lint checks.")
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
	files, err := base.ExpandDirectories(inputs)
	if err != nil {
		return nil, err
	}

	rewriterFactory, err := base.GuessRewriterFactoryFunc(files)
	if err != nil {
		return nil, err
	}

	findings, err := buildifier.Lint(ctx, base.PathLoader, files, lr.checks, rewriterFactory.GetRewriter)
	for _, f := range findings {
		if text := f.Format(); text != "" {
			fmt.Fprintf(os.Stderr, "%s", text)
		}
	}
	return &lintResult{LinterFindings: findings}, err
}
