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

// Package pkg implements lucicfg packages functionality.
package pkg

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"

	"go.chromium.org/luci/lucicfg/buildifier"
	"go.chromium.org/luci/lucicfg/internal"
	"go.chromium.org/luci/lucicfg/lockfilepb"
)

const (
	// PackageScript is a name of the script with the package definition.
	PackageScript = "PACKAGE.star"
	// LockfileName is the name of the lockfile (sibling of PACKAGE.star).
	LockfileName = "PACKAGE.lock"
	// LegacyPackageNamePlaceholder is used as package name of legacy packages.
	LegacyPackageNamePlaceholder = "@__main__"
)

// Entry is a main package plus an entry point executable file in it.
//
// This package and all its dependencies are fully resolved and prefetched and
// ready for execution.
//
// It can be loaded either from files on disk or from some abstracted storage.
// For that reason it doesn't include absolute paths.
type Entry struct {
	// Main contains the code of the main package itself.
	Main interpreter.Loader
	// Deps contains the code of all dependencies at resolved versions.
	Deps map[string]interpreter.Loader
	// DepGraph maps a package name to a list of its direct dependencies.
	DepGraph map[string][]string
	// Package is the name of the package being executed from its PACKAGE.star.
	Package string
	// Path is a slash-separate path from the repo root to the package root.
	Path string
	// Script is a slash-separated path to the script within the package to run.
	Script string
	// LucicfgVersionConstraints is the requirements for the lucicfg version.
	LucicfgVersionConstraints []LucicfgVersionConstraint
	// Local is set only if this entry was loaded from the local disk.
	Local *Local
}

// LucicfgVersionConstraint puts a constraint on the current lucicfg version.
type LucicfgVersionConstraint struct {
	Min     LucicfgVersion // minimal required version
	Package string         // name of the package that required it
	Main    bool           // true if this is the main package (i.e. contains the entry point)
}

// RepoOverride can be used to override a remote repository with a local one for
// the scenario of locally developing packages from different repos that depend
// on one another.
//
// Overrides work on repository level to simplify dealing with local
// dependencies of overridden packages: when overriding a whole repository all
// such dependencies are overridden as well and there's no ambiguity with what
// their version to use.
//
// The expected workflow is that there's a local git checkout of the remote repo
// with some local changes that needs to be tested against a root package in
// another git checkout.
type RepoOverride struct {
	// Host is a googlesource host of the repository to override.
	Host string
	// Repo is the repository name on the host to override.
	Repo string
	// Ref is a git ref in the repository to override.
	Ref string
	// LocalRoot is an absolute native path to the local repository checkout.
	LocalRoot string
}

// RepoOverrideFromSpec creates a RepoOverride from the given spec.
//
// Will verify the local root directory actually exists and looks like
// a repository root.
//
// Expected format of the spec is: "[protocol://]host[.domain]/repo/[+/ref]".
// We'll extract Host, Repo and Ref from it, discarding all the rest.
func RepoOverrideFromSpec(spec, localRoot string) (*RepoOverride, error) {
	repoKey, err := RepoKeyFromSpec(spec)
	switch {
	case err != nil:
		return nil, err
	case repoKey.Root:
		return nil, errors.Reason("%q cannot be overridden", spec).Err()
	}
	abs, err := filepath.Abs(localRoot)
	if err != nil {
		return nil, err
	}
	switch stat, err := os.Stat(abs); {
	case err != nil:
		return nil, err
	case !stat.IsDir():
		return nil, errors.Reason("%s is not a directory", localRoot).Err()
	}
	return &RepoOverride{
		Host:      repoKey.Host,
		Repo:      repoKey.Repo,
		Ref:       repoKey.Ref,
		LocalRoot: abs,
	}, nil
}

