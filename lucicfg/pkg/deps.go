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
	"cmp"
	"context"
	"fmt"
	"path"
	"slices"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/internal"
	"go.chromium.org/luci/lucicfg/pkg/mvs"
)

// DepContext points to a "current" package when traversing dependencies.
//
// Given a DepContext of a package and some its direct dependency, it is
// possible to construct a DepContext for this dependency (this is what
// Follow does).
type DepContext struct {
	// Package is the package being explored.
	Package string
	// Version is the package version string or "@pinned" for root-local deps.
	Version string
	// Repo is the repository the package resides in.
	Repo Repo
	// Path is a slash-separated path to the package within the repository.
	Path string
	// RepoManager knows how to access repositories with other dependencies.
	RepoManager RepoManager
	// Known is the package definition if known in advance (e.g. the root).
	Known *Definition

	once sync.Once
	def  *Definition
	err  error
}

// Definition lazily loads the package definition.
//
// Can do network calls and be slow on the first call. Subsequent calls are
// fast (they just return the cached value).
func (d *DepContext) Definition(ctx context.Context) (*Definition, error) {
	var def *Definition
	if d.Known != nil {
		def = d.Known
	} else {
		d.once.Do(func() {
			d.def, d.err = func() (*Definition, error) {
				blob, err := d.Repo.Fetch(ctx, d.Version, path.Join(d.Path, PackageScript))
				if err != nil {
					return nil, err
				}
				validator, err := d.Repo.LoaderValidator(ctx, d.Version, d.Path)
				if err != nil {
					return nil, err
				}
				if validator == nil {
					validator = NoopLoaderValidator{}
				}
				return LoadDefinition(ctx, blob, validator)
			}()
		})
		if d.err != nil {
			return nil, d.err
		}
		def = d.def
	}
	if def.Name != d.Package {
		return nil, errors.Reason("expected to find package %q, but found %q instead", d.Package, def.Name).Err()
	}
	return def, nil
}

// Follow builds a *DepContext representing a direct dependency.
//
// This is generally fast, it doesn't fetch anything. Note that some constructed
// *DepContext may be silently discarded (in case it actually points to an
// already explored dependency: we learn about this only after constructing this
// *DepContext).
func (d *DepContext) Follow(ctx context.Context, decl *DepDecl) (*DepContext, error) {
	// If this is a local dependency between packages, inherit the repository and
	// the version.
	if decl.LocalPath != "" {
		repoPath := path.Join(d.Path, decl.LocalPath)
		if repoPath == ".." || strings.HasPrefix(repoPath, "../") {
			return nil, errors.Reason("local dependency on %q points to a path outside the repository: %q", decl.Name, decl.LocalPath).Err()
		}
		return &DepContext{
			Package:     decl.Name,
			Version:     d.Version,
			Repo:        d.Repo,
			Path:        repoPath,
			RepoManager: d.RepoManager,
		}, nil
	}

	// If this is a remote dependency, grab the corresponding repo from
	// the RepoManager cache.
	repo, err := d.RepoManager.Repo(ctx, RepoKey{
		Host: decl.Host,
		Repo: decl.Repo,
		Ref:  decl.Ref,
	})
	if err != nil {
		return nil, errors.Annotate(err, "dependency on %q", decl.Name).Err()
	}

	// If this repository is a local override, ignore the actually requested
	// remote revision of the dependency. We always pick the override. This also
	// guarantees PickMostRecent call later is never confused, since it will see
	// only one revision.
	revision := decl.Revision
	if repo.IsOverride() {
		revision = OverriddenVersion
	}

	return &DepContext{
		Package:     decl.Name,
		Version:     revision,
		Repo:        repo,
		Path:        decl.Path,
		RepoManager: d.RepoManager,
	}, nil
}

