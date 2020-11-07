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
	"fmt"

	"go.chromium.org/luci/common/errors"
)

// Sync synchronizes the graph with commits in g.Commit..tillRev.
//
// If the callback is not nil, it is called after each commit is processed.
//
// If the returned error is non-nil, then the graph might have been mutated.
func (g *Graph) Sync(ctx context.Context, repoDir, tillRev string, callback func() error) error {
	g.ensureInitialized()
	s := syncer{g: g}
	// TODO(crbug.com/1136280): handle multi-parent commits correctly.
	return readLog(ctx, repoDir, g.Commit, tillRev, func(c change) error {
		if err := s.apply(c.Files); err != nil {
			return err
		}
		g.Commit = c.Hash

		if callback != nil {
			return callback()
		}
		return nil
	})
}

// syncer updates the graph according to git commits.
type syncer struct {
	g *Graph
}

func (s *syncer) apply(fileChanges []fileChange) error {
	files := make([]*node, 0, len(fileChanges))
	for _, c := range fileChanges {
		name := "//" + c.Path

		switch c.Status {
		case 'D':
			// The file was deleted.
			if _, err := s.moveFile(name, ""); err != nil {
				return err
			}

		case 'R':
			// The file was renamed.
			file, err := s.moveFile(name, "//"+c.Path2)
			if err != nil {
				return err
			}
			files = append(files, file)

		default:
			files = append(files, s.g.ensureNode(name))
		}
	}

	// TODO(nodir): improve this significantly.
	// Commits with a large number of modified files do not provide a strong
	// signal about their relatedness. Examples: a large directory was moved,
	// or a symbol was renamed in many files.
	// The loop below is O(N^2), so large number of files takes a long time to
	// process. Skip such commits for now.
	// Ideally come up with a formula, so that the weight added by a commit
	// goes down as the number of modified files grows.
	if len(files) == 1 || len(files) > 100 {
		return nil
	}

	fileSet := make(map[*node]struct{}, len(files))
	for _, file := range files {
		file.commits++

		// Ensure all files are in fileSet. For simplicity, add `file` too.
		for _, to := range files {
			fileSet[to] = struct{}{}
		}

		// Increment the commit count in file's edges that point to any other.
		for i := range file.edges {
			to := file.edges[i].to
			if _, ok := fileSet[to]; ok {
				delete(fileSet, to) // mark it as processed by deleting it
				file.edges[i].commonCommits++
			}
		}

		// Add the missing edges.
		for to := range fileSet {
			if to != file {
				file.edges = append(file.edges, edge{to: to, commonCommits: 1})
			}
		}
	}

	return nil
}

// moveFile moves a file and returns it. If newName is empty, removes the file.
// Assumes the names are valid.
// If the returned error is non-nil, then the graph might have been mutated.
func (s *syncer) moveFile(oldName, newName string) (*node, error) {
	if oldName == "//" {
		return nil, errors.Reason("// is not a file").Err()
	}
	// Remove the node from the old parent, and potentially remove the now-empty
	// ancestors.
	file := s.g.remove(oldName)
	switch {
	case file == nil:
		return nil, errors.Reason("not found").Err()
	case len(file.children) > 0:
		return nil, errors.Reason("expected %q to be a file, but it is a dir", oldName).Err()
	}

	if newName == "" {
		// newName being empty means the file must be removed, as opposed to moved.
		// Remove all incoming edges.
		for _, e := range file.edges {
			if !e.to.removeEdge(file) {
				// This should never happen because an outgoing edge exists if and only
				// if the counterpart incoming edge exists.
				panic(fmt.Sprintf("edge %q -> %q exists, but its counterpart does not", file.name, e.to.name))
			}
		}
		return file, nil
	}

	// Attach the node to the new parent.
	// Ensure the new parent exists.
	newParentName, newBaseName := baseName(newName)
	newParent := s.g.ensureNode(newParentName)
	switch {
	case newParent.children == nil:
		newParent.children = map[string]*node{}
	case newParent.children[newBaseName] != nil:
		return nil, errors.Reason("%q already exists", newName).Err()
	}

	file.name = newName
	newParent.children[newBaseName] = file
	return file, nil
}
