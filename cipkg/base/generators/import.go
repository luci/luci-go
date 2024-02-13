// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"context"
	"crypto"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"go.chromium.org/luci/cipkg/core"
)

type ImportTarget struct {
	Source  string
	Version string
	Mode    fs.FileMode

	FollowSymlinks bool

	// Tf true, the import target will be considered different if source path
	// changed. Otherwise only Version will be take into account.
	SourcePathDependent bool
}

// ImportTargets is used to import file/directory from host environment. The
// builder itself won't detect the change of the imported file/directory. A
// version string should be generated to indicate the change if it matters.
// By default, target will be symlinked. When Mode in target is set to anything
// other than symlink, a hash version will be generated if there is no version
// provided.
type ImportTargets struct {
	Name     string
	Metadata *core.Action_Metadata
	Targets  map[string]ImportTarget
}

func (i *ImportTargets) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	files := make(map[string]*core.ActionFilesCopy_Source)
	for k, v := range i.Targets {
		src := filepath.FromSlash(v.Source)
		dst := filepath.FromSlash(k)
		if !filepath.IsAbs(src) {
			return nil, fmt.Errorf("import target source must be absolute path: %s", src)
		}

		m := getMode(v)

		// Always generate a version if target is not a symlink and no version is
		// provided. Otherwise we won't be able to track the change.
		ver := v.Version
		if m.Type() != fs.ModeSymlink && ver == "" {
			h := crypto.SHA256.New()
			switch m.Type() {
			case fs.ModeDir:
				if err := getHashFromFS(os.DirFS(src), h); err != nil {
					return nil, fmt.Errorf("failed to generate hash from src: %s: %w", src, err)
				}
			default: // Regular File
				f, err := os.Open(src)
				if err != nil {
					return nil, fmt.Errorf("failed to open src: %s: %w", src, err)
				}
				defer f.Close()
				if err := getHashFromFile(src, f, h); err != nil {
					return nil, fmt.Errorf("failed to generate hash from src: %s: %w", src, err)
				}
			}
			ver = fmt.Sprintf("%x", h.Sum(nil))
		}
		// Append source path to version so the version in action changed if source
		// path changed.
		if ver != "" && v.SourcePathDependent {
			ver = fmt.Sprintf("%s:%s", ver, v.Source)
		}

		// By default, create a symlink for the target.
		files[dst] = &core.ActionFilesCopy_Source{
			Content: &core.ActionFilesCopy_Source_Local_{
				Local: &core.ActionFilesCopy_Source_Local{Path: src, Version: ver, FollowSymlinks: v.FollowSymlinks},
			},
			Mode: uint32(m),
		}
	}

	// If any file is symlink, mark the output as imported to help e.g. docker
	// avoid using its content.
	for _, f := range files {
		if fs.FileMode(f.Mode).Type() == fs.ModeSymlink {
			files[filepath.Join("build-support", "base_import.stamp")] = &core.ActionFilesCopy_Source{
				Content: &core.ActionFilesCopy_Source_Raw{},
				Mode:    0o666,
			}
			break
		}
	}

	return &core.Action{
		Name:     i.Name,
		Metadata: i.Metadata,
		Spec: &core.Action_Copy{
			Copy: &core.ActionFilesCopy{
				Files: files,
			},
		},
	}, nil
}

// 1. If any permission bit set, return mode as it is.
// 2. If mode is empty, use ModeSymlink by default.
// 3. Use 0o777 as default permission for directories.
// 4. Use 0o666 as default permission for file.
func getMode(i ImportTarget) fs.FileMode {
	if i.Mode.Perm() != 0 || i.Mode.Type() == fs.ModeSymlink {
		return i.Mode
	}

	m := i.Mode
	if mt := i.Mode.Type(); mt.IsDir() {
		m |= 0o777
	} else if mt.IsRegular() {
		m |= 0o666
	}

	return m
}

var importFromPathMap = make(map[string]struct {
	target ImportTarget
	err    error
})

// FindBinaryFunc returns a slash separated path for the provided binary name.
type FindBinaryFunc func(bin string) (path string, err error)

// LookPath looks up file in the PATH and returns a slash separated path if
// the file exists.
func lookPath(file string) (string, error) {
	p, err := exec.LookPath(file)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(p), err
}

// FromPathBatch is a wrapper for builtins.Import generator. It finds binaries
// using finder func and caches the result based on the name. if finder is nil,
// binaries will be searched from the PATH environment.
func FromPathBatch(name string, finder FindBinaryFunc, bins ...string) (*ImportTargets, error) {
	if finder == nil {
		finder = lookPath
	}

	i := &ImportTargets{
		Name:    name,
		Targets: make(map[string]ImportTarget),
	}
	for _, bin := range bins {
		ret, ok := importFromPathMap[bin]
		if !ok {
			ret.target, ret.err = func() (ImportTarget, error) {
				path, err := finder(bin)
				if err != nil {
					return ImportTarget{}, fmt.Errorf("failed to find binary: %s: %w", bin, err)
				}
				return ImportTarget{
					Source: path,
					Mode:   fs.ModeSymlink,
				}, nil
			}()

			importFromPathMap[bin] = ret
		}

		if ret.err != nil {
			return nil, ret.err
		}
		i.Targets[path.Join("bin", path.Base(ret.target.Source))] = ret.target
	}
	return i, nil
}
