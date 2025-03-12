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
	"os"
	"path/filepath"
	"strings"
	"sync"

	"go.starlark.net/starlark"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/starlark/interpreter"
)

// diskPackageLoader returns a loader that can load files belonging to the
// package at the given root path (which contains PACKAGE.star) on the local
// disk.
//
// It is aware of possible nested packages (i.e. subdirectories of the root that
// have their own PACKAGE.star). Files inside such nested packages are
// invisible.
//
// TODO: Add resource file checks as well.
func diskPackageLoader(root string) interpreter.Loader {
	root, err := filepath.Abs(root)
	if err != nil {
		panic(err)
	}

	loader := &diskLoaderState{root: root}

	return func(_ context.Context, path string) (_ starlark.StringDict, src string, err error) {
		abs := filepath.Join(root, filepath.FromSlash(path))
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			return nil, "", errors.Annotate(err, "failed to calculate relative path").Err()
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, "", errors.New("outside the package root")
		}
		if err := loader.checkDirectoryVisible(filepath.Dir(rel)); err != nil {
			return nil, "", err
		}
		body, err := os.ReadFile(abs)
		if os.IsNotExist(err) {
			return nil, "", interpreter.ErrNoModule
		}
		return nil, string(body), err
	}
}

type diskLoaderState struct {
	root string // clean absolute path to the directory with PACKAGE.star

	m   sync.RWMutex
	vis map[string]bool // relative dir path => true if can load from it
}

func (l *diskLoaderState) checkDirectoryVisible(rel string) error {
	l.m.RLock()
	vis, found := l.vis[rel]
	l.m.RUnlock()
	if !found {
		l.m.Lock()
		defer l.m.Unlock()

		pkgRoot, found, err := findRoot(filepath.Join(l.root, rel), PackageScript)
		if err != nil {
			return err
		}
		if !found {
			// This should not normally be happening, since we know l.root is a
			// package root. It can theoretically happen if it was deleted on disk
			// after we started running lucicfg.
			return errors.Reason("path %s is not inside of any package", rel).Err()
		}
		vis = pkgRoot == l.root

		if l.vis == nil {
			l.vis = make(map[string]bool, 1)
		}
		l.vis[rel] = vis
	}

	if vis {
		return nil
	}
	return errors.Reason("directory %q belongs to a different (nested) package and files from it cannot be loaded directly", filepath.ToSlash(rel)).Err()
}
