// Copyright 2014 The LUCI Authors.
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

package deployer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"

	. "go.chromium.org/luci/cipd/common"
)

func mkSymlink(t testing.TB, target string, link string) {
	err := os.Symlink(target, link)
	assert.Loosely(t, err, should.BeNil)
}

func withMemLogger() context.Context {
	return memlogger.Use(context.Background())
}

func loggerWarnings(ctx context.Context) (out []string) {
	for _, m := range logging.Get(ctx).(*memlogger.MemLogger).Messages() {
		if m.Level >= logging.Warning {
			out = append(out, m.Msg)
		}
	}
	return
}

func TestUtilities(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		// Wrappers that accept paths relative to tempDir.
		touch := func(rel string) {
			abs := filepath.Join(tempDir, filepath.FromSlash(rel))
			err := os.MkdirAll(filepath.Dir(abs), 0777)
			assert.Loosely(t, err, should.BeNil)
			f, err := os.Create(abs)
			assert.Loosely(t, err, should.BeNil)
			f.Close()
		}
		ensureLink := func(symlinkRel string, target string) {
			err := os.Symlink(target, filepath.Join(tempDir, symlinkRel))
			assert.Loosely(t, err, should.BeNil)
		}

		t.Run("scanPackageDir works with empty dir", func(t *ftt.Test) {
			err := os.Mkdir(filepath.Join(tempDir, "dir"), 0777)
			assert.Loosely(t, err, should.BeNil)
			files, err := scanPackageDir(ctx, filepath.Join(tempDir, "dir"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(files), should.BeZero)
		})

		t.Run("scanPackageDir works", func(t *ftt.Test) {
			touch("unrelated/1")
			touch("dir/a/1")
			touch("dir/a/2")
			touch("dir/b/1")
			touch("dir/.cipdpkg/abc")
			touch("dir/.cipd/abc")

			runScanPackageDir := func() sort.StringSlice {
				files, err := scanPackageDir(ctx, filepath.Join(tempDir, "dir"))
				assert.Loosely(t, err, should.BeNil)
				names := sort.StringSlice{}
				for _, f := range files {
					names = append(names, f.Name)
				}
				names.Sort()
				return names
			}

			// Symlinks doesn't work on Windows, test them only on Posix.
			if runtime.GOOS == "windows" {
				t.Run("works on Windows", func(t *ftt.Test) {
					assert.Loosely(t, runScanPackageDir(), should.Resemble(sort.StringSlice{
						"a/1",
						"a/2",
						"b/1",
					}))
				})
			} else {
				t.Run("works on Posix", func(t *ftt.Test) {
					ensureLink("dir/a/sym_link", "target")
					assert.Loosely(t, runScanPackageDir(), should.Resemble(sort.StringSlice{
						"a/1",
						"a/2",
						"a/sym_link",
						"b/1",
					}))
				})
			}
		})
	})
}

func TestDeployInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("Try to deploy package instance with bad package name", func(t *ftt.Test) {
			_, err := New(tempDir).DeployInstance(
				ctx, "", makeTestInstance("../test/package", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.ErrLike("invalid package name"))
		})

		t.Run("Try to deploy package instance with bad instance ID", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			inst.instanceID = "../000000000"
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.ErrLike("not a valid package instance ID"))
		})

		t.Run("Try to deploy package instance in bad subdir", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			inst.instanceID = "../000000000"
			_, err := New(tempDir).DeployInstance(ctx, "/abspath", inst, "", 0)
			assert.Loosely(t, err, should.ErrLike("bad subdir"))
		})
	})
}

