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

package pkg

import (
	"context"
	"path"
	"strings"
	"sync"

	"golang.org/x/sync/semaphore"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/fileset"
	"go.chromium.org/luci/lucicfg/pkg/source"
	"go.chromium.org/luci/lucicfg/pkg/source/gitilessource"
	"go.chromium.org/luci/lucicfg/pkg/source/gitsource"
)

// RemoteRepoManager implements RepoManager that knows how to work with remote
// git repositories using a local disk cache.
type RemoteRepoManager struct {
	DiskCache           DiskCache                // usually implemented by lucicfg.Cache
	DiskCacheDirGit     string                   // a root directory inside of DiskCache to use
	DiskCacheDirGitiles string                   // a root directory inside of DiskCache to use
	Options             RemoteRepoManagerOptions // git related tweaks
	// Auth options; Should include user.email and gitiles oauth scopes.
	AuthOpts auth.Options

	m     sync.Mutex
	err   error
	cache source.Cache
	repos map[RepoKey]*remoteRepoImpl
	sem   *semaphore.Weighted
}

// RemoteRepoManagerOptions are populated based on CLI flags.
type RemoteRepoManagerOptions struct {
	GitDebug       bool // if true, emit verbose git logs
	GitConcurrency int  // if >0, run at most given number of git fetches at once
}

// DiskCache is a subset of lucicfg.Cache used by RemoteRepoManager
//
// Exists to break a reference cycle and to allow delaying creation of the
// cache directory until it is really used.
type DiskCache interface {
	// Subdir creates a cache subdirectory if doesn't exist yet.
	Subdir(path string) (string, error)
}

// Shutdown terminates any lingering git subprocesses.
func (r *RemoteRepoManager) Shutdown(ctx context.Context) {
	r.m.Lock()
	defer r.m.Unlock()
	if r.cache != nil {
		r.cache.Shutdown(ctx)
	}
}

// Repo implements RepoManager interface.
func (r *RemoteRepoManager) Repo(ctx context.Context, repoKey RepoKey) (Repo, error) {
	if !repoKey.IsRemote() {
		return nil, errors.Fmt("not a valid remote RepoKey: %s", repoKey)
	}

	r.m.Lock()
	defer r.m.Unlock()

	if err := r.initLocked(); err != nil {
		return nil, err
	}
	if repo := r.repos[repoKey]; repo != nil {
		return repo, nil
	}

	repoCache, err := r.cache.ForRepo(ctx, repoKey.RepoURL())
	if err != nil {
		return nil, err
	}

	repo := &remoteRepoImpl{
		repoKey:   repoKey,
		repoCache: repoCache,
		sem:       r.sem,
	}
	r.repos[repoKey] = repo
	return repo, nil
}

// init lazy-initializes RemoteRepoManager.
func (r *RemoteRepoManager) initLocked() error {
	if r.err == nil && r.cache == nil {
		r.err = func() error {
			gitDir, err := r.DiskCache.Subdir(r.DiskCacheDirGit)
			if err != nil {
				return err
			}
			gsource, err := gitsource.New(gitDir, r.Options.GitDebug)
			if err != nil {
				return err
			}
			gitilesDir, err := r.DiskCache.Subdir(r.DiskCacheDirGitiles)
			if err != nil {
				return err
			}
			cache, err := gitilessource.New(gitilesDir, r.AuthOpts, gsource)
			if err != nil {
				return err
			}
			r.cache = cache
			r.repos = make(map[RepoKey]*remoteRepoImpl, 1)
			if r.Options.GitConcurrency > 0 {
				r.sem = semaphore.NewWeighted(int64(r.Options.GitConcurrency))
			}
			return nil
		}()
	}
	return r.err
}

// remoteRepoImpl implements Repo.
type remoteRepoImpl struct {
	repoKey   RepoKey
	repoCache source.RepoCache
	sem       *semaphore.Weighted
}

func (r *remoteRepoImpl) acquireFetchConcurrencySlot(ctx context.Context) (done func()) {
	if r.sem == nil {
		return func() {}
	}
	if err := r.sem.Acquire(ctx, 1); err != nil {
		return func() {}
	}
	return func() { r.sem.Release(1) }
}

// Fetch implements Repo.
func (r *remoteRepoImpl) Fetch(ctx context.Context, rev string, repoPath string) ([]byte, error) {
	defer r.acquireFetchConcurrencySlot(ctx)()

	basePath := path.Base(repoPath)
	fetcher, err := r.repoCache.Fetcher(ctx, r.repoKey.Ref, rev, path.Dir(repoPath), func(kind source.ObjectKind, pkgRelPath string) bool {
		return pkgRelPath == basePath
	})
	if err != nil {
		return nil, errors.WrapIf(err, "making fetcher for %q of %s/%s", rev, r.repoKey, repoPath)
	}

	dat, err := fetcher.Read(ctx, basePath)
	return dat, errors.WrapIf(err, "fetching %q of %s/%s", rev, r.repoKey, repoPath)
}

// IsOverride implements Repo.
func (r *remoteRepoImpl) IsOverride() bool {
	return false
}

// Loader implements Repo.
func (r *remoteRepoImpl) Loader(ctx context.Context, rev string, pkgDir string, pkgName string, resources *fileset.Set) (interpreter.Loader, error) {
	defer r.acquireFetchConcurrencySlot(ctx)()

	fetcher, err := r.repoCache.Fetcher(ctx, r.repoKey.Ref, rev, pkgDir, func(kind source.ObjectKind, pkgRelPath string) bool {
		if kind == source.TreeKind {
			return true
		}
		if strings.HasSuffix(pkgRelPath, ".star") {
			return true // want all Starlark files unconditionally
		}
		// Ignore errors here. They will be rediscovered and bubble up more
		// naturally when these files are fetched for real (if this ever happens,
		// if not - even better).
		yes, _ := resources.Contains(pkgRelPath)
		return yes
	})

	switch {
	case errors.Is(err, source.ErrMissingObject):
		return nil, errors.Fmt("%s doesn't not contain %q", r.repoKey, rev)
	case err != nil:
		return nil, errors.Fmt("prefetching %q of %s/%s: %w", rev, r.repoKey, pkgDir, err)
	}

	return GenericLoader(GenericLoaderParams{
		Package:   pkgName,
		Resources: resources,
		Fetch: func(ctx context.Context, path string) ([]byte, error) {
			// Note: these are assumed to be local ops at this point and they do not
			// require acquireFetchConcurrencySlot.
			switch r, err := fetcher.Read(ctx, path); {
			case errors.Is(err, source.ErrObjectNotPrefetched):
				return nil, interpreter.ErrNoModule
			case err != nil:
				return nil, err
			default:
				return r, nil
			}
		},
	}), nil
}

// LoaderValidator implements Repo.
func (r *remoteRepoImpl) LoaderValidator(ctx context.Context, rev string, pkgDir string) (LoaderValidator, error) {
	return nil, nil // no validation for remote PACKAGE.star
}

// PickMostRecent implements Repo.
func (r *remoteRepoImpl) PickMostRecent(ctx context.Context, vers []string) (string, error) {
	if len(vers) == 1 {
		return vers[0], nil
	}

	defer r.acquireFetchConcurrencySlot(ctx)()

	return r.repoCache.PickMostRecent(ctx, r.repoKey.Ref, vers)
}

// RepoKey implements Repo.
func (r *remoteRepoImpl) RepoKey() RepoKey {
	return r.repoKey
}
