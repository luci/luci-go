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
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/filegraph"
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
			r.Flags.Float64Var(&r.fgMaxDistance, "fg-max-distance", 1, "Max distance from the changed files")
			r.Flags.Float64Var(&r.fgSilbingRelevance, "fg-sibling-relevance", 0.3, "Relevance between siblings")
			return r
		},
	}
}

type evalRun struct {
	baseCommandRun
	ev       eval.Eval
	checkout string

	fg                 *filegraph.Graph
	fgMaxDistance      float64
	fgSilbingRelevance float64
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
	if r.fg, err = filegraph.LoadGraphFromRepo(ctx, r.checkout); err != nil {
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
	// Run the test by default
	out.ShouldRunAny = true

	getNode := func(filePath string) *filegraph.Node {
		nodePath := filegraph.ParsePath(strings.TrimPrefix(filePath, "//"))
		return r.fg.Node(nodePath)
	}

	testFileNodes := make(map[*filegraph.Node]struct{}, len(in.TestVariants))
	for _, tv := range in.TestVariants {
		switch {
		// Android does not have locations.
		case tv.FileName == "":
			return
		}
		n := getNode(tv.FileName)
		if n == nil {
			return
		}
		testFileNodes[n] = struct{}{}
	}

	changeFileNodes := make([]*filegraph.Node, len(in.ChangedFiles))
	for i, f := range in.ChangedFiles {
		switch {
		case f.Repo != "https://chromium-review.googlesource.com/chromium/src":
			err = errors.Reason("unexpected repo %q", f.Repo).Err()
			return
		case strings.HasPrefix(f.Path, "//testing/") && strings.HasSuffix(f.Path, ".json"):
			return
		case strings.HasPrefix(f.Path, "//base/"):
			return
		case f.Path == "//DEPS":
			return
		}

		n := getNode(f.Path)
		if n == nil {
			return
		}
		changeFileNodes[i] = n
	}

	q := filegraph.Query{
		Roots:            changeFileNodes,
		Backwards:        true,
		MinRelevance:     filegraph.Relevance(r.fgMaxDistance),
		SiblingRelevance: r.fgSilbingRelevance,
	}

	// Execute the query.
	out.ShouldRunAny = false
	err = r.fg.Query(q, func(res *filegraph.ResultItem) error {
		if filegraph.Distance(res.Relevance) > r.fgMaxDistance {
			// We got too far from the changed files.
			return filegraph.ErrStop
		}

		if _, ok := testFileNodes[res.Node]; ok {
			// One of the test files is close enough to one of the changed files.
			out.ShouldRunAny = true
			return filegraph.ErrStop
		}

		return nil
	})
	return
}