func TestDeployInstanceSymlinkMode(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no symlinks")
	}

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("DeployInstance new empty package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeSymlink)
			info, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info, should.Resemble(inst.Pin()))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
			}))
			fInfo, err := os.Stat(filepath.Join(tempDir, ".cipd", "pkgs", "0"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fInfo.Mode(), should.Equal(os.FileMode(0755)|os.ModeDir))

			t.Run("in subdir", func(t *ftt.Test) {
				info, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, info, should.Resemble(inst.Pin()))
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"subdir!",
				}))
			})
		})

		t.Run("DeployInstance new non-empty package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
				fs.NewTestFile(".cipd/pkg/0/description.json", "{}", fs.TestFileOpts{}), // should be ignored
			}, pkg.InstallModeSymlink)
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable:../.cipd/pkgs/0/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
				"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
			}))
			// Ensure symlinks are actually traversable.
			body, err := os.ReadFile(filepath.Join(tempDir, "some", "file", "path"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(body), should.Equal("data a"))
			// Symlink to symlink is traversable too.
			body, err = os.ReadFile(filepath.Join(tempDir, "some", "symlink"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(body), should.Equal("data b"))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"some/executable:../.cipd/pkgs/0/_current/some/executable",
					"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
					"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
					"subdir/some/executable:../../.cipd/pkgs/1/_current/some/executable",
					"subdir/some/file/path:../../../.cipd/pkgs/1/_current/some/file/path",
					"subdir/some/symlink:../../.cipd/pkgs/1/_current/some/symlink",
				}))

				// Ensure symlinks are actually traversable.
				body, err := os.ReadFile(filepath.Join(tempDir, "subdir", "some", "file", "path"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(body), should.Equal("data a"))
				// Symlink to symlink is traversable too.
				body, err = os.ReadFile(filepath.Join(tempDir, "subdir", "some", "symlink"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, string(body), should.Equal("data b"))
			})
		})

		t.Run("DeployInstance new non-empty package instance (copy mode override)", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
				fs.NewTestFile(".cipd/pkg/0/description.json", "{}", fs.TestFileOpts{}), // should be ignored
			}, pkg.InstallModeSymlink)
			_, err := New(tempDir).DeployInstance(ctx, "", inst, pkg.InstallModeCopy, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			}))
			// Ensure symlinks are actually traversable.
			body, err := os.ReadFile(filepath.Join(tempDir, "some", "file", "path"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(body), should.Equal("data a"))
			// Symlink to symlink is traversable too.
			body, err = os.ReadFile(filepath.Join(tempDir, "some", "symlink"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(body), should.Equal("data b"))
			// manifest lists actual install mode
			manifest, err := New(tempDir).(*deployerImpl).readManifest(
				ctx, filepath.Join(tempDir, ".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, manifest.InstallMode, should.Equal(pkg.InstallModeSymlink))
			assert.Loosely(t, manifest.ActualInstallMode, should.Equal(pkg.InstallModeCopy))
		})

		t.Run("Redeploy same package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
			}, pkg.InstallModeSymlink)
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				".cipd/trash!",
				"some/executable:../.cipd/pkgs/0/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
				"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					".cipd/trash!",
					"some/executable:../.cipd/pkgs/0/_current/some/executable",
					"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
					"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
					"subdir/some/executable:../../.cipd/pkgs/1/_current/some/executable",
					"subdir/some/file/path:../../../.cipd/pkgs/1/_current/some/file/path",
					"subdir/some/symlink:../../.cipd/pkgs/1/_current/some/symlink",
				}))
			})
		})

		t.Run("DeployInstance package update", func(t *ftt.Test) {
			oldPkg := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a old", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("some/to-be-empty-dir/file", "data", fs.TestFileOpts{}),
				fs.NewTestFile("old only", "data c old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 1", "data d", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 2", "data e", fs.TestFileOpts{}),
				fs.NewTestSymlink("symlink unchanged", "target"),
				fs.NewTestSymlink("symlink changed", "old target"),
				fs.NewTestSymlink("symlink removed", "target"),
			}, pkg.InstallModeSymlink)
			oldPkg.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			newPkg := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a new", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b new", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 1", "data d", fs.TestFileOpts{}),
				fs.NewTestFile("mode change 2", "data d", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("symlink unchanged", "target"),
				fs.NewTestSymlink("symlink changed", "new target"),
			}, pkg.InstallModeSymlink)
			newPkg.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			_, err := New(tempDir).DeployInstance(ctx, "", oldPkg, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", newPkg, "", 0)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/mode change 1",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/mode change 2*",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/symlink changed:new target",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/symlink unchanged:target",
				".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"mode change 1:.cipd/pkgs/0/_current/mode change 1",
				"mode change 2:.cipd/pkgs/0/_current/mode change 2",
				"some/executable:../.cipd/pkgs/0/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
				"symlink changed:.cipd/pkgs/0/_current/symlink changed",
				"symlink unchanged:.cipd/pkgs/0/_current/symlink unchanged",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", oldPkg, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", newPkg, "", 0)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/mode change 1",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/mode change 2*",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/symlink changed:new target",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/symlink unchanged:target",
					".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/mode change 1",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/mode change 2*",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/symlink changed:new target",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/symlink unchanged:target",
					".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"mode change 1:.cipd/pkgs/0/_current/mode change 1",
					"mode change 2:.cipd/pkgs/0/_current/mode change 2",
					"some/executable:../.cipd/pkgs/0/_current/some/executable",
					"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
					"subdir/mode change 1:../.cipd/pkgs/1/_current/mode change 1",
					"subdir/mode change 2:../.cipd/pkgs/1/_current/mode change 2",
					"subdir/some/executable:../../.cipd/pkgs/1/_current/some/executable",
					"subdir/some/file/path:../../../.cipd/pkgs/1/_current/some/file/path",
					"subdir/symlink changed:../.cipd/pkgs/1/_current/symlink changed",
					"subdir/symlink unchanged:../.cipd/pkgs/1/_current/symlink unchanged",
					"symlink changed:.cipd/pkgs/0/_current/symlink changed",
					"symlink unchanged:.cipd/pkgs/0/_current/symlink unchanged",
				}))
			})
		})

		t.Run("DeployInstance two different packages", func(t *ftt.Test) {
			pkg1 := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a old", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("pkg1 file", "data c", fs.TestFileOpts{}),
			}, pkg.InstallModeSymlink)
			pkg1.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			// Nesting in package names is allowed.
			pkg2 := makeTestInstance("test/package/another", []fs.File{
				fs.NewTestFile("some/file/path", "data a new", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b new", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("pkg2 file", "data d", fs.TestFileOpts{}),
			}, pkg.InstallModeSymlink)
			pkg2.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			_, err := New(tempDir).DeployInstance(ctx, "", pkg1, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", pkg2, "", 0)
			assert.Loosely(t, err, should.BeNil)

			// TODO: Conflicting symlinks point to last installed package, it is not
			// very deterministic.
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/pkg1 file",
				".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
				".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
				".cipd/pkgs/0/_current:000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/pkg2 file",
				".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
				".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
				".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/1/description.json",
				".cipd/tmp!",
				"pkg1 file:.cipd/pkgs/0/_current/pkg1 file",
				"pkg2 file:.cipd/pkgs/1/_current/pkg2 file",
				"some/executable:../.cipd/pkgs/1/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/1/_current/some/file/path",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", pkg1, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", pkg2, "", 0)
				assert.Loosely(t, err, should.BeNil)

				// TODO: Conflicting symlinks point to last installed package, it is not
				// very deterministic.
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/pkg1 file",
					".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/0/_current:000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/pkg2 file",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/pkgs/2/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/2/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/pkg1 file",
					".cipd/pkgs/2/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/2/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/2/_current:000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/2/description.json",
					".cipd/pkgs/3/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/3/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/pkg2 file",
					".cipd/pkgs/3/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/3/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/3/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/3/description.json",
					".cipd/tmp!",
					"pkg1 file:.cipd/pkgs/0/_current/pkg1 file",
					"pkg2 file:.cipd/pkgs/1/_current/pkg2 file",
					"some/executable:../.cipd/pkgs/1/_current/some/executable",
					"some/file/path:../../.cipd/pkgs/1/_current/some/file/path",
					"subdir/pkg1 file:../.cipd/pkgs/2/_current/pkg1 file",
					"subdir/pkg2 file:../.cipd/pkgs/3/_current/pkg2 file",
					"subdir/some/executable:../../.cipd/pkgs/3/_current/some/executable",
					"subdir/some/file/path:../../../.cipd/pkgs/3/_current/some/file/path",
				}))
			})
		})
	})
}

func TestDeployInstanceCopyModePosix(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows")
	}

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("DeployInstance new empty package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			info, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info, should.Resemble(inst.Pin()))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
				info, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, info, should.Resemble(inst.Pin()))
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"subdir!",
				}))
			})
		})

		t.Run("DeployInstance new non-empty package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
				fs.NewTestFile(".cipd/pkg/0/description.json", "{}", fs.TestFileOpts{}), // should be ignored
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"some/executable*",
					"some/file/path",
					"some/symlink:executable",
					"subdir/some/executable*",
					"subdir/some/file/path",
					"subdir/some/symlink:executable",
				}))
			})
		})

		t.Run("Redeploy same package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				".cipd/trash!",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "somedir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "somedir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					".cipd/trash!",
					"some/executable*",
					"some/file/path",
					"some/symlink:executable",
					"somedir/some/executable*",
					"somedir/some/file/path",
					"somedir/some/symlink:executable",
				}))
			})
		})

		t.Run("DeployInstance package update", func(t *ftt.Test) {
			oldPkg := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a old", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("some/to-be-empty-dir/file", "data", fs.TestFileOpts{}),
				fs.NewTestFile("old only", "data c old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 1", "data d", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 2", "data e", fs.TestFileOpts{}),
				fs.NewTestSymlink("symlink unchanged", "target"),
				fs.NewTestSymlink("symlink changed", "old target"),
				fs.NewTestSymlink("symlink removed", "target"),
			}, pkg.InstallModeCopy)
			oldPkg.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			newPkg := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a new", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b new", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 1", "data d", fs.TestFileOpts{}),
				fs.NewTestFile("mode change 2", "data d", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("symlink unchanged", "target"),
				fs.NewTestSymlink("symlink changed", "new target"),
			}, pkg.InstallModeCopy)
			newPkg.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			_, err := New(tempDir).DeployInstance(ctx, "", oldPkg, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", newPkg, "", 0)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"mode change 1",
				"mode change 2*",
				"some/executable*",
				"some/file/path",
				"symlink changed:new target",
				"symlink unchanged:target",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", oldPkg, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", newPkg, "", 0)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"mode change 1",
					"mode change 2*",
					"some/executable*",
					"some/file/path",
					"subdir/mode change 1",
					"subdir/mode change 2*",
					"subdir/some/executable*",
					"subdir/some/file/path",
					"subdir/symlink changed:new target",
					"subdir/symlink unchanged:target",
					"symlink changed:new target",
					"symlink unchanged:target",
				}))
			})
		})

		t.Run("DeployInstance two different packages", func(t *ftt.Test) {
			pkg1 := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a old", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("pkg1 file", "data c", fs.TestFileOpts{}),
			}, pkg.InstallModeCopy)
			pkg1.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			// Nesting in package names is allowed.
			pkg2 := makeTestInstance("test/package/another", []fs.File{
				fs.NewTestFile("some/file/path", "data a new", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b new", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("pkg2 file", "data d", fs.TestFileOpts{}),
			}, pkg.InstallModeCopy)
			pkg2.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			_, err := New(tempDir).DeployInstance(ctx, "", pkg1, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", pkg2, "", 0)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/1/description.json",
				".cipd/tmp!",
				"pkg1 file",
				"pkg2 file",
				"some/executable*",
				"some/file/path",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "somedir", pkg1, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "somedir", pkg2, "", 0)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/pkgs/2/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current:000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/2/description.json",
					".cipd/pkgs/3/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/3/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/3/description.json",
					".cipd/tmp!",
					"pkg1 file",
					"pkg2 file",
					"some/executable*",
					"some/file/path",
					"somedir/pkg1 file",
					"somedir/pkg2 file",
					"somedir/some/executable*",
					"somedir/some/file/path",
				}))
			})
		})
	})
}

