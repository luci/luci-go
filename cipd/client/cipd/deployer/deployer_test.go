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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	. "go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func mkTempDir() string {
	tempDir, err := ioutil.TempDir("", "cipd_test")
	So(err, ShouldBeNil)
	Reset(func() { os.RemoveAll(tempDir) })
	return tempDir
}

func withMemLogger() context.Context {
	return memlogger.Use(context.Background())
}

func loggerWarnings(c context.Context) (out []string) {
	for _, m := range logging.Get(c).(*memlogger.MemLogger).Messages() {
		if m.Level >= logging.Warning {
			out = append(out, m.Msg)
		}
	}
	return
}

func TestUtilities(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		// Wrappers that accept paths relative to tempDir.
		touch := func(rel string) {
			abs := filepath.Join(tempDir, filepath.FromSlash(rel))
			err := os.MkdirAll(filepath.Dir(abs), 0777)
			So(err, ShouldBeNil)
			f, err := os.Create(abs)
			So(err, ShouldBeNil)
			f.Close()
		}
		ensureLink := func(symlinkRel string, target string) {
			err := os.Symlink(target, filepath.Join(tempDir, symlinkRel))
			So(err, ShouldBeNil)
		}

		Convey("scanPackageDir works with empty dir", func() {
			err := os.Mkdir(filepath.Join(tempDir, "dir"), 0777)
			So(err, ShouldBeNil)
			files, err := scanPackageDir(ctx, filepath.Join(tempDir, "dir"))
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 0)
		})

		Convey("scanPackageDir works", func() {
			touch("unrelated/1")
			touch("dir/a/1")
			touch("dir/a/2")
			touch("dir/b/1")
			touch("dir/.cipdpkg/abc")
			touch("dir/.cipd/abc")

			runScanPackageDir := func() sort.StringSlice {
				files, err := scanPackageDir(ctx, filepath.Join(tempDir, "dir"))
				So(err, ShouldBeNil)
				names := sort.StringSlice{}
				for _, f := range files {
					names = append(names, f.Name)
				}
				names.Sort()
				return names
			}

			// Symlinks doesn't work on Windows, test them only on Posix.
			if runtime.GOOS == "windows" {
				Convey("works on Windows", func() {
					So(runScanPackageDir(), ShouldResemble, sort.StringSlice{
						"a/1",
						"a/2",
						"b/1",
					})
				})
			} else {
				Convey("works on Posix", func() {
					ensureLink("dir/a/sym_link", "target")
					So(runScanPackageDir(), ShouldResemble, sort.StringSlice{
						"a/1",
						"a/2",
						"a/sym_link",
						"b/1",
					})
				})
			}
		})
	})
}

func TestDeployInstance(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("Try to deploy package instance with bad package name", func() {
			_, err := New(tempDir).DeployInstance(
				ctx, "", makeTestInstance("../test/package", nil, pkg.InstallModeCopy))
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Try to deploy package instance with bad instance ID", func() {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			inst.instanceID = "../000000000"
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldErrLike, "not a valid package instance ID")
		})

		Convey("Try to deploy package instance in bad subdir", func() {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			inst.instanceID = "../000000000"
			_, err := New(tempDir).DeployInstance(ctx, "/abspath", inst)
			So(err, ShouldErrLike, "bad subdir")
		})
	})
}

