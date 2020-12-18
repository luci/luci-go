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
	"math"
	"os"

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
		ShortDesc: "evaluate Chromium's test selection strategy",
		LongDesc:  "Evaluate Chromium's test selection strategy",
		CommandRun: func() subcommands.CommandRun {
			r := &evalRun{}
			if err := r.ev.RegisterFlags(&r.Flags); err != nil {
				panic(err) // should never happen
			}
			r.Flags.StringVar(&r.checkout, "checkout", "", "Path to a src.git checkout")
			r.Flags.IntVar(&r.loadOptions.MaxCommitSize, "fg-max-commit-size", 100, text.Doc(`
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

	r.ev.Strategy = r.selectTests
	res, err := r.ev.Run(ctx)
	if err != nil {
		return err
	}
	res.Print(os.Stdout)
	return nil
}

func (r *evalRun) selectTests(ctx context.Context, in eval.Input, out *eval.Output) error {
	// Run Dijkstra from the modified files and try to find all test files.

	changedFiles := make([]string, len(in.ChangedFiles))
	for i, f := range in.ChangedFiles {
		if f.Repo != "https://chromium-review.googlesource.com/chromium/src" {
			return errors.Reason("unexpected repo %q", f.Repo).Err()
		}
		changedFiles[i] = f.Path
	}

	affectedness := make(map[filegraph.Node]eval.Affectedness, len(in.TestVariants))
	testNodes := make([]filegraph.Node, len(in.TestVariants))
	for i, tv := range in.TestVariants {
		// Android does not have locations.
		if tv.FileName == "" {
			return nil
		}
		n := r.fg.Node(tv.FileName)
		if n == nil {
			return nil
		}
		// Unaffected and unranked by default.
		affectedness[n] = eval.Affectedness{Distance: math.Inf(1)}
		testNodes[i] = n
	}

	found := 0
	err := queryFileGraph(r.fg, changedFiles, func(fd fileDistance) error {
		if _, ok := affectedness[fd.Node]; ok {
			affectedness[fd.Node] = eval.Affectedness{
				Distance: fd.Distance,
				Rank:     fd.Rank,
			}
			found++
			if found == len(affectedness) {
				// We have found everything.
				return errStop
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	for i, n := range testNodes {
		out.TestVariantAffectedness[i] = affectedness[n]
	}
	return nil
}