func TestDeployInstanceCopyModeWindows(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("Skipping on posix")
	}

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("DeployInstance new empty package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			info, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, info, should.Resemble(inst.Pin()))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
			}))
			cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
			assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))

			t.Run("in subdir", func(t *ftt.Test) {
				info, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, info, should.Resemble(inst.Pin()))
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"subdir!",
				}))
				cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
				assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
				cur = readFile(t, tempDir, ".cipd/pkgs/1/_current.txt")
				assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
			})
		})

		t.Run("DeployInstance new non-empty package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile(".cipd/pkg/0/description.json", "{}", fs.TestFileOpts{}), // should be ignored
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable",
				"some/file/path",
			}))
			cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
			assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"some/executable",
					"some/file/path",
					"subdir/some/executable",
					"subdir/some/file/path",
				}))
				cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
				assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
				cur = readFile(t, tempDir, ".cipd/pkgs/1/_current.txt")
				assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
			})
		})

		t.Run("Redeploy same package instance", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				".cipd/trash!",
				"some/executable",
				"some/file/path",
			}))
			cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
			assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					".cipd/trash!",
					"some/executable",
					"some/file/path",
					"subdir/some/executable",
					"subdir/some/file/path",
				}))
				cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
				assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
				cur = readFile(t, tempDir, ".cipd/pkgs/1/_current.txt")
				assert.Loosely(t, cur, should.Equal("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
			})
		})

		t.Run("DeployInstance package update", func(t *ftt.Test) {
			oldPkg := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a old", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("some/to-be-empty-dir/file", "data", fs.TestFileOpts{}),
				fs.NewTestFile("old only", "data c old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 1", "data d", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 2", "data e", fs.TestFileOpts{}),
			}, pkg.InstallModeCopy)
			oldPkg.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			newPkg := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a new", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b new", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("mode change 1", "data d", fs.TestFileOpts{}),
				fs.NewTestFile("mode change 2", "data d", fs.TestFileOpts{Executable: true}),
			}, pkg.InstallModeCopy)
			newPkg.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			_, err := New(tempDir).DeployInstance(ctx, "", oldPkg, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", newPkg, "", 0)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"mode change 1",
				"mode change 2",
				"some/executable",
				"some/file/path",
			}))
			cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
			assert.Loosely(t, cur, should.Equal("111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", oldPkg, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", newPkg, "", 0)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"mode change 1",
					"mode change 2",
					"some/executable",
					"some/file/path",
					"subdir/mode change 1",
					"subdir/mode change 2",
					"subdir/some/executable",
					"subdir/some/file/path",
				}))
				cur := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
				assert.Loosely(t, cur, should.Equal("111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
				cur = readFile(t, tempDir, ".cipd/pkgs/1/_current.txt")
				assert.Loosely(t, cur, should.Equal("111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
			})
		})

		t.Run("DeployInstance two different packages", func(t *ftt.Test) {
			pkg1 := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a old", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b old", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("pkg1 file", "data c", fs.TestFileOpts{}),
			}, pkg.InstallModeCopy)
			pkg1.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			// Nesting in package names is allowed.
			pkg2 := makeTestInstance("test/package/another", []fs.File{
				fs.NewTestFile("some/file/path", "data a new", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b new", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("pkg2 file", "data d", fs.TestFileOpts{}),
			}, pkg.InstallModeCopy)
			pkg2.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

			_, err := New(tempDir).DeployInstance(ctx, "", pkg1, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", pkg2, "", 0)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/1/_current.txt",
				".cipd/pkgs/1/description.json",
				".cipd/tmp!",
				"pkg1 file",
				"pkg2 file",
				"some/executable",
				"some/file/path",
			}))
			cur1 := readFile(t, tempDir, ".cipd/pkgs/1/_current.txt")
			assert.Loosely(t, cur1, should.Equal("111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
			cur2 := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
			assert.Loosely(t, cur2, should.Equal("000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))

			t.Run("in subdir", func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", pkg1, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", pkg2, "", 0)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					".cipd/pkgs/2/000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current.txt",
					".cipd/pkgs/2/description.json",
					".cipd/pkgs/3/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/3/_current.txt",
					".cipd/pkgs/3/description.json",
					".cipd/tmp!",
					"pkg1 file",
					"pkg2 file",
					"some/executable",
					"some/file/path",
					"subdir/pkg1 file",
					"subdir/pkg2 file",
					"subdir/some/executable",
					"subdir/some/file/path",
				}))
				cur1 := readFile(t, tempDir, ".cipd/pkgs/1/_current.txt")
				assert.Loosely(t, cur1, should.Equal("111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
				cur2 := readFile(t, tempDir, ".cipd/pkgs/0/_current.txt")
				assert.Loosely(t, cur2, should.Equal("000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"))
			})
		})
	})
}

func TestDeployInstanceSwitchingModes(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no symlinks")
	}

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		files := []fs.File{
			fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
			fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
			fs.NewTestSymlink("some/symlink", "executable"),
		}

		t.Run("InstallModeCopy => InstallModeSymlink", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", files, pkg.InstallModeCopy)
			inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)

			inst = makeTestInstance("test/package", files, pkg.InstallModeSymlink)
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err = New(tempDir).DeployInstance(ctx, "", inst, "", 0)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
				".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable:../.cipd/pkgs/0/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
				"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
			}))

			t.Run("in subidr", func(t *ftt.Test) {
				inst := makeTestInstance("test/package", files, pkg.InstallModeCopy)
				inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)

				inst = makeTestInstance("test/package", files, pkg.InstallModeSymlink)
				inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
					".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable*",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/file/path",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink:executable",
					".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"some/executable:../.cipd/pkgs/0/_current/some/executable",
					"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
					"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
					"subdir/some/executable:../../.cipd/pkgs/1/_current/some/executable",
					"subdir/some/file/path:../../../.cipd/pkgs/1/_current/some/file/path",
					"subdir/some/symlink:../../.cipd/pkgs/1/_current/some/symlink",
				}))
			})
		})

		t.Run("InstallModeSymlink => InstallModeCopy", func(t *ftt.Test) {
			inst := makeTestInstance("test/package", files, pkg.InstallModeSymlink)
			inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := New(tempDir).DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)

			inst = makeTestInstance("test/package", files, pkg.InstallModeCopy)
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err = New(tempDir).DeployInstance(ctx, "", inst, "", 0)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				inst := makeTestInstance("test/package", files, pkg.InstallModeSymlink)
				inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)

				inst = makeTestInstance("test/package", files, pkg.InstallModeCopy)
				inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst, "", 0)

				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"some/executable*",
					"some/file/path",
					"some/symlink:executable",
					"subdir/some/executable*",
					"subdir/some/file/path",
					"subdir/some/symlink:executable",
				}))
			})
		})
	})
}

