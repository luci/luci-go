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
	"embed"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
)

type EmbeddedFiles struct {
	name string

	ref string
	dir string
	efs embed.FS

	// modeOverride lets you customize file permissions within embed.FS, which
	// otherwise always set to 0o444.
	modeOverride func(name string) (fs.FileMode, error)
}

// InitEmbeddedFS registers the embed.FS to copy executor and returns the
// corresponding generator.
// It should only be used in init() or on global variables e.g.
// //go:embed something
// var somethingEmbed embed.FS
// var somethingGen = InitEmbeddedFS("derivationName", somethingEmbed)
func InitEmbeddedFS(name string, e embed.FS) *EmbeddedFiles {
	h := crypto.SHA256.New()
	if err := getHashFromFS(e, h); err != nil {
		// Embedded files are frozen after build. Panic since this is more like a
		// build failure.
		panic(err)
	}
	ref := fmt.Sprintf("%x", h.Sum([]byte(name)))
	actions.RegisterEmbed(ref, e)
	return &EmbeddedFiles{
		name: name,

		ref: ref,
		dir: ".",
		efs: e,
	}
}

func (e *EmbeddedFiles) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	efs, err := fs.Sub(e.efs, e.dir)
	if err != nil {
		return nil, err
	}

	files := make(map[string]*core.ActionFilesCopy_Source)
	if err := fs.WalkDir(efs, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if name == "." {
			return fmt.Errorf("root dir can't be file")
		}

		mode := fs.FileMode(0o444)
		if e.modeOverride != nil {
			if mode, err = e.modeOverride(name); err != nil {
				return err
			}
		}

		files[filepath.FromSlash(name)] = &core.ActionFilesCopy_Source{
			Content: &core.ActionFilesCopy_Source_Embed_{
				Embed: &core.ActionFilesCopy_Source_Embed{Ref: e.ref, Path: path.Join(e.dir, name)},
			},
			Mode: uint32(mode),
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return &core.Action{
		Name: e.name,
		Spec: &core.Action_Copy{
			Copy: &core.ActionFilesCopy{Files: files},
		},
	}, nil
}

// SubDir returns a generator copies files in the sub directory of the source.
func (e *EmbeddedFiles) SubDir(dir string) *EmbeddedFiles {
	ret := *e
	ret.dir = path.Join(e.dir, dir)
	return &ret
}

// WithModeOverride overrides file modes while copying.
func (e *EmbeddedFiles) WithModeOverride(f func(name string) (fs.FileMode, error)) *EmbeddedFiles {
	ret := *e
	ret.modeOverride = f
	return &ret
}
