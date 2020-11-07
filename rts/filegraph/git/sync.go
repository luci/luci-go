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
)

// Sync synchronizes a graph with a git repository.
// This is the only way to mutate the Graph.
type Sync struct {
	// RepoDir is the path to the git repository.
	RepoDir string

	// Rev is the target revision. The graph is updated with commits reachable
	// from Rev, and not reachable from Graph.Commit.
	// Defaults to "HEAD".
	Rev string

	// Callback, if not nil, is called after each commit is processed.
	Callback func() error

	// MaxCommitSize is the maximum number of files touched by a commit.
	// Commits that exceed this limit are ignored.
	// The rationale is that large commits provide a weak signal of file
	// relatedness and are expensive to process, O(N^2).
	MaxCommitSize int
}

// Update updates the graph.
func (s *Sync) Update(ctx context.Context, g *Graph) error {
	g.ensureInitialized()

	rev := s.Rev
	if rev == "" {
		rev = "HEAD"
	}

	// TODO(crbug.com/1136280): handle multi-parent commits correctly.
	return readLog(ctx, s.RepoDir, g.Commit, rev, func(c commit) error {
		switch {
		case len(c.Files) == 1:
			// Skip this commit. It provides no signal about file relatedness.
			return nil
		case s.MaxCommitSize != 0 && len(c.Files) > s.MaxCommitSize:
			return nil
		}

		if err := s.apply(g, c.Files); err != nil {
			return err
		}
		g.Commit = c.Hash

		if s.Callback != nil {
			return s.Callback()
		}
		return nil
	})
}

// apply applies the file changes to the graph.
func (s *Sync) apply(g *Graph, fileChanges []fileChange) error {
	files := make([]*node, 0, len(fileChanges))
	for _, c := range fileChanges {
		name := "//" + c.Path

		switch c.Status {
		case 'D':
			// The file was deleted.
			if _, err := g.moveFile(name, ""); err != nil {
				return err
			}

		case 'R':
			// The file was renamed.
			newFile, err := g.moveFile(name, "//"+c.Path2)
			if err != nil {
				return err
			}
			files = append(files, newFile)

		// TODO(nodir): consider handling 'C' status more intelligently.

		default:
			files = append(files, g.ensureNode(name))
		}
	}

	for _, file := range files {
		file.commits++

		otherFiles := make(map[*node]struct{}, len(files)-1)
		for _, f := range files {
			if f != file {
				otherFiles[f] = struct{}{}
			}
		}

		// TODO(nodir): when updating weights, take the commit size into account.
		// Smaller commits provide stronger signal of file relatedness.

		// Increment the commit count in file's edges that point to otherFiles.
		for i := range file.edges {
			to := file.edges[i].to
			if _, ok := otherFiles[to]; ok {
				delete(otherFiles, to) // mark it as processed by removing from the set.
				file.edges[i].commonCommits++
			}
		}

		// Add the missing edges.
		if len(otherFiles) > 0 {
			file.prepareToAppendEdges()
			for to := range otherFiles {
				file.edges = append(file.edges, edge{to: to, commonCommits: 1})
			}
		}
	}

	return nil
}