func TestDeployInstanceUpgradeFileToDir(t *testing.T) {
	t.Parallel()

	ftt.Run("DeployInstance can replace files with directories", t, func(t *ftt.Test) {
		ctx := withMemLogger()
		tempDir := t.TempDir()

		// Here "some/path" is a file.
		oldPkg := makeTestInstance("test/package", []fs.File{
			fs.NewTestFile("some/path", "data old", fs.TestFileOpts{}),
		}, pkg.InstallModeCopy)
		oldPkg.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

		// And here "some/path" is a directory.
		newPkg := makeTestInstance("test/package", []fs.File{
			fs.NewTestFile("some/path/file", "data new", fs.TestFileOpts{}),
		}, pkg.InstallModeCopy)
		newPkg.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

		// Note: specifically use '/' even on Windows here, to make sure deployer
		// guts do slash conversion correctly inside.
		_, err := New(tempDir).DeployInstance(ctx, "sub/dir", oldPkg, "", 0)
		assert.Loosely(t, err, should.BeNil)
		_, err = New(tempDir).DeployInstance(ctx, "sub/dir", newPkg, "", 0)
		assert.Loosely(t, err, should.BeNil)

		// The new file is deployed successfully.
		assert.Loosely(t, readFile(t, tempDir, "sub/dir/some/path/file"), should.Equal("data new"))

		// No complaints during the upgrade.
		assert.Loosely(t, loggerWarnings(ctx), should.Resemble([]string(nil)))
	})
}

func TestDeployInstanceDirAndSymlinkSwaps(t *testing.T) {
	t.Parallel()

	ftt.Run("With packages", t, func(t *ftt.Test) {
		ctx := withMemLogger()
		tempDir := t.TempDir()

		// Here "some/path" is a directory.
		pkgWithDir := makeTestInstance("test/package", []fs.File{
			fs.NewTestFile("some/path/file1", "old data 1", fs.TestFileOpts{}),
			fs.NewTestFile("some/path/a/file2", "old data 2", fs.TestFileOpts{}),
			fs.NewTestFile("some/path/a/b/file3", "old data 3", fs.TestFileOpts{}),
		}, pkg.InstallModeCopy)
		pkgWithDir.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

		// And here "some/path" is a symlink to some new directory that also has
		// the same files (and a bunch more).
		pkgWithSym := makeTestInstance("test/package", []fs.File{
			fs.NewTestSymlink("some/path", "another"),
			fs.NewTestFile("some/another/file1", "new data 1", fs.TestFileOpts{}),
			fs.NewTestFile("some/another/a/file2", "new data 2", fs.TestFileOpts{}),
			fs.NewTestFile("some/another/a/b/file3", "new data 3", fs.TestFileOpts{}),
			fs.NewTestFile("some/another/a/file4", "new data 4", fs.TestFileOpts{}),
		}, pkg.InstallModeCopy)
		pkgWithSym.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

		for _, subDir := range []string{"", "sub", "sub/dir"} {
			subDirAbsPath := filepath.Join(tempDir, filepath.FromSlash(subDir))

			t.Run(fmt.Sprintf("Replace directory with symlink (subdir %q)", subDir), func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, subDir, pkgWithDir, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, subDir, pkgWithSym, "", 0)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, scanDirAndSkipGuts(subDirAbsPath), should.Resemble([]string{
					"some/another/a/b/file3",
					"some/another/a/file2",
					"some/another/a/file4",
					"some/another/file1",
					"some/path:another",
				}))

				assert.Loosely(t, readFile(t, tempDir, path.Join(subDir, "some/path/file1")), should.Equal("new data 1"))
				assert.Loosely(t, readFile(t, tempDir, path.Join(subDir, "some/path/a/file2")), should.Equal("new data 2"))

				assert.Loosely(t, loggerWarnings(ctx), should.Resemble([]string(nil)))
			})

			t.Run(fmt.Sprintf("Replace symlink with directory (subdir %q)", subDir), func(t *ftt.Test) {
				_, err := New(tempDir).DeployInstance(ctx, subDir, pkgWithSym, "", 0)
				assert.Loosely(t, err, should.BeNil)
				_, err = New(tempDir).DeployInstance(ctx, subDir, pkgWithDir, "", 0)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, scanDirAndSkipGuts(subDirAbsPath), should.Resemble([]string{
					"some/path/a/b/file3",
					"some/path/a/file2",
					"some/path/file1",
				}))

				assert.Loosely(t, readFile(t, tempDir, path.Join(subDir, "some/path/file1")), should.Equal("old data 1"))
				assert.Loosely(t, readFile(t, tempDir, path.Join(subDir, "some/path/a/file2")), should.Equal("old data 2"))

				assert.Loosely(t, loggerWarnings(ctx), should.Resemble([]string(nil)))
			})
		}

		t.Run("Sub_lk is a symlink and it shouldn't be deleted", func(t *ftt.Test) {
			caseSensitiveFS, err := New(tempDir).(*deployerImpl).fs.CaseSensitive()
			assert.Loosely(t, err, should.BeNil)
			err = os.Mkdir(filepath.Join(tempDir, "sub"), 0777)
			assert.Loosely(t, err, should.BeNil)

			linkName := "Sub_lk"
			if !caseSensitiveFS {
				linkName = "sub_lk"
			}
			mkSymlink(t, filepath.Join(tempDir, "sub"), filepath.Join(tempDir, linkName))

			_, err = New(tempDir).DeployInstance(ctx, "Sub_lk/dir", pkgWithSym, "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = New(tempDir).DeployInstance(ctx, "Sub_lk/dir", pkgWithDir, "", 0)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, scanDir(filepath.Join(tempDir, "sub", "dir")), should.Resemble([]string{
				"some/path/a/b/file3",
				"some/path/a/file2",
				"some/path/file1",
			}))

			assert.Loosely(t, readFile(t, tempDir, "sub/dir/some/path/file1"), should.Equal("old data 1"))
			assert.Loosely(t, readFile(t, tempDir, "sub/dir/some/path/a/file2"), should.Equal("old data 2"))
			assert.Loosely(t, readFile(t, tempDir, "Sub_lk/dir/some/path/file1"), should.Equal("old data 1"))
			assert.Loosely(t, readFile(t, tempDir, "Sub_lk/dir/some/path/a/file2"), should.Equal("old data 2"))

			assert.Loosely(t, loggerWarnings(ctx), should.Resemble([]string(nil)))
		})
	})
}

