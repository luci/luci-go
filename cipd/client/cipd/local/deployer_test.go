// Copyright 2014 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package local

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/cipd/client/cipd/common"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func mkTempDir() string {
	tempDir, err := ioutil.TempDir("", "cipd_test")
	So(err, ShouldBeNil)
	Reset(func() { os.RemoveAll(tempDir) })
	return tempDir
}

func TestUtilities(t *testing.T) {
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
	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("Try to deploy package instance with bad package name", func() {
			_, err := NewDeployer(tempDir).DeployInstance(
				ctx, "", makeTestInstance("../test/package", nil, InstallModeCopy))
			So(err, ShouldErrLike, "invalid package name")
		})

		Convey("Try to deploy package instance with bad instance ID", func() {
			inst := makeTestInstance("test/package", nil, InstallModeCopy)
			inst.instanceID = "../000000000"
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldErrLike, "not a valid package instance ID")
		})

		Convey("Try to deploy package instance in bad subdir", func() {
			inst := makeTestInstance("test/package", nil, InstallModeCopy)
			inst.instanceID = "../000000000"
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "/abspath", inst)
			So(err, ShouldErrLike, "bad subdir")
		})
	})
}

func TestDeployInstanceSymlinkMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no symlinks")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("DeployInstance new empty package instance", func() {
			inst := makeTestInstance("test/package", nil, InstallModeSymlink)
			info, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, inst.Pin())
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
				".cipd/pkgs/0/description.json",
			})
			fInfo, err := os.Stat(filepath.Join(tempDir, ".cipd", "pkgs", "0"))
			So(err, ShouldBeNil)
			So(fInfo.Mode(), ShouldEqual, os.FileMode(0755)|os.ModeDir)

			Convey("in subdir", func() {
				info, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(info, ShouldResemble, inst.Pin())
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/1/description.json",
				})
			})
		})

		Convey("DeployInstance new non-empty package instance", func() {
			inst := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a", false),
				NewTestFile("some/executable", "data b", true),
				NewTestSymlink("some/symlink", "executable"),
			}, InstallModeSymlink)
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/executable*",
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/file/path",
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/symlink:executable",
				".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
				".cipd/pkgs/0/description.json",
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
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/executable*",
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/file/path",
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/symlink:executable",
					".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/some/executable*",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/some/file/path",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/some/symlink:executable",
					".cipd/pkgs/1/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/1/description.json",
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
			inst := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a", false),
				NewTestFile("some/executable", "data b", true),
				NewTestSymlink("some/symlink", "executable"),
			}, InstallModeSymlink)
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/executable*",
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/file/path",
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/symlink:executable",
				".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
				".cipd/pkgs/0/description.json",
				"some/executable:../.cipd/pkgs/0/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
				"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
			})

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/executable*",
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/file/path",
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/some/symlink:executable",
					".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/some/executable*",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/some/file/path",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/some/symlink:executable",
					".cipd/pkgs/1/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/1/description.json",
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
			oldPkg := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a old", false),
				NewTestFile("some/executable", "data b old", true),
				NewTestFile("old only", "data c old", true),
				NewTestFile("mode change 1", "data d", true),
				NewTestFile("mode change 2", "data e", false),
				NewTestSymlink("symlink unchanged", "target"),
				NewTestSymlink("symlink changed", "old target"),
				NewTestSymlink("symlink removed", "target"),
			}, InstallModeSymlink)
			oldPkg.instanceID = "0000000000000000000000000000000000000000"

			newPkg := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a new", false),
				NewTestFile("some/executable", "data b new", true),
				NewTestFile("mode change 1", "data d", false),
				NewTestFile("mode change 2", "data d", true),
				NewTestSymlink("symlink unchanged", "target"),
				NewTestSymlink("symlink changed", "new target"),
			}, InstallModeSymlink)
			newPkg.instanceID = "1111111111111111111111111111111111111111"

			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", oldPkg)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", newPkg)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/mode change 1",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/mode change 2*",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/some/executable*",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/some/file/path",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/symlink changed:new target",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/symlink unchanged:target",
				".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
				".cipd/pkgs/0/description.json",
				"mode change 1:.cipd/pkgs/0/_current/mode change 1",
				"mode change 2:.cipd/pkgs/0/_current/mode change 2",
				"some/executable:../.cipd/pkgs/0/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
				"symlink changed:.cipd/pkgs/0/_current/symlink changed",
				"symlink unchanged:.cipd/pkgs/0/_current/symlink unchanged",
			})

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", oldPkg)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", newPkg)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/mode change 1",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/mode change 2*",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/some/executable*",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/some/file/path",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/symlink changed:new target",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/symlink unchanged:target",
					".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/mode change 1",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/mode change 2*",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/some/executable*",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/some/file/path",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/symlink changed:new target",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/symlink unchanged:target",
					".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/1/description.json",
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
			pkg1 := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a old", false),
				NewTestFile("some/executable", "data b old", true),
				NewTestFile("pkg1 file", "data c", false),
			}, InstallModeSymlink)
			pkg1.instanceID = "0000000000000000000000000000000000000000"

			// Nesting in package names is allowed.
			pkg2 := makeTestInstance("test/package/another", []File{
				NewTestFile("some/file/path", "data a new", false),
				NewTestFile("some/executable", "data b new", true),
				NewTestFile("pkg2 file", "data d", false),
			}, InstallModeSymlink)
			pkg2.instanceID = "1111111111111111111111111111111111111111"

			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", pkg1)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", pkg2)
			So(err, ShouldBeNil)

			// TODO: Conflicting symlinks point to last installed package, it is not
			// very deterministic.
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/0000000000000000000000000000000000000000/pkg1 file",
				".cipd/pkgs/0/0000000000000000000000000000000000000000/some/executable*",
				".cipd/pkgs/0/0000000000000000000000000000000000000000/some/file/path",
				".cipd/pkgs/0/_current:0000000000000000000000000000000000000000",
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/1/1111111111111111111111111111111111111111/pkg2 file",
				".cipd/pkgs/1/1111111111111111111111111111111111111111/some/executable*",
				".cipd/pkgs/1/1111111111111111111111111111111111111111/some/file/path",
				".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
				".cipd/pkgs/1/description.json",
				"pkg1 file:.cipd/pkgs/0/_current/pkg1 file",
				"pkg2 file:.cipd/pkgs/1/_current/pkg2 file",
				"some/executable:../.cipd/pkgs/1/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/1/_current/some/file/path",
			})

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", pkg1)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", pkg2)
				So(err, ShouldBeNil)

				// TODO: Conflicting symlinks point to last installed package, it is not
				// very deterministic.
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/0000000000000000000000000000000000000000/pkg1 file",
					".cipd/pkgs/0/0000000000000000000000000000000000000000/some/executable*",
					".cipd/pkgs/0/0000000000000000000000000000000000000000/some/file/path",
					".cipd/pkgs/0/_current:0000000000000000000000000000000000000000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/pkg2 file",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/some/executable*",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/some/file/path",
					".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/1/description.json",
					".cipd/pkgs/2/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
					".cipd/pkgs/2/0000000000000000000000000000000000000000/pkg1 file",
					".cipd/pkgs/2/0000000000000000000000000000000000000000/some/executable*",
					".cipd/pkgs/2/0000000000000000000000000000000000000000/some/file/path",
					".cipd/pkgs/2/_current:0000000000000000000000000000000000000000",
					".cipd/pkgs/2/description.json",
					".cipd/pkgs/3/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/3/1111111111111111111111111111111111111111/pkg2 file",
					".cipd/pkgs/3/1111111111111111111111111111111111111111/some/executable*",
					".cipd/pkgs/3/1111111111111111111111111111111111111111/some/file/path",
					".cipd/pkgs/3/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/3/description.json",
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
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("DeployInstance new empty package instance", func() {
			inst := makeTestInstance("test/package", nil, InstallModeCopy)
			info, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, inst.Pin())
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
				".cipd/pkgs/0/description.json",
			})

			Convey("in subdir", func() {
				inst := makeTestInstance("test/package", nil, InstallModeCopy)
				info, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(info, ShouldResemble, inst.Pin())
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/1/description.json",
				})
			})
		})

		Convey("DeployInstance new non-empty package instance", func() {
			inst := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a", false),
				NewTestFile("some/executable", "data b", true),
				NewTestSymlink("some/symlink", "executable"),
			}, InstallModeCopy)
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
				".cipd/pkgs/0/description.json",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			})

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/1/description.json",
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
			inst := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a", false),
				NewTestFile("some/executable", "data b", true),
				NewTestSymlink("some/symlink", "executable"),
			}, InstallModeCopy)
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
				".cipd/pkgs/0/description.json",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			})

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "somedir", inst)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "somedir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/1/description.json",
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
			oldPkg := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a old", false),
				NewTestFile("some/executable", "data b old", true),
				NewTestFile("old only", "data c old", true),
				NewTestFile("mode change 1", "data d", true),
				NewTestFile("mode change 2", "data e", false),
				NewTestSymlink("symlink unchanged", "target"),
				NewTestSymlink("symlink changed", "old target"),
				NewTestSymlink("symlink removed", "target"),
			}, InstallModeCopy)
			oldPkg.instanceID = "0000000000000000000000000000000000000000"

			newPkg := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a new", false),
				NewTestFile("some/executable", "data b new", true),
				NewTestFile("mode change 1", "data d", false),
				NewTestFile("mode change 2", "data d", true),
				NewTestSymlink("symlink unchanged", "target"),
				NewTestSymlink("symlink changed", "new target"),
			}, InstallModeCopy)
			newPkg.instanceID = "1111111111111111111111111111111111111111"

			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", oldPkg)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", newPkg)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
				".cipd/pkgs/0/description.json",
				"mode change 1",
				"mode change 2*",
				"some/executable*",
				"some/file/path",
				"symlink changed:new target",
				"symlink unchanged:target",
			})

			Convey("in subdir", func() {

				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", oldPkg)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", newPkg)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/1/description.json",
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
			pkg1 := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a old", false),
				NewTestFile("some/executable", "data b old", true),
				NewTestFile("pkg1 file", "data c", false),
			}, InstallModeCopy)
			pkg1.instanceID = "0000000000000000000000000000000000000000"

			// Nesting in package names is allowed.
			pkg2 := makeTestInstance("test/package/another", []File{
				NewTestFile("some/file/path", "data a new", false),
				NewTestFile("some/executable", "data b new", true),
				NewTestFile("pkg2 file", "data d", false),
			}, InstallModeCopy)
			pkg2.instanceID = "1111111111111111111111111111111111111111"

			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", pkg1)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", pkg2)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:0000000000000000000000000000000000000000",
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
				".cipd/pkgs/1/description.json",
				"pkg1 file",
				"pkg2 file",
				"some/executable*",
				"some/file/path",
			})

			Convey("in subdir", func() {

				_, err := NewDeployer(tempDir).DeployInstance(ctx, "somedir", pkg1)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "somedir", pkg2)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:0000000000000000000000000000000000000000",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/1/description.json",
					".cipd/pkgs/2/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current:0000000000000000000000000000000000000000",
					".cipd/pkgs/2/description.json",
					".cipd/pkgs/3/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/3/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/3/description.json",
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
	if runtime.GOOS != "windows" {
		t.Skip("Skipping on posix")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("DeployInstance new empty package instance", func() {
			inst := makeTestInstance("test/package", nil, InstallModeCopy)
			info, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, inst.Pin())
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")

			Convey("in subdir", func() {
				info, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(info, ShouldResemble, inst.Pin())
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
				})
				cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")
			})
		})

		Convey("DeployInstance new non-empty package instance", func() {
			inst := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a", false),
				NewTestFile("some/executable", "data b", true),
			}, InstallModeCopy)
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				"some/executable",
				"some/file/path",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					"some/executable",
					"some/file/path",
					"subdir/some/executable",
					"subdir/some/file/path",
				})
				cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")
			})
		})

		Convey("Redeploy same package instance", func() {
			inst := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a", false),
				NewTestFile("some/executable", "data b", true),
			}, InstallModeCopy)
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				"some/executable",
				"some/file/path",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					"some/executable",
					"some/file/path",
					"subdir/some/executable",
					"subdir/some/file/path",
				})
				cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "0123456789abcdef00000123456789abcdef0000")
			})
		})

		Convey("DeployInstance package update", func() {
			oldPkg := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a old", false),
				NewTestFile("some/executable", "data b old", true),
				NewTestFile("old only", "data c old", true),
				NewTestFile("mode change 1", "data d", true),
				NewTestFile("mode change 2", "data e", false),
			}, InstallModeCopy)
			oldPkg.instanceID = "0000000000000000000000000000000000000000"

			newPkg := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a new", false),
				NewTestFile("some/executable", "data b new", true),
				NewTestFile("mode change 1", "data d", false),
				NewTestFile("mode change 2", "data d", true),
			}, InstallModeCopy)
			newPkg.instanceID = "1111111111111111111111111111111111111111"

			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", oldPkg)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", newPkg)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				"mode change 1",
				"mode change 2",
				"some/executable",
				"some/file/path",
			})
			cur := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur, ShouldEqual, "1111111111111111111111111111111111111111")

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", oldPkg)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", newPkg)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
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
				So(cur, ShouldEqual, "1111111111111111111111111111111111111111")
				cur = readFile(tempDir, ".cipd/pkgs/1/_current.txt")
				So(cur, ShouldEqual, "1111111111111111111111111111111111111111")
			})
		})

		Convey("DeployInstance two different packages", func() {
			pkg1 := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path", "data a old", false),
				NewTestFile("some/executable", "data b old", true),
				NewTestFile("pkg1 file", "data c", false),
			}, InstallModeCopy)
			pkg1.instanceID = "0000000000000000000000000000000000000000"

			// Nesting in package names is allowed.
			pkg2 := makeTestInstance("test/package/another", []File{
				NewTestFile("some/file/path", "data a new", false),
				NewTestFile("some/executable", "data b new", true),
				NewTestFile("pkg2 file", "data d", false),
			}, InstallModeCopy)
			pkg2.instanceID = "1111111111111111111111111111111111111111"

			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", pkg1)
			So(err, ShouldBeNil)
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", pkg2)
			So(err, ShouldBeNil)

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/1/_current.txt",
				".cipd/pkgs/1/description.json",
				"pkg1 file",
				"pkg2 file",
				"some/executable",
				"some/file/path",
			})
			cur1 := readFile(tempDir, ".cipd/pkgs/1/_current.txt")
			So(cur1, ShouldEqual, "1111111111111111111111111111111111111111")
			cur2 := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
			So(cur2, ShouldEqual, "0000000000000000000000000000000000000000")

			Convey("in subdir", func() {
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", pkg1)
				So(err, ShouldBeNil)
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", pkg2)
				So(err, ShouldBeNil)

				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current.txt",
					".cipd/pkgs/1/description.json",
					".cipd/pkgs/2/0000000000000000000000000000000000000000/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current.txt",
					".cipd/pkgs/2/description.json",
					".cipd/pkgs/3/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/3/_current.txt",
					".cipd/pkgs/3/description.json",
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
				So(cur1, ShouldEqual, "1111111111111111111111111111111111111111")
				cur2 := readFile(tempDir, ".cipd/pkgs/0/_current.txt")
				So(cur2, ShouldEqual, "0000000000000000000000000000000000000000")
			})
		})
	})
}

func TestDeployInstanceSwitchingModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no symlinks")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		files := []File{
			NewTestFile("some/file/path", "data a", false),
			NewTestFile("some/executable", "data b", true),
			NewTestSymlink("some/symlink", "executable"),
		}

		Convey("InstallModeCopy => InstallModeSymlink", func() {
			inst := makeTestInstance("test/package", files, InstallModeCopy)
			inst.instanceID = "0000000000000000000000000000000000000000"
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			inst = makeTestInstance("test/package", files, InstallModeSymlink)
			inst.instanceID = "1111111111111111111111111111111111111111"
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", inst)

			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/some/executable*",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/some/file/path",
				".cipd/pkgs/0/1111111111111111111111111111111111111111/some/symlink:executable",
				".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
				".cipd/pkgs/0/description.json",
				"some/executable:../.cipd/pkgs/0/_current/some/executable",
				"some/file/path:../../.cipd/pkgs/0/_current/some/file/path",
				"some/symlink:../.cipd/pkgs/0/_current/some/symlink",
			})

			Convey("in subidr", func() {
				inst := makeTestInstance("test/package", files, InstallModeCopy)
				inst.instanceID = "0000000000000000000000000000000000000000"
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)

				inst = makeTestInstance("test/package", files, InstallModeSymlink)
				inst.instanceID = "1111111111111111111111111111111111111111"
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)

				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/some/executable*",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/some/file/path",
					".cipd/pkgs/0/1111111111111111111111111111111111111111/some/symlink:executable",
					".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/some/executable*",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/some/file/path",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/some/symlink:executable",
					".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/1/description.json",
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
			inst := makeTestInstance("test/package", files, InstallModeSymlink)
			inst.instanceID = "0000000000000000000000000000000000000000"
			_, err := NewDeployer(tempDir).DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			inst = makeTestInstance("test/package", files, InstallModeCopy)
			inst.instanceID = "1111111111111111111111111111111111111111"
			_, err = NewDeployer(tempDir).DeployInstance(ctx, "", inst)

			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
				".cipd/pkgs/0/description.json",
				"some/executable*",
				"some/file/path",
				"some/symlink:executable",
			})

			Convey("in subdir", func() {
				inst := makeTestInstance("test/package", files, InstallModeSymlink)
				inst.instanceID = "0000000000000000000000000000000000000000"
				_, err := NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)
				So(err, ShouldBeNil)

				inst = makeTestInstance("test/package", files, InstallModeCopy)
				inst.instanceID = "1111111111111111111111111111111111111111"
				_, err = NewDeployer(tempDir).DeployInstance(ctx, "subdir", inst)

				So(err, ShouldBeNil)
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/1/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
					".cipd/pkgs/1/_current:1111111111111111111111111111111111111111",
					".cipd/pkgs/1/description.json",
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

func TestFindDeployed(t *testing.T) {
	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("FindDeployed works with empty dir", func() {
			out, err := NewDeployer(tempDir).FindDeployed(ctx)
			So(err, ShouldBeNil)
			So(out, ShouldBeNil)
		})

		Convey("FindDeployed works", func() {
			d := NewDeployer(tempDir)

			// Deploy a bunch of stuff.
			_, err := d.DeployInstance(ctx, "", makeTestInstance("test/pkg/123", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test/pkg/456", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test/pkg", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "", makeTestInstance("test", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg/123", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg/456", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test/pkg", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			_, err = d.DeployInstance(ctx, "subdir", makeTestInstance("test", nil, InstallModeCopy))
			So(err, ShouldBeNil)

			// including some broken packages
			_, err = d.DeployInstance(ctx, "", makeTestInstance("broken", nil, InstallModeCopy))
			So(err, ShouldBeNil)
			if runtime.GOOS == "windows" {
				err = os.Remove(filepath.Join(tempDir, SiteServiceDir, "pkgs", "8", "_current.txt"))
			} else {
				err = os.Remove(filepath.Join(tempDir, SiteServiceDir, "pkgs", "8", "_current"))
			}
			So(err, ShouldBeNil)

			// Verify it is discoverable.
			out, err := d.FindDeployed(ctx)
			So(err, ShouldBeNil)
			So(out, ShouldResemble, PinSliceBySubdir{
				"": PinSlice{
					{"test", "0123456789abcdef00000123456789abcdef0000"},
					{"test/pkg", "0123456789abcdef00000123456789abcdef0000"},
					{"test/pkg/123", "0123456789abcdef00000123456789abcdef0000"},
					{"test/pkg/456", "0123456789abcdef00000123456789abcdef0000"},
				},
				"subdir": PinSlice{
					{"test", "0123456789abcdef00000123456789abcdef0000"},
					{"test/pkg", "0123456789abcdef00000123456789abcdef0000"},
					{"test/pkg/123", "0123456789abcdef00000123456789abcdef0000"},
					{"test/pkg/456", "0123456789abcdef00000123456789abcdef0000"},
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
			err := NewDeployer(tempDir).RemoveDeployed(ctx, "", "package/path")
			So(err, ShouldBeNil)
			err = NewDeployer(tempDir).RemoveDeployed(ctx, "subdir", "package/path")
			So(err, ShouldBeNil)
		})
	})
}

func TestRemoveDeployedPosix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on windows")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("RemoveDeployed works", func() {
			d := NewDeployer(tempDir)

			// Deploy some instance (to keep it).
			inst := makeTestInstance("test/package/123", []File{
				NewTestFile("some/file/path1", "data a", false),
				NewTestFile("some/executable1", "data b", true),
			}, InstallModeCopy)
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			// Deploy another instance (to remove it).
			inst2 := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path2", "data a", false),
				NewTestFile("some/executable2", "data b", true),
				NewTestSymlink("some/symlink", "executable"),
			}, InstallModeCopy)
			_, err = d.DeployInstance(ctx, "", inst2)
			So(err, ShouldBeNil)

			// Now remove the second package.
			err = d.RemoveDeployed(ctx, "", "test/package")
			So(err, ShouldBeNil)

			// Verify the final state (only first package should survive).
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
				".cipd/pkgs/0/description.json",
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
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/0/description.json",
					// it's 2 because we flipped inst2 and inst in the installation order.
					// When we RemoveDeployed, we remove index 1.
					".cipd/pkgs/2/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current:0123456789abcdef00000123456789abcdef0000",
					".cipd/pkgs/2/description.json",
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
	if runtime.GOOS != "windows" {
		t.Skip("Skipping on posix")
	}

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir := mkTempDir()

		Convey("RemoveDeployed works", func() {
			d := NewDeployer(tempDir)

			// Deploy some instance (to keep it).
			inst := makeTestInstance("test/package/123", []File{
				NewTestFile("some/file/path1", "data a", false),
				NewTestFile("some/executable1", "data b", true),
			}, InstallModeCopy)
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)

			// Deploy another instance (to remove it).
			inst2 := makeTestInstance("test/package", []File{
				NewTestFile("some/file/path2", "data a", false),
				NewTestFile("some/executable2", "data b", true),
			}, InstallModeCopy)
			_, err = d.DeployInstance(ctx, "", inst2)
			So(err, ShouldBeNil)

			// Now remove the second package.
			err = d.RemoveDeployed(ctx, "", "test/package")
			So(err, ShouldBeNil)

			// Verify the final state (only first package should survive).
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/0/_current.txt",
				".cipd/pkgs/0/description.json",
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
					".cipd/pkgs/0/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/0/_current.txt",
					".cipd/pkgs/0/description.json",
					".cipd/pkgs/2/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					".cipd/pkgs/2/_current.txt",
					".cipd/pkgs/2/description.json",
					"some/executable1",
					"some/file/path1",
					"subdir/some/executable1",
					"subdir/some/file/path1",
				})
			})
		})
	})
}

