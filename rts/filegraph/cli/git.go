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

package cli

import (
	"context"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/rts/filegraph"
)

// gitGraph loads a file graph from a git log.
type gitGraph struct {
	ref   string
	graph filegraph.Graph
}

func (g *gitGraph) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&g.ref, "ref", "refs/heads/main", text.Doc(`
		load the file graph for this git ref.
		For refs/heads/main, refs/heads/master is read if main doesn't exist.
	`))
}

func (g *gitGraph) Validate() error {
	if !strings.HasPrefix(g.ref, "refs/") {
		return errors.Reason("-ref %q doesn't start with refs/", g.ref).Err()
	}
	return nil
}

// loadSyncedNodes calls loadSyncedGraph for filePaths' repo, and then loads a
// node for each of the files.
func (g *gitGraph) loadSyncedNodes(ctx context.Context, filePaths ...string) ([]filegraph.Node, error) {
	repoDir, err := ensureSameRepo(filePaths...)
	if err != nil {
		return nil, err
	}

	// Load the graph.
	if err := g.loadSyncedGraph(ctx, repoDir); err != nil {
		return nil, err
	}

	// Load the nodes.
	nodes := make([]filegraph.Node, len(filePaths))
	for i, f := range filePaths {
		// Convert the filename to a node name.
		if f, err = filepath.Abs(f); err != nil {
			return nil, err
		}
		rel, err := filepath.Rel(repoDir, f)
		if err != nil {
			return nil, err
		}
		var name filegraph.Name
		if rel != "." {
			name = filegraph.ParseName(strings.Trim(filepath.ToSlash(rel), "/"))
		}

		// Load the node.
		node := g.graph.Node(name)
		if node == nil {
			return nil, errors.Reason("node %q not found", rel).Err()
		}
		nodes[i] = node
	}

	return nodes, nil
}

// loadSyncedGraph loads a file graph for g.ref in the the given repo, syncs to
// the latest commit in the ref, and caches the result on the file system.
func (g *gitGraph) loadSyncedGraph(ctx context.Context, repoDir string) error {
	// TODO(crbug.com/1136280): implement.
	panic("not implemented")
}

// ensureSameRepo ensures that all files belong to the same git repository
// and returns its absolute path.
func ensureSameRepo(files ...string) (repoDir string, err error) {
	if len(files) == 0 {
		return "", errors.New("no files")
	}
	for _, f := range files {
		switch fRepo, err := execGit(f, "rev-parse", "--show-toplevel"); {
		case err != nil:
			return "", errors.Annotate(err, "file %q", f).Err()
		case repoDir == "":
			repoDir = fRepo
		case repoDir != fRepo:
			return "", errors.Reason("%q and %q reside in different git repositories", files[0], f).Err()
		}
	}
	return repoDir, nil
}

// execGit executes a git command and returns its standard output.
// The context must be a path to an existing file or directory.
//
// It is suitable only for commands that exit quickly and have small
// output, e.g. rev-parse.
func execGit(context string, args ...string) (out string, err error) {
	dir, err := dirFromPath(context)
	if err != nil {
		return "", err
	}

	exe := "git"
	if runtime.GOOS == "windows" {
		exe = "git.exe"
	}

	args = append([]string{"-C", dir}, args...)
	cmd := exec.Command(exe, args...)
	outBytes, err := cmd.Output()
	out = strings.TrimSuffix(string(outBytes), "\n")
	return out, errors.Annotate(err, "git %q failed; output: %q", args, out).Err()
}

// dirFromPath returns fileName as is if it points to a dir, otherwise returns
// fileName's dir. The file/dir must exist.
func dirFromPath(fileName string) (dir string, err error) {
	switch stat, err := os.Stat(fileName); {
	case err != nil:
		return "", err
	case stat.IsDir():
		return fileName, nil
	default:
		return filepath.Dir(fileName), nil
	}
}