func TestDeployInstanceChangeInCase(t *testing.T) {
	t.Parallel()

	ftt.Run("DeployInstance handle change in file name case", t, func(c *ftt.Test) {
		ctx := withMemLogger()
		tempDir := t.TempDir()
		deployer := New(tempDir).(*deployerImpl)

		caseSensitiveFS, err := deployer.fs.CaseSensitive()
		assert.Loosely(c, err, should.BeNil)
		c.Logf("File system is case-sensitive: %v", caseSensitiveFS)

		pkg1 := makeTestInstance("test/package", []fs.File{
			fs.NewTestFile("keep", "keep", fs.TestFileOpts{}),
			fs.NewTestFile("some/dir/file_b", "old data 1", fs.TestFileOpts{}),
			fs.NewTestFile("some/D/f1", "old data 2", fs.TestFileOpts{}),
			fs.NewTestFile("some/d/f2", "old data 3", fs.TestFileOpts{}),
			fs.NewTestFile("some/path/file_a", "old data 4", fs.TestFileOpts{}),
		}, pkg.InstallModeCopy)
		pkg1.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

		pkg2 := makeTestInstance("test/package", []fs.File{
			fs.NewTestFile("keep", "keep", fs.TestFileOpts{}),
			fs.NewTestFile("some/DIR/file_b", "new data 1", fs.TestFileOpts{}),
			fs.NewTestFile("some/D/f2", "new data 2", fs.TestFileOpts{}),
			fs.NewTestFile("some/d/f1", "new data 3", fs.TestFileOpts{}),
			fs.NewTestFile("some/path/file_A", "new data 4", fs.TestFileOpts{}),
		}, pkg.InstallModeCopy)
		pkg2.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"

		_, err = deployer.DeployInstance(ctx, "sub/dir", pkg1, "", 0)
		assert.Loosely(c, err, should.BeNil)
		_, err = deployer.DeployInstance(ctx, "sub/dir", pkg2, "", 0)
		assert.Loosely(c, err, should.BeNil)

		got := scanDir(filepath.Join(tempDir, "sub", "dir"))
		sort.Strings(got)

		if caseSensitiveFS {
			// File case matches pkg2 exactly.
			assert.Loosely(c, got, should.Resemble([]string{
				"keep",
				"some/D/f2",
				"some/DIR/file_b",
				"some/d/f1",
				"some/path/file_A",
			}))
		} else {
			// Case of the files is somewhat random... It shouldn't really matter much
			// (since the file system is case-insensitive anyway).
			assert.Loosely(c, got[:len(got)-1], should.Resemble([]string{
				"keep",
				"some/D/f1",
				"some/D/f2",
				"some/dir/file_b",
			}))
			// On some file systems the original case (fila_a) is preserved, on some
			// it is switched to a new case (file_A). Go figure...
			assert.Loosely(c, got[len(got)-1], should.BeIn([]string{
				"some/path/file_A",
				"some/path/file_a",
			}...))
		}

		// Can read files through their new names, regardless of case-sensitivity of
		// the file system.
		assert.Loosely(c, readFile(t, tempDir, "sub/dir/some/DIR/file_b"), should.Equal("new data 1"))
		assert.Loosely(c, readFile(t, tempDir, "sub/dir/some/D/f2"), should.Equal("new data 2"))
		assert.Loosely(c, readFile(t, tempDir, "sub/dir/some/d/f1"), should.Equal("new data 3"))
		assert.Loosely(c, readFile(t, tempDir, "sub/dir/some/path/file_A"), should.Equal("new data 4"))

		assert.Loosely(c, loggerWarnings(ctx), should.Resemble([]string(nil)))
	})
}

func TestFindDeployed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("FindDeployed works with empty dir", func(t *ftt.Test) {
			out, err := New(tempDir).FindDeployed(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.BeNil)
		})

		t.Run("FindDeployed works", func(t *ftt.Test) {
			d := New(tempDir)

			// Deploy a bunch of stuff.
			_, err := d.DeployInstance(ctx, "", makeTestInstance("test/pkg/123", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test/pkg/456", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test/pkg", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg/123", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg/456", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)

			// including some broken packages
			_, err = d.DeployInstance(ctx, "", makeTestInstance("broken", nil, pkg.InstallModeCopy), "", 0)
			assert.Loosely(t, err, should.BeNil)
			if runtime.GOOS == "windows" {
				err = os.Remove(filepath.Join(tempDir, fs.SiteServiceDir, "pkgs", "8", "_current.txt"))
			} else {
				err = os.Remove(filepath.Join(tempDir, fs.SiteServiceDir, "pkgs", "8", "_current"))
			}
			assert.Loosely(t, err, should.BeNil)

			// Verify it is discoverable.
			out, err := d.FindDeployed(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out, should.Resemble(PinSliceBySubdir{
				"": PinSlice{
					{"test", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
					{"test/pkg", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
					{"test/pkg/123", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
					{"test/pkg/456", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
				},
				"subdir": PinSlice{
					{"test", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
					{"test/pkg", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
					{"test/pkg/123", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
					{"test/pkg/456", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
				},
			}))
		})
	})
}

func TestRemoveDeployedCommon(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("RemoveDeployed works with missing package", func(t *ftt.Test) {
			err := New(tempDir).RemoveDeployed(ctx, "", "package/path")
			assert.Loosely(t, err, should.BeNil)
			err = New(tempDir).RemoveDeployed(ctx, "subdir", "package/path")
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestRemoveDeployedPosix(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows")
	}

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("RemoveDeployed works", func(t *ftt.Test) {
			d := New(tempDir)

			// Deploy some instance (to keep it).
			inst := makeTestInstance("test/package/123", []fs.File{
				fs.NewTestFile("some/file/path1", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable1", "data b", fs.TestFileOpts{Executable: true}),
			}, pkg.InstallModeCopy)
			_, err := d.DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)

			// Deploy another instance (to remove it).
			inst2 := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path2", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/to-be-empty-dir/file", "data", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable2", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
			}, pkg.InstallModeCopy)
			_, err = d.DeployInstance(ctx, "", inst2, "", 0)
			assert.Loosely(t, err, should.BeNil)

			// Now remove the second package.
			err = d.RemoveDeployed(ctx, "", "test/package")
			assert.Loosely(t, err, should.BeNil)

			// Verify the final state (only first package should survive).
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable1*",
				"some/file/path1",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				// Deploy some instance (to keep it).
				_, err := d.DeployInstance(ctx, "subdir", inst2, "", 0)
				assert.Loosely(t, err, should.BeNil)

				// Deploy another instance (to remove it).
				_, err = d.DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)

				// Now remove the second package.
				err = d.RemoveDeployed(ctx, "subdir", "test/package")
				assert.Loosely(t, err, should.BeNil)

				// Verify the final state (only first package should survive).
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					// it's 2 because we flipped inst2 and inst in the installation order.
					// When we RemoveDeployed, we remove index 1.
					".cipd/pkgs/2/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/2/description.json",
					".cipd/tmp!",
					"some/executable1*",
					"some/file/path1",
					"subdir/some/executable1*",
					"subdir/some/file/path1",
				}))
			})
		})
	})
}

func TestRemoveDeployedWindows(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("Skipping on posix")
	}

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("RemoveDeployed works", func(t *ftt.Test) {
			d := New(tempDir)

			// Deploy some instance (to keep it).
			inst := makeTestInstance("test/package/123", []fs.File{
				fs.NewTestFile("some/file/path1", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable1", "data b", fs.TestFileOpts{Executable: true}),
			}, pkg.InstallModeCopy)
			_, err := d.DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)

			// Deploy another instance (to remove it).
			inst2 := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path2", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable2", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("some/to-be-empty-dir/file", "data", fs.TestFileOpts{}),
			}, pkg.InstallModeCopy)
			_, err = d.DeployInstance(ctx, "", inst2, "", 0)
			assert.Loosely(t, err, should.BeNil)

			// Now remove the second package.
			err = d.RemoveDeployed(ctx, "", "test/package")
			assert.Loosely(t, err, should.BeNil)

			// Verify the final state (only first package should survive).
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable1",
				"some/file/path1",
			}))

			t.Run("in subdir", func(t *ftt.Test) {
				// Deploy some instance (to keep it).
				_, err := d.DeployInstance(ctx, "subdir", inst2, "", 0)
				assert.Loosely(t, err, should.BeNil)

				// Deploy another instance (to remove it).
				_, err = d.DeployInstance(ctx, "subdir", inst, "", 0)
				assert.Loosely(t, err, should.BeNil)

				// Now remove the second package.
				err = d.RemoveDeployed(ctx, "subdir", "test/package")
				assert.Loosely(t, err, should.BeNil)

				// Verify the final state (only first package should survive).
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/2/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current.txt",
					".cipd/pkgs/2/description.json",
					".cipd/tmp!",
					"some/executable1",
					"some/file/path1",
					"subdir/some/executable1",
					"subdir/some/file/path1",
				}))
			})
		})
	})
}

