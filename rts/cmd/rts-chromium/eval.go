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
			r.Flags.Float64Var(&r.q.MaxDistance, "fg-max-distance", 1, "Max distance from the changed files")
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
	ev       eval.Eval
	checkout string

	fg          *git.Graph
	loadOptions git.LoadOptions
	q           filegraph.Query
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

func (r *evalRun) selectTests(ctx context.Context, in eval.Input) (out eval.Output, err error) {
	// Try to prepare the query.
	// On any failure, run all tests.
	out.ShouldRunAny = true

	q := r.q // make a copy
	q.Reversed = true
	q.Sources = make([]filegraph.Node, len(in.ChangedFiles))
	for i, f := range in.ChangedFiles {
		switch {
		case f.Repo != "https://chromium-review.googlesource.com/chromium/src":
			err = errors.Reason("unexpected repo %q", f.Repo).Err()
			return
		case strings.HasPrefix(f.Path, "//testing/buildbot/") && strings.HasSuffix(f.Path, ".json"):
			return
		case strings.HasPrefix(f.Path, "//base/"):
			return
		case f.Path == "//DEPS":
			return
		}

		n := r.fg.Node(f.Path)
		if n == nil {
			return
		}
		q.Sources[i] = n
	}

	testFileNodes := make(map[filegraph.Node]struct{}, len(in.TestVariants))
	for _, tv := range in.TestVariants {
		switch {
		// Android does not have locations.
		case tv.FileName == "":
			return
		}
		n := r.fg.Node(tv.FileName)
		if n == nil {
			return
		}
		testFileNodes[n] = struct{}{}
	}

	// The initial checks passed - now try execute the query.
	// Change the default to "do not run any tests".
	out.ShouldRunAny = false
	q.Run(func(sp *filegraph.ShortestPath) (keepGoing bool) {
		if _, ok := testFileNodes[sp.Node]; ok {
			// One of the test files is close enough to one of the changed files.
			out.ShouldRunAny = true
			return false
		}

		return true
	})
	return
}