func TestDeployInstanceSymlinkMode(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no symlinks")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("DeployInstance new empty package instance", func() {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeSymlink)
			info, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, inst.Pin())
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
			})
			fInfo, err := os.Stat(filepath.Join(tempDir, ".cipd", "pkgs", "0"))
			So(err, ShouldBeNil)
			So(fInfo.Mode(), ShouldEqual, os.FileMode(0755)|os.ModeDir)

			Convey("in subdir", func() {
				info, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(info, ShouldResemble, inst.Pin())
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"subdir!",
				})
			})
		})

		Convey("DeployInstance new non-empty package instance", func() {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
				fs.NewTestFile(".cipd/pkg/0/description.json", "{}", fs.TestFileOpts{}), // should be ignored
			}, pkg.InstallModeSymlink)
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
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
			})
			// Ensure symlinks are actually traversable.
			body, err := ioutil.ReadFile(filepath.Join(tempDir, "some", "file", "path"))
			So(err, ShouldBeNil)
			So(string(body), ShouldEqual, "data a")
			// Symlink to symlink is traversable too.
			body, err = ioutil.ReadFile(filepath.Join(tempDir, "some", "symlink"))
			So(err, ShouldBeNil)
			So(string(body), ShouldEqual, "data b")

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})

				// Ensure symlinks are actually traversable.
				body, err := ioutil.ReadFile(filepath.Join(tempDir, "subdir", "some", "file", "path"))
				So(err, ShouldBeNil)
				So(string(body), ShouldEqual, "data a")
				// Symlink to symlink is traversable too.
				body, err = ioutil.ReadFile(filepath.Join(tempDir, "subdir", "some", "symlink"))
				So(err, ShouldBeNil)
				So(string(body), ShouldEqual, "data b")
			})
		})

		Convey("Redeploy same package instance", func() {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
			}, pkg.InstallModeSymlink)
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
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
			})

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})

		Convey("DeployInstance package update", func() {
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

			_, err := New(tempDir).DeployInstance(ctx, "", oldPkg)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", newPkg)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
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
			})

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", oldPkg)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", newPkg)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})

		Convey("DeployInstance two different packages", func() {
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

			_, err := New(tempDir).DeployInstance(ctx, "", pkg1)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", pkg2)
			So(err, ShouldBeNil)

			// TODO: Conflicting symlinks point to last installed package, it is not
			// very deterministic.
			So(scanDir(tempDir), ShouldResemble, []string{
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
			})

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", pkg1)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", pkg2)
				So(err, ShouldBeNil)

				// TODO: Conflicting symlinks point to last installed package, it is not
				// very deterministic.
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
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

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("DeployInstance new empty package instance", func() {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			info, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, inst.Pin())
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
			})

			Convey("in subdir", func() {
				inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
				info, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(info, ShouldResemble, inst.Pin())
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"subdir!",
				})
			})
		})

		Convey("DeployInstance new non-empty package instance", func() {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
				fs.NewTestFile(".cipd/pkg/0/description.json", "{}", fs.TestFileOpts{}), // should be ignored
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			})

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})

		Convey("Redeploy same package instance", func() {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				".cipd/trash!",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			})

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "somedir", inst)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "somedir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})

		Convey("DeployInstance package update", func() {
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

			_, err := New(tempDir).DeployInstance(ctx, "", oldPkg)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", newPkg)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
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
			})

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", oldPkg)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", newPkg)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})

		Convey("DeployInstance two different packages", func() {
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

			_, err := New(tempDir).DeployInstance(ctx, "", pkg1)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", pkg2)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
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
			})

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "somedir", pkg1)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "somedir", pkg2)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
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

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("DeployInstance new empty package instance", func() {
			inst := makeTestInstance("test/package", nil, pkg.InstallModeCopy)
			info, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, inst.Pin())
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")

			Convey("in subdir", func() {
				info, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(info, ShouldResemble, inst.Pin())
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					".cipd/tmp!",
					"subdir!",
				})
				cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
			})
		})

		Convey("DeployInstance new non-empty package instance", func() {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile(".cipd/pkg/0/description.json", "{}", fs.TestFileOpts{}), // should be ignored
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable",
				"some/file/path",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
				cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
			})
		})

		Convey("Redeploy same package instance", func() {
			inst := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
			}, pkg.InstallModeCopy)
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				".cipd/trash!",
				"some/executable",
				"some/file/path",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
				cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
			})
		})

		Convey("DeployInstance package update", func() {
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

			_, err := New(tempDir).DeployInstance(ctx, "", oldPkg)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", newPkg)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"mode change 1",
				"mode change 2",
				"some/executable",
				"some/file/path",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", oldPkg)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", newPkg)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
				cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur, ShouldEqual, "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
			})
		})

		Convey("DeployInstance two different packages", func() {
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

			_, err := New(tempDir).DeployInstance(ctx, "", pkg1)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "", pkg2)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
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
			})
			cur1 := readFile(tempDir, ".cipd/pkgs/1/_current.txt")
			So(cur1, ShouldEqual, "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
			cur2 := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur2, ShouldEqual, "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")

			Convey("in subdir", func() {
				_, err := New(tempDir).DeployInstance(ctx, "subdir", pkg1)
				So(err, ShouldBeNil)
				_, err = New(tempDir).DeployInstance(ctx, "subdir", pkg2)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
				cur1 := readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur1, ShouldEqual, "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
				cur2 := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur2, ShouldEqual, "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC")
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

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		files := []fs.File{
			fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
			fs.NewTestFile("some/executable", "data b", fs.TestFileOpts{Executable: true}),
			fs.NewTestSymlink("some/symlink", "executable"),
		}

		Convey("InstallModeCopy => InstallModeSymlink", func() {
			inst := makeTestInstance("test/package", files, pkg.InstallModeCopy)
			inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			inst = makeTestInstance("test/package", files, pkg.InstallModeSymlink)
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err = New(tempDir).DeployInstance(ctx, "", inst)

			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
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
			})

			Convey("in subidr", func() {
				inst := makeTestInstance("test/package", files, pkg.InstallModeCopy)
				inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)

				inst = makeTestInstance("test/package", files, pkg.InstallModeSymlink)
				inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst)

				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})

		Convey("InstallModeSymlink => InstallModeCopy", func() {
			inst := makeTestInstance("test/package", files, pkg.InstallModeSymlink)
			inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := New(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			inst = makeTestInstance("test/package", files, pkg.InstallModeCopy)
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err = New(tempDir).DeployInstance(ctx, "", inst)

			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			})

			Convey("in subdir", func() {
				inst := makeTestInstance("test/package", files, pkg.InstallModeSymlink)
				inst.instanceID = "000000000_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err := New(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)

				inst = makeTestInstance("test/package", files, pkg.InstallModeCopy)
				inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
				_, err = New(tempDir).DeployInstance(ctx, "subdir", inst)

				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})
	})
}

