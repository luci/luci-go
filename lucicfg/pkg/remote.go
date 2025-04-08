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
	"io"
	"path"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/depsource/gitsource"
	"go.chromium.org/luci/lucicfg/fileset"
)

// RemoteRepoManager implements RepoManager that knows how to work with remote
// git repositories using a local disk cache.
type RemoteRepoManager struct {
	DiskCache    DiskCache // usually implemented by lucicfg.Cache
	DiskCacheDir string    // a root directory inside of DiskCache to use

	m     sync.Mutex
	err   error
	cache *gitsource.Cache
	repos map[RepoKey]*remoteRepoImpl
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
func (r *RemoteRepoManager) Shutdown() {
	r.m.Lock()
	defer r.m.Unlock()
	if r.cache != nil {
		r.cache.Shutdown()
	}
}

// Repo implements RepoManager interface.
func (r *RemoteRepoManager) Repo(ctx context.Context, repoKey RepoKey) (Repo, error) {
	if !repoKey.IsRemote() {
		return nil, errors.Reason("not a valid remote RepoKey: %s", repoKey).Err()
	}

	r.m.Lock()
	defer r.m.Unlock()

	if err := r.init(); err != nil {
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
	}
	r.repos[repoKey] = repo
	return repo, nil
}

// init lazy-initializes RemoteRepoManager.
func (r *RemoteRepoManager) init() error {
	if r.err == nil {
		r.err = func() error {
			dir, err := r.DiskCache.Subdir(r.DiskCacheDir)
			if err != nil {
				return err
			}
			cache, err := gitsource.New(dir)
			if err != nil {
				return err
			}
			r.cache = cache
			r.repos = make(map[RepoKey]*remoteRepoImpl, 1)
			return nil
		}()
	}
	return r.err
}

// remoteRepoImpl implements Repo.
type remoteRepoImpl struct {
	repoKey   RepoKey
	repoCache *gitsource.RepoCache
}

// Fetch implements Repo.
func (r *remoteRepoImpl) Fetch(ctx context.Context, rev string, repoPath string) ([]byte, error) {
	dir, file := path.Split(repoPath)
	dir = path.Clean(dir)
	file = path.Clean(file)

	fetcher, err := r.repoCache.Fetcher(ctx, r.repoKey.Ref, rev, dir, func(kind gitsource.ObjectKind, pkgRelPath string) bool {
		return kind == gitsource.BlobKind && pkgRelPath == file
	})
	switch {
	case errors.Is(err, gitsource.ErrMissingObject):
		return nil, errors.Reason("%s doesn't not contain %q", r.repoKey, rev).Err()
	case err != nil:
		return nil, errors.Annotate(err, "fetching %q of %s/%s", rev, r.repoKey, repoPath).Err()
	}

	switch reader, err := fetcher.Read(ctx, file); {
	case errors.Is(err, gitsource.ErrObjectNotPrefetched):
		return nil, ErrFileNotInRepo
	case err != nil:
		return nil, errors.Annotate(err, "fetching %q of %s/%s", rev, r.repoKey, repoPath).Err()
	default:
		defer func() { _ = reader.Close() }()
		return io.ReadAll(reader)
	}
}

// IsOverride implements Repo.
func (r *remoteRepoImpl) IsOverride() bool {
	return false
}

// Loader implements Repo.
func (r *remoteRepoImpl) Loader(ctx context.Context, rev string, pkgDir string, pkgName string, resources *fileset.Set) (interpreter.Loader, error) {
	fetcher, err := r.repoCache.Fetcher(ctx, r.repoKey.Ref, rev, pkgDir, func(kind gitsource.ObjectKind, pkgRelPath string) bool {
		if kind == gitsource.TreeKind {
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
	case errors.Is(err, gitsource.ErrMissingObject):
		return nil, errors.Reason("%s doesn't not contain %q", r.repoKey, rev).Err()
	case err != nil:
		return nil, errors.Annotate(err, "prefetching %q of %s/%s", rev, r.repoKey, pkgDir).Err()
	}

	return GenericLoader(GenericLoaderParams{
		Package:   pkgName,
		Resources: resources,
		Fetch: func(ctx context.Context, path string) ([]byte, error) {
			switch r, err := fetcher.Read(ctx, path); {
			case errors.Is(err, gitsource.ErrObjectNotPrefetched):
				return nil, interpreter.ErrNoModule
			case err != nil:
				return nil, err
			default:
				defer func() { _ = r.Close() }()
				return io.ReadAll(r)
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
	ordered, err := r.repoCache.Order(ctx, r.repoKey.Ref, vers)
	if err != nil {
		return "", err
	}
	return ordered[len(ordered)-1], nil
}

// RepoKey implements Repo.
func (r *remoteRepoImpl) RepoKey() RepoKey {
	return r.repoKey
}
