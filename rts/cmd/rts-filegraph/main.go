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
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"
	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/presubmit/eval"
)

func main() {
	rand.Seed(time.Now().Unix())

	ctx := context.Background()
	var logCfg = gologger.LoggerConfig{
		Format: `%{message}`,
		Out:    os.Stderr,
	}
	ctx = logCfg.Use(ctx)

	// Install signal handler because we call git.
	ctx, cancel := context.WithCancel(ctx)
	defer signals.HandleInterrupt(cancel)

	if err := (&program{}).run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %s\n", err)
		os.Exit(1)
	}
}

type program struct {
	g           *filegraph.Graph
	ev          eval.Eval
	repoRoot    string
	maxDistance float64
}

func (p *program) parseFlags() error {
	if err := p.ev.RegisterFlags(flag.CommandLine); err != nil {
		return err
	}
	flag.StringVar(&p.repoRoot, "repo", "", "Path to chromium/src checkout")
	flag.Float64Var(&p.maxDistance, "max-distance", 1, "Max distance from the changed files")
	flag.Parse()

	switch err := p.ev.ValidateFlags(); {
	case err != nil:
		return err

	case len(flag.Args()) > 0:
		return errors.New("unexpected positional arguments")

	case p.repoRoot == "":
		return errors.New("-repo is required")

	default:
		return nil
	}
}

func (p *program) run(ctx context.Context) error {
	if err := p.parseFlags(); err != nil {
		return err
	}

	var err error
	if p.g, err = filegraph.LoadGraphFromRepo(ctx, p.repoRoot); err != nil {
		return errors.Annotate(err, "failed to load the file graph").Err()
	}

	p.ev.Algorithm = p.algo
	res, err := p.ev.Run(ctx)
	if err != nil {
		return err
	}
	res.Print(os.Stdout)
	return nil
}

func (p *program) algo(ctx context.Context, in eval.Input) (out eval.Output, err error) {
	// Run the test by default
	out.ShouldRunAny = true

	getNode := func(filePath string) *filegraph.Node {
		nodePath := filegraph.ParsePath(strings.TrimPrefix(filePath, "//"))
		return p.g.Node(nodePath)
	}

	testFileNodes := make(map[*filegraph.Node]struct{}, len(in.TestVariants))
	for _, tv := range in.TestVariants {
		// Android does not have locations.
		if strings.Contains(tv.Variant["builder"], "android") {
			return
		}
		if tv.FileName == "" {
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
		if f.Repo != "https://chromium-review.googlesource.com/chromium/src" {
			err = errors.Reason("unexpected repo %q", f.Repo).Err()
			return
		}

		if strings.HasPrefix(f.Path, "//base/") || f.Path == "//DEPS" {
			return
		}

		n := getNode(f.Path)
		if n == nil {
			return
		}
		changeFileNodes[i] = n
	}

	q := filegraph.Query{
		Roots:     changeFileNodes,
		Backwards: true,
	}

	// Execute the query.
	out.ShouldRunAny = false
	err = p.g.Query(q, func(r *filegraph.ResultItem) error {
		if filegraph.Distance(r.Relevance) > p.maxDistance {
			// We got too far from the changed files.
			return filegraph.ErrStop
		}

		if _, ok := testFileNodes[r.Node]; ok {
			// One of the test files is close enough to one of the changed files.
			out.ShouldRunAny = true
			return filegraph.ErrStop
		}

		return nil
	})
	return
}