func TestDeployInstanceUpgradeFileToDir(t *testing.T) {
	t.Parallel()

	Convey("DeployInstance can replace files with directories", t, func() {
		ctx := withMemLogger()
		tempDir := mkTempDir()

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
		_, err := New(tempDir).DeployInstance(ctx, "sub/dir", oldPkg)
		So(err, ShouldBeNil)
		_, err = New(tempDir).DeployInstance(ctx, "sub/dir", newPkg)
		So(err, ShouldBeNil)

		// The new file is deployed successfully.
		So(readFile(tempDir, "sub/dir/some/path/file"), ShouldEqual, "data new")

		// No complaints during the upgrade.
		So(loggerWarnings(ctx), ShouldResemble, []string(nil))
	})
}

func TestDeployInstanceDirAndSymlinkSwaps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows")
	}

	t.Parallel()

	Convey("With packages", t, func() {
		ctx := withMemLogger()
		tempDir := mkTempDir()

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

		Convey("Replace directory with symlink", func() {
			_, err := New(tempDir).DeployInstance(ctx, "sub/dir", pkgWithDir)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "sub/dir", pkgWithSym)
			So(err, ShouldBeNil)

			So(scanDir(filepath.Join(tempDir, "sub", "dir")), ShouldResemble, []string{
				"some/another/a/b/file3",
				"some/another/a/file2",
				"some/another/a/file4",
				"some/another/file1",
				"some/path:another",
			})

			So(readFile(tempDir, "sub/dir/some/path/file1"), ShouldEqual, "new data 1")
			So(readFile(tempDir, "sub/dir/some/path/a/file2"), ShouldEqual, "new data 2")

			So(loggerWarnings(ctx), ShouldResemble, []string(nil))
		})

		Convey("Replace symlink with directory", func() {
			_, err := New(tempDir).DeployInstance(ctx, "sub/dir", pkgWithSym)
			So(err, ShouldBeNil)
			_, err = New(tempDir).DeployInstance(ctx, "sub/dir", pkgWithDir)
			So(err, ShouldBeNil)

			So(scanDir(filepath.Join(tempDir, "sub", "dir")), ShouldResemble, []string{
				"some/path/a/b/file3",
				"some/path/a/file2",
				"some/path/file1",
			})

			So(readFile(tempDir, "sub/dir/some/path/file1"), ShouldEqual, "old data 1")
			So(readFile(tempDir, "sub/dir/some/path/a/file2"), ShouldEqual, "old data 2")

			So(loggerWarnings(ctx), ShouldResemble, []string(nil))
		})
	})
}