// PrefetchDep constructs a loader that can fetch package code through the Repo
// and returns it as a part of *Dep: the final fully constructed dependency
// ready to be used by the interpreter.
//
// Takes care of prefetching all Starlark files and resources declared by the
// package. Can do network calls and be slow.
func (d *DepContext) PrefetchDep(ctx context.Context) (*Dep, error) {
	def, err := d.Definition(ctx)
	if err != nil {
		return nil, err
	}

	var basePath string
	if d.Path != "." {
		basePath = d.Path + "/"
	}

	err = d.Repo.Prefetch(ctx, d.Version, func(p string) bool {
		rel, ok := strings.CutPrefix(p, basePath)
		if !ok || rel == "" {
			return false // outside of the package directory
		}
		if strings.HasSuffix(rel, ".star") {
			return true // want all Starlark files unconditionally
		}
		// Ignore errors here. They will be rediscovered and bubble up more
		// naturally when these files are fetched for real (if this ever happens,
		// if not - even better).
		yes, _ := def.ResourcesSet.Contains(rel)
		return yes
	})
	if err != nil {
		return nil, err
	}

	code, err := d.Repo.Loader(ctx, d.Version, d.Path, d.Package, def.ResourcesSet)
	if err != nil {
		return nil, err
	}
	return &Dep{
		Package:    d.Package,
		Min:        def.MinLucicfgVersion,
		Code:       code,
		DirectDeps: def.DirectDeps(),
	}, nil
}

// Dep is a discovered transitive dependency of the main package.
type Dep struct {
	// Package is the package name as a "@name" string.
	Package string
	// Min is the minimum required lucicfg version.
	Min LucicfgVersion
	// Code is the loader with the package code.
	Code interpreter.Loader
	// DirectDeps are direct dependencies of this package as "@name" strings.
	DirectDeps []string
}

// pkgVer is a package version together with the repository it is relative to.
type pkgVer struct {
	ver  string
	repo RepoKey
}

// String is used to debug-print this version.
func (v pkgVer) String() string {
	return v.ver
}

// edgeMeta is used as a mvs.Dep.Meta value.
type edgeMeta struct {
	src *DepContext // the context of the package that declares the dependency
	dcl *DepDecl    // the declaration of the dependency
	dst *DepContext // the context of the dependency itself
}

// msvPkg converts a *DepContext to a msv.Package.
func msvPkg(dep *DepContext) mvs.Package[pkgVer] {
	return mvs.Package[pkgVer]{
		Package: dep.Package,
		Version: pkgVer{
			ver:  dep.Version,
			repo: dep.Repo.RepoKey(),
		},
	}
}