func TestCheckDeployedAndRepair(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		dep := New(tempDir)

		deployInst := func(mode pkg.InstallMode, f ...fs.File) pkg.Instance {
			inst := makeTestInstance("test/package", f, mode)
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := dep.DeployInstance(ctx, "subdir", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			return inst
		}

		rm := func(p string) {
			assert.Loosely(t, os.Remove(filepath.Join(tempDir, filepath.FromSlash(p))), should.BeNil)
		}

		check := func(expected *DeployedPackage) *DeployedPackage {
			dp, err := dep.CheckDeployed(ctx, "subdir", "test/package", CheckPresence, pkg.WithoutManifest)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dp.Manifest, should.NotBeNil)
			dp.Manifest = nil
			assert.Loosely(t, dp, should.Resemble(expected))
			return dp
		}

		repair := func(p *DeployedPackage, inst pkg.Instance) {
			if len(p.ToRedeploy) == 0 {
				inst = nil
			}
			err := dep.RepairDeployed(ctx, "subdir", p.Pin, "", 1, RepairParams{
				Instance:   inst,
				ToRedeploy: p.ToRedeploy,
				ToRelink:   p.ToRelink,
			})
			assert.Loosely(t, err, should.BeNil)
		}

		checkHealthy := func() {
			dp, err := dep.CheckDeployed(ctx, "subdir", "test/package", CheckPresence, pkg.WithoutManifest)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dp.Deployed, should.BeTrue)
			assert.Loosely(t, dp.ToRedeploy, should.HaveLength(0))
			assert.Loosely(t, dp.ToRelink, should.HaveLength(0))
		}

		t.Run("Copy install mode, no symlinks", func(t *ftt.Test) {
			inst := deployInst(pkg.InstallModeCopy,
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("another-file", "data b", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data c", fs.TestFileOpts{Executable: true}),
			)

			expected := check(&DeployedPackage{
				Deployed:          true,
				Pin:               inst.Pin(),
				Subdir:            "subdir",
				InstallMode:       pkg.InstallModeCopy,
				ActualInstallMode: pkg.InstallModeCopy,
				packagePath:       filepath.Join(tempDir, ".cipd/pkgs/0"),
				instancePath:      filepath.Join(tempDir, ".cipd/pkgs/0/"+inst.Pin().InstanceID),
			})

			rm("subdir/some/file/path")
			rm("subdir/another-file")
			expected.ToRedeploy = []string{"some/file/path", "another-file"}

			check(expected)
			repair(expected, inst)
			checkHealthy()
		})

		if runtime.GOOS != "windows" {
			t.Run("Copy install mode, with symlink files", func(t *ftt.Test) {
				inst := deployInst(pkg.InstallModeCopy,
					fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
					fs.NewTestFile("another-file", "data b", fs.TestFileOpts{}),
					fs.NewTestFile("some/executable", "data c", fs.TestFileOpts{Executable: true}),
					fs.NewTestSymlink("some/symlink", "executable"),
					fs.NewTestSymlink("working_abs_symlink", "/bin"),
					// Even though this symlink points to a missing file, it should not
					// be considered broken, since it was specified like this in the
					// package file (so "repairing" it won't change anything).
					fs.NewTestSymlink("broken_abs_symlink", "/i_hope_this_dir_is_missing_on_bots"),
				)

				expected := check(&DeployedPackage{
					Deployed:          true,
					Pin:               inst.Pin(),
					Subdir:            "subdir",
					InstallMode:       pkg.InstallModeCopy,
					ActualInstallMode: pkg.InstallModeCopy,
					packagePath:       filepath.Join(tempDir, ".cipd/pkgs/0"),
					instancePath:      filepath.Join(tempDir, ".cipd/pkgs/0/"+inst.Pin().InstanceID),
				})

				t.Run("Symlink itself is gone but the target is not", func(t *ftt.Test) {
					rm("subdir/some/symlink")
					expected.ToRelink = []string{"some/symlink"}

					check(expected)
					repair(expected, inst)
					checkHealthy()
				})

				t.Run("Target is gone, but the symlink is not", func(t *ftt.Test) {
					rm("subdir/some/executable")
					expected.ToRedeploy = []string{"some/executable"}
					expected.ToRelink = []string{"some/symlink"} // ~ noop

					check(expected)
					repair(expected, inst)
					checkHealthy()
				})

				t.Run("Absolute symlinks are gone", func(t *ftt.Test) {
					rm("subdir/working_abs_symlink")
					rm("subdir/broken_abs_symlink")
					expected.ToRelink = []string{"working_abs_symlink", "broken_abs_symlink"}

					check(expected)
					repair(expected, inst)
					checkHealthy()
				})
			})

			t.Run("Symlink install mode", func(t *ftt.Test) {
				inst := deployInst(pkg.InstallModeSymlink,
					fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
					fs.NewTestFile("another-file", "data b", fs.TestFileOpts{}),
					fs.NewTestFile("some/executable", "data c", fs.TestFileOpts{Executable: true}),
					fs.NewTestSymlink("some/symlink", "executable"),
					fs.NewTestSymlink("working_abs_symlink", "/bin"),
					fs.NewTestSymlink("broken_abs_symlink", "/i_hope_this_dir_is_missing_on_bots"),
				)

				expected := check(&DeployedPackage{
					Deployed:          true,
					Pin:               inst.Pin(),
					Subdir:            "subdir",
					InstallMode:       pkg.InstallModeSymlink,
					ActualInstallMode: pkg.InstallModeSymlink,
					packagePath:       filepath.Join(tempDir, ".cipd/pkgs/0"),
					instancePath:      filepath.Join(tempDir, ".cipd/pkgs/0/"+inst.Pin().InstanceID),
				})

				t.Run("Site root files are gone, but gut files are OK", func(t *ftt.Test) {
					rm("subdir/some/file/path")
					rm("subdir/some/symlink")
					rm("subdir/broken_abs_symlink")
					expected.ToRelink = []string{"some/file/path", "some/symlink", "broken_abs_symlink"}

					check(expected)
					repair(expected, inst)
					checkHealthy()
				})

				t.Run("Regular gut files are gone", func(t *ftt.Test) {
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable")
					expected.ToRedeploy = []string{"some/executable"}
					expected.ToRelink = []string{"some/symlink"}

					check(expected)
					repair(expected, inst)
					checkHealthy()
				})

				t.Run("Rel symlink gut files are gone", func(t *ftt.Test) {
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink")
					expected.ToRelink = []string{"some/symlink"}

					check(expected)
					repair(expected, inst)
					checkHealthy()
				})

				t.Run("Abs symlink gut files are gone", func(t *ftt.Test) {
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/working_abs_symlink")
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/broken_abs_symlink")
					expected.ToRelink = []string{"working_abs_symlink", "broken_abs_symlink"}

					check(expected)
					repair(expected, inst)
					checkHealthy()
				})
			})
		}
	})
}