func TestDeployInstanceChangeInCase(t *testing.T) {
	t.Parallel()

	Convey("DeployInstance handle change in file name case", t, func(c C) {
		ctx := withMemLogger()
		tempDir := mkTempDir()
		deployer := New(tempDir).(*deployerImpl)

		caseSensitiveFS, err := deployer.fs.CaseSensitive()
		So(err, ShouldBeNil)
		c.Printf("File system is case-sensitive: %v", caseSensitiveFS)

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

		_, err = deployer.DeployInstance(ctx, "sub/dir", pkg1)
		So(err, ShouldBeNil)
		_, err = deployer.DeployInstance(ctx, "sub/dir", pkg2)
		So(err, ShouldBeNil)

		got := scanDir(filepath.Join(tempDir, "sub", "dir"))
		sort.Strings(got)

		if caseSensitiveFS {
			// File case matches pkg2 exactly.
			So(got, ShouldResemble, []string{
				"keep",
				"some/D/f2",
				"some/DIR/file_b",
				"some/d/f1",
				"some/path/file_A",
			})
		} else {
			// Case of the files is somewhat random... It shouldn't really matter much
			// (since the file system is case-insensitive anyway).
			So(got[:len(got)-1], ShouldResemble, []string{
				"keep",
				"some/D/f1",
				"some/D/f2",
				"some/dir/file_b",
			})
			// On some file systems the original case (fila_a) is preserved, on some
			// it is switched to a new case (file_A). Go figure...
			So(got[len(got)-1], ShouldBeIn, []string{
				"some/path/file_A",
				"some/path/file_a",
			})
		}

		// Can read files through their new names, regardless of case-sensitivity of
		// the file system.
		So(readFile(tempDir, "sub/dir/some/DIR/file_b"), ShouldEqual, "new data 1")
		So(readFile(tempDir, "sub/dir/some/D/f2"), ShouldEqual, "new data 2")
		So(readFile(tempDir, "sub/dir/some/d/f1"), ShouldEqual, "new data 3")
		So(readFile(tempDir, "sub/dir/some/path/file_A"), ShouldEqual, "new data 4")

		So(loggerWarnings(ctx), ShouldResemble, []string(nil))
	})
}

func TestFindDeployed(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("FindDeployed works with empty dir", func() {
			out, err := New(tempDir).FindDeployed(ctx)
			So(err, ShouldBeNil)
			So(out, ShouldBeNil)
		})

		Convey("FindDeployed works", func() {
			d := New(tempDir)

			// Deploy a bunch of stuff.
			_, err := d.DeployInstance(ctx, "", makeTestInstance("test/pkg/123", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test/pkg/456", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test/pkg", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg/123", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg/456", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)

			// including some broken packages
			_, err = d.DeployInstance(ctx, "", makeTestInstance("broken", nil, pkg.InstallModeCopy))
			So(err, ShouldBeNil)
			if runtime.GOOS == "windows" {
				err = os.Remove(filepath.Join(tempDir, fs.SiteServiceDir, "pkgs", "8", "_current.txt"))
			} else {
				err = os.Remove(filepath.Join(tempDir, fs.SiteServiceDir, "pkgs", "8", "_current"))
			}
			So(err, ShouldBeNil)

			// Verify it is discoverable.
			out, err := d.FindDeployed(ctx)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, PinSliceBySubdir{
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
			})
		})
	})
}

func TestRemoveDeployedCommon(t *testing.T) {
	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("RemoveDeployed works with missing package", func() {
			err := New(tempDir).RemoveDeployed(ctx, "", "package/path")
			So(err, ShouldBeNil)
			err = New(tempDir).RemoveDeployed(ctx, "subdir", "package/path")
			So(err, ShouldBeNil)
		})
	})
}

func TestRemoveDeployedPosix(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("RemoveDeployed works", func() {
			d := New(tempDir)

			// Deploy some instance (to keep it).
			inst := makeTestInstance("test/package/123", []fs.File{
				fs.NewTestFile("some/file/path1", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable1", "data b", fs.TestFileOpts{Executable: true}),
			}, pkg.InstallModeCopy)
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			// Deploy another instance (to remove it).
			inst2 := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path2", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/to-be-empty-dir/file", "data", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable2", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestSymlink("some/symlink", "executable"),
			}, pkg.InstallModeCopy)
			_, err = d.DeployInstance(ctx, "", inst2)
			So(err, ShouldBeNil)

			// Now remove the second package.
			err = d.RemoveDeployed(ctx, "", "test/package")
			So(err, ShouldBeNil)

			// Verify the final state (only first package should survive).
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable1*",
				"some/file/path1",
			})

			Convey("in subdir", func() {
				// Deploy some instance (to keep it).
				_, err := d.DeployInstance(ctx, "subdir", inst2)
				So(err, ShouldBeNil)

				// Deploy another instance (to remove it).
				_, err = d.DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)

				// Now remove the second package.
				err = d.RemoveDeployed(ctx, "subdir", "test/package")
				So(err, ShouldBeNil)

				// Verify the final state (only first package should survive).
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
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

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("RemoveDeployed works", func() {
			d := New(tempDir)

			// Deploy some instance (to keep it).
			inst := makeTestInstance("test/package/123", []fs.File{
				fs.NewTestFile("some/file/path1", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable1", "data b", fs.TestFileOpts{Executable: true}),
			}, pkg.InstallModeCopy)
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			// Deploy another instance (to remove it).
			inst2 := makeTestInstance("test/package", []fs.File{
				fs.NewTestFile("some/file/path2", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable2", "data b", fs.TestFileOpts{Executable: true}),
				fs.NewTestFile("some/to-be-empty-dir/file", "data", fs.TestFileOpts{}),
			}, pkg.InstallModeCopy)
			_, err = d.DeployInstance(ctx, "", inst2)
			So(err, ShouldBeNil)

			// Now remove the second package.
			err = d.RemoveDeployed(ctx, "", "test/package")
			So(err, ShouldBeNil)

			// Verify the final state (only first package should survive).
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/tmp!",
				"some/executable1",
				"some/file/path1",
			})

			Convey("in subdir", func() {
				// Deploy some instance (to keep it).
				_, err := d.DeployInstance(ctx, "subdir", inst2)
				So(err, ShouldBeNil)

				// Deploy another instance (to remove it).
				_, err = d.DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)

				// Now remove the second package.
				err = d.RemoveDeployed(ctx, "subdir", "test/package")
				So(err, ShouldBeNil)

				// Verify the final state (only first package should survive).
				So(scanDir(tempDir), ShouldResemble, []string{
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
				})
			})
		})
	})
}

