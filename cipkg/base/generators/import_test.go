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
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
)

func TestImport(t *testing.T) {
	ftt.Run("Test import", t, func(t *ftt.Test) {
		ctx := context.Background()
		plats := Platforms{}

		t.Run("symlink", func(t *ftt.Test) {
			test1 := ImportTarget{Source: "//path/to/host1", Mode: fs.ModeSymlink}

			t.Run("normal", func(t *ftt.Test) {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"test1":     test1,
						"dir/test2": {Source: "//path/to/host2", Mode: fs.ModeSymlink, Version: "v2"},
					},
				}
				a, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)

				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				assert.Loosely(t, imports.Copy.Files, should.Match(map[string]*core.ActionFilesCopy_Source{
					"test1": {
						Content: &core.ActionFilesCopy_Source_Local_{
							Local: &core.ActionFilesCopy_Source_Local{Path: filepath.FromSlash("//path/to/host1")},
						},
						Mode: uint32(fs.ModeSymlink),
					},
					filepath.FromSlash("dir/test2"): {
						Content: &core.ActionFilesCopy_Source_Local_{
							Local: &core.ActionFilesCopy_Source_Local{Path: filepath.FromSlash("//path/to/host2")},
						},
						Mode:    uint32(fs.ModeSymlink),
						Version: "v2",
					},
					filepath.FromSlash("build-support/base_import.stamp"): {
						Content: &core.ActionFilesCopy_Source_Raw{},
						Mode:    0o666,
					},
				}))
			})
			t.Run("bat shim", func(t *ftt.Test) {
				test1.GenerateBatShim = true
				g := ImportTargets{
					Targets: map[string]ImportTarget{"test1": test1},
				}
				a, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)

				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				assert.Loosely(t, imports.Copy.Files, should.Match(map[string]*core.ActionFilesCopy_Source{
					"test1.bat": {
						Content: &core.ActionFilesCopy_Source_Raw{
							Raw: []byte("@" + filepath.FromSlash("//path/to/host1") + " %*"),
						},
						Mode: 0o666,
					},
				}))
			})
			t.Run("mingw", func(t *ftt.Test) {
				test1.MinGWSymlink = true
				g := ImportTargets{
					Targets: map[string]ImportTarget{"test1": test1},
				}
				a, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)

				buf := bytes.NewBufferString("!<symlink>")
				enc := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder()
				_, err = transform.NewWriter(buf, enc).Write([]byte(filepath.FromSlash("//path/to/host1") + "\x00"))
				assert.Loosely(t, err, should.BeNil)

				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				assert.Loosely(t, imports.Copy.Files, should.Match(map[string]*core.ActionFilesCopy_Source{
					"test1": {
						Content: &core.ActionFilesCopy_Source_Raw{
							Raw: buf.Bytes(),
						},
						Mode:     0o666,
						WinAttrs: 0x4,
					},
				}))
			})
		})
		t.Run("copy", func(t *ftt.Test) {
			dir := t.TempDir()
			file := filepath.Join(dir, "file")
			f, err := os.Create(file)
			assert.Loosely(t, err, should.BeNil)
			_, err = f.WriteString("something")
			assert.Loosely(t, err, should.BeNil)
			err = f.Close()
			assert.Loosely(t, err, should.BeNil)

			t.Run("ok", func(t *ftt.Test) {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"dir":  {Source: dir, Mode: fs.ModeDir},
						"file": {Source: file},
					},
				}

				a, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)
				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				assert.Loosely(t, imports.Copy.Files["dir"].Version, should.NotBeEmpty)
				assert.Loosely(t, fs.FileMode(imports.Copy.Files["dir"].Mode).IsDir(), should.BeTrue)
				assert.Loosely(t, imports.Copy.Files["file"].Version, should.NotBeEmpty)
				assert.Loosely(t, fs.FileMode(imports.Copy.Files["file"].Mode).IsRegular(), should.BeTrue)
			})
			t.Run("source path independent", func(t *ftt.Test) {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir, Mode: fs.ModeDir, Version: "123"},
					},
				}
				a, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)
				imports1 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				err = os.Rename(dir, dir+".else")
				assert.Loosely(t, err, should.BeNil)
				g = ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir + ".else", Mode: fs.ModeDir, Version: "123"},
					},
				}
				a, err = g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)
				imports2 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				assert.Loosely(t, imports1.Copy.Files["dir"].Version, should.Equal(imports2.Copy.Files["dir"].Version))
			})
			t.Run("source path dependent", func(t *ftt.Test) {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir, Mode: fs.ModeDir, Version: "123", SourcePathDependent: true},
					},
				}
				a, err := g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)
				imports1 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				err = os.Rename(dir, dir+".else")
				assert.Loosely(t, err, should.BeNil)
				g = ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir + ".else", Mode: fs.ModeDir, Version: "123", SourcePathDependent: true},
					},
				}
				a, err = g.Generate(ctx, plats)
				assert.Loosely(t, err, should.BeNil)
				imports2 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				assert.Loosely(t, imports1.Copy.Files["dir"].Version, should.NotEqual(imports2.Copy.Files["dir"].Version))
			})
		})
	})
}