// EntryOnDisk loads the entry point based on a Starlark file on disk.
//
// It searches for the closest (up the tree) PACKAGE.star file to find the
// package the entry point the script belongs to. It then loads this package and
// all its dependencies (based on PACKAGE.lock file).
//
// If there's no PACKAGE.star, synthesizes a minimal package using the given
// file's directory as the package root. This is useful during the migration to
// packages.
//
// The given RepoManager will be used to load all remote (non-local) transitive
// dependencies. It is used only if this package has a remote (perhaps
// transitive) dependency. If nil, remote dependencies won't be supported.
//
// Local dependencies are always supported.
//
// Returns the loaded entry point with Local populated and the Lockfile
// generated during the traversal. This lockfile encodes all the same
// information as contained in the returned Entry and it can be used to
// reconstruct Entry later without redoing the traversal.
//
// The returned error may be backtracable.
func EntryOnDisk(ctx context.Context, path string, remotes RepoManager, overrides []*RepoOverride) (*Entry, *lockfilepb.Lockfile, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, nil, errors.Annotate(err, "taking absolute path of %q", path).Err()
	}

	statCache := unsyncStatCache()

	// Find PACKAGE.star indicating the root of the package.
	var found bool
	var pkgScript string
	scriptDir, main := filepath.Split(abs)
	pkgRoot, found, err := findRoot(scriptDir, PackageScript, "", statCache)
	if err != nil {
		return nil, nil, errors.Annotate(err, "searching for %s", PackageScript).Err()
	}
	if found {
		main, err = filepath.Rel(pkgRoot, abs)
		if err != nil {
			return nil, nil, errors.Annotate(err, "getting relative path from %s to %s", pkgRoot, abs).Err()
		}
		pkgScript = filepath.Join(pkgRoot, PackageScript)
	} else {
		// Fallback to the pre-PACKAGE.star behavior where entry point scripts were
		// assumed to be at the root of the main package.
		pkgRoot = scriptDir
	}

	// Drop trailing "/" from the root path.
	pkgRoot = filepath.Clean(pkgRoot)

	// Calculate path from the repo root to the package root. This is needed by
	// some lucicfg introspection functionality (these paths end up in
	// project.cfg) and when loading local dependencies (to verify they are indeed
	// local).
	repoRoot, _, err := findRoot(pkgRoot, "", "", statCache)
	if err != nil {
		return nil, nil, errors.Annotate(err, "could not determine the repository or volume root of %q", abs).Err()
	}
	rel, err := filepath.Rel(repoRoot, pkgRoot)
	if err != nil {
		return nil, nil, errors.Annotate(err, "calculating path of %q relative to %q", pkgRoot, repoRoot).Err()
	}
	script := filepath.ToSlash(main)

	// If no PACKAGE.star, return minimal package for compatibility with old code.
	if pkgScript == "" {
		code := interpreter.FileSystemLoader(pkgRoot)
		return &Entry{
			Main:     code,
			DepGraph: map[string][]string{LegacyPackageNamePlaceholder: nil},
			Package:  LegacyPackageNamePlaceholder,
			Path:     filepath.ToSlash(rel),
			Script:   script,
			Local: &Local{
				Code:       code,
				DiskPath:   pkgRoot,
				Definition: legacyDefinition(),
				Formatter:  legacyFormatter(pkgRoot),
			},
		}, legacyLockfile(script), nil
	}

	// Path to the package relative to the repository root.
	repoPath := filepath.ToSlash(rel)

	// Construct a trivial Repo implementation that just fetches files directly
	// from disk. Since we don't really know what this local repo represents (it
	// may not even exists as a remote repo, e.g. for new packages being
	// developed), use a special magical RepoKey for it. See also a comment for
	// PinnedVersion for more details on special status of root-local packages.
	localRepo := &LocalDiskRepo{
		Root:    repoRoot,
		Key:     RepoKey{Root: true},
		Version: PinnedVersion,
	}

	// Load the package definition from PACKAGE.star.
	pkgBody, err := os.ReadFile(pkgScript)
	if err != nil {
		return nil, nil, errors.Annotate(err, "reading %s", cwdRel(pkgScript)).Err()
	}
	validator, err := localRepo.LoaderValidator(ctx, PinnedVersion, repoPath)
	if err != nil {
		return nil, nil, errors.Annotate(err, "loading %s", cwdRel(pkgScript)).Err()
	}
	def, err := LoadDefinition(ctx, pkgBody, validator)
	if err != nil {
		return nil, nil, errors.Annotate(err, "loading %s", cwdRel(pkgScript)).Err()
	}

	// Verify the entry point is known.
	if !internal.GetTestingTweaks(ctx).SkipEntrypointCheck {
		if !slices.Contains(def.Entrypoints, script) {
			return nil, nil, errors.Reason(
				"%s is not declared as a pkg.entrypoint(...) in %s and "+
					"thus cannot be executed. Available entrypoints: %v",
				script, PackageScript, def.Entrypoints).Err()
		}
	}

	if remotes == nil {
		remotes = &ErroringRepoManager{
			Error: errors.Reason("remote dependencies are not supported in this context").Err(),
		}
	}

	// Inject local repositories acting as overrides. Note that discoverDeps(...)
	// is aware that some Repos are overrides and will slightly tweak how it does
	// version selection (to guarantee an overridden package is always "the most
	// recent" version).
	overriddenRepos := make([]Repo, len(overrides))
	for i, override := range overrides {
		overriddenRepos[i] = &LocalDiskRepo{
			Root: override.LocalRoot,
			Key: RepoKey{
				Host: override.Host,
				Repo: override.Repo,
				Ref:  override.Ref,
			},
			Version: OverriddenVersion,
		}
	}

	// Load transitive closure of all dependencies.
	deps, err := discoverDeps(ctx, &DepContext{
		Package: def.Name,
		Version: PinnedVersion,
		Repo:    localRepo,
		Path:    repoPath,
		RepoManager: &PreconfiguredRepoManager{
			Repos: append([]Repo{localRepo}, overriddenRepos...),
			Other: remotes,
		},
		Known: def,
	})
	if err != nil {
		return nil, nil, err
	}

	// Convert them to a form suitable for Entry.
	depsLoaders := make(map[string]interpreter.Loader, len(deps))
	constraints := []LucicfgVersionConstraint{
		{
			Min:     def.MinLucicfgVersion,
			Package: def.Name,
			Main:    true,
		},
	}
	depGraph := map[string][]string{
		def.Name: def.DirectDeps(),
	}
	for _, dep := range deps {
		name, ok := strings.CutPrefix(dep.Package, "@")
		if !ok {
			panic(fmt.Sprintf("unexpected package name %q", dep.Package))
		}
		depsLoaders[name] = dep.Code
		depGraph[dep.Package] = dep.DirectDeps
		constraints = append(constraints, LucicfgVersionConstraint{
			Min:     dep.Min,
			Package: dep.Package,
		})
	}

	lockfile := &lockfilepb.Lockfile{
		Packages: []*lockfilepb.Lockfile_Package{
			{
				Name:        def.Name,
				Deps:        def.DirectDeps(),
				Lucicfg:     def.MinLucicfgVersion.String(),
				Resources:   slices.Sorted(slices.Values(def.Resources)),
				Entrypoints: slices.Sorted(slices.Values(def.Entrypoints)),
			},
		},
	}
	for _, dep := range deps {
		lockfile.Packages = append(lockfile.Packages, &lockfilepb.Lockfile_Package{
			Name:      dep.Package,
			Source:    lockfilePkgSource(localRepo, repoPath, dep.Repo, dep.Path, dep.Version),
			Deps:      dep.DirectDeps,
			Lucicfg:   dep.Min.String(),
			Resources: dep.Resources,
		})
	}

	code, err := localRepo.Loader(ctx, PinnedVersion, repoPath, def.Name, def.ResourcesSet)
	if err != nil {
		return nil, nil, err
	}
	return &Entry{
		Main:                      code,
		Deps:                      depsLoaders,
		DepGraph:                  depGraph,
		Package:                   def.Name,
		Path:                      repoPath,
		Script:                    script,
		LucicfgVersionConstraints: constraints,
		Local: &Local{
			Code:       code,
			DiskPath:   pkgRoot,
			Definition: def,
			Formatter:  legacyCompatibleFormatter(pkgRoot, def.FmtRules),
		},
	}, lockfile, nil
}