func TestCheckDeployedAndRepair(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()
		dep := New(tempDir)

		deployInst := func(mode pkg.InstallMode, f ...fs.File) pkg.Instance {
			inst := makeTestInstance("test/package", f, mode)
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := dep.DeployInstance(ctx, "subdir", inst)
			So(err, ShouldBeNil)
			return inst
		}

		rm := func(p string) {
			So(os.Remove(filepath.Join(tempDir, filepath.FromSlash(p))), ShouldBeNil)
		}

		check := func(expected *DeployedPackage) *DeployedPackage {
			dp, err := dep.CheckDeployed(ctx, "subdir", "test/package", CheckPresence, pkg.WithoutManifest)
			So(err, ShouldBeNil)
			So(dp.Manifest, ShouldNotBeNil)
			dp.Manifest = nil
			So(dp, ShouldResemble, expected)
			return dp
		}

		repair := func(p *DeployedPackage, inst pkg.Instance) {
			if len(p.ToRedeploy) == 0 {
				inst = nil
			}
			err := dep.RepairDeployed(ctx, "subdir", p.Pin, RepairParams{
				Instance:   inst,
				ToRedeploy: p.ToRedeploy,
				ToRelink:   p.ToRelink,
			})
			So(err, ShouldBeNil)
		}

		checkHealty := func() {
			dp, err := dep.CheckDeployed(ctx, "subdir", "test/package", CheckPresence, pkg.WithoutManifest)
			So(err, ShouldBeNil)
			So(dp.Deployed, ShouldBeTrue)
			So(dp.ToRedeploy, ShouldHaveLength, 0)
			So(dp.ToRelink, ShouldHaveLength, 0)
		}

		Convey("Copy install mode, no symlinks", func() {
			inst := deployInst(pkg.InstallModeCopy,
				fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
				fs.NewTestFile("another-file", "data b", fs.TestFileOpts{}),
				fs.NewTestFile("some/executable", "data c", fs.TestFileOpts{Executable: true}),
			)

			expected := check(&DeployedPackage{
				Deployed:     true,
				Pin:          inst.Pin(),
				Subdir:       "subdir",
				InstallMode:  pkg.InstallModeCopy,
				packagePath:  filepath.Join(tempDir, ".cipd/pkgs/0"),
				instancePath: filepath.Join(tempDir, ".cipd/pkgs/0/"+inst.Pin().InstanceID),
			})

			rm("subdir/some/file/path")
			rm("subdir/another-file")
			expected.ToRedeploy = []string{"some/file/path", "another-file"}

			check(expected)
			repair(expected, inst)
			checkHealty()
		})

		if runtime.GOOS != "windows" {
			Convey("Copy install mode, with symlink files", func() {
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
					Deployed:     true,
					Pin:          inst.Pin(),
					Subdir:       "subdir",
					InstallMode:  pkg.InstallModeCopy,
					packagePath:  filepath.Join(tempDir, ".cipd/pkgs/0"),
					instancePath: filepath.Join(tempDir, ".cipd/pkgs/0/"+inst.Pin().InstanceID),
				})

				Convey("Symlink itself is gone but the target is not", func() {
					rm("subdir/some/symlink")
					expected.ToRelink = []string{"some/symlink"}

					check(expected)
					repair(expected, inst)
					checkHealty()
				})

				Convey("Target is gone, but the symlink is not", func() {
					rm("subdir/some/executable")
					expected.ToRedeploy = []string{"some/executable"}
					expected.ToRelink = []string{"some/symlink"} // ~ noop

					check(expected)
					repair(expected, inst)
					checkHealty()
				})

				Convey("Absolute symlinks are gone", func() {
					rm("subdir/working_abs_symlink")
					rm("subdir/broken_abs_symlink")
					expected.ToRelink = []string{"working_abs_symlink", "broken_abs_symlink"}

					check(expected)
					repair(expected, inst)
					checkHealty()
				})
			})

			Convey("Symlink install mode", func() {
				inst := deployInst(pkg.InstallModeSymlink,
					fs.NewTestFile("some/file/path", "data a", fs.TestFileOpts{}),
					fs.NewTestFile("another-file", "data b", fs.TestFileOpts{}),
					fs.NewTestFile("some/executable", "data c", fs.TestFileOpts{Executable: true}),
					fs.NewTestSymlink("some/symlink", "executable"),
					fs.NewTestSymlink("working_abs_symlink", "/bin"),
					fs.NewTestSymlink("broken_abs_symlink", "/i_hope_this_dir_is_missing_on_bots"),
				)

				expected := check(&DeployedPackage{
					Deployed:     true,
					Pin:          inst.Pin(),
					Subdir:       "subdir",
					InstallMode:  pkg.InstallModeSymlink,
					packagePath:  filepath.Join(tempDir, ".cipd/pkgs/0"),
					instancePath: filepath.Join(tempDir, ".cipd/pkgs/0/"+inst.Pin().InstanceID),
				})

				Convey("Site root files are gone, but gut files are OK", func() {
					rm("subdir/some/file/path")
					rm("subdir/some/symlink")
					rm("subdir/broken_abs_symlink")
					expected.ToRelink = []string{"some/file/path", "some/symlink", "broken_abs_symlink"}

					check(expected)
					repair(expected, inst)
					checkHealty()
				})

				Convey("Regular gut files are gone", func() {
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/executable")
					expected.ToRedeploy = []string{"some/executable"}
					expected.ToRelink = []string{"some/symlink"}

					check(expected)
					repair(expected, inst)
					checkHealty()
				})

				Convey("Rel symlink gut files are gone", func() {
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/some/symlink")
					expected.ToRelink = []string{"some/symlink"}

					check(expected)
					repair(expected, inst)
					checkHealty()
				})

				Convey("Abs symlink gut files are gone", func() {
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/working_abs_symlink")
					rm(".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/broken_abs_symlink")
					expected.ToRelink = []string{"working_abs_symlink", "broken_abs_symlink"}

					check(expected)
					repair(expected, inst)
					checkHealty()
				})
			})
		}
	})
}

