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

package gitilessource

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"

	"go.chromium.org/luci/lucicfg/pkg/source"
)

const (
	// The default entry TTL for cache entries.
	//
	// Using a cache entry will set it's usage time to 'now'; for an entry to be
	// considered stale, it's usage time will need to be older than the EntryTTL.
	DefaultEntryTTL = time.Minute * 30

	// The default stale entry ratio for cache entries.
	//
	// The Gitiles cache will, on a per-repo basis, retain this many MORE stale
	// entries than it finds fresh entries.
	//
	// So, if the cache has 10 fresh entries (all with usage time < TTL), and
	// a ratio of 1.2, it would retain the newest 12 stale entries in addition to
	// the 10 fresh entries.
	//
	// Retention of stale entries is useful to prevent flapping on e.g. bots which
	// are processing multiple different revisions in subsequent builds.
	//
	// Because the most common source of change is modification of version, not
	// modification of module usage within a given version, this seems like
	// a fairly natural self-tuning mechanism.
	DefaultStaleEntryRatio = 1.2
)

// cache maintains all state related to an on-disk cache for one or more
// gitiles repoCache's.
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
type cache struct {
	cacheRoot string
	authOpts  auth.Options
	fallback  source.Cache

	mu sync.RWMutex

	staleEntryRatio float64
	entryTTL        time.Duration
	repos           map[string]source.RepoCache
}

func (c *cache) SetEntryTTL(dur time.Duration) {
	if dur <= 0 {
		dur = DefaultEntryTTL
	}

	c.mu.Lock()
	c.entryTTL = dur
	c.mu.Unlock()
}

func (c *cache) SetStaleEntryRatio(ratio float64) {
	if ratio < 0 {
		ratio = DefaultStaleEntryRatio
	}

	c.mu.Lock()
	c.staleEntryRatio = ratio
	c.mu.Unlock()
}

// Returns a gitiles client which can pull a single commit from HEAD of `url`.
//
// `url` must be a valid googlesource.com gitiles URL.
func (c *cache) getWorkingGitilesClientFor(ctx context.Context, url string) (gitilespb.GitilesClient, string, error) {
	host, project, err := gitiles.ParseRepoURL(url)
	if err != nil {
		return nil, "", err
	}
	authn := auth.NewAuthenticator(ctx, auth.SilentLogin, c.authOpts)
	httpClient, err := authn.Client()
	if err != nil {
		logging.Infof(ctx, "Failed to make client w/ gerritcodereview scope: %s", err)
		logging.Warningf(ctx, "Trying to access Gitiles without auth! If you think this this repo should work with auth, try `lucicfg auth-login`.")
		httpClient = http.DefaultClient
	}
	client, err := gitiles.NewRESTClient(httpClient, host, httpClient != http.DefaultClient)
	if err != nil {
		return nil, "", err
	}

	_, err = client.Log(ctx, &gitilespb.LogRequest{
		Project:            project,
		Committish:         "HEAD",
		ExcludeAncestorsOf: "HEAD~",
	})
	if err != nil {
		return nil, "", errors.Fmt("unable to resolve HEAD: %w", err)
	}

	logging.Debugf(ctx, "Enabled `gitilessource` for host=%q project=%q", host, project)
	return client, project, nil
}

func (c *cache) ForRepo(ctx context.Context, url string) (source.RepoCache, error) {
	c.mu.RLock()
	cur := c.repos[url]
	c.mu.RUnlock()
	if cur != nil {
		return cur, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if cur := c.repos[url]; cur != nil {
		return cur, nil
	}

	if c.repos == nil {
		c.repos = map[string]source.RepoCache{}
	}

	gitilesClient, project, err := c.getWorkingGitilesClientFor(ctx, url)
	switch {
	case err == nil:
		// OK; we have a working client
	case status.Code(err) == codes.Internal:
		// Something is really wrong; just return the error.
		return nil, err
	default:
		if c.fallback == nil {
			return nil, err
		}
		// All other cases, attempt to fallback.
		logging.Warningf(ctx, "Failed to get gitiles client for %q: %s", url, err)
		logging.Warningf(ctx, "Falling back to: %v", c.fallback)
		ret, err := c.fallback.ForRepo(ctx, url)
		if err != nil {
			return nil, err
		}
		c.repos[url] = ret
		return ret, nil
	}

	id := source.HashString(url)

	ret, err := newRepoCache(gitilesClient, project, filepath.Join(c.cacheRoot, id))
	if err != nil {
		return nil, err
	}

	c.repos[url] = ret
	return ret, nil
}

func (c *cache) Shutdown(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, repo := range c.repos {
		if myRepoCache, ok := repo.(*repoCache); ok {
			myRepoCache.shutdown(ctx, c.entryTTL, c.staleEntryRatio)
		}
	}

	c.fallback.Shutdown(ctx)

	c.repos = nil
}

type GitilesCache interface {
	source.Cache

	// Sets the minimum TTL for cache entries.
	//
	// Pruning happens when the Cache is Shutdown, and using an entry in the cache
	// will 'touch' it to time.Now().
	//
	// See [DefaultEntryTTL].
	//
	// Values <= 0 will be set to DefaultEntryTTL.
	SetEntryTTL(time.Duration)

	// Set the stale entry ratio.
	//
	// See [DefaultStaleEntryRatio].
	//
	// Values < 0 will be set to DefaultStaleEntryRatio.
	SetStaleEntryRatio(float64)
}

// New initializes a new cache in the path `cacheRoot`.
//
// `cacheRoot` should be a possibly empty (possibly missing) directory.
// The Cache will write a new subdirectory for each ForRepo call with a unique
// url.
//
// `perRepoMaxEntries` indicates how many cache entries this will preserve for
// a given target repo. If <= 0, defaults to DefaultPerRepoMaxEntries.
//
// `fallback` is required because not all git sources can support gitiles
// prefetching.
func New(cacheRoot string, authOpts auth.Options, fallback source.Cache) (GitilesCache, error) {
	if !filepath.IsAbs(cacheRoot) {
		return nil, fmt.Errorf("cacheRoot is not absolute: %q", cacheRoot)
	}
	if cleaned := filepath.Clean(cacheRoot); cacheRoot != cleaned {
		return nil, fmt.Errorf("cacheRoot is not clean: %q (cleaned=%q)", cacheRoot, cleaned)
	}
	return &cache{
		cacheRoot:       cacheRoot,
		authOpts:        authOpts,
		fallback:        fallback,
		staleEntryRatio: DefaultStaleEntryRatio,
		entryTTL:        DefaultEntryTTL,
	}, os.MkdirAll(cacheRoot, 0755)
}
