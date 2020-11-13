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

	"go.chromium.org/luci/common/errors"
)

// Updater updates a graph based on changes in a git repository.
// This is the only way to mutate the Graph.
type Updater struct {
	// RepoDir is the path to the git repository.
	RepoDir string

	// Rev is the target revision. The graph is updated with commits reachable
	// from Rev, and not reachable from Graph.Commit.
	// Defaults to "HEAD".
	Rev string

	// Callback, if not nil, is called each time after each commit is processed
	// and Graph.Commit is updated.
	Callback func() error

	// MaxCommitSize is the maximum number of files touched by a commit.
	// Commits that exceed this limit are ignored.
	// The rationale is that large commits provide a weak signal of file
	// relatedness and are expensive to process, O(N^2).
	MaxCommitSize int
}

// Update updates the graph.
// If returns an error which wasn't returned by the callback, then it is
// possible that the graph is corrupted.
func (u *Updater) Update(ctx context.Context, g *Graph) error {
	g.ensureInitialized()

	rev := u.Rev
	if rev == "" {
		rev = "HEAD"
	}

	return readLog(ctx, u.RepoDir, g.Commit, rev, func(c commit) error {
		switch {
		case len(c.Files) == 1:
			// Skip this commit. It provides no signal about file relatedness.
			return nil
		case u.MaxCommitSize != 0 && len(c.Files) > u.MaxCommitSize:
			// Skip this commit - too large.
			return nil
		}

		if err := u.apply(g, c.Files); err != nil {
			return errors.Annotate(err, "failed to apply commit %s", c.Hash).Err()
		}

		// TODO(nodir): do not call the callback if we are in the middle of
		// processing a second parent, because it is not a safe stopping point,
		// because the graph already incorporated commits that are not reachable
		// by c.Hash. The graph must not be saved in this state.
		g.Commit = c.Hash
		if u.Callback != nil {
			return u.Callback()
		}
		return nil
	})
}

// apply applies the file changes to the graph.
func (u *Updater) apply(g *Graph, fileChanges []fileChange) error {
	files := make([]*node, 0, len(fileChanges))
	for _, fc := range fileChanges {
		switch {
		case fc.Status == 'R':
			// The file was renamed.
			oldFile := g.ensureNode("//" + fc.Path)
			newFile := g.ensureNode("//" + fc.Path2)
			oldFile.ensureAlias(newFile)
			newFile.ensureAlias(oldFile)
			files = append(files, newFile)

		case fc.Status == 'D':
			// Ignore this file.
			// If this file is re-added later, it is likely to be a revert, where we'd
			// record the relation.
			// And if the file is never coming back, then its relations do not matter.

		case fc.Path2 != "":
			return errors.Reason("unexpected non-empty path2 %q for file status %c", fc.Path2, fc.Status).Err()

		default:
			files = append(files, g.ensureNode("//"+fc.Path))
		}
	}

	// Create edges between each file pair.
	// This is O(FILES * (FILES + EDGES_PER_FILE))

	fileSet := make(map[*node]struct{}, len(files))
	for _, f := range files {
		fileSet[f] = struct{}{}
	}
	for _, file := range files {
		file.commits++

		// TODO(nodir): take the commit size into account of the distance.
		// Smaller commits provide stronger signal of file relatedness.
		// Specifically, consider incrementing commonCommits by 1/(len(files) - 1).

		updated := make(map[*node]struct{}, len(files)-1)
		// Increment the commit count in file's edges that point to other files.
		for i, e := range file.edges {
			if _, ok := fileSet[e.to]; ok {
				updated[e.to] = struct{}{}

				// Increment the common commit count unless it is an alias edge.
				if e.commonCommits != 0 {
					file.edges[i].commonCommits++
				}
			}
		}

		// Add the missing edges.
		for _, to := range files {
			if to != file {
				if _, ok := updated[to]; !ok {
					file.prepareToAppendEdges()
					file.edges = append(file.edges, edge{to: to, commonCommits: 1})
				}
			}
		}
	}

	return nil
}