func TestUpgradeOldPkgDir(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given an old-style pkgs dir", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		d := New(tempDir)
		trashDir := filepath.Join(tempDir, fs.SiteServiceDir, "trash")
		fs := fs.NewFileSystem(tempDir, trashDir)

		inst := makeTestInstance("test/package", nil, pkg.InstallModeSymlink)
		_, err := d.DeployInstance(ctx, "", inst, "", 0)
		assert.Loosely(t, err, should.BeNil)

		currentLine := func(folder, inst string) string {
			if runtime.GOOS == "windows" {
				return fmt.Sprintf(".cipd/pkgs/%s/_current.txt", folder)
			}
			return fmt.Sprintf(".cipd/pkgs/%s/_current:%s", folder, inst)
		}

		pkg0 := filepath.Join(tempDir, ".cipd", "pkgs", "0")
		pkgOldStyle := filepath.Join(tempDir, ".cipd", "pkgs", "test_package-deadbeef")
		assert.Loosely(t, fs.EnsureFileGone(ctx, filepath.Join(pkg0, descriptionName)), should.BeNil)
		assert.Loosely(t, fs.Replace(ctx, pkg0, pkgOldStyle), should.BeNil)
		assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
			".cipd/pkgs/test_package-deadbeef/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
			currentLine("test_package-deadbeef", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
			".cipd/tmp!",
		}))

		t.Run("reading the packages finds it", func(t *ftt.Test) {
			pins, err := d.FindDeployed(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pins, should.Resemble(PinSliceBySubdir{
				"": PinSlice{
					{"test/package", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
				},
			}))

			t.Run("and upgrades the package", func(t *ftt.Test) {
				assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
					".cipd/pkgs/test_package-deadbeef/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					currentLine("test_package-deadbeef", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
					".cipd/pkgs/test_package-deadbeef/description.json",
					".cipd/tmp!",
				}))
			})
		})

		t.Run("can deploy new instance", func(t *ftt.Test) {
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := d.DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/test_package-deadbeef/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				currentLine("test_package-deadbeef", "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
				".cipd/pkgs/test_package-deadbeef/description.json",
				".cipd/tmp!",
			}))
		})

		t.Run("can deploy other package", func(t *ftt.Test) {
			inst.packageName = "something/cool"
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := d.DeployInstance(ctx, "", inst, "", 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				currentLine("0", "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/test_package-deadbeef/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				currentLine("test_package-deadbeef", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
				".cipd/pkgs/test_package-deadbeef/description.json",
				".cipd/tmp!",
			}))
		})

	})
}

func TestNumSet(t *testing.T) {
	t.Parallel()

	ftt.Run("numSet", t, func(t *ftt.Test) {
		ns := numSet{}

		t.Run("can add numbers out of order", func(t *ftt.Test) {
			for _, n := range []int{392, 1, 7, 29, 4} {
				ns.addNum(n)
			}
			assert.Loosely(t, ns, should.Resemble(numSet{1, 4, 7, 29, 392}))

			t.Run("and rejects duplicates", func(t *ftt.Test) {
				ns.addNum(7)
				assert.Loosely(t, ns, should.Resemble(numSet{1, 4, 7, 29, 392}))
			})
		})

		t.Run("smallestNewNum", func(t *ftt.Test) {
			ns = numSet{1, 4, 7, 29, 392}

			smallNums := []int{0, 2, 3, 5, 6, 8}
			for _, sn := range smallNums {
				assert.Loosely(t, ns.smallestNewNum(), should.Equal(sn))
				ns.addNum(sn)
			}
		})

	})
}

func TestResolveValidPackageDirs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("resolveValidPackageDirs", t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		d := New(tempDir).(*deployerImpl)
		pkgdir, err := d.fs.RootRelToAbs(filepath.FromSlash(packagesDir))
		assert.Loosely(t, err, should.BeNil)

		writeFiles := func(files ...fs.File) {
			for _, f := range files {
				name := filepath.Join(tempDir, f.Name())
				if f.Symlink() {
					targ, err := f.SymlinkTarget()
					assert.Loosely(t, err, should.BeNil)
					err = d.fs.EnsureSymlink(ctx, name, targ)
					assert.Loosely(t, err, should.BeNil)
				} else {
					err := d.fs.EnsureFile(ctx, name, func(wf *os.File) error {
						reader, err := f.Open()
						if err != nil {
							return err
						}
						defer reader.Close()
						_, err = io.Copy(wf, reader)
						return err
					})
					assert.Loosely(t, err, should.BeNil)
				}
			}
		}
		desc := func(pkgFolder, subdir, packageName string) fs.File {
			return fs.NewTestFile(
				fmt.Sprintf(".cipd/pkgs/%s/description.json", pkgFolder),
				fmt.Sprintf(`{"subdir": %q, "package_name": %q}`, subdir, packageName),
				fs.TestFileOpts{},
			)
		}
		resolve := func() (numSet, map[description]string) {
			nums, all := d.resolveValidPackageDirs(ctx, pkgdir)
			for desc, absPath := range all {
				rel, err := filepath.Rel(tempDir, absPath)
				assert.Loosely(t, err, should.BeNil)
				all[desc] = strings.Replace(filepath.Clean(rel), "\\", "/", -1)
			}
			return nums, all
		}

		t.Run("packagesDir with just description.json", func(t *ftt.Test) {
			writeFiles(
				desc("0", "", "some/package/name"),
			)
			nums, all := resolve()
			assert.Loosely(t, nums, should.Resemble(numSet{0}))
			assert.Loosely(t, all, should.Resemble(map[description]string{
				{"", "some/package/name"}: ".cipd/pkgs/0",
			}))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/description.json",
			}))
		})

		t.Run("packagesDir with duplicates", func(t *ftt.Test) {
			writeFiles(
				desc("0", "", "some/package/name"),
				desc("some_other", "", "some/package/name"),
				desc("1", "", "some/package/name"),
			)
			nums, all := resolve()
			assert.Loosely(t, nums, should.Resemble(numSet{0}))
			assert.Loosely(t, all, should.Resemble(map[description]string{
				{"", "some/package/name"}: ".cipd/pkgs/0",
			}))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/0/description.json",
			}))
		})

		t.Run("bogus file", func(t *ftt.Test) {
			writeFiles(
				fs.NewTestFile(".cipd/pkgs/wat", "hello", fs.TestFileOpts{}),
			)
			nums, all := resolve()
			assert.Loosely(t, nums, should.Resemble(numSet(nil)))
			assert.Loosely(t, all, should.Resemble(map[description]string{}))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{".cipd/pkgs!"}))
		})

		t.Run("bad description.json", func(t *ftt.Test) {
			writeFiles(
				fs.NewTestFile(".cipd/pkgs/0/description.json", "hello", fs.TestFileOpts{}),
			)
			nums, all := resolve()
			assert.Loosely(t, nums, should.Resemble(numSet(nil)))
			assert.Loosely(t, all, should.Resemble(map[description]string{}))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{".cipd/pkgs!"}))
		})

		t.Run("package with no manifest", func(t *ftt.Test) {
			writeFiles(
				fs.NewTestFile(".cipd/pkgs/0/deadbeef/something", "hello", fs.TestFileOpts{}),
				fs.NewTestSymlink(".cipd/pkgs/0/_current", "deadbeef"),
			)
			nums, all := resolve()
			assert.Loosely(t, nums, should.Resemble(numSet(nil)))
			assert.Loosely(t, all, should.Resemble(map[description]string{}))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{".cipd/pkgs!"}))
		})

		t.Run("package with manifest", func(t *ftt.Test) {
			curLink := fs.NewTestSymlink(".cipd/pkgs/oldskool/_current", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
			if runtime.GOOS == "windows" {
				curLink = fs.NewTestFile(".cipd/pkgs/oldskool/_current.txt", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC", fs.TestFileOpts{})
			}
			writeFiles(
				fs.NewTestFile(".cipd/pkgs/oldskool/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/something", "hello", fs.TestFileOpts{}),
				fs.NewTestFile(".cipd/pkgs/oldskool/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					`{"format_version": "1", "package_name": "cool/cats"}`, fs.TestFileOpts{}),
				curLink,
			)
			nums, all := resolve()
			assert.Loosely(t, nums, should.Resemble(numSet(nil)))
			assert.Loosely(t, all, should.Resemble(map[description]string{
				{"", "cool/cats"}: ".cipd/pkgs/oldskool",
			}))
			linkExpect := curLink.Name()
			if curLink.Symlink() {
				targ, err := curLink.SymlinkTarget()
				assert.Loosely(t, err, should.BeNil)
				linkExpect = fmt.Sprintf("%s:%s", curLink.Name(), targ)
			}

			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{
				".cipd/pkgs/oldskool/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/oldskool/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/something",
				linkExpect,
				".cipd/pkgs/oldskool/description.json",
			}))
		})

	})

}