func TestUpgradeOldPkgDir(t *testing.T) {
	ctx := context.Background()

	Convey("Given an old-style pkgs dir", t, func() {
		tempDir := mkTempDir()

		d := NewDeployer(tempDir)
		trashDir := filepath.Join(tempDir, SiteServiceDir, "trash")
		fs := NewFileSystem(tempDir, trashDir)

		inst := makeTestInstance("test/package", nil, InstallModeSymlink)
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
			".cipd/pkgs/test_package-deadbeef/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
			currentLine("test_package-deadbeef", "0123456789abcdef00000123456789abcdef0000"),
		})

		Convey("reading the packages finds it", func() {
			pins, err := d.FindDeployed(ctx)
			So(err, ShouldBeNil)
			So(pins, ShouldResemble, PinSliceBySubdir{
				"": PinSlice{
					{"test/package", "0123456789abcdef00000123456789abcdef0000"},
				},
			})

			Convey("and upgrades the package", func() {
				So(scanDir(tempDir), ShouldResemble, []string{
					".cipd/pkgs/test_package-deadbeef/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					currentLine("test_package-deadbeef", "0123456789abcdef00000123456789abcdef0000"),
					".cipd/pkgs/test_package-deadbeef/description.json",
				})
			})
		})

		Convey("can deploy new instance", func() {
			inst.instanceID = "1111111111111111111111111111111111111111"
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/test_package-deadbeef/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				currentLine("test_package-deadbeef", "1111111111111111111111111111111111111111"),
				".cipd/pkgs/test_package-deadbeef/description.json",
			})
		})

		Convey("can deploy other package", func() {
			inst.packageName = "something/cool"
			inst.instanceID = "1111111111111111111111111111111111111111"
			_, err := d.DeployInstance(ctx, "", inst)
			So(err, ShouldBeNil)
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/1111111111111111111111111111111111111111/.cipdpkg/manifest.json",
				currentLine("0", "1111111111111111111111111111111111111111"),
				".cipd/pkgs/0/description.json",
				".cipd/pkgs/test_package-deadbeef/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				currentLine("test_package-deadbeef", "0123456789abcdef00000123456789abcdef0000"),
				".cipd/pkgs/test_package-deadbeef/description.json",
			})
		})

	})
}

