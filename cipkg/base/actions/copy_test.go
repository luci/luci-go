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

	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/cipkg/internal/testutils"
)

func TestProcessCopy(t *testing.T) {
	ftt.Run("Test action processor for copy", t, func(t *ftt.Test) {
		ap := NewActionProcessor()
		pm := testutils.NewMockPackageManage("")

		t.Run("ok", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			checkReexecArg(t, pkg.Derivation.Args, copy)
			assert.Loosely(t, environ.New(pkg.Derivation.Env).Get("something"), should.Equal(pkg.BuildDependencies[0].Handler.OutputDirectory()))
		})

		// DerivationID shouldn't depend on path if version is set.
		t.Run("version", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

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
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, pkg1.Derivation.Args, should.NotMatch(pkg2.Derivation.Args))
			assert.Loosely(t, pkg1.DerivationID, should.Equal(pkg2.DerivationID))
		})
	})
}

//go:embed copy_test.go
var actionCopyTestEmbed embed.FS

func TestEmbedFSRegistration(t *testing.T) {
	ftt.Run("Test embedFS registration", t, func(t *ftt.Test) {
		e := newFilesCopyExecutor()

		t.Run("ok", func(t *ftt.Test) {
			assert.Loosely(t, func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, should.NotPanic)
			_, ok := e.LoadEmbed("ref1")
			assert.Loosely(t, ok, should.BeTrue)
			_, ok = e.LoadEmbed("ref2")
			assert.Loosely(t, ok, should.BeFalse)
		})
		t.Run("same ref", func(t *ftt.Test) {
			assert.Loosely(t, func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, should.NotPanic)
			assert.Loosely(t, func() { e.StoreEmbed("ref2", actionCopyTestEmbed) }, should.NotPanic)
			assert.Loosely(t, func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, should.Panic)
		})
		t.Run("sealed", func(t *ftt.Test) {
			assert.Loosely(t, func() { e.StoreEmbed("ref1", actionCopyTestEmbed) }, should.NotPanic)
			e.LoadEmbed("ref1")
			assert.Loosely(t, func() { e.StoreEmbed("ref2", actionCopyTestEmbed) }, should.Panic)
		})

		t.Run("global", func(t *ftt.Test) {
			RegisterEmbed("ref1", actionCopyTestEmbed)
			_, ok := defaultFilesCopyExecutor.LoadEmbed("ref1")
			assert.Loosely(t, ok, should.BeTrue)
		})
	})
}

func TestExecuteCopy(t *testing.T) {
	ftt.Run("Test execute action copy", t, func(t *ftt.Test) {
		ctx := context.Background()
		e := newFilesCopyExecutor()
		out := t.TempDir()

		t.Run("output", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			{
				l, err := os.Readlink(filepath.Join(out, "test/file"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, l, should.Equal(filepath.FromSlash("/abc/efg/something")))
			}
		})

		t.Run("local", func(t *ftt.Test) {
			src := t.TempDir()
			f, err := os.Create(filepath.Join(src, "file"))
			assert.Loosely(t, err, should.BeNil)
			_, err = f.WriteString("content")
			assert.Loosely(t, err, should.BeNil)
			err = f.Close()
			assert.Loosely(t, err, should.BeNil)

			t.Run("dir", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)

				{
					f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/dir/file")))
					assert.Loosely(t, err, should.BeNil)
					defer f.Close()
					b, err := io.ReadAll(f)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, string(b), should.Equal("content"))
				}
			})

			t.Run("file", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)

				{
					f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
					assert.Loosely(t, err, should.BeNil)
					defer f.Close()
					b, err := io.ReadAll(f)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, string(b), should.Equal("content"))
				}
			})

			t.Run("symlink", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)

				{
					l, err := os.Readlink(filepath.Join(out, "test/file"))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, l, should.Equal(filepath.FromSlash("something")))
				}
			})
		})

		t.Run("embed", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			{
				f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/files/copy_test.go")))
				assert.Loosely(t, err, should.BeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(b), should.ContainSubstring("Test copy ref embed"))
			}
			{
				f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
				assert.Loosely(t, err, should.BeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(b), should.ContainSubstring("Test copy ref embed"))
			}
		})

		t.Run("raw", func(t *ftt.Test) {
			t.Run("symlink", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, err.Error(), should.ContainSubstring("symlink is not supported"))
			})

			t.Run("dir", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)

				{
					info, err := os.Stat(filepath.Join(out, filepath.FromSlash("test/file")))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, info.IsDir(), should.BeTrue)
				}
			})

			t.Run("file", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)

				{
					f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
					assert.Loosely(t, err, should.BeNil)
					defer f.Close()
					b, err := io.ReadAll(f)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, string(b), should.Equal("something"))

					// Windows will ignore mode bits
					if runtime.GOOS != "windows" {
						info, err := f.Stat()
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, info.Mode().Perm(), should.Equal(0o777))
					}
				}
			})
		})

		t.Run("mode/attrs", func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)

			{
				f, err := os.Open(filepath.Join(out, filepath.FromSlash("test/file")))
				assert.Loosely(t, err, should.BeNil)
				defer f.Close()
				b, err := io.ReadAll(f)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(b), should.Equal("something"))

				info, err := f.Stat()
				assert.Loosely(t, err, should.BeNil)
				if runtime.GOOS == "windows" {
					// *syscall.Win32FileAttributeData
					assert.Loosely(t, reflect.ValueOf(info.Sys()).Elem().FieldByName("FileAttributes").Uint()&0x4, should.NotBeZero)
				} else {
					assert.Loosely(t, info.Mode().Perm(), should.Equal(0o777))
				}
			}
		})
	})
}

func TestReexecExecuteCopy(t *testing.T) {
	ftt.Run("Test re-execute action copy", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		runWithDrv(t, ctx, pkg.Derivation, out)

		{
			f, err := os.Open(filepath.Join(out, "test", "file"))
			assert.Loosely(t, err, should.BeNil)
			defer f.Close()
			b, err := io.ReadAll(f)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(b), should.Equal("something"))
			// Windows will ignore mode bits
			if runtime.GOOS != "windows" {
				info, err := f.Stat()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, info.Mode().Perm(), should.Equal(0o777))
			}
		}
	})
}
