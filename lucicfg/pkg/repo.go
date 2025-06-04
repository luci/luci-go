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
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/fileset"
)

// PinnedVersion is a version placeholder that is used only by the root package
// and its local dependencies and it means "use the exact same revision as the
// root package".
//
// When loading packages from disk, we don't really know their real revision.
// We could ask the local git repo, but it will often return some local revision
// not comparable with anything in the remote repo. Essentially local
// dependencies exists at some indeterminable "virtual" version not comparable
// with any other remote version (and consequently this version can't
// participate in the Minimal Version Selection algorithm). "@pinned" represents
// such a version.
//
// Also, when doing Minimal Version Selection, there's a potential edge case of
// some remote dependency depending back on a root-local dependency. It is not
// clear what is the expected behavior in that case. As already mentioned we
// generally can't compare the local version to the requested remote version
// (to pick the most recent one). Arbitrarily deciding the local version is
// automatically "the most recent" can potentially violate version constraints
// (if this local version actually happens to be quite ancient).
//
// For now this edge case is forbidden and encountering it while evaluating
// dependencies will result in an error.
//
// Finally, when loading a root package from some remote storage, even if we
// know its correct (i.e. comparable) version, we still must preserve the exact
// same logic of handling of root-local dependencies. Otherwise executions that
// use a local disk and executions that use a remote storage will differ, even
// when the actual inputs are identical. For that reason we use "@pinned" for
// root-local packages even when the root is actually remote and has a known
// comparable revision.
const PinnedVersion = "@pinned"

// OverriddenVersion is a version placeholder that is used as versions of
// packages overridden using RepoOverride local overrides.
//
// It exists for similar reasons as "@pinned". One major difference is that
// we allow (non-overridden) remote dependencies to depend on overridden
// packages. In that case version requirements will be completely ignored and
// the overridden (local) version will always be used.
const OverriddenVersion = "@overridden"

// IsRemoteVersion is true for versions that point to remote packages.
func IsRemoteVersion(ver string) bool {
	return ver != PinnedVersion && ver != OverriddenVersion
}

// RepoKey identifies a repository.
//
// It is either a remote repository or a local root package repository (which
// is essentially equivalent to a local disk: the local repository can't
// generally compare versions).
type RepoKey struct {
	// Root is true for the local root repo (we assume we don't know what it is).
	Root bool
	// Host is a googlesource host of this package or "" for local root repo.
	Host string
	// Repo is the repository name on the host or "" for local root repo.
	Repo string
	// Ref is a git ref in the repository or "" for local root repo.
	Ref string
}

// String returns the representation of RepoKey for error messages and logs.
func (k RepoKey) String() string {
	if k.Root {
		return "the root package repository"
	}
	return k.Spec()
}

// Spec constructs a canonical spec string as appearing in the lockfile.
func (k RepoKey) Spec() string {
	if k.Root {
		return "@root" // this should never actually appear in the lockfile
	}
	return fmt.Sprintf("https://%s.googlesource.com/%s/+/%s", k.Host, k.Repo, k.Ref)
}

// IsRemote is true if this RepoKey is a fully populated remote repo key.
func (k RepoKey) IsRemote() bool {
	return !k.Root && k.Host != "" && k.Repo != "" && k.Ref != ""
}

// RepoURL constructs a canonical repository URL (without the ref).
//
// Panics when used with a non-remote RepoKey.
func (k RepoKey) RepoURL() string {
	if !k.IsRemote() {
		panic(fmt.Sprintf("not a valid remote RepoKey: %s", k))
	}
	return fmt.Sprintf("https://%s.googlesource.com/%s", k.Host, k.Repo)
}

// RepoKeyFromSpec reconstructs RepoKey from its spec.
func RepoKeyFromSpec(spec string) (RepoKey, error) {
	if spec == "@root" {
		return RepoKey{Root: true}, nil
	}
	if !strings.Contains(spec, "://") {
		spec = "https://" + spec
	}
	u, err := url.Parse(spec)
	if err != nil || u.Host == "" || u.Path == "" {
		return RepoKey{}, errors.New("the repo spec doesn't look like a repository URL")
	}
	host, _, _ := strings.Cut(u.Host, ".")
	repo, ref, _ := strings.Cut(u.Path, "/+/")
	repo = strings.TrimPrefix(repo, "/")
	if repo == "" {
		return RepoKey{}, errors.New("the repo spec doesn't look like a repository URL")
	}
	if ref == "" {
		return RepoKey{}, errors.New("the repo spec is missing a ref")
	}
	return RepoKey{
		Host: host,
		Repo: repo,
		Ref:  ref,
	}, nil
}

