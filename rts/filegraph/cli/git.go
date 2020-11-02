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
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"go.chromium.org/luci/common/data/text"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/rts/filegraph"
	"go.chromium.org/luci/rts/filegraph/git"
)

// gitGraph loads a file graph from a git log.
type gitGraph struct {
	ref string
	git.Graph
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
		name, err := filepath.Rel(repoDir, f)
		if err != nil {
			return nil, err
		}
		name = filepath.ToSlash(name)
		switch {
		case name == ".":
			name = "//" // the root
		case strings.HasPrefix(name, "/") || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "./"):
			return nil, errors.Reason("unexpected path %q", name).Err()
		default:
			name = "//" + name
		}

		// Load the node.
		node := g.Node(name)
		if node == nil {
			return nil, errors.Reason("node %q not found", name).Err()
		}
		nodes[i] = node
	}

	return nodes, nil
}

// loadSyncedGraph loads a file graph for g.ref in the the given repo, syncs to
// the latest commit in the ref, and caches the result on the file system.
func (g *gitGraph) loadSyncedGraph(ctx context.Context, repoDir string) error {
	gitDir, err := execGit(repoDir, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return err
	}

	f, err := os.OpenFile(
		filepath.Join(gitDir, "filegraph", filepath.FromSlash(g.ref), "fg.v0"),
		os.O_RDWR|os.O_CREATE,
		0777,
	)
	if err != nil {
		return err
	}
	defer f.Close()

	// Read the cache.
	switch err := g.Read(bufio.NewReader(f)); {
	case os.IsNotExist(err):
		logging.Infof(ctx, "populating cache; this may take minutes...")
	case err != nil:
		logging.Warningf(ctx, "cache is corrupted; populating cache; this may take minutes...")
	}

	// Fallback from main to master if needed.
	tillRev := g.ref
	if g.ref == "refs/heads/main" {
		if _, err := execGit(repoDir, "rev-parse", g.ref, "--"); err != nil {
			if !strings.Contains(err.Error(), "bad revision") {
				return err
			}
			tillRev = "refs/heads/main"
		}
	}

	write := func() error {
		if _, err := f.Seek(0, 0); err != nil {
			return err
		}
		return g.Write(f)
	}

	// Read the new commits.
	processed := 0
	dirty := false
	err = g.Sync(ctx, repoDir, tillRev, func() error {
		dirty = true

		processed++
		if processed%10000 == 0 {
			if err := write(); err != nil {
				logging.Errorf(ctx, "failed to save cache: %s", err)
			}
			fmt.Printf("processed %d commits\n", processed)
			dirty = false
		}
		return nil
	})
	switch {
	case err != nil:
		return err
	case dirty:
		return write()
	default:
		return nil
	}
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

	// On Windows, git produces slash-based paths.
	repoDir = filepath.FromSlash(repoDir)
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
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	outBytes, err := cmd.Output()
	out = strings.TrimSuffix(string(outBytes), "\n")
	return out, errors.Annotate(err, "git %q failed; output: %q", args, stderr.Bytes()).Err()
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
