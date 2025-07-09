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
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/bazelbuild/buildtools/build"
	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/cli/base"
	"go.chromium.org/luci/lucicfg/pkg"
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
			fr.Flags.BoolVar(&fr.stdio, "stdio", false,
				"If set, file content will be read from stdin and output to "+
					"stdout. Exactly one filename is required when using stdio. "+
					"It will be assumed that the stdin content belongs to this file. "+
					"The file does not need to exist")
			return fr
		},
	}
}

type fmtRun struct {
	base.Subcommand

	dryRun bool
	stdio  bool
}

// fmtResult is returned as JSON output.
//
// All paths are absolute.
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

func (fr *fmtRun) run(ctx context.Context, inputs []string) (*fmtResult, error) {
	if fr.stdio {
		if len(inputs) != 1 {
			return nil, fmt.Errorf("-stdin requires exactly one file containing the name of the file being formatted")
		}
		path := inputs[0]
		root, err := pkg.FindRootForFile(path, "")
		if err != nil {
			return nil, err
		}
		local, err := pkg.PackageOnDisk(ctx, root)
		if err == nil && local.Formatter != nil {
			err = local.Formatter.CheckValid(ctx)
		}
		if err != nil {
			return nil, err
		}

		unformatted, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, err
		}
		buildFile, err := build.ParseDefault(path, unformatted)
		if err != nil {
			return nil, err
		}
		formatted, err := buildifier.Format(ctx, buildFile, local.Formatter)
		if err != nil {
			return nil, err
		}

		if _, err := io.Copy(os.Stdout, bytes.NewReader(formatted)); err != nil {
			return nil, err
		}
		// No need to distinguish between formatted files and already formatted ones.
		// It's not relevant for this particular mode.
		return &fmtResult{Formatted: inputs}, nil
	}

	roots, err := pkg.ScanForRoots(inputs)
	if err != nil {
		return nil, err
	}

	outcomes := outcomesCollector{
		roots:   roots,
		merrs:   make([]errors.MultiError, len(roots)),
		perFile: map[string]outcomeAndErr{},
	}

	eg, _ := errgroup.WithContext(ctx)
	eg.SetLimit(5) // note that buildifier.Visit is itself parallelized
	for idx, root := range roots {
		eg.Go(func() error {
			local, err := pkg.PackageOnDisk(ctx, root.Root)
			if err == nil && local.Formatter != nil {
				err = local.Formatter.CheckValid(ctx)
			}
			if err != nil {
				outcomes.recordErr(idx, errors.MultiError{err})
				return nil
			}
			outcomes.recordErr(idx, buildifier.Visit(ctx, local.Code, root.RelFiles(), func(body []byte, f *build.File) errors.MultiError {
				abs := filepath.Join(local.DiskPath, filepath.FromSlash(f.Path))
				formatted, err := buildifier.Format(ctx, f, local.Formatter)
				if err != nil {
					outcomes.record(abs, outcomeFailed, err)
					return errors.NewMultiError(err)
				}
				if bytes.Equal(body, formatted) {
					outcomes.record(abs, outcomeGood, nil)
					return nil
				}
				if fr.dryRun {
					outcomes.record(abs, outcomeUnformatted, nil)
					return nil
				}
				if err := os.WriteFile(abs, formatted, 0666); err != nil {
					outcomes.record(abs, outcomeFailed, err)
					return errors.NewMultiError(err)
				}
				outcomes.record(abs, outcomeFormatted, nil)
				return nil
			}))
			return nil
		})
	}

	_ = eg.Wait()

	res := outcomes.collectAndLogResult()
	errs := outcomes.errors()
	if len(res.Unformatted) > 0 && len(errs) == 0 {
		// This happens in dry run mode. Need to fail if there are unformatted
		// files.
		errs = errors.MultiError{errors.New("Some files need formatting")}
	}
	if len(errs) != 0 {
		return res, errs
	}
	return res, nil
}

type outcome string

const (
	outcomeGood        outcome = "good"
	outcomeUnformatted outcome = "needs formatting"
	outcomeFormatted   outcome = "formatted"
	outcomeFailed      outcome = "failure"
)

type outcomesCollector struct {
	m       sync.Mutex
	roots   []*pkg.ScanResult
	merrs   []errors.MultiError
	perFile map[string]outcomeAndErr
}

type outcomeAndErr struct {
	outcome outcome
	err     error
}

func (o *outcomesCollector) record(abs string, outcome outcome, err error) {
	o.m.Lock()
	defer o.m.Unlock()
	o.perFile[abs] = outcomeAndErr{outcome, err}
}

func (o *outcomesCollector) recordErr(rootIdx int, err errors.MultiError) {
	o.merrs[rootIdx] = err
}

func (o *outcomesCollector) collectAndLogResult() *fmtResult {
	o.m.Lock()
	defer o.m.Unlock()
	// Preserve the order of files in the output.
	res := &fmtResult{}
	for _, root := range o.roots {
		for _, path := range root.Files {
			pair := o.perFile[path]
			switch {
			case pair.outcome == outcomeFailed:
				fmt.Fprintf(os.Stderr, "%s: %s\n", path, pair.err)
			case pair.outcome == "":
				// Was skipped due to an error. The error is logged already.
			case pair.outcome != outcomeGood:
				fmt.Fprintf(os.Stderr, "%s: %s\n", path, pair.outcome)
			}
			switch pair.outcome {
			case outcomeGood:
				res.Good = append(res.Good, path)
			case outcomeUnformatted:
				res.Unformatted = append(res.Unformatted, path)
			case outcomeFormatted:
				res.Formatted = append(res.Formatted, path)
			case outcomeFailed, "":
				res.Failed = append(res.Failed, path)
				res.Unformatted = append(res.Unformatted, path) // still need to format it
			}
		}
	}
	return res
}

func (o *outcomesCollector) errors() errors.MultiError {
	o.m.Lock()
	defer o.m.Unlock()
	var merr errors.MultiError
	for _, errs := range o.merrs {
		merr = append(merr, errs...)
	}
	return merr
}