func TestNumSet(t *testing.T) {
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
	ctx := context.Background()

	Convey("resolveValidPackageDirs", t, func() {
		tempDir := mkTempDir()
		d := NewDeployer(tempDir).(*deployerImpl)
		pkgdir, err := d.fs.RootRelToAbs(filepath.FromSlash(packagesDir))
		So(err, ShouldBeNil)

		writeFiles := func(files ...File) {
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
		desc := func(pkgFolder, subdir, packageName string) File {
			return NewTestFile(
				fmt.Sprintf(".cipd/pkgs/%s/description.json", pkgFolder),
				fmt.Sprintf(`{"subdir": %q, "package_name": %q}`, subdir, packageName),
				false,
			)
		}
		resolve := func() (numSet, map[Description]string) {
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
			So(all, ShouldResemble, map[Description]string{
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
			So(all, ShouldResemble, map[Description]string{
				{"", "some/package/name"}: ".cipd/pkgs/0",
			})
			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/0/description.json",
			})
		})

		Convey("bogus file", func() {
			writeFiles(
				NewTestFile(".cipd/pkgs/wat", "hello", false),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[Description]string{})
			So(scanDir(tempDir), ShouldResemble, []string(nil))
		})

		Convey("bad description.json", func() {
			writeFiles(
				NewTestFile(".cipd/pkgs/0/description.json", "hello", false),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[Description]string{})
			So(scanDir(tempDir), ShouldResemble, []string(nil))
		})

		Convey("package with no manifest", func() {
			writeFiles(
				NewTestFile(".cipd/pkgs/0/deadbeef/something", "hello", false),
				NewTestSymlink(".cipd/pkgs/0/_current", "deadbeef"),
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[Description]string{})
			So(scanDir(tempDir), ShouldResemble, []string(nil))
		})

		Convey("package with manifest", func() {
			curLink := NewTestSymlink(".cipd/pkgs/oldskool/_current", "0123456789abcdef00000123456789abcdef0000")
			if runtime.GOOS == "windows" {
				curLink = NewTestFile(".cipd/pkgs/oldskool/_current.txt", "0123456789abcdef00000123456789abcdef0000", false)
			}
			writeFiles(
				NewTestFile(".cipd/pkgs/oldskool/0123456789abcdef00000123456789abcdef0000/something", "hello", false),
				NewTestFile(".cipd/pkgs/oldskool/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
					`{"format_version": "1", "package_name": "cool/cats"}`, false),
				curLink,
			)
			nums, all := resolve()
			So(nums, ShouldResemble, numSet(nil))
			So(all, ShouldResemble, map[Description]string{
				{"", "cool/cats"}: ".cipd/pkgs/oldskool",
			})
			linkExpect := curLink.Name()
			if curLink.Symlink() {
				targ, err := curLink.SymlinkTarget()
				So(err, ShouldBeNil)
				linkExpect = fmt.Sprintf("%s:%s", curLink.Name(), targ)
			}

			So(scanDir(tempDir), ShouldResemble, []string{
				".cipd/pkgs/oldskool/0123456789abcdef00000123456789abcdef0000/.cipdpkg/manifest.json",
				".cipd/pkgs/oldskool/0123456789abcdef00000123456789abcdef0000/something",
				linkExpect,
				".cipd/pkgs/oldskool/description.json",
			})
		})

	})

}

