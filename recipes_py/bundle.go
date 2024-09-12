// Copyright 2024 The LUCI Authors.
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

package recipespy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"text/template"

	"github.com/bmatcuk/doublestar"
	"github.com/go-git/go-git/v5/plumbing/format/gitattributes"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/filesystem"
)

// Bundle creates a hermetically runnable recipe bundle.
//
// This is done by packaging the repo and all dependency repos into a folder
// and then generating an entrypoint script with `-O` override flags to this
// folder.
// Optionally provides dep repos with local override paths to pick up the repo
// from local override instead of repos checked out to .recipe_deps directory
// during the recipe bootstrap process.
//
// The general principle is that the input to bundle is:
//   - a fully bootstrapped recipe repo. All dependency repos specified in
//     recipes.cfg are checked out to .recipe_deps directory under the recipe
//     root dir.
//   - files tagged with the `recipes` gitattribute value (see
//     `git help gitattributes`).
//
// and the output is a runnable folder at `dest` for the named repo.
//
// # Included files
//
// By default, bundle will include all recipes/ and recipe_modules/ files in
// your repo, plus the `recipes.cfg` file, and excluding all json expectation
// files. Recipe bundle also uses the standard `gitattributes` mechanism for
// tagging files within the repo, and will also include these files when
// generating the bundle. In particular, it looks for files tagged with the
// string `recipes`. As an example, you could put this in a `.gitattributes`
// file in your repo:
//
//	*.py       recipes
//	*_test.py -recipes
//
// That would include all .py files, but exclude all _test.py files. See the
// page `git help gitattributes` for more information on how gitattributes work.
//
// The recipe repo to bundle may or may not be a git repo. There is a slight
// difference when bundling a recipe repo that is a git repo that the bundling
// process leverages the git index, so any untracked file will NOT be in the
// final bundle.
func Bundle(ctx context.Context, repoPath string, dest string, overrides map[string]string) error {
	switch {
	case repoPath == "":
		return errors.New("recipes_py.Bundle: repoPath is required")
	case dest == "":
		return errors.New("recipes_py.Bundle: destination is required")
	}

	mainRepo, err := RepoFromPath(repoPath)
	if err != nil {
		return err
	}
	if err := mainRepo.ensureDeps(ctx); err != nil {
		return err
	}
	depRepos, err := calculateDepRepos(mainRepo, overrides)
	if err != nil {
		return err
	}

	if dest, err = prepareDestDir(ctx, dest); err != nil {
		return err
	}
	copyCh := make(chan copyItem, 100)
	eg, ectx := errgroup.WithContext(ctx)
	startCopyWorkers(ectx, copyCh, eg)
	var wg sync.WaitGroup
	for _, repo := range append([]*Repo{mainRepo}, depRepos...) {
		wg.Add(1)
		eg.Go(func() error {
			defer wg.Done()
			return exportRepo(ectx, repo, dest, copyCh)
		})
	}
	wg.Add(1)
	eg.Go(func() error {
		defer wg.Done()
		return exportProtos(ctx, mainRepo, dest, copyCh)
	})
	// Close the channel after sending all copy items to the channel
	wg.Wait()
	close(copyCh)
	// Now wait for completion of all copy workers.
	if err := eg.Wait(); err != nil {
		return err
	}
	return prepareScripts(mainRepo, depRepos, dest)
}