func TestUpgradeOldPkgDir(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given an old-style pkgs dir", t, func() {
		tempDir := mkTempDir()

		d := New(tempDir)
		trashDir := filepath.Join(tempDir, fs.SiteServiceDir, "trash")
		fs := fs.NewFileSystem(tempDir, trashDir)

		inst := makeTestInstance("test/package", nil, pkg.InstallModeSymlink)
		_, err := d.DeployInstance(ctx, "", inst)
		So(err, ShouldBeNil)

		currentLine := func(folder, inst string) string {
			if runtime.GOOS == "windows" {
				return fmt.Sprintf(".cipd/pkgs/%s/_current.txt", folder)
			}
			return fmt.Sprintf(".cipd/pkgs/%s/_current:%s", folder, inst)
		}

		pkg0 := filepath.Join(tempDir, ".cipd", "pkgs", "0")
		pkgOldStyle := filepath.Join(tempDir, ".cipd", "pkgs", "test_package-deadbeef")
		So(fs.EnsureFileGone(ctx, filepath.Join(pkg0, descriptionName)), ShouldBeNil)
		So(fs.Replace(ctx, pkg0, pkgOldStyle), ShouldBeNil)
		So(scanDir(tempDir), ShouldResemble, []string{
			".cipd/pkgs/test_package-deadbeef/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
			currentLine("test_package-deadbeef", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
			".cipd/tmp!",
		})

		Convey("reading the packages finds it", func() {
			pins, err := d.FindDeployed(ctx)
			So(err, ShouldBeNil)
			So(pins, ShouldResemble, PinSliceBySubdir{
				"": PinSlice{
					{"test/package", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"},
				},
			})

			Convey("and upgrades the package", func() {
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/test_package-deadbeef/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
					currentLine("test_package-deadbeef", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
					".cipd/pkgs/test_package-deadbeef/description.json",
					".cipd/tmp!",
				})
			})
		})

		Convey("can deploy new instance", func() {
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/test_package-deadbeef/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				currentLine("test_package-deadbeef", "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
				".cipd/pkgs/test_package-deadbeef/description.json",
				".cipd/tmp!",
			})
		})

		Convey("can deploy other package", func() {
			inst.packageName = "something/cool"
			inst.instanceID = "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				currentLine("0", "111111111_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/test_package-deadbeef/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				currentLine("test_package-deadbeef", "-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC"),
				".cipd/pkgs/test_package-deadbeef/description.json",
				".cipd/tmp!",
			})
		})

	})
}

