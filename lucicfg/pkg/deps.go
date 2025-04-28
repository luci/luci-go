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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/internal"
	"go.chromium.org/luci/lucicfg/internal/ui"
	"go.chromium.org/luci/lucicfg/pkg/gitsource"
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
	// Parent is what package introduced this dependency.
	Parent *DepContext
	// RepoManager knows how to access repositories with other dependencies.
	RepoManager RepoManager
	// Known is the package definition if known in advance (e.g. the root).
	Known *Definition
	// Activity to set as the current when loading the package definition.
	Activity *ui.Activity

	once sync.Once
	def  *Definition
	err  error
}

// DepError is an error happened when processing a particular DepContext.
type DepError struct {
	Err error       // the actual error
	Dep *DepContext // where it happened
}

// Error implements error interface.
func (e *DepError) Error() string {
	return e.Err.Error()
}

// Unwrap allows to traverse this error.
func (e *DepError) Unwrap() error {
	return e.Err
}

// ExtraContext implements errs.WithExtraContext.
func (e *DepError) ExtraContext() string {
	var out strings.Builder
	out.WriteString("\nHappened when evaluating\n")
	cur := e.Dep
	for cur != nil {
		rev := ""
		if IsRemoteVersion(cur.Version) {
			rev = fmt.Sprintf(" (rev %s)", cur.Version)
		}
		_, _ = fmt.Fprintf(&out, "  package %q at %q of %s%s", cur.Package, cur.Path, cur.Repo.RepoKey(), rev)
		cur = cur.Parent
		if cur != nil {
			out.WriteString("\nimported by\n")
		}
	}
	return out.String()
}

// Definition lazily loads the package definition.
//
// Can do network calls and be slow on the first call. Subsequent calls are
// fast (they just return the cached value).
func (d *DepContext) Definition(ctx context.Context) (*Definition, error) {
	if d.Known != nil {
		if d.Known.Name != d.Package {
			return nil, &DepError{
				Err: errors.Reason("expected to find package %q, but found %q instead", d.Package, d.Known.Name).Err(),
				Dep: d,
			}
		}
		return d.Known, nil
	}

	d.once.Do(func() {
		d.def, d.err = func() (def *Definition, err error) {
			scriptPath := path.Join(d.Path, PackageScript)
			if d.Activity != nil {
				ctx = ui.ActivityStart(ctx, d.Activity, "fetching %s", scriptPath)
				defer func() {
					if err == nil {
						ui.ActivityDone(ctx, "")
					} else {
						ui.ActivityError(ctx, "%s", errorForActivity(err, scriptPath))
					}
				}()
			}
			blob, err := d.Repo.Fetch(ctx, d.Version, scriptPath)
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
			def, err = LoadDefinition(ctx, blob, validator)
			if err != nil {
				return nil, err
			}
			if def.Name != d.Package {
				return nil, errors.Reason("expected to find package %q, but found %q instead", d.Package, def.Name).Err()
			}
			return def, nil
		}()
	})

	if d.err != nil {
		return nil, &DepError{
			Err: d.err,
			Dep: d,
		}
	}

	return d.def, nil
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
			return nil, &DepError{
				Err: errors.Reason("local dependency on %q points to a path outside the repository: %q", decl.Name, decl.LocalPath).Err(),
				Dep: d,
			}
		}
		return &DepContext{
			Package:     decl.Name,
			Version:     d.Version,
			Repo:        d.Repo,
			Path:        repoPath,
			Parent:      d,
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
		return nil, &DepError{
			Err: errors.Annotate(err, "dependency on %q", decl.Name).Err(),
			Dep: d,
		}
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
		Parent:      d,
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
		return nil, err // already a *DepError
	}
	code, err := d.Repo.Loader(ctx, d.Version, d.Path, d.Package, def.ResourcesSet)
	if err != nil {
		return nil, &DepError{
			Err: err,
			Dep: d,
		}
	}
	return &Dep{
		Package:    d.Package,
		Version:    d.Version,
		Repo:       d.Repo,
		Path:       d.Path,
		Min:        def.MinLucicfgVersion,
		Code:       code,
		DirectDeps: def.DirectDeps(),
		Resources:  slices.Sorted(slices.Values(def.Resources)),
	}, nil
}

