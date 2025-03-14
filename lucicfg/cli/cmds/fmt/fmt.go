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

// Package fmt implements 'fmt' subcommand.
package fmt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/bazelbuild/buildtools/build"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/cli/base"
)

// Cmd is 'fmt' subcommand.
func Cmd(params base.Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "fmt [options] [files...]",
		ShortDesc: "applies standard formatting to *.star files",
		LongDesc: `Applies standard formatting to the given Starlark files.

Accepts zero or more paths via positional arguments, where each path is
either a file or a directory. Directories will be searched for *.star files
recursively. If no positional arguments are given, processes *.star files
recursively starting from the current directory.

By default reformats and rewrites improperly formatted files. Pass -dry-run flag
to just check formatting without overwriting files.
`,
		CommandRun: func() subcommands.CommandRun {
			fr := &fmtRun{}
			fr.Init(params)
			fr.Flags.BoolVar(&fr.dryRun, "dry-run", false,
				"If set, just check the formatting without rewriting files and "+
					"return non-zero exit code if some files need to be formatted")
			return fr
		},
	}
}

type fmtRun struct {
	base.Subcommand

	dryRun bool
}

type fmtResult struct {
	// Good is a list of already formatted files.
	Good []string `json:"good,omitempty"`
	// Unformatted is a list of files that still need formatting.
	Unformatted []string `json:"unformatted,omitempty"`
	// Formatted is a list of files formatted during this run.
	Formatted []string `json:"formatted,omitempty"`
	// Failed is a list of files that failed to be formatted.
	Failed []string `json:"failed,omitempty"`
}

func (fr *fmtRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !fr.CheckArgs(args, 0, -1) {
		return 1
	}
	ctx := cli.GetContext(a, fr, env)
	return fr.Done(fr.run(ctx, args))
}

func (fr *fmtRun) run(ctx context.Context, inputs []string) (res *fmtResult, err error) {
	files, err := base.ExpandDirectories(inputs)
	if err != nil {
		return nil, err
	}

	l := sync.Mutex{}
	outcomes := make(map[string]string, len(files))

	const (
		outcomeGood        = "good"
		outcomeUnformatted = "needs formatting"
		outcomeFormatted   = "formatted"
		outcomeFailed      = "failure"
	)

	outcome := func(path, val string, err error) {
		l.Lock()
		outcomes[path] = val
		switch {
		case val == outcomeFailed:
			fmt.Fprintf(os.Stderr, "%s: %s\n", path, err)
		case val != outcomeGood:
			fmt.Fprintf(os.Stderr, "%s: %s\n", path, val)
		}
		l.Unlock()
	}

	formatter, err := base.GuessFormatterPolicy(files)
	if err != nil {
		return nil, err
	}
	if formatter != nil {
		if err := formatter.CheckValid(ctx); err != nil {
			return nil, err
		}
	}

	// The visit method will track down all the wanted files within the directory.
	// TODO: Use package-relative paths and pass the correct root.
	errs := buildifier.Visit(ctx, base.PathLoader, files, func(body []byte, f *build.File) errors.MultiError {
		formatted, err := buildifier.Format(ctx, f, formatter)
		if err != nil {
			return errors.NewMultiError(err)
		}
		if bytes.Equal(body, formatted) {
			outcome(f.Path, outcomeGood, nil)
			return nil
		}
		if fr.dryRun {
			outcome(f.Path, outcomeUnformatted, nil)
			return nil
		}
		if err := os.WriteFile(f.Path, formatted, 0666); err != nil {
			outcome(f.Path, outcomeFailed, err)
			return errors.NewMultiError(err)
		}
		outcome(f.Path, outcomeFormatted, nil)
		return nil
	})

	// Preserve the order of files in the output.
	res = &fmtResult{}
	for _, p := range files {
		switch outcome := outcomes[p]; outcome {
		case outcomeGood:
			res.Good = append(res.Good, p)
		case outcomeUnformatted:
			res.Unformatted = append(res.Unformatted, p)
		case outcomeFormatted:
			res.Formatted = append(res.Formatted, p)
		case outcomeFailed, "":
			res.Failed = append(res.Failed, p)
			res.Unformatted = append(res.Unformatted, p) // still need to format it
			if outcome == "" {
				fmt.Fprintf(os.Stderr, "%s: skipped due to parsing error\n", p)
			}
		}
	}

	if len(res.Unformatted) > 0 { // only happens in dry run
		errs = append(errs, fmt.Errorf("Some files need formatting"))
	}
	if len(errs) != 0 {
		return res, errs
	}
	return res, nil
}