// calculateDepRepos creates the dep Repos from .recipe_deps directory or
// the overrides.
//
// Returns are sorted by the repo name.
// Returns error if the provided overridden dep is not the dep of the main
// repo.
func calculateDepRepos(main *Repo, overrides map[string]string) ([]*Repo, error) {
	unusedOverrides := stringset.New(len(overrides))
	for dep := range overrides {
		unusedOverrides.Add(dep)
	}
	depRepos := make([]*Repo, 0, len(main.Spec.GetDeps()))
	for dep := range main.Spec.GetDeps() {
		depRepoPath, overridden := overrides[dep]
		if !overridden {
			depRepoPath = filepath.Join(main.RecipeDepsPath(), dep)
		} else {
			unusedOverrides.Del(dep)
		}
		repo, err := RepoFromPath(depRepoPath)
		if err != nil {
			return nil, err
		}
		depRepos = append(depRepos, repo)
	}
	if unusedOverrides.Len() > 0 {
		return nil, fmt.Errorf("overrides %s provided but not used", strings.Join(unusedOverrides.ToSortedSlice(), ", "))
	}
	slices.SortFunc(depRepos, func(left, right *Repo) int {
		return strings.Compare(left.Name(), right.Name())
	})
	return depRepos, nil
}

// prepareDestDir creates the destination dir if it doesn't exists.
//
// It will error if
//   - dest exists but it is not a dir
//   - dest exists and it is a dir but it is not empty
//
// Returns the absolute path to the destination dir.
func prepareDestDir(ctx context.Context, dest string) (string, error) {
	logging.Infof(ctx, "preparing destination directory %q", dest)
	ret, err := filepath.Abs(dest)
	if err != nil {
		return "", fmt.Errorf("failed to convert dest path %q to absolute path: %w", dest, err)
	}

	switch empty, err := filesystem.IsEmptyDir(ret); {
	case errors.Is(err, fs.ErrNotExist):
		if err := os.MkdirAll(ret, fs.ModePerm); err != nil {
			return "", fmt.Errorf("failed to create dir %q: %w", ret, err)
		}
	case err != nil:
		return "", err
	case !empty:
		return "", fmt.Errorf("dest path %q exists but the directory is not empty", dest)
	}
	return ret, nil
}

// copyItem specifies the file to copy from and to.
type copyItem struct {
	src, dest string
}

