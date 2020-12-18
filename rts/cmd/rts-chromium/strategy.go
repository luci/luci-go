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
	"errors"
	"strings"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/filegraph/git"
)

type fileDistance struct {
	*filegraph.ShortestPath
	Rank int
}

var errStop = errors.New("stop")

// queryFileGraph walks the file graph from the changed files and calls
// callback for each found file.
// If the changedFiles contains unsupported files, returns nil without calling
// back.
//
// If callback returns errStop, queryFileGraph returns nil immediately.
func queryFileGraph(fg *git.Graph, changedFiles []string, callback func(fileDistance) error) (err error) {
	q := &filegraph.Query{
		Sources: make([]filegraph.Node, len(changedFiles)),
		EdgeReader: &git.EdgeReader{
			// We run the query from changed files, but we need distance
			// from test files to changed files, and not the other way around.
			Reversed: true,
		},
	}

	for i, f := range changedFiles {
		switch {
		case strings.HasPrefix(f, "//testing/"):
			// This CL changes the way tests run or their configurations.
			// Run all tests.
			return nil
		case f == "//DEPS":
			// The full list of modified files is not available, and the
			// graph does not include DEPSed file changes anyway.
			return nil
		}

		if q.Sources[i] = fg.Node(f); q.Sources[i] == nil {
			return nil
		}
	}

	rank := 0
	q.Run(func(sp *filegraph.ShortestPath) (keepGoing bool) {
		// Note: the files are enumerated in the order of distance.
		rank++

		err = callback(fileDistance{
			ShortestPath: sp,
			Rank:         rank,
		})
		return err == nil
	})
	if err == errStop {
		err = nil
	}
	return
}
