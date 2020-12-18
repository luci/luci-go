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

// RTSPredict implements a selection strategy.
type RTSPredict struct {
	Graph       *Graph
	TestFiles   stringset.Set
	MaxDistance float64
	MaxRank     int
}

// SelectTests calls excludeFile for each test file that should be skipped.
func (r *RTSPredict) SelectTests(changedFiles []string, excludeFile func(testFile string) error) error {
	return runRTSQuery(r.Graph, changedFiles, func(fd fileDistance) error {
		if fd.Rank <= r.MaxRank || fd.Distance <= r.MaxDistance {
			// This file too close to exclude it.
			return nil
		}

		fileName := fd.node.name
		if !r.TestFiles.Has(fileName) {
			// This is not a test file.
			return nil
		}

		return excludeFile(fileName)
	})
}

// RTSEval implements the interface for selection strategy evaluation.
// It designed to be used with go.chromium.org/luci/rts/presubmit/eval
// package.
type RTSEval struct {
	Graph *Graph
}

// Strategy implements eval.Strategy.
//
// This function is not designed to be called directly by the RTS evaluation
// framework. Instead it should be wrapped with another strategy function
// that validates input. For example, this function does not check
// in.ChangedFiles[i].Repo and does not check for file patterns that must be
// exempted from RTS.
func (r *RTSEval) Strategy(ctx context.Context, in eval.Input, out *eval.Output) error {
	changedFiles := make([]string, len(in.ChangedFiles))
	for i, f := range in.ChangedFiles {
		changedFiles[i] = f.Path
	}

	affectedness := make(map[*node]eval.Affectedness, len(in.TestVariants))
	testNodes := make([]*node, len(in.TestVariants))
	for i, tv := range in.TestVariants {
		if tv.FileName == "" {
			return nil
		}
		n := r.Graph.node(tv.FileName)
		if n == nil {
			return errors.Reason("not not found for test file %q", tv.FileName).Err()
		}
		// Unaffected and unranked by default.
		affectedness[n] = eval.Affectedness{Distance: math.Inf(1)}
		testNodes[i] = n
	}

	found := 0
	err := runRTSQuery(r.Graph, changedFiles, func(fd fileDistance) error {
		if _, ok := affectedness[fd.node]; ok {
			affectedness[fd.node] = eval.Affectedness{
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

type fileDistance struct {
	*filegraph.ShortestPath
	node *node
	Rank int
}

var errStop = errors.New("stop")

// runRTSQuery walks the file graph from the changed files and calls
// callback for each found file.
// If the changedFiles contains unsupported files, returns nil without calling
// back.
//
// If callback returns errStop, runRTSQuery returns nil immediately.
func runRTSQuery(g *Graph, changedFiles []string, callback func(fileDistance) error) (err error) {
	// Run Dijkstra from the modified files and try to find all test.
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
			// We are bailing at each new file.
			return nil
		}
	}

	rank := 0
	q.Run(func(sp *filegraph.ShortestPath) (keepGoing bool) {
		// Note: the files are enumerated in the order of distance.
		rank++
		err = callback(fileDistance{
			ShortestPath: sp,
			node:         sp.Node.(*node),
			Rank:         rank,
		})
		return err == nil
	})
	if err == errStop {
		err = nil
	}
	return
}