// Local represents some local package without any of its dependencies.
//
// Information here is used for purely local operations (like formatting and
// linting) that don't need remote dependencies.
type Local struct {
	// Code contains the code of the package.
	Code interpreter.Loader
	// DiskPath is an absolute path to the package directory on disk.
	DiskPath string
	// Definition is the full package definition.
	Definition *Definition
	// Formatter defines how to format files in the package.
	Formatter buildifier.FormatterPolicy
}

// PackageOnDisk loads a local package from a directory on disk.
//
// This is similar to EntryOnDisk, except it doesn't fetch (or validate) any
// dependencies and doesn't require (nor validates) an entry point.
//
// This is used for local "fmt" and "lint" checks that don't care about external
// dependencies and do not actually run any Starlark code (but they still need
// options from PACKAGE.star and an interpreter.Loader to read file bodies).
//
// The given directory should contain PACKAGE.star. If it doesn't, this
// directory will be treated as a legacy package directory for compatibility
// with pre-PACKAGE.star code (some minimal package definition will be
// synthesized for it).
//
// The returned error may be backtracable.
func PackageOnDisk(ctx context.Context, dir string) (*Local, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.Annotate(err, "taking absolute path of %q", dir).Err()
	}

	pkgScript := filepath.Join(abs, PackageScript)
	switch body, err := os.ReadFile(pkgScript); {
	case err == nil:
		def, err := LoadDefinition(ctx, body, NoopLoaderValidator{})
		if err != nil {
			return nil, errors.Annotate(err, "loading %s", cwdRel(pkgScript)).Err()
		}
		code, err := (&LocalDiskRepo{
			Root:    abs,
			Key:     RepoKey{Root: true},
			Version: PinnedVersion,
		}).Loader(ctx, PinnedVersion, ".", def.Name, def.ResourcesSet)
		if err != nil {
			return nil, err
		}
		return &Local{
			Code:       code,
			DiskPath:   abs,
			Definition: def,
			Formatter:  legacyCompatibleFormatter(abs, def.FmtRules),
		}, nil

	case errors.Is(err, os.ErrNotExist):
		// Legacy mode.
		return &Local{
			Code:       interpreter.FileSystemLoader(abs),
			DiskPath:   abs,
			Definition: legacyDefinition(),
			Formatter:  legacyFormatter(abs),
		}, nil

	default:
		return nil, errors.Annotate(err, "reading %s", PackageScript).Err()
	}
}