// RepoManager knows how to work with repositories to fetch package files.
type RepoManager interface {
	// Repo returns a representation of the given repository.
	//
	// Calling Repo with some repoKey twice should produce the exact same Repo
	// instance.
	Repo(ctx context.Context, repoKey RepoKey) (Repo, error)
}

// Repo is a representation of a repository (all revisions of some ref at once).
//
// All paths are slash-separated and relative to the repository root.
type Repo interface {
	// RepoKey identifies this particular repository.
	RepoKey() RepoKey
	// IsOverride is true if this is a local override repo.
	IsOverride() bool
	// PickMostRecent returns the most recent version from the given list.
	PickMostRecent(ctx context.Context, vers []string) (string, error)
	// Fetch fetches a single file at some revision.
	Fetch(ctx context.Context, rev, path string) ([]byte, error)
	// Loader fetches package files and returns a loader with them.
	Loader(ctx context.Context, rev, pkgDir, pkgName string, resources *fileset.Set) (interpreter.Loader, error)
	// LoaderValidator returns a validator of PACKAGE.star, if available.
	LoaderValidator(ctx context.Context, rev, pkgDir string) (LoaderValidator, error)
}

// ErroringRepoManager implements RepoManager by returning errors.
type ErroringRepoManager struct {
	// Error will be wrapped and returned.
	Error error
}

// Repo is a part of RepoManager interface.
func (rm *ErroringRepoManager) Repo(ctx context.Context, repoKey RepoKey) (Repo, error) {
	if rm.Error == nil {
		panic("error should not be nil")
	}
	return nil, errors.Fmt("repo %s: %w", repoKey, rm.Error)
}

// PreconfiguredRepoManager uses an existing static set of Repo instances with
// a RepoManager to use as a fallback to find other repos.
type PreconfiguredRepoManager struct {
	// Repos are known "preconfigured" repositories.
	Repos []Repo
	// Other is used to construct all other repositories.
	Other RepoManager
}

// Repo is a part of RepoManager interface.
func (rm *PreconfiguredRepoManager) Repo(ctx context.Context, repoKey RepoKey) (Repo, error) {
	for _, r := range rm.Repos {
		if r.RepoKey() == repoKey {
			return r, nil
		}
	}
	return rm.Other.Repo(ctx, repoKey)
}

// LocalDiskRepo implements Repo by fetching files from the local disk.
//
// It always expects to see the given Version ("@pinned" or "@overridden") as
// a revision in all calls. Attempting to compare this version to anything else
// is an error (see the PinnedVersion comment for details).
type LocalDiskRepo struct {
	// Root is an absolute native path to the repository checkout on disk.
	Root string
	// Key is the RepoKey this repository is known as.
	Key RepoKey
	// Version is either PinnedVersion or OverriddenVersion.
	Version string

	once      sync.Once
	statCache *statCache
}

// RepoKey is a part of Repo interface.
func (r *LocalDiskRepo) RepoKey() RepoKey {
	return r.Key
}

// IsOverride is a part of Repo interface.
func (r *LocalDiskRepo) IsOverride() bool {
	return r.Version == OverriddenVersion
}

// PickMostRecent is a part of Repo interface.
func (r *LocalDiskRepo) PickMostRecent(ctx context.Context, vers []string) (string, error) {
	var remote []string
	for _, ver := range vers {
		if ver != r.Version {
			remote = append(remote, ver)
		}
	}
	if len(remote) == 0 {
		return r.Version, nil
	}
	return "", errors.Fmt("local disk repo was unexpectedly asked to compare remote revisions: %v", remote)
}

// Fetch is a part of Repo interface.
func (r *LocalDiskRepo) Fetch(ctx context.Context, rev, path string) ([]byte, error) {
	if rev != r.Version {
		return nil, errors.Fmt("local disk repo was unexpectedly asked to fetch a remote version %q", rev)
	}
	blob, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(path)))
	if err != nil {
		return nil, errors.Fmt("local file: %s: %w", path, err)
	}
	return blob, nil
}

// Loader is a part of Repo interface.
func (r *LocalDiskRepo) Loader(ctx context.Context, rev, pkgDir, pkgName string, resources *fileset.Set) (interpreter.Loader, error) {
	if rev != r.Version {
		return nil, errors.Fmt("local disk repo was unexpectedly asked to fetch a remote version %q", rev)
	}
	r.once.Do(func() { r.statCache = syncStatCache() })
	return diskPackageLoader(filepath.Join(r.Root, filepath.FromSlash(pkgDir)), pkgName, resources, r.statCache)
}

