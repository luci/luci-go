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
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/rts/filegraph"
)

type gitGraph struct {
	ref   string
	graph filegraph.Graph
}

func (g *gitGraph) loadNodes(ctx context.Context, filePaths ...string) ([]filegraph.Node, error) {
	repoDir, err := ensureSameRepo(filePaths...)
	if err != nil {
		return nil, err
	}

	if err := g.loadSyncedGraph(ctx, repoDir); err != nil {
		return nil, err
	}

	nodes := make([]filegraph.Node, len(filePaths))
	prefix := filepath.ToSlash(filepath.Clean(repoDir))
	for i, f := range filePaths {
		if f, err = filepath.Abs(f); err != nil {
			return nil, err
		}
		f = filepath.Clean(f)
		f = filepath.ToSlash(f)
		rel := strings.TrimPrefix(f, prefix)
		rel = strings.Trim(rel, "/")
		node := g.graph.Node(filegraph.ParseName(rel))
		if node == nil {
			return nil, errors.Reason("node %q not found", rel).Err()
		}
		nodes[i] = node
	}

	return nodes, nil
}

func (g *gitGraph) loadSyncedGraph(ctx context.Context, repoRoot string) error {
	// TODO(1136280): implement.
	panic("not implemented")
}

// ensureSameRepo ensures that all files belong to the same git repository
// and returns an absolute path to the .git directory.
func ensureSameRepo(files ...string) (repoDir string, err error) {
	if len(files) == 0 {
		return "", errors.New("no files")
	}
	for _, f := range files {
		switch rd, err := execGit(f, "rev-parse", "--show-toplevel"); {
		case err != nil:
			return "", err
		case repoDir == "":
			repoDir = rd
		case repoDir != rd:
			return "", errors.Reason("%q and %q reside in different git repositories", files[0], f).Err()
		}
	}
	return repoDir, nil
}

// execGit executes a git command in the git repo that fileName belongs to,
// and returns standard output.
func execGit(fileName string, args ...string) (out string, err error) {
	dir, err := dirFromPath(fileName)
	if err != nil {
		return "", err
	}

	args = append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", args...)
	outBytes, err := cmd.Output()
	out = strings.TrimSuffix(string(outBytes), "\n")
	return out, errors.Annotate(err, "git %q failed", args).Err()
}

// dirFromPath returns fileName as is if it points to a dir, otherwise returns
// fileName's dir.
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
