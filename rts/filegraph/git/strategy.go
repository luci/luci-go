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
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/presubmit/eval"
)

// SelectionStrategy implements a selection strategy based on a git graph.
type SelectionStrategy struct {
	Graph       *Graph
	TestFiles   stringset.Set
	MaxDistance float64
	MaxRank     int
}

// Select calls skipTestFile for each test file that should be skipped.
func (s *SelectionStrategy) Select(changedFiles []string, skipTestFile func(testFile string) error) error {
	return s.runQuery(changedFiles, func(sp *filegraph.ShortestPath, rank int) error {
		if rank <= s.MaxRank || sp.Distance <= s.MaxDistance {
			// This file too close to exclude it.
			return nil
		}

		fileName := sp.Node.Name()
		if !s.TestFiles.Has(fileName) {
			// This is not a test file.
			return nil
		}

		return skipTestFile(fileName)
	})
}

// Eval implements eval.SelectionStrategy.
//
// This function is not designed to be called directly by the RTS evaluation
// framework. Instead it should be wrapped with another strategy function
// that validates input. For example, this function does not check
// in.ChangedFiles[i].Repo and does not check for file patterns that must be
// exempted from RTS.
//
// Ignores TestFiles, MaxDistance and MaxRank.
func (s *SelectionStrategy) Eval(ctx context.Context, in eval.Input, out *eval.Output) error {
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
		n := s.Graph.Node(tv.FileName)
		if n == nil {
			// TODO(nodir): consider not bailing. We should have all files.
			return nil
		}
		// Too far away by default.
		affectedness[n] = eval.Affectedness{Distance: math.Inf(1), Rank: math.MaxInt32}
		testNodes[i] = n
	}

	found := 0
	err := s.runQuery(changedFiles, func(sp *filegraph.ShortestPath, rank int) error {
		if _, ok := affectedness[sp.Node]; ok {
			affectedness[sp.Node] = eval.Affectedness{Distance: sp.Distance, Rank: rank}
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

// errStop returned by a callback means the iteration must stop.
var errStop = errors.New("stop")

// runQuery walks the file graph from the changed files, along reversed edges
// and calls back for each found file.
// If callback returns errStop, runQuery returns nil immediately.
func (s *SelectionStrategy) runQuery(changedFiles []string, callback func(sp *filegraph.ShortestPath, rank int) error) (err error) {
	q := &filegraph.Query{
		Sources: make([]filegraph.Node, len(changedFiles)),
		EdgeReader: &EdgeReader{
			// We run the query from changed files, but we need distance
			// from test files to changed files, and not the other way around.
			Reversed: true,
		},
	}

	for i, f := range changedFiles {
		if q.Sources[i] = s.Graph.Node(f); q.Sources[i] == nil {
			// TODO(nodir): consider not bailing.
			// We are bailing on all CLs where at least one file is new.
			return nil
		}
	}

	rank := 0
	q.Run(func(sp *filegraph.ShortestPath) (keepGoing bool) {
		// Note: the files are enumerated in the order of distance.
		rank++
		err = callback(sp, rank)
		return err == nil
	})
	if err == errStop {
		err = nil
	}
	return
}
