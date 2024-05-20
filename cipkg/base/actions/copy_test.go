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

package actions

import (
	"context"
	"embed"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
	"go.chromium.org/luci/common/system/environ"

	. "github.com/smartystreets/goconvey/convey"
)

func TestProcessCopy(t *testing.T) {
	Convey("Test action processor for copy", t, func() {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		Convey("ok", func() {
			copy := &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					"test/file": {
						Content: &core.ActionFilesCopy_Source_Local_{
							Local: &core.ActionFilesCopy_Source_Local{Path: "something"},
						},
						Mode: uint32(fs.ModeSymlink),
					},
				},
			}

			pkg, err := ap.Process("", pm, &core.Action{
				Name: "copy",
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Copy{Copy: copy},
			})
			So(err, ShouldBeNil)

			checkReexecArg(pkg.Derivation.Args, copy)
			So(environ.New(pkg.Derivation.Env).Get("something"), ShouldEqual, pkg.BuildDependencies[0].Handler.OutputDirectory())
		})

		// DerivationID shouldn't depend on path if version is set.
		Convey("version", func() {
			copy1 := &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					"test/file": {
						Content: &core.ActionFilesCopy_Source_Local_{
							Local: &core.ActionFilesCopy_Source_Local{Path: "something"},
						},
						Mode:    uint32(fs.ModeDir),
						Version: "abc",
					},
				},
			}

			pkg1, err := ap.Process("", pm, &core.Action{
				Name: "copy",
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Copy{Copy: copy1},
			})
			So(err, ShouldBeNil)

			copy2 := &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					"test/file": {
						Content: &core.ActionFilesCopy_Source_Local_{
							Local: &core.ActionFilesCopy_Source_Local{Path: "somethingelse"},
						},
						Mode:    uint32(fs.ModeDir),
						Version: "abc",
					},
				},
			}

			pkg2, err := ap.Process("", pm, &core.Action{
				Name: "copy",
				Deps: []*core.Action{
					{Name: "something", Spec: &core.Action_Command{Command: &core.ActionCommand{}}},
				},
				Spec: &core.Action_Copy{Copy: copy2},
			})
			So(err, ShouldBeNil)

			So(pkg1.Derivation.Args, ShouldNotEqual, pkg2.Derivation.Args)
			So(pkg1.DerivationID, ShouldEqual, pkg2.DerivationID)
		})
	})
}

//go:embed copy_test.go
var actionCopyTestEmbed embed.FS

func TestEmbedFSRegistration(t *testing.T) {
	Convey("Test embedFS registration", t, func() {
		e := newFilesCopyExecutor()

		Convey("ok", func() {
			So(func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, ShouldNotPanic)
			_, ok := e.LoadEmbed("ref1")
			So(ok, ShouldBeTrue)
			_, ok = e.LoadEmbed("ref2")
			So(ok, ShouldBeFalse)
		})
		Convey("same ref", func() {
			So(func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, ShouldNotPanic)
			So(func() { e.StoreEmbed("ref2", actionCopyTestEmbed) }, ShouldNotPanic)
			So(func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, ShouldPanic)
		})
		Convey("sealed", func() {
			So(func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, ShouldNotPanic)
			e.LoadEmbed("ref1")
			So(func() { e.StoreEmbed("ref2", actionCopyTestEmbed) }, ShouldPanic)
		})

		Convey("global", func() {
			RegisterEmbed("ref1", actionCopyTestEmbed)
			_, ok := defaultFilesCopyExecutor.LoadEmbed("ref1")
			So(ok, ShouldBeTrue)
		})
	})
}