func TestNumSet(t *testing.T) {
	t.Parallel()

	Convey("numSet", t, func() {
		ns := numSet{}

		Convey("can add numbers out of order", func() {
			for _, n := range []int{392, 1, 7, 29, 4} {
				ns.addNum(n)
			}
			So(ns, ShouldResemble, numSet{1, 4, 7, 29, 392})

			Convey("and rejects duplicates", func() {
				ns.addNum(7)
				So(ns, ShouldResemble, numSet{1, 4, 7, 29, 392})
			})
		})

		Convey("smallestNewNum", func() {
			ns = numSet{1, 4, 7, 29, 392}

			smallNums := []int{0, 2, 3, 5, 6, 8}
			for _, sn := range smallNums {
				So(ns.smallestNewNum(), ShouldEqual, sn)
				ns.addNum(sn)
			}
		})

	})
}

func TestResolveValidPackageDirs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("resolveValidPackageDirs", t, func() {
		tempDir := mkTempDir()
		d := New(tempDir).(*deployerImpl)
		pkgdir, err := d.fs.RootRelToAbs(filepath.FromSlash(packagesDir))
		So(err, ShouldBeNil)

		writeFiles := func(files ...fs.File) {
			for _, f := range files {
				name := filepath.Join(tempDir, f.Name())
				if f.Symlink() {
					targ, err := f.SymlinkTarget()
					So(err, ShouldBeNil)
					err = d.fs.EnsureSymlink(ctx, name, targ)
					So(err, ShouldBeNil)
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
					So(err, ShouldBeNil)
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
				So(err, ShouldBeNil)
				all[desc] = strings.Replace(filepath.Clean(rel), "\\", "/", -1)
			}
			return nums, all
		}

		Convey("packagesDir with just description.json", func() {
			writeFiles(
				desc("0", "", "some/package/name"),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet{0})
			So(all, ShouldResemble, map[description]string{
				{"", "some/package/name"}: ".cipd/pkgs/0",
			})
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/description.json",
			})
		})

		Convey("packagesDir with duplicates", func() {
			writeFiles(
				desc("0", "", "some/package/name"),
				desc("some_other", "", "some/package/name"),
				desc("1", "", "some/package/name"),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet{0})
			So(all, ShouldResemble, map[description]string{
				{"", "some/package/name"}: ".cipd/pkgs/0",
			})
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/description.json",
			})
		})

		Convey("bogus file", func() {
			writeFiles(
				fs.NewTestFile(".cipd/pkgs/wat", "hello", fs.TestFileOpts{}),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[description]string{})
			So(scanDir(tempDir), ShouldResemble, []string{".cipd/pkgs!"})
		})

		Convey("bad description.json", func() {
			writeFiles(
				fs.NewTestFile(".cipd/pkgs/0/description.json", "hello", fs.TestFileOpts{}),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[description]string{})
			So(scanDir(tempDir), ShouldResemble, []string{".cipd/pkgs!"})
		})

		Convey("package with no manifest", func() {
			writeFiles(
				fs.NewTestFile(".cipd/pkgs/0/deadbeef/something", "hello", fs.TestFileOpts{}),
				fs.NewTestSymlink(".cipd/pkgs/0/_current", "deadbeef"),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[description]string{})
			So(scanDir(tempDir), ShouldResemble, []string{".cipd/pkgs!"})
		})

		Convey("package with manifest", func() {
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
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[description]string{
				{"", "cool/cats"}: ".cipd/pkgs/oldskool",
			})
			linkExpect := curLink.Name()
			if curLink.Symlink() {
				targ, err := curLink.SymlinkTarget()
				So(err, ShouldBeNil)
				linkExpect = fmt.Sprintf("%s:%s", curLink.Name(), targ)
			}

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/oldskool/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/.cipdpkg/manifest.json",
				".cipd/pkgs/oldskool/-wEu41lw0_aOomrCDp4gKs0uClIlMg25S2j-UMHKwFYC/something",
				linkExpect,
				".cipd/pkgs/oldskool/description.json",
			})
		})

	})

}