func TestPackagePathCollision(t *testing.T) {
	t.Parallel()

	const threads = 50
	const packages = 4

	ctx := context.Background()

	ftt.Run("Collide packagePath", t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		d := New(tempDir).(*deployerImpl)

		results := make([]struct {
			pkg  string
			path string
			err  error
		}, threads)

		wg := sync.WaitGroup{}
		wg.Add(threads)
		for th := 0; th < threads; th++ {
			th := th
			go func() {
				defer wg.Done()
				r := &results[th]
				r.pkg = fmt.Sprintf("pkg/%d", th%packages)
				r.path, r.err = d.packagePath(ctx, "", r.pkg, true)
			}()
		}
		wg.Wait()

		// No errors.
		seen := stringset.New(0)
		for _, r := range results {
			if r.err != nil { // avoid gazillion of goconvey checkmarks
				assert.Loosely(t, r.err, should.BeNil)
			} else {
				seen.Add(fmt.Sprintf("%s:%s", r.pkg, r.path))
			}
		}

		// Each 'pkg' got exactly one 'path'.
		assert.Loosely(t, seen.ToSlice(), should.HaveLength(packages))
	})
}

func TestDeployInstanceCollision(t *testing.T) {
	t.Parallel()

	const threads = 50
	const instances = 5

	ctx := context.Background()

	installMode := pkg.InstallModeSymlink
	if runtime.GOOS == "windows" {
		installMode = pkg.InstallModeCopy
	}

	ftt.Run("Collide DeployInstance", t, func(t *ftt.Test) {
		testInstances := make([]*testPackageInstance, instances)
		for i := range testInstances {
			testInstances[i] = makeTestInstance(fmt.Sprintf("pkg/%d", i), []fs.File{
				fs.NewTestFile(fmt.Sprintf("private_%d", i), "data", fs.TestFileOpts{}),
				fs.NewTestFile("path/shared", "data", fs.TestFileOpts{}),
			}, installMode)
			testInstances[i].instanceID = fmt.Sprintf("-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwF%02d", i)
		}

		tempDir := t.TempDir()
		d := New(tempDir).(*deployerImpl)

		wg := sync.WaitGroup{}
		wg.Add(threads)
		for th := 0; th < threads; th++ {
			th := th
			go func() {
				defer wg.Done()
				_, err := d.DeployInstance(ctx, "", testInstances[th%instances], "", 0)
				if err != nil {
					panic(err)
				}
			}()
		}
		wg.Wait()
	})
}

func TestRemoveEmptyTrees(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		absPath := func(rel string) string {
			return filepath.Join(tempDir, filepath.FromSlash(rel))
		}
		touch := func(rel string) {
			abs := absPath(rel)
			err := os.MkdirAll(filepath.Dir(abs), 0777)
			assert.Loosely(t, err, should.BeNil)
			f, err := os.Create(abs)
			assert.Loosely(t, err, should.BeNil)
			f.Close()
		}
		delete := func(rel string) {
			assert.Loosely(t, os.Remove(absPath(rel)), should.BeNil)
		}

		dirSet := func(rel ...string) stringset.Set {
			out := stringset.New(len(rel))
			for _, r := range rel {
				out.Add(absPath(r))
			}
			return out
		}

		t.Run("Simple case", func(t *ftt.Test) {
			touch("1/2/3/4")
			delete("1/2/3/4")
			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3"))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{"1!"}))
		})

		t.Run("Non empty", func(t *ftt.Test) {
			touch("1/2/3/4")
			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3"))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{"1/2/3/4"}))
		})

		t.Run("Multiple empty", func(t *ftt.Test) {
			touch("1/2/3a/4")
			touch("1/2/3b/4")

			delete("1/2/3a/4")
			delete("1/2/3b/4")

			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3a", "1/2/3b"))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{"1!"}))
		})

		t.Run("Respects 'empty' set", func(t *ftt.Test) {
			touch("1/2/3a/4")
			touch("1/2/3b/4")

			delete("1/2/3a/4")
			delete("1/2/3b/4")

			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3b"))
			assert.Loosely(t, scanDir(tempDir), should.Resemble([]string{"1/2/3a!"}))
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type testPackageInstance struct {
	packageName string
	instanceID  string
	files       []fs.File
}

// makeTestInstance returns pkg.Instance implementation with mocked guts.
func makeTestInstance(name string, files []fs.File, manifestInstallMode pkg.InstallMode) *testPackageInstance {
	// Generate and append manifest file.
	out := bytes.Buffer{}
	err := pkg.WriteManifest(&pkg.Manifest{
		FormatVersion: pkg.ManifestFormatVersion,
		PackageName:   name,
		InstallMode:   manifestInstallMode,
	}, &out)
	if err != nil {
		panic("Failed to write a manifest")
	}
	files = append(files, fs.NewTestFile(pkg.ManifestName, string(out.Bytes()), fs.TestFileOpts{}))
	return &testPackageInstance{
		packageName: name,
		instanceID:  "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC", // some "representative" SHA256 IID
		files:       files,
	}
}

func (f *testPackageInstance) Pin() Pin                          { return Pin{f.packageName, f.instanceID} }
func (f *testPackageInstance) Files() []fs.File                  { return f.files }
func (f *testPackageInstance) Source() pkg.Source                { panic("Not implemented") }
func (f *testPackageInstance) Close(context.Context, bool) error { return nil }

////////////////////////////////////////////////////////////////////////////////

// scanDir returns list of files (regular, symlinks and directories if they are
// empty) it finds in a directory. Symlinks are returned as "path:target".
// Empty directories are suffixed with '!'. Regular executable files are
// suffixed with '*'. All paths are relative to the scanned directory and slash
// separated. Symlink targets are slash separated too, but otherwise not
// modified. Does not look inside symlinked directories.
func scanDir(root string) (out []string) {
	err := filepath.WalkDir(root, func(path string, entry iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == fsLockName {
			return nil // .lock files are not interesting
		}

		rel, err := filepath.Rel(root, path)
		switch {
		case err != nil:
			return err
		case rel == ".":
			return nil
		case entry.IsDir() && !isEmptyDir(path):
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)
		item := rel

		if !info.Mode().IsRegular() && !info.Mode().IsDir() { // probably a symlink
			if target, err := os.Readlink(path); err == nil {
				item = fmt.Sprintf("%s:%s", rel, filepath.ToSlash(target))
			} else {
				item = fmt.Sprintf("%s:??????", rel)
			}
		}

		suffix := ""
		switch {
		case info.Mode().IsRegular() && (info.Mode().Perm()&0100) != 0:
			suffix = "*"
		case info.Mode().IsDir():
			suffix = "!"
		}

		out = append(out, item+suffix)
		return nil
	})
	if err != nil {
		panic(fmt.Sprintf("Failed to walk a directory: %s", err))
	}
	return
}

// scanDirAndSkipGuts is like scanDir, but it omits .cipd/* from the output.
func scanDirAndSkipGuts(root string) (out []string) {
	for _, p := range scanDir(root) {
		if !strings.HasPrefix(p, ".cipd/") {
			out = append(out, p)
		}
	}
	return
}

// isEmptyDir return true if 'path' refers to an empty directory.
func isEmptyDir(path string) bool {
	entries, err := os.ReadDir(path)
	return err == nil && len(entries) == 0
}

// readFile reads content of an existing text file. Root path is provided as
// a native path, rel - as a slash-separated path.
func readFile(t testing.TB, root, rel string) string {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	assert.Loosely(t, err, should.BeNil)
	return string(body)
}