// startCopyWorkers starts N goroutines that listens to the channels and perform
// copy operation once a copy item is received.
//
// N is equal to the number of cores.
func startCopyWorkers(ctx context.Context, copyCh <-chan copyItem, eg *errgroup.Group) {
	for range runtime.NumCPU() {
		eg.Go(func() error {
			for {
				select {
				case item, ok := <-copyCh:
					if !ok {
						return nil
					}
					info, err := os.Stat(item.src)
					if err != nil {
						return fmt.Errorf("failed to stat file %q", item.src)
					}
					if err := filesystem.Copy(item.dest, item.src, info.Mode().Perm()); err != nil {
						return fmt.Errorf("failed to copy the file %q to %q: %w", item.src, item.dest, err)
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		})
	}
}

// exportRepo sends all files to copy for the given repo to copyCh.
func exportRepo(ctx context.Context, repo *Repo, dest string, copyCh chan<- copyItem) error {
	bundleDest := filepath.Join(dest, repo.Name())
	logging.Infof(ctx, "bundling repo %q to %s", repo.Name(), bundleDest)
	createdDir := stringset.New(20)
	var ignorePatterns []gitignore.Pattern
	bi := mkBundleInclusion(repo)
	repoFS := os.DirFS(repo.Path)

	// Perform a filesystem walk from the root of the repo and do
	// 1. Constantly updating gitignores and gitattributes as we come across them.
	// 2. Skip walking the whole directory if the directory is ignored
	// 3. Skip bundling a file if the file is marked ignored in gitignore.
	// 4. Consult `BundleInclusion` on whether a file should be bundled
	// 5. If yes, prepare the directory in the bundle destination and send
	//    the files to bundle to the `copyCh`.
	return fs.WalkDir(repoFS, ".", func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case ctx.Err() != nil:
			return ctx.Err() // stop walking if the context is cancelled.
		}
		var pathSegs []string
		if p != "." {
			pathSegs = strings.Split(p, "/")
		}
		if d.IsDir() {
			switch ps, err := readIgnoreFile(repoFS, path.Join(p, ".gitignore"), pathSegs); {
			case err != nil:
				return err
			default:
				ignorePatterns = append(ignorePatterns, ps...)
			}
			switch attrs, err := readAttrFile(repoFS, path.Join(p, ".gitattributes"), pathSegs); {
			case err != nil:
				return err
			default:
				bi.gitAttrs = append(bi.gitAttrs, attrs...)
			}
		}
		switch ignored := gitignore.NewMatcher(ignorePatterns).Match(pathSegs, d.IsDir()); {
		case ignored && d.IsDir():
			return fs.SkipDir // not bundling anything in this dir
		case ignored:
			return nil
		case d.IsDir():
			return nil
		}

		switch shouldInclude, err := bi.shouldInclude(p); {
		case err != nil:
			return err
		case shouldInclude:
			logging.Debugf(ctx, "will bundle file %q in repo %q", p, repo.Name())
			fp := filepath.FromSlash(p)
			ci := copyItem{
				src:  filepath.Join(repo.Path, fp),
				dest: filepath.Join(bundleDest, fp),
			}
			if destDir := filepath.Dir(ci.dest); !createdDir.Has(destDir) {
				if err := os.MkdirAll(destDir, 0755); err != nil {
					return fmt.Errorf("failed to create dir: %w", err)
				}
				createdDir.Add(destDir)
			}
			select {
			case copyCh <- ci:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
}

func readIgnoreFile(fileSys fs.FS, file string, pathPrefix []string) ([]gitignore.Pattern, error) {
	f, err := fileSys.Open(file)
	switch {
	case errors.Is(err, fs.ErrNotExist): // okay
		return nil, nil
	case err != nil:
		return nil, err
	}
	defer f.Close()
	var ret []gitignore.Pattern
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if s := scanner.Text(); !strings.HasPrefix(s, "#") && len(strings.TrimSpace(s)) > 0 {
			ret = append(ret, gitignore.ParsePattern(s, pathPrefix))
		}
	}
	return ret, nil
}

const recipesAttrName = "recipes"

func readAttrFile(fileSys fs.FS, file string, pathPrefix []string) ([]gitattributes.MatchAttribute, error) {
	f, err := fileSys.Open(file)
	switch {
	case errors.Is(err, fs.ErrNotExist): // okay
		return nil, nil
	case err != nil:
		return nil, err
	}
	defer f.Close()
	gitAttrs, err := gitattributes.ReadAttributes(f, pathPrefix, true)
	if err != nil {
		return nil, err
	}
	// only keep gitattributes for recipe.
	var ret []gitattributes.MatchAttribute
	for _, ma := range gitAttrs {
		for _, attr := range ma.Attributes {
			if attr.Name() == recipesAttrName {
				ret = append(ret, ma)
			}
		}
	}
	return ret, nil
}

// exportProtos sends all the proto files to copy to copyCh.
func exportProtos(ctx context.Context, repo *Repo, dest string, copyCh chan<- copyItem) error {
	depsPath := repo.RecipeDepsPath()
	createdDir := stringset.New(20)
	return fs.WalkDir(os.DirFS(depsPath), path.Join("_pb3", "PB"), func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case ctx.Err() != nil:
			return ctx.Err() // stop walking if the context is cancelled.
		case d.IsDir():
		case path.Ext(p) == ".pyc":
		default:
			ci := copyItem{
				src:  filepath.Join(depsPath, filepath.FromSlash(p)),
				dest: filepath.Join(dest, filepath.FromSlash(p)),
			}
			if dir := filepath.Dir(ci.dest); !createdDir.Has(dir) {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create dir: %w", err)
				}
				createdDir.Add(dir)
			}
			select {
			case copyCh <- ci:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})
}

var shellScriptTemplate = template.Must(template.New("shell").
	Funcs(template.FuncMap{
		"toSlash": filepath.ToSlash,
	}).
	Parse(`#!/usr/bin/env bash
export PYTHONPATH=${BASH_SOURCE[0]%/*}/recipe_engine
exec vpython3 -u ${BASH_SOURCE[0]%/*}/recipe_engine/recipe_engine/main.py \
 --package ${BASH_SOURCE[0]%/*}/{{.MainRepo.Name}}/{{toSlash .MainRepo.CfgPathRel}} \
 --proto-override ${BASH_SOURCE[0]%/*}/_pb3 \{{range .DepRepos}}
 -O {{.Name}}=${BASH_SOURCE[0]%/*}/{{.Name}} \{{end}}
 {{.Command}}
`))

var batScriptTemplate = template.Must(template.New("bat").
	Funcs(template.FuncMap{
		"toBackwardSlash": func(path string) string {
			return strings.ReplaceAll(path, string(filepath.Separator), `\`)
		},
	}).
	Parse(`@echo off
set PYTHONPATH="%~dp0\recipe_engine"
call vpython3.bat -u "%~dp0\recipe_engine\recipe_engine\main.py" ^
 --package "%~dp0\{{.MainRepo.Name}}\{{toBackwardSlash .MainRepo.CfgPathRel}}" ^
 --proto-override "%~dp0\_pb3" ^{{range .DepRepos}}
 -O {{.Name}}=%~dp0\{{.Name}} ^{{end}}
 {{.Command}}
`))

// prepareScripts creates entrypoint scripts for various platforms.
func prepareScripts(main *Repo, deps []*Repo, dest string) error {
	type Script struct {
		Name     string
		tmpl     *template.Template
		Command  string
		MainRepo *Repo
		DepRepos []*Repo
		perm     fs.FileMode
	}
	for _, script := range []Script{
		{"recipes", shellScriptTemplate, `"$@"`, main, deps, 0744},
		{"recipes.bat", batScriptTemplate, `%*`, main, deps, 0644},
		{"luciexe", shellScriptTemplate, `luciexe "$@"`, main, deps, 0744},
		{"luciexe.bat", batScriptTemplate, `luciexe %*`, main, deps, 0644},
	} {
		f, err := os.OpenFile(filepath.Join(dest, script.Name), os.O_RDWR|os.O_CREATE, script.perm)
		if err != nil {
			return fmt.Errorf("failed to create script %q: %w", script.Name, err)
		}
		defer func() { _ = f.Close() }()
		if err := script.tmpl.Execute(f, script); err != nil {
			return fmt.Errorf("failed to write script %q: %w", script.Name, err)
		}
	}
	return nil
}

// bundleInclusion helps decide whether a file in recipe should be included in
// in the final bundle.
type bundleInclusion struct {
	gitAttrs []gitattributes.MatchAttribute
	cfgPath  string
	patterns []string
}

func mkBundleInclusion(repo *Repo) *bundleInclusion {
	recipeRootPath := repo.RecipesRootPathRel()
	return &bundleInclusion{
		cfgPath: filepath.ToSlash(repo.CfgPathRel()),
		patterns: []string{
			// all the recipes stuff
			path.Join(recipeRootPath, "recipes", "**", "*"),
			// all the recipe_modules stuff
			path.Join(recipeRootPath, "recipe_modules", "**", "*"),
			// all the protos in recipe_proto
			path.Join(recipeRootPath, "recipe_proto", "**", "*.proto"),
		},
	}
}

// p is expected to be slash path.
func (bi *bundleInclusion) shouldInclude(p string) (bool, error) {
	// exclude all the json expectations
	if path.Ext(p) == ".json" && strings.HasSuffix(path.Dir(p), ".expected") {
		return false, nil
	}
	if p == bi.cfgPath {
		return true, nil // always keep recipes.cfg
	}
	match, _ := gitattributes.NewMatcher(bi.gitAttrs).Match(strings.Split(p, "/"), []string{recipesAttrName})
	switch attr, ok := match[recipesAttrName]; {
	case !ok:
	case attr.IsSet():
		return true, nil
	case attr.IsUnset():
		return false, nil
	}

	for _, pat := range bi.patterns {
		switch matched, err := doublestar.Match(pat, p); {
		case err != nil:
			return false, err
		case matched:
			return true, nil
		}
	}
	return false, nil
}