func TestExecuteCopy(t *testing.T) {
	Convey("Test execute action copy", t, func() {
		ctx := context.Background()
		e := newFilesCopyExecutor()
		out := t.TempDir()

		Convey("output", func() {
			ctx = environ.New([]string{"somedrv=/abc/efg"}).SetInCtx(ctx)
			a := &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					filepath.FromSlash("test/file"): {
						Content: &core.ActionFilesCopy_Source_Output_{
							Output: &core.ActionFilesCopy_Source_Output{Name: "somedrv", Path: "something"},
						},
						Mode: uint32(fs.ModeSymlink),
					},
				},
			}

			err := e.Execute(ctx, a, out)
			So(err, ShouldBeNil)

			{
				l, err := os.Readlink(filepath.Join(out, "test/file"))
				So(err, ShouldBeNil)
				So(l, ShouldEqual, filepath.FromSlash("/abc/efg/something"))
			}
		})

		Convey("local", func() {
			src := t.TempDir()
			f, err := os.Create(filepath.Join(src, "file"))
			So(err, ShouldBeNil)
			_, err = f.WriteString("content")
			So(err, ShouldBeNil)
			err = f.Close()
			So(err, ShouldBeNil)

			Convey("dir", func() {
				a := &core.ActionFilesCopy{
					Files: map[string]*core.ActionFilesCopy_Source{
						filepath.FromSlash("test/dir"): {
							Content: &core.ActionFilesCopy_Source_Local_{
								Local: &core.ActionFilesCopy_Source_Local{Path: src},
							},
							Mode: uint32(fs.ModeDir),
						},
					},
				}

				err := e.Execute(ctx, a, out)
				So(err, ShouldBeNil)

				{
					f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/dir/file")))
					So(err, ShouldBeNil)
					defer f.Close()
					b, err := io.ReadAll(f)
					So(err, ShouldBeNil)
					So(string(b), ShouldEqual, "content")
				}
			})

			Convey("file", func() {
				a := &core.ActionFilesCopy{
					Files: map[string]*core.ActionFilesCopy_Source{
						filepath.FromSlash("test/file"): {
							Content: &core.ActionFilesCopy_Source_Local_{
								Local: &core.ActionFilesCopy_Source_Local{Path: filepath.Join(src, "file")},
							},
							Mode: uint32(0o666),
						},
					},
				}

				err := e.Execute(ctx, a, out)
				So(err, ShouldBeNil)

				{
					f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
					So(err, ShouldBeNil)
					defer f.Close()
					b, err := io.ReadAll(f)
					So(err, ShouldBeNil)
					So(string(b), ShouldEqual, "content")
				}
			})

			Convey("symlink", func() {
				a := &core.ActionFilesCopy{
					Files: map[string]*core.ActionFilesCopy_Source{
						filepath.FromSlash("test/file"): {
							Content: &core.ActionFilesCopy_Source_Local_{
								Local: &core.ActionFilesCopy_Source_Local{Path: "something"},
							},
							Mode: uint32(fs.ModeSymlink),
						},
					},
				}

				err := e.Execute(ctx, a, out)
				So(err, ShouldBeNil)

				{
					l, err := os.Readlink(filepath.Join(out, "test/file"))
					So(err, ShouldBeNil)
					So(l, ShouldEqual, filepath.FromSlash("something"))
				}
			})
		})

		Convey("embed", func() {
			e.StoreEmbed("something", actionCopyTestEmbed)

			a := &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					filepath.FromSlash("test/files"): {
						Content: &core.ActionFilesCopy_Source_Embed_{
							Embed: &core.ActionFilesCopy_Source_Embed{Ref: "something"},
						},
						Mode: uint32(fs.ModeDir),
					},
					filepath.FromSlash("test/file"): {
						Content: &core.ActionFilesCopy_Source_Embed_{
							Embed: &core.ActionFilesCopy_Source_Embed{Ref: "something", Path: "copy_test.go"},
						},
						Mode: 0o666,
					},
				},
			}

			err := e.Execute(ctx, a, out)
			So(err, ShouldBeNil)

			{
				f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/files/copy_test.go")))
				So(err, ShouldBeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				So(err, ShouldBeNil)
				So(string(b), ShouldContainSubstring, "Test copy ref embed")
			}
			{
				f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
				So(err, ShouldBeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				So(err, ShouldBeNil)
				So(string(b), ShouldContainSubstring, "Test copy ref embed")
			}
		})

		Convey("raw", func() {
			Convey("symlink", func() {
				a := &core.ActionFilesCopy{
					Files: map[string]*core.ActionFilesCopy_Source{
						filepath.FromSlash("test/file"): {
							Content: &core.ActionFilesCopy_Source_Raw{Raw: []byte("something")},
							Mode:    uint32(os.ModeSymlink),
						},
					},
				}

				e := newFilesCopyExecutor()
				err := e.Execute(ctx, a, out)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "symlink is not supported")
			})

			Convey("dir", func() {
				a := &core.ActionFilesCopy{
					Files: map[string]*core.ActionFilesCopy_Source{
						filepath.FromSlash("test/file"): {
							Content: &core.ActionFilesCopy_Source_Raw{},
							Mode:    uint32(fs.ModeDir),
						},
					},
				}

				e := newFilesCopyExecutor()
				err := e.Execute(ctx, a, out)
				So(err, ShouldBeNil)

				{
					info, err := os.Stat(filepath.Join(out, filepath.FromSlash("test/file")))
					So(err, ShouldBeNil)
					So(info.IsDir(), ShouldBeTrue)
				}
			})

			Convey("file", func() {
				a := &core.ActionFilesCopy{
					Files: map[string]*core.ActionFilesCopy_Source{
						filepath.FromSlash("test/file"): {
							Content: &core.ActionFilesCopy_Source_Raw{Raw: []byte("something")},
							Mode:    0o777,
						},
					},
				}

				e := newFilesCopyExecutor()
				err := e.Execute(ctx, a, out)
				So(err, ShouldBeNil)

				{
					f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
					So(err, ShouldBeNil)
					defer f.Close()
					b, err := io.ReadAll(f)
					So(err, ShouldBeNil)
					So(string(b), ShouldEqual, "something")

					// Windows will ignore mode bits
					if runtime.GOOS != "windows" {
						info, err := f.Stat()
						So(err, ShouldBeNil)
						So(info.Mode().Perm(), ShouldEqual, 0o777)
					}
				}
			})
		})

		Convey("mode/attrs", func() {
			a := &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					filepath.FromSlash("test/file"): {
						Content:  &core.ActionFilesCopy_Source_Raw{Raw: []byte("something")},
						Mode:     0o777,
						WinAttrs: 0x4, // FILE_ATTRIBUTE_SYSTEM
					},
				},
			}

			e := newFilesCopyExecutor()
			err := e.Execute(ctx, a, out)
			So(err, ShouldBeNil)

			{
				f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
				So(err, ShouldBeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				So(err, ShouldBeNil)
				So(string(b), ShouldEqual, "something")

				info, err := f.Stat()
				So(err, ShouldBeNil)
				if runtime.GOOS == "windows" {
					// *syscall.Win32FileAttributeData
					So(reflect.ValueOf(info.Sys()).Elem().FieldByName("FileAttributes").Uint()&0x4, ShouldNotBeZeroValue)
				} else {
					So(info.Mode().Perm(), ShouldEqual, 0o777)
				}
			}
		})
	})
}

func TestReexecExecuteCopy(t *testing.T) {
	Convey("Test re-execute action copy", t, func() {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")
		ctx := context.Background()
		out := t.TempDir()

		pkg, err := ap.Process("", pm, &core.Action{
			Name: "copy",
			Spec: &core.Action_Copy{Copy: &core.ActionFilesCopy{
				Files: map[string]*core.ActionFilesCopy_Source{
					filepath.FromSlash("test/file"): {
						Content: &core.ActionFilesCopy_Source_Raw{Raw: []byte("something")},
						Mode:    0o777,
					},
				},
			}},
		})
		So(err, ShouldBeNil)

		runWithDrv(ctx, pkg.Derivation, out)

		{
			f, err := os.Open(filepath.Join(out, "test", "file"))
			So(err, ShouldBeNil)
			defer f.Close()
			b, err := io.ReadAll(f)
			So(err, ShouldBeNil)
			So(string(b), ShouldEqual, "something")
			// Windows will ignore mode bits
			if runtime.GOOS != "windows" {
				info, err := f.Stat()
				So(err, ShouldBeNil)
				So(info.Mode().Perm(), ShouldEqual, 0o777)
			}
		}
	})
}