////////////////////////////////////////////////////////////////////////////////

type testPackageInstance struct {
	packageName string
	instanceID  string
	files       []File
	installMode InstallMode
}

// makeTestInstance returns PackageInstance implementation with mocked guts.
func makeTestInstance(name string, files []File, installMode InstallMode) *testPackageInstance {
	// Generate and append manifest file.
	out := bytes.Buffer{}
	err := writeManifest(&Manifest{
		FormatVersion: manifestFormatVersion,
		PackageName:   name,
		InstallMode:   installMode,
	}, &out)
	if err != nil {
		panic("Failed to write a manifest")
	}
	files = append(files, NewTestFile(manifestName, string(out.Bytes()), false))
	return &testPackageInstance{
		packageName: name,
		instanceID:  "0123456789abcdef00000123456789abcdef0000",
		files:       files,
	}
}

func (f *testPackageInstance) Close() error              { return nil }
func (f *testPackageInstance) Pin() Pin                  { return Pin{f.packageName, f.instanceID} }
func (f *testPackageInstance) Files() []File             { return f.files }
func (f *testPackageInstance) DataReader() io.ReadSeeker { panic("Not implemented") }

////////////////////////////////////////////////////////////////////////////////

// scanDir returns list of files (regular and symlinks) it finds in a directory.
// Symlinks are returned as "path:target". Regular executable files are suffixed
// with '*'. All paths are relative to the scanned directory and slash
// separated. Symlink targets are slash separated too, but otherwise not
// modified. Does not look inside symlinked directories.
func scanDir(root string) (out []string) {
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.Mode().IsDir() {
			return nil
		}

		rel = filepath.ToSlash(rel)
		target, err := os.Readlink(path)
		var item string
		if err == nil {
			item = fmt.Sprintf("%s:%s", rel, filepath.ToSlash(target))
		} else {
			if info.Mode().IsRegular() {
				item = rel
			} else {
				item = fmt.Sprintf("%s:??????", rel)
			}
		}

		suffix := ""
		if info.Mode().IsRegular() && (info.Mode().Perm()&0100) != 0 {
			suffix = "*"
		}

		out = append(out, item+suffix)
		return nil
	})
	if err != nil {
		panic("Failed to walk a directory")
	}
	return
}

// readFile reads content of an existing text file. Root path is provided as
// a native path, rel - as a slash-separated path.
func readFile(root, rel string) string {
	body, err := ioutil.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	So(err, ShouldBeNil)
	return string(body)
}