// discoverDeps finds all transitive dependencies of the root package.
//
// The returned list excludes the root itself.
func discoverDeps(ctx context.Context, root *DepContext) ([]*Dep, error) {
	graph := mvs.NewGraph[pkgVer, *edgeMeta](msvPkg(root))

	// All successfully visited *DepContext (with their *Definition).
	var visited internal.SyncWriteMap[mvs.Package[pkgVer], *DepContext]

	wq, wqctx := internal.NewWorkQueue[*DepContext](ctx)
	wq.Launch(func(src *DepContext) error {
		// Load the package definition (this can be slow).
		pkgDef, err := src.Definition(wqctx)
		if err != nil {
			return err
		}

		// Construct all edges to other packages.
		deps := make([]mvs.Dep[pkgVer, *edgeMeta], len(pkgDef.Deps))
		for i, dcl := range pkgDef.Deps {
			dst, err := src.Follow(wqctx, dcl)
			if err != nil {
				return err
			}
			deps[i] = mvs.Dep[pkgVer, *edgeMeta]{
				Package: msvPkg(dst),
				Meta: &edgeMeta{
					src: src,
					dcl: dcl,
					dst: dst,
				},
			}
		}

		// Declare edges and enqueue work to explore never seen before packages.
		srcMsvPkg := msvPkg(src)
		for _, unexplored := range graph.Require(srcMsvPkg, deps) {
			wq.Submit(unexplored.Meta.dst)
		}

		// Successfully processed this dependency.
		visited.Put(srcMsvPkg, src)
		return nil
	})

	// Chew on the graph from the root, transitively loading all dependencies.
	wq.Submit(root)
	if err := wq.Wait(); err != nil {
		return nil, err
	}
	if !graph.Finalize() {
		panic("somehow the graph has unexplored nodes")
	}

	// Get all observed versions of all packages and for each package select the
	// most recent version based on the ordering implemented by the corresponding
	// Repo. This is potentially slow, so do it in parallel.
	var selected internal.SyncWriteMap[string, pkgVer]
	eg, ectx := errgroup.WithContext(ctx)
	for _, pkg := range graph.Packages() {
		eg.Go(func() error {
			vers := graph.Versions(pkg)
			if len(vers) == 0 {
				panic("impossible")
			}

			// Check all versions agree on the package repository. In particular
			// this will bark if a root-local "@pinned" package is also imported as
			// a remote package. Also collect actual version strings to compare them
			// to one another later.
			var repoKey RepoKey
			strVers := make([]string, 0, len(vers))
			seenRepoKeys := make(map[RepoKey]struct{}, 1)
			for _, ver := range vers {
				strVers = append(strVers, ver.ver)
				seenRepoKeys[ver.repo] = struct{}{}
				repoKey = ver.repo
			}
			if len(seenRepoKeys) != 1 {
				var report []string
				for key := range seenRepoKeys {
					report = append(report, key.String())
				}
				slices.Sort(report)
				return errors.Reason(
					"package %q is imported from multiple different repositories:\n%s",
					pkg, strings.Join(report, "\n"),
				).Err()
			}

			// Note that all string versions will be different, since mvs.Graph
			// deduplicates completely identical pkgVer values and we already verified
			// all pkgVer have identical pkgVer.repo field, so all version strings
			// must be different then). Sort them lexicographically just to remove
			// any non-determinism from calls to repo.PickMostRecent.
			slices.Sort(strVers)

			// Ask the repository to find the most recent version.
			repo, err := root.RepoManager.Repo(ectx, repoKey)
			if err != nil {
				return errors.Annotate(err, "examining %q", pkg).Err()
			}
			mostRecent, err := repo.PickMostRecent(ectx, strVers)
			if err != nil {
				return errors.Annotate(err, "determining the most recent version of %q", pkg).Err()
			}
			selected.Put(pkg, pkgVer{
				ver:  mostRecent,
				repo: repoKey,
			})
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	// The final selected list of dependencies.
	var depsList []mvs.Package[pkgVer]

	// Traverse the graph from the root, following selected versions. Keep only
	// dependencies that were actually visited. It is possible some packages are
	// no longer referenced if we use the selected versions of dependencies.
	err := graph.Traverse(func(cur mvs.Package[pkgVer], edges []mvs.Dep[pkgVer, *edgeMeta]) ([]mvs.Package[pkgVer], error) {
		depsList = append(depsList, cur)
		var visit []mvs.Package[pkgVer]
		for _, edge := range edges {
			visit = append(visit, mvs.Package[pkgVer]{
				Package: edge.Package.Package,
				Version: selected.Get(edge.Package.Package),
			})
		}
		return visit, nil
	})
	if err != nil {
		panic(fmt.Sprintf("somehow encountered an unknown package version: %s", err))
	}

	// Chop off the root (it is always first). Sort the rest by the package name
	// (the order doesn't really matter, just makes the result deterministic).
	depsList = depsList[1:]
	slices.SortFunc(depsList, func(a, b mvs.Package[pkgVer]) int {
		return cmp.Compare(a.Package, b.Package)
	})

	// Construct final loaders. This does prefetching and can be slow. Do it in
	// parallel.
	out := make([]*Dep, len(depsList))
	eg, ectx = errgroup.WithContext(ctx)
	for i, dep := range depsList {
		eg.Go(func() error {
			var err error
			out[i], err = visited.Get(dep).PrefetchDep(ectx)
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}
