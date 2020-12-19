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

package git

import (
	"context"
	"math"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/presubmit/eval"
)

// TestSelectionModel implements a selection strategy based on a git graph.
type TestSelectionModel struct {
	Graph *Graph

	// TestFiles are all existing test files.
	// A test file must be in this set in order to be skipped.
	TestFiles stringset.Set

	// MaxDistance is the maximum distance to select a test.
	MaxDistance float64
	// MaxRank is the maximum rank to select a test.
	MaxRank int
}

// Select calls skipTestFile for each test file that should be skipped.
//
// A file is skipped iff it is in the s.TestFiles, has distance greater than
// s.MaxDistance and has rank greater than s.MaxRank.
func (s *TestSelectionModel) Select(changedFiles []string, skipTestFile func(testFile string) error) (err error) {
	runRTSQuery(s.Graph, changedFiles, func(sp *filegraph.ShortestPath, rank int) bool {
		if rank <= s.MaxRank || sp.Distance <= s.MaxDistance {
			// This file too close to skip it.
			return true
		}

		fileName := sp.Node.Name()
		if !s.TestFiles.Has(fileName) {
			// This file is not recognized as a test file.
			return true
		}

		err = skipTestFile(fileName)
		return err == nil
	})
	return
}

// EvalStrategy implements eval.Strategy. It can be used to evaluate data
// quality of the graph.
//
// This function has minimal input validation. It is not designed to be called
// by the evaluation framework directly. Instead it should be wrapped with
// another strategy function that does the validates. In particular, this
// function does not check in.ChangedFiles[i].Repo and does not check for file
// patterns that must be exempted from RTS.
func (g *Graph) EvalStrategy(ctx context.Context, in eval.Input, out *eval.Output) error {
	changedFiles := make([]string, len(in.ChangedFiles))
	for i, f := range in.ChangedFiles {
		changedFiles[i] = f.Path
	}

	affectedness := make(map[filegraph.Node]eval.Affectedness, len(in.TestVariants))
	testNodes := make([]filegraph.Node, len(in.TestVariants))
	for i, tv := range in.TestVariants {
		if tv.FileName == "" {
			return nil
		}
		n := g.Node(tv.FileName)
		if n == nil {
			// TODO(nodir): consider not bailing completely.
			// It is probably just a new test file name that never made it to the
			// repo.
			return nil
		}
		// Too far away by default.
		affectedness[n] = eval.Affectedness{Distance: math.Inf(1), Rank: math.MaxInt32}
		testNodes[i] = n
	}

	found := 0
	runRTSQuery(g, changedFiles, func(sp *filegraph.ShortestPath, rank int) (keepGoing bool) {
		if _, ok := affectedness[sp.Node]; ok {
			affectedness[sp.Node] = eval.Affectedness{Distance: sp.Distance, Rank: rank}
			found++
			if found == len(affectedness) {
				// We have found everything.
				return false
			}
		}
		return true
	})

	for i, n := range testNodes {
		out.TestVariantAffectedness[i] = affectedness[n]
	}
	return nil
}

type rtsCallback func(sp *filegraph.ShortestPath, rank int) (keepGoing bool)

// runQuery walks the file graph from the changed files, along reversed edges
// and calls back for each found file.
func runRTSQuery(g *Graph, changedFiles []string, callback rtsCallback) {
	q := &filegraph.Query{
		Sources: make([]filegraph.Node, len(changedFiles)),
		EdgeReader: &EdgeReader{
			// We run the query from changed files, but we need distance
			// from test files to changed files, and not the other way around.
			Reversed: true,
		},
	}

	for i, f := range changedFiles {
		if q.Sources[i] = g.Node(f); q.Sources[i] == nil {
			// TODO(nodir): consider not bailing.
			// We are bailing on all CLs where at least one file is new.
			return
		}
	}

	rank := 0
	q.Run(func(sp *filegraph.ShortestPath) (keepGoing bool) {
		// Note: the files are enumerated in the order of distance.
		rank++
		return callback(sp, rank)
	})
}