func TestPackagePathCollision(t *testing.T) {
	t.Parallel()

	const threads = 50
	const packages = 4

	ctx := context.Background()

	Convey("Collide packagePath", t, func() {
		tempDir := mkTempDir()
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
				So(r.err, ShouldBeNil)
			} else {
				seen.Add(fmt.Sprintf("%s:%s", r.pkg, r.path))
			}
		}

		// Each 'pkg' got exactly one 'path'.
		So(seen.ToSlice(), ShouldHaveLength, packages)
	})
}

func TestRemoveEmptyTrees(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		absPath := func(rel string) string {
			return filepath.Join(tempDir, filepath.FromSlash(rel))
		}
		touch := func(rel string) {
			abs := absPath(rel)
			err := os.MkdirAll(filepath.Dir(abs), 0777)
			So(err, ShouldBeNil)
			f, err := os.Create(abs)
			So(err, ShouldBeNil)
			f.Close()
		}
		delete := func(rel string) {
			So(os.Remove(absPath(rel)), ShouldBeNil)
		}

		dirSet := func(rel ...string) stringset.Set {
			out := stringset.New(len(rel))
			for _, r := range rel {
				out.Add(absPath(r))
			}
			return out
		}

		Convey("Simple case", func() {
			touch("1/2/3/4")
			delete("1/2/3/4")
			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3"))
			So(scanDir(tempDir), ShouldResemble, []string{"1!"})
		})

		Convey("Non empty", func() {
			touch("1/2/3/4")
			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3"))
			So(scanDir(tempDir), ShouldResemble, []string{"1/2/3/4"})
		})

		Convey("Multiple empty", func() {
			touch("1/2/3a/4")
			touch("1/2/3b/4")

			delete("1/2/3a/4")
			delete("1/2/3b/4")

			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3a", "1/2/3b"))
			So(scanDir(tempDir), ShouldResemble, []string{"1!"})
		})

		Convey("Respects 'empty' set", func() {
			touch("1/2/3a/4")
			touch("1/2/3b/4")

			delete("1/2/3a/4")
			delete("1/2/3b/4")

			removeEmptyTrees(ctx, absPath("1/2"), dirSet("1/2/3b"))
			So(scanDir(tempDir), ShouldResemble, []string{"1/2/3a!"})
		})
	})
}

////////////////////////////////////////////////////////////////////////////////

type testPackageInstance struct {
	packageName string
	instanceID  string
	files       []fs.File
	installMode pkg.InstallMode
}

// makeTestInstance returns pkg.Instance implementation with mocked guts.
func makeTestInstance(name string, files []fs.File, installMode pkg.InstallMode) *testPackageInstance {
	// Generate and append manifest file.
	out := bytes.Buffer{}
	err := pkg.WriteManifest(&pkg.Manifest{
		FormatVersion: pkg.ManifestFormatVersion,
		PackageName:   name,
		InstallMode:   installMode,
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
func (f *testPackageInstance) Source() io.ReadSeeker             { panic("Not implemented") }
func (f *testPackageInstance) Close(context.Context, bool) error { return nil }

////////////////////////////////////////////////////////////////////////////////

// scanDir returns list of files (regular, symlinks and directories if they are
// empty) it finds in a directory. Symlinks are returned as "path:target".
// Empty directories are suffixed with '!'. Regular executable files are
// suffixed with '*'. All paths are relative to the scanned directory and slash
// separated. Symlink targets are slash separated too, but otherwise not
// modified. Does not look inside symlinked directories.
func scanDir(root string) (out []string) {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		switch {
		case err != nil:
			return err
		case rel == ".":
			return nil
		case info.Mode().IsDir() && !isEmptyDir(path):
			return nil
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

// isEmptyDir return true if 'path' refers to an empty directory.
func isEmptyDir(path string) bool {
	infos, err := ioutil.ReadDir(path)
	return err == nil && len(infos) == 0
}

// readFile reads content of an existing text file. Root path is provided as
// a native path, rel - as a slash-separated path.
func readFile(root, rel string) string {
	body, err := ioutil.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	So(err, ShouldBeNil)
	return string(body)
}
