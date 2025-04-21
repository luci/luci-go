// Copyright 2025 The LUCI Authors.
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

package gitsource

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"go.chromium.org/luci/common/logging"
)

// Cache maintains all state related to an on-disk cache for one or more
// RepoCache's.
//
// This is safe to use from multiple goroutines or processes.
//
// Because of the way that git promisors work, we need to keep a separate bare
// repo for each remote - specifically, if you configure multiple promisors in
// the same repo, git will assume that ALL of them potentially have a view of
// the whole repo content, and will ask each of them, in the order they appear
// in gitconfig, for missing objects.
//
// We expect to have multiple, unrelated, remotes, so we need to keep it
// separated.
type Cache struct {
	cacheRoot string
	debugLogs bool

	repos   map[string]*RepoCache
	reposMu sync.RWMutex
}

// New initializes a new cache in the path `cacheRoot`.
//
// `cacheRoot` should be a possibly empty (possibly missing) directory. If the
// directory exists, the Cache will write a new subdirectory for each ForRepo
// call with a unique url.
//
// If `debugLogs` is true, the Cache and related objects will produce fairly
// prodigious debugging logs via luci logging.Debugf and directly to
// stderr for internal git commands. Otherwise, the Cache and related
// objects won't log at all.
func New(cacheRoot string, debugLogs bool) (*Cache, error) {
	if !filepath.IsAbs(cacheRoot) {
		return nil, fmt.Errorf("cacheRoot is not absolute: %q", cacheRoot)
	}
	if cleaned := filepath.Clean(cacheRoot); cacheRoot != cleaned {
		return nil, fmt.Errorf("cacheRoot is not clean: %q (cleaned=%q)", cacheRoot, cleaned)
	}
	return &Cache{
		cacheRoot: cacheRoot,
		debugLogs: debugLogs,
	}, os.MkdirAll(cacheRoot, 0777)
}

func (c *Cache) prepDebugContext(ctx context.Context) context.Context {
	if c.debugLogs {
		return ctx
	}
	return logging.SetLevel(ctx, logging.Info)
}

// ForRepo returns a repo-specific cache object which allows you to create new
// GitFetchers which implement [depsource.Fetcher].
//
// The state for the RepoCache will be a subdirectory of the Cache's cacheRoot,
// using Base64URLSafeRaw(SHA256(url)) as the subdirectory name. The
// responsibility of normalizing repo URLs falls on the caller (e.g. if
// `.../blah` and `.../blah.git` actually are the same repo, this function will
// return two separate caches for them).
//
// Calling ForRepo multiple times with the same `url` will return an identical
// *RepoCache.
func (c *Cache) ForRepo(ctx context.Context, url string) (*RepoCache, error) {
	ctx = c.prepDebugContext(ctx)

	if c.cacheRoot == "" {
		return nil, errors.New("gitsource.Cache must be constructed with gitsource.New")
	}

	c.reposMu.RLock()
	cur := c.repos[url]
	c.reposMu.RUnlock()
	if cur != nil {
		return cur, nil
	}

	c.reposMu.Lock()
	defer c.reposMu.Unlock()
	if cur := c.repos[url]; cur != nil {
		return cur, nil
	}

	h := sha256.New()
	io.WriteString(h, url)
	id := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	ret, err := newRepoCache(filepath.Join(c.cacheRoot, id), c.debugLogs)
	if err != nil {
		return nil, err
	}

	if err := ret.git(ctx, "init", "-b", "main", "--bare"); err != nil {
		return nil, fmt.Errorf("failed to init repoRoot %q: %w", ret.repoRoot, err)
	}

	err = ret.setConfigBlock(ctx, configBlock{
		section: "remote.origin",
		config: map[string]string{
			"url":                url,
			"fetch":              "+refs/*:refs/*",
			"promisor":           "true",
			"partialclonefilter": "blob:none",
		},
	})
	if err != nil {
		return nil, err
	}

	// We can't ensure that the commits we fetch are reachable from the refs
	// - it's easiest to just disable GC for now.
	if err := ret.git(ctx, "config", "gc.auto", "0"); err != nil {
		return nil, err
	}

	if c.repos == nil {
		c.repos = make(map[string]*RepoCache)
	}
	c.repos[url] = ret
	return ret, nil
}

// Shutdown terminates long-running processes which may be associated with this
// Cache.
func (c *Cache) Shutdown() {
	c.reposMu.RLock()
	caches := slices.Collect(maps.Values(c.repos))
	c.reposMu.RUnlock()
	for _, c := range caches {
		c.Shutdown()
	}
}
