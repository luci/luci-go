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

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	. "github.com/smartystreets/goconvey/convey"
)

func TestImport(t *testing.T) {
	Convey("Test import", t, func() {
		ctx := context.Background()
		plats := Platforms{}

		Convey("symlink", func() {
			test1 := ImportTarget{Source: "//path/to/host1", Mode: fs.ModeSymlink}

			Convey("normal", func() {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"test1":     test1,
						"dir/test2": {Source: "//path/to/host2", Mode: fs.ModeSymlink, Version: "v2"},
					},
				}
				a, err := g.Generate(ctx, plats)
				So(err, ShouldBeNil)

				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				So(imports.Copy.Files, ShouldResemble, map[string]*core.ActionFilesCopy_Source{
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
				})
			})
			Convey("bat shim", func() {
				test1.GenerateBatShim = true
				g := ImportTargets{
					Targets: map[string]ImportTarget{"test1": test1},
				}
				a, err := g.Generate(ctx, plats)
				So(err, ShouldBeNil)

				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				So(imports.Copy.Files, ShouldResemble, map[string]*core.ActionFilesCopy_Source{
					"test1.bat": {
						Content: &core.ActionFilesCopy_Source_Raw{
							Raw: []byte("@" + filepath.FromSlash("//path/to/host1") + " %*"),
						},
						Mode: 0o666,
					},
				})
			})
			Convey("mingw", func() {
				test1.MinGWSymlink = true
				g := ImportTargets{
					Targets: map[string]ImportTarget{"test1": test1},
				}
				a, err := g.Generate(ctx, plats)
				So(err, ShouldBeNil)

				buf := bytes.NewBufferString("!<symlink>")
				enc := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder()
				_, err = transform.NewWriter(buf, enc).Write([]byte(filepath.FromSlash("//path/to/host1") + "\x00"))
				So(err, ShouldBeNil)

				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				So(imports.Copy.Files, ShouldResemble, map[string]*core.ActionFilesCopy_Source{
					"test1": {
						Content: &core.ActionFilesCopy_Source_Raw{
							Raw: buf.Bytes(),
						},
						Mode:     0o666,
						WinAttrs: 0x4,
					},
				})
			})
		})
		Convey("copy", func() {
			dir := t.TempDir()
			file := filepath.Join(dir, "file")
			f, err := os.Create(file)
			So(err, ShouldBeNil)
			_, err = f.WriteString("something")
			So(err, ShouldBeNil)
			err = f.Close()
			So(err, ShouldBeNil)

			Convey("ok", func() {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"dir":  {Source: dir, Mode: fs.ModeDir},
						"file": {Source: file},
					},
				}

				a, err := g.Generate(ctx, plats)
				So(err, ShouldBeNil)
				imports := testutils.Assert[*core.Action_Copy](t, a.Spec)
				So(imports.Copy.Files["dir"].Version, ShouldNotBeEmpty)
				So(fs.FileMode(imports.Copy.Files["dir"].Mode).IsDir(), ShouldBeTrue)
				So(imports.Copy.Files["file"].Version, ShouldNotBeEmpty)
				So(fs.FileMode(imports.Copy.Files["file"].Mode).IsRegular(), ShouldBeTrue)
			})
			Convey("source path independent", func() {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir, Mode: fs.ModeDir, Version: "123"},
					},
				}
				a, err := g.Generate(ctx, plats)
				So(err, ShouldBeNil)
				imports1 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				err = os.Rename(dir, dir+".else")
				So(err, ShouldBeNil)
				g = ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir + ".else", Mode: fs.ModeDir, Version: "123"},
					},
				}
				a, err = g.Generate(ctx, plats)
				So(err, ShouldBeNil)
				imports2 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				So(imports1.Copy.Files["dir"].Version, ShouldEqual, imports2.Copy.Files["dir"].Version)
			})
			Convey("source path dependent", func() {
				g := ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir, Mode: fs.ModeDir, Version: "123", SourcePathDependent: true},
					},
				}
				a, err := g.Generate(ctx, plats)
				So(err, ShouldBeNil)
				imports1 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				err = os.Rename(dir, dir+".else")
				So(err, ShouldBeNil)
				g = ImportTargets{
					Targets: map[string]ImportTarget{
						"dir": {Source: dir + ".else", Mode: fs.ModeDir, Version: "123", SourcePathDependent: true},
					},
				}
				a, err = g.Generate(ctx, plats)
				So(err, ShouldBeNil)
				imports2 := testutils.Assert[*core.Action_Copy](t, a.Spec)

				So(imports1.Copy.Files["dir"].Version, ShouldNotEqual, imports2.Copy.Files["dir"].Version)
			})
		})
	})
}