// Sources is a sorted list of all *.star files in the main package.
//
// Paths are slash-separated and relative to the package root (i.e. ready to be
// passed to the interpreter.Loader).
func (l *Local) Sources() ([]string, error) {
	// Here we know l.DiskPath is a package root (perhaps of a legacy package).
	// By passing it as a stop dir to scanForRoots we make sure all "orphan" files
	// that don't otherwise belong to a (nested) package, will end up attributed
	// to l.DiskPath root (NOT the repository root, as would happen without
	// a stop directory). This is important for enumerating files in a legacy
	// package.
	res, err := scanForRoots([]string{l.DiskPath}, l.DiskPath)
	if err != nil {
		return nil, err
	}
	for _, root := range res {
		if root.Root == l.DiskPath {
			return root.RelFiles(), nil
		}
	}
	return nil, errors.Reason("unexpectedly found no *.star files in %q", l.DiskPath).Err()
}

// cwdRel converts the given path to be relative to the current working
// directory, if possible.
//
// Exclusively for error messages. Not very rigorous and gives up on errors.
func cwdRel(abs string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return abs
	}
	rel, err := filepath.Rel(cwd, abs)
	if err != nil {
		return abs
	}
	return rel
}

// legacyDefinition represents PACKAGE.star for legacy packages without it.
func legacyDefinition() *Definition {
	return &Definition{
		Name:      LegacyPackageNamePlaceholder,
		Resources: []string{"**/*"},
	}
}

// legacyLockfile is a lockfile for legacy packages without PACKAGE.star.
func legacyLockfile(entrypoint string) *lockfilepb.Lockfile {
	return &lockfilepb.Lockfile{
		Packages: []*lockfilepb.Lockfile_Package{
			{
				Name:        LegacyPackageNamePlaceholder,
				Entrypoints: []string{entrypoint},
				Resources:   []string{"**/*"},
			},
		},
	}
}

// lockfilePkgSource generates a lockfile source spec for a dependency.
//
// It takes:
//  1. Repo representing the main package (the one for which we generate
//     the lockfile) and a path to the main package within it.
//  2. Repo that holds the dependency with a path within it to this dependency.
//  3. The version of the dependency.
func lockfilePkgSource(mainRepo Repo, mainPath string, depRepo Repo, depPath string, version string) *lockfilepb.Lockfile_Package_Source {
	if mainRepo == depRepo {
		rel, err := internal.RelRepoPath(mainPath, depPath)
		if err != nil {
			panic(fmt.Sprintf("impossible in-repo paths %q and %q: %s", mainPath, depPath, err))
		}
		if !strings.HasPrefix(rel, "../") {
			rel = "./" + rel
		}
		return &lockfilepb.Lockfile_Package_Source{
			Path: rel,
		}
	}
	return &lockfilepb.Lockfile_Package_Source{
		Repo:     depRepo.RepoKey().Spec(),
		Revision: version, // this will be OverriddenVersion when using overrides
		Path:     depPath,
	}
}