// LoaderValidator is a part of Repo interface.
func (r *LocalDiskRepo) LoaderValidator(ctx context.Context, rev, pkgDir string) (LoaderValidator, error) {
	if rev != r.Version {
		return nil, errors.Fmt("local disk repo was unexpectedly asked to fetch a remote version %q", rev)
	}
	return &diskLoaderValidator{
		pkgRoot:   filepath.Join(r.Root, filepath.FromSlash(pkgDir)),
		repoRoot:  r.Root,
		statCache: syncStatCache(),
	}, nil
}

// TestRepoManager is used in tests as a mock for remote repositories.
//
// Repositories are represented by directories on disk matching the repository
// name (the host and the ref are ignored). Versions are "vN" strings (ordered
// as one would expect, e.g. v2 > v1). Each version is represented by
// a subdirectory of the repository directory.
type TestRepoManager struct {
	// Root is the path to the directory with test repositories.
	Root string

	m     sync.Mutex
	cache map[RepoKey]*TestRepo
}

// Repo is a part of RepoManager interface.
//
// It returns *TestRepo.
func (rm *TestRepoManager) Repo(ctx context.Context, repoKey RepoKey) (Repo, error) {
	if repoKey.Root {
		return nil, errors.New("unexpected request for a root repository")
	}
	rm.m.Lock()
	defer rm.m.Unlock()
	if r, ok := rm.cache[repoKey]; ok {
		return r, nil
	}
	r := &TestRepo{
		Path: filepath.Join(rm.Root, filepath.FromSlash(repoKey.Repo)),
		Key:  repoKey,
	}
	if rm.cache == nil {
		rm.cache = map[RepoKey]*TestRepo{}
	}
	rm.cache[repoKey] = r
	return r, nil
}

// TestRepo implements Repo as used in tests.
type TestRepo struct {
	// Path is a path to the test repository directory.
	Path string
	// Key is the corresponding RepoKey.
	Key RepoKey
}

func parseTestVer(v string) (int, bool) {
	if val, ok := strings.CutPrefix(v, "v"); ok {
		if parsed, err := strconv.ParseInt(val, 10, 32); err == nil {
			return int(parsed), true
		}
	}
	return 0, false
}

func (r *TestRepo) verDir(v string) (string, error) {
	if _, ok := parseTestVer(v); !ok {
		return "", errors.Fmt("test repo %s got unexpected version %q", r.Key, v)
	}
	d := filepath.Join(r.Path, v)
	if _, err := os.Stat(d); err != nil {
		return "", errors.Fmt("test repo %s doesn have version %q: %w", r.Key, v, err)
	}
	return d, nil
}

// RepoKey is a part of Repo interface.
func (r *TestRepo) RepoKey() RepoKey {
	return r.Key
}

// IsOverride is a part of Repo interface.
func (r *TestRepo) IsOverride() bool {
	return false
}

// PickMostRecent is a part of Repo interface.
func (r *TestRepo) PickMostRecent(ctx context.Context, vers []string) (string, error) {
	var nums []int
	for _, v := range vers {
		if _, err := r.verDir(v); err != nil {
			return "", err
		}
		val, _ := parseTestVer(v)
		nums = append(nums, val)
	}
	return fmt.Sprintf("v%d", slices.Max(nums)), nil
}

// Fetch is a part of Repo interface.
func (r *TestRepo) Fetch(ctx context.Context, rev, path string) ([]byte, error) {
	dir, err := r.verDir(rev)
	if err != nil {
		return nil, err
	}
	blob, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		return nil, errors.Fmt("repository file %s: %w", path, err)
	}
	return blob, nil
}

// LoaderValidator is a part of Repo interface.
func (r *TestRepo) LoaderValidator(ctx context.Context, rev, pkgDir string) (LoaderValidator, error) {
	// There's no validation currently when fetching remote PACKAGE.star.
	return nil, nil
}

// Loader is a part of Repo interface.
func (r *TestRepo) Loader(ctx context.Context, rev, pkgDir, pkgName string, resources *fileset.Set) (interpreter.Loader, error) {
	dir, err := r.verDir(rev)
	if err != nil {
		return nil, err
	}
	// Note TestRepo mimics a remote repository, not the local disk. For that
	// reason we use a loader that is oblivious of any local disk details (i.e. we
	// purposefully do not reuse diskPackageLoader here).
	return GenericLoader(GenericLoaderParams{
		Package:   pkgName,
		Resources: resources,
		Fetch: func(ctx context.Context, path string) ([]byte, error) {
			blob, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(pkgDir), filepath.FromSlash(path)))
			if errors.Is(err, os.ErrNotExist) {
				return nil, interpreter.ErrNoModule
			}
			return blob, err
		},
	}), nil
}