// Dep is a discovered transitive dependency of the main package.
type Dep struct {
	// Package is the package name as a "@name" string.
	Package string
	// Version is the package version string (or "@pinned" or "@overridden").
	Version string
	// Repo is the repository the package resides in.
	Repo Repo
	// Path is a slash-separated path to the package within the repository.
	Path string
	// Min is the minimum required lucicfg version.
	Min LucicfgVersion
	// Code is the loader with the package code.
	Code interpreter.Loader
	// DirectDeps are direct dependencies of this package as "@name" strings.
	DirectDeps []string
	// Resources is a sorted list of resource file patterns.
	Resources []string
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

// depsGraph holds the graph of dependencies.
type depsGraph struct {
	graph   *mvs.Graph[pkgVer]
	root    *DepContext
	visited internal.SyncWriteMap[mvs.Package[pkgVer], *DepContext]
}

// discoverDeps finds all transitive dependencies of the root package.
//
// The returned list excludes the root itself.
func discoverDeps(ctx context.Context, root *DepContext) (deps []*Dep, err error) {
	start := clock.Now(ctx)
	defer func() {
		// Log only if really did something or failed to do something.
		if len(deps) != 0 || err != nil {
			logging.Infof(ctx, "Processed dependencies in %.1fs", clock.Since(ctx, start).Seconds())
		}
	}()

	// Populate the graph with all transitive dependencies across all versions.
	dg := &depsGraph{
		graph: mvs.NewGraph[pkgVer](msvPkg(root)),
		root:  root,
	}
	if err := traverseDeps(ctx, dg); err != nil {
		return nil, err
	}

	// Get all observed versions of all packages and for each package select the
	// most recent version based on the ordering implemented by the corresponding
	// Repo.
	depsList, err := resolveVersions(ctx, dg)
	if err != nil {
		return nil, err
	}

	// Construct final loaders.
	return prefetchDeps(ctx, dg, depsList)
}

// traverseDeps populates the graph by traversing pkg.depend(...) edges.
func traverseDeps(ctx context.Context, dg *depsGraph) error {
	ctx, done := ui.NewActivityGroup(ctx, "Traversing remote dependencies")
	defer done()

	wq, wqctx := internal.NewWorkQueue[*DepContext](ctx)
	wq.Launch(func(src *DepContext) error {
		// Load the package definition (this can be slow).
		pkgDef, err := src.Definition(wqctx)
		if err != nil {
			return err
		}

		// Construct all edges to other packages.
		edges := make([]mvs.Package[pkgVer], len(pkgDef.Deps))
		ctxs := make(map[mvs.Package[pkgVer]]*DepContext, len(pkgDef.Deps))
		for i, dcl := range pkgDef.Deps {
			dst, err := src.Follow(wqctx, dcl)
			if err != nil {
				return err
			}
			edges[i] = msvPkg(dst)
			ctxs[edges[i]] = dst
		}

		// Declare edges and enqueue work to explore never seen before packages.
		srcMsvPkg := msvPkg(src)
		for _, depPkg := range dg.graph.Require(srcMsvPkg, edges) {
			// Pre-register activities now to make sure they show up in the UI in
			// deterministic order. Omit local dependencies from the UI, they are
			// fetched "instantly" and not worth showing.
			depCtx := ctxs[depPkg]
			if IsRemoteVersion(depCtx.Version) {
				depCtx.Activity = ui.NewActivity(ctx, ui.ActivityInfo{
					Package: depCtx.Package,
					Version: depCtx.Version,
				})
			}
			wq.Submit(depCtx)
		}

		// Successfully processed this dependency.
		dg.visited.Put(srcMsvPkg, src)
		return nil
	})

	// Chew on the graph from the root, transitively loading all dependencies.
	wq.Submit(dg.root)
	if err := wq.Wait(); err != nil {
		return err
	}
	if !dg.graph.Finalize() {
		panic("somehow the graph has unexplored nodes")
	}
	return nil
}

// resolveVersions runs MVS algorithms on the populated graph.
func resolveVersions(ctx context.Context, dg *depsGraph) ([]mvs.Package[pkgVer], error) {
	ctx, done := ui.NewActivityGroup(ctx, "Selecting versions to fetch")
	defer done()

	var selected internal.SyncWriteMap[string, pkgVer]
	eg, ectx := errgroup.WithContext(ctx)

	for _, pkg := range dg.graph.Packages() {
		vers := dg.graph.Versions(pkg)
		if len(vers) == 0 {
			panic("impossible")
		}

		// If there's only one version, there's nothing to resolve, pick it right
		// away (skipping the activities UI). On an error we'll proceed and report
		// this error inside the activity, where it will be handled appropriately
		// (showing in the UI and aborting the errgroup).
		repoKey, strVers, conflictErr := collectVersions(vers)
		if conflictErr == nil && len(strVers) == 1 {
			selected.Put(pkg, pkgVer{
				ver:  strVers[0],
				repo: repoKey,
			})
			continue
		}

		activity := ui.NewActivity(ectx, ui.ActivityInfo{
			Package: pkg,
		})

		eg.Go(func() (err error) {
			var mostRecent string

			ctx := ui.ActivityStart(ectx, activity, "resolving")
			defer func() {
				if err == nil {
					ui.ActivityDone(ctx, "%s", mostRecent)
				} else {
					ui.ActivityError(ctx, "%s", errorForActivity(err, ""))
				}
			}()

			// Report the repo conflict error though the activity to abort
			// the errgroup.
			if conflictErr != nil {
				return errors.Annotate(conflictErr, "examining %q", pkg).Err()
			}

			// Ask the repository to find the most recent version.
			repo, err := dg.root.RepoManager.Repo(ctx, repoKey)
			if err != nil {
				return errors.Annotate(err, "examining %q", pkg).Err()
			}
			mostRecent, err = repo.PickMostRecent(ctx, strVers)
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
	err := dg.graph.Traverse(func(cur mvs.Package[pkgVer], edges []mvs.Package[pkgVer]) ([]mvs.Package[pkgVer], error) {
		depsList = append(depsList, cur)
		var visit []mvs.Package[pkgVer]
		for _, edge := range edges {
			visit = append(visit, mvs.Package[pkgVer]{
				Package: edge.Package,
				Version: selected.Get(edge.Package),
			})
		}
		return visit, nil
	})
	if err != nil {
		panic(fmt.Sprintf("somehow encountered an unknown package version: %s", err))
	}

	// Chop off the root (it is always first). Sort the rest by the package name
	// (this eventually defines how packages are ordered in the lockfile).
	depsList = depsList[1:]
	slices.SortFunc(depsList, func(a, b mvs.Package[pkgVer]) int {
		return cmp.Compare(a.Package, b.Package)
	})

	return depsList, nil
}

// prefetchDeps prefetches all dependencies at their resolved versions.
func prefetchDeps(ctx context.Context, gr *depsGraph, depsList []mvs.Package[pkgVer]) ([]*Dep, error) {
	ctx, done := ui.NewActivityGroup(ctx, "Fetching remote dependencies")
	defer done()

	out := make([]*Dep, len(depsList))
	eg, ectx := errgroup.WithContext(ctx)

	for i, dep := range depsList {
		var activity *ui.Activity
		if IsRemoteVersion(dep.Version.ver) {
			activity = ui.NewActivity(ectx, ui.ActivityInfo{
				Package: dep.Package,
				Version: dep.Version.ver,
			})
		}

		eg.Go(func() (err error) {
			ctx := ectx
			if activity != nil {
				ctx = ui.ActivityStart(ectx, activity, "fetching")
				defer func() {
					if err == nil {
						ui.ActivityDone(ctx, "")
					} else {
						ui.ActivityError(ctx, "%s", errorForActivity(err, ""))
					}
				}()
			}
			out[i], err = gr.visited.Get(dep).PrefetchDep(ctx)
			return
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return out, nil
}

type conflictErr struct {
	conflictingRepos []string
}

func (e *conflictErr) Error() string {
	return fmt.Sprintf(
		"the package is imported from multiple different repositories:\n%s",
		strings.Join(e.conflictingRepos, "\n"),
	)
}

// collectVersions returns an alphabetically sorted list of version strings.
//
// It also checks all versions agree on the package repository, returning this
// repository as well. In particular this will bark if a root-local "@pinned"
// package is also imported as a remote package.
func collectVersions(vers []pkgVer) (repoKey RepoKey, strVers []string, err error) {
	strVers = make([]string, 0, len(vers))
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
		err = &conflictErr{conflictingRepos: report}
	} else {
		// Note that all string versions will be different, since mvs.Graph
		// deduplicates completely identical pkgVer values and we already verified
		// all pkgVer have identical pkgVer.repo field, so all version strings
		// must be different then). Sort them lexicographically just to remove
		// any non-determinism from calls to repo.PickMostRecent.
		slices.Sort(strVers)
	}
	return
}

// errorForActivity extracts the most relevant error message to display in the
// activities UI.
func errorForActivity(err error, path string) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, gitsource.ErrMissingCommit):
		return "no such commit"
	case errors.Is(err, gitsource.ErrMissingObject):
		return fmt.Sprintf("%s: no such file in the commit or no such commit", path)
	}

	// The conflict error is too verbose, shorten it. It will be reported in full
	// it the final error log.
	var confErr *conflictErr
	if errors.As(err, &confErr) {
		return "conflict"
	}

	// Shed any wrappers around git errors, they duplicate information already
	// present in activities UI (like the repo and commit being worked on). Full
	// errors are displayed in the log at the end of the failed run. Activities UI
	// shows only the root cause.
	var gitErr *gitsource.GitError
	if errors.As(err, &gitErr) {
		return fmt.Sprintf("git: %s", gitErr.Err)
	}

	return err.Error()
}
