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

package main

import (
	"context"
	"flag"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/filegraph/git"
	"go.chromium.org/luci/rts/presubmit/eval"
)

func cmdEval() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `eval`,
		ShortDesc: "evaluate Chromium's RTS algorithm",
		LongDesc:  "Evaluate Chromium's RTS algorithm",
		CommandRun: func() subcommands.CommandRun {
			r := &evalRun{}
			if err := r.ev.RegisterFlags(&r.Flags); err != nil {
				panic(err) // should never happen
			}
			r.Flags.StringVar(&r.checkout, "checkout", "", "Path to chromium/src checkout")
			r.Flags.Float64Var(&r.fgQuery.MaxDistance, "fg-max-distance", 1, "Max distance to the changed files")
			r.Flags.IntVar(&r.loadOptions.MaxCommitSize, "max-commit-size", 100, text.Doc(`
				Maximum number of files touched by a commit.
				Commits that exceed this limit are ignored.
				The rationale is that large commits provide a weak signal of file
				relatedness and are expensive to process, O(N^2).
			`))

			// TODO(nodir): add -fg-sibling-relevance flag.
			return r
		},
	}
}

type evalRun struct {
	baseCommandRun
	ev          eval.Eval
	checkout    string
	loadOptions git.LoadOptions
	fgQuery     filegraph.Query

	fg *git.Graph
}

func (r *evalRun) validate() error {
	switch err := r.ev.ValidateFlags(); {
	case err != nil:
		return err

	case len(flag.Args()) > 0:
		return errors.New("unexpected positional arguments")

	case r.checkout == "":
		return errors.New("-checkout is required")

	default:
		return nil
	}
}

func (r *evalRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	return r.done(r.run(ctx))
}

func (r *evalRun) run(ctx context.Context) error {
	if err := r.validate(); err != nil {
		return err
	}

	var err error
	if r.fg, err = git.Load(ctx, r.checkout, r.loadOptions); err != nil {
		return errors.Annotate(err, "failed to load the file graph").Err()
	}

	r.ev.Algorithm = r.selectTests
	res, err := r.ev.Run(ctx)
	if err != nil {
		return err
	}
	res.Print(os.Stdout)
	return nil
}

func (r *evalRun) selectTests(ctx context.Context, in eval.Input, out *eval.Output) error {
	q := r.fgQuery // make a copy
	q.Reversed = true
	q.Sources = make([]filegraph.Node, len(in.ChangedFiles))
	for i, f := range in.ChangedFiles {
		switch {
		case f.Repo != "https://chromium-review.googlesource.com/chromium/src":
			return errors.Reason("unexpected repo %q", f.Repo).Err()
		case strings.HasPrefix(f.Path, "//testing/"):
			// This CL changes the way tests run or their configurations.
			// Run all tests.
			return nil
		case strings.HasPrefix(f.Path, "//base/"):
			// Base affects everything. Run all tests. TODO(nodir): revisit.
			return nil
		case f.Path == "//DEPS":
			// The full list of modified files is not available, and the
			// graph does not include DEPSed file changes anyway.
			return nil
		}

		if q.Sources[i] = r.fg.Node(f.Path); q.Sources[i] == nil {
			return nil
		}
	}

	// These are the test files that we should skip.
	shouldSkip := make(map[filegraph.Node]struct{}, len(in.TestVariants))
	tvNodes := make([]filegraph.Node, len(in.TestVariants))
	for i, tv := range in.TestVariants {
		// Android does not have locations.
		if tv.FileName == "" {
			return nil
		}
		n := r.fg.Node(tv.FileName)
		if n == nil {
			return nil
		}
		shouldSkip[n] = struct{}{}
		tvNodes[i] = n
	}

	// If a changed file is close to a test file, then skip it.
	q.Run(func(sp *filegraph.ShortestPath) (keepGoing bool) {
		delete(shouldSkip, sp.Node)
		return len(shouldSkip) > 0
	})

	for i, tv := range in.TestVariants {
		if _, ok := shouldSkip[tvNodes[i]]; ok {
			out.ShouldSkip = append(out.ShouldSkip, tv)
		}
	}

	return nil
}
