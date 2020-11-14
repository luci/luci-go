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
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/rts/filegraph/internal/gitutil"
)

// LoadOptions are options for Load() function.
type LoadOptions struct {
	UpdateOptions

	// Ref is the git ref to load the graph for.
	// Defaults to refs/heads/main.
	//
	// If it is refs/heads/main, but it does not exist, then falls back to
	// refs/heads/master.
	Ref string
}

// Load returns a file graph for a git repository.
// Caches the graph under the .git directory.
// May take minutes and log progress if the cache is cold.
//
// If the cache exists, but no longer matches the current ref commit, then
// applies new changes to the loaded graph and updates the cache.
func Load(ctx context.Context, repoDir string, opt LoadOptions) (*Graph, error) {
	switch {
	case opt.Ref == "":
		opt.Ref = "refs/heads/main"
	case !strings.HasPrefix(opt.Ref, "refs/"):
		return nil, errors.Reason(`opt.Ref must start with "refs/"`).Err()
	}

	// Open the cache file and try to read it.
	cache, err := openGraphCache(repoDir, opt)
	if err != nil {
		return nil, err
	}
	defer cache.Close()

	g, err := cache.tryReading(ctx)
	if err != nil {
		return nil, err
	}

	// Fallback from main to master if needed.
	tillRev := opt.Ref
	if tillRev == "refs/heads/main" {
		switch exists, err := gitutil.RefExists(repoDir, tillRev); {
		case err != nil:
			return nil, err
		case !exists:
			tillRev = "refs/heads/master"
		}
	}

	// Sync the graph with new commits.
	processed := 0
	dirty := false
	uopt := opt.UpdateOptions // make a copy
	uopt.Callback = func() error {
		dirty = true
		processed++
		if processed%1e5 == 0 {
			if err := cache.write(g); err != nil {
				return errors.Annotate(err, "failed to write the graph to %q", cache.Name()).Err()
			}
			dirty = false
			logging.Infof(ctx, "processed %d commits; currently at %s", processed, g.Commit)
		}

		// Call the original callback, if any.
		if opt.Callback != nil {
			return opt.Callback()
		}
		return nil
	}
	switch err := g.Update(ctx, repoDir, tillRev, uopt); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to update the graph").Err()
	case dirty:
		if err := cache.write(g); err != nil {
			return nil, errors.Annotate(err, "failed to write the graph to %q", cache.Name()).Err()
		}
	}
	return g, nil
}

type graphCache struct {
	*os.File
}

// openGraphCache returns a graphCache.
// The caller is responsible for closing it.
func openGraphCache(repoDir string, opt LoadOptions) (*graphCache, error) {
	gitDir, err := gitutil.Exec(repoDir)("rev-parse", "--absolute-git-dir")
	if err != nil {
		return nil, err
	}

	fileName := filepath.Join(
		gitDir,
		"filegraph",
		filepath.FromSlash(opt.Ref),
		fmt.Sprintf("fg.max-commit-size-%d.v0", opt.MaxCommitSize),
	)

	if err := os.MkdirAll(filepath.Dir(fileName), 0777); err != nil {
		return nil, err
	}

	f, err := os.OpenFile(fileName, os.O_RDWR|os.O_CREATE, 0777)
	if err != nil {
		return nil, err
	}

	return &graphCache{File: f}, nil
}

// tryReading tries to read the graph from the cache file.
// On cache miss returns an empty graph.
// On a non-fatal error, logs the error and clears g.Graph.
func (c *graphCache) tryReading(ctx context.Context) (*Graph, error) {
	r := bufio.NewReader(c)
	g := &Graph{}

	// Check for cache-miss.
	switch _, err := r.Peek(1); {
	case err == io.EOF:
		// The file is empty => cache miss.
		logging.Infof(ctx, "populating cache...")
		return g, nil
	case err != nil:
		return nil, err
	}

	// Read the cache.
	if err := g.Read(r); err != nil {
		logging.Warningf(ctx, "cache is corrupted: %s\npopulating cache...", err)
		// Reset the state.
		*g = Graph{}
	}

	return g, nil
}

// write writes the graph to the cache.
func (c *graphCache) write(g *Graph) error {
	// Write the graph to the beginning of the file.
	if _, err := c.Seek(0, 0); err != nil {
		return err
	}
	bufW := bufio.NewWriter(c)
	if err := g.Write(bufW); err != nil {
		return err
	}
	if err := bufW.Flush(); err != nil {
		return err
	}

	// Truncate to the current length.
	curLen, err := c.Seek(0, 1)
	if err != nil {
		return err
	}
	return c.Truncate(curLen)
}
