// Copyright 2017 The LUCI Authors.
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

package filesystem

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.chromium.org/luci/common/errors"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestIsNotExist(t *testing.T) {
	t.Parallel()

	tdir := t.TempDir()
	dnePath := filepath.Join(tdir, "anything")
	_, err := os.Open(dnePath)
	if !os.IsNotExist(err) {
		t.Fatalf("failed to get IsNotExist error: %s", err)
	}

	Convey(`IsNotExist works`, t, func() {
		So(IsNotExist(nil), ShouldBeFalse)
		So(IsNotExist(errors.New("something")), ShouldBeFalse)

		So(IsNotExist(err), ShouldBeTrue)
		So(IsNotExist(errors.Annotate(err, "annotated").Err()), ShouldBeTrue)
	})
}

func TestAbsPath(t *testing.T) {
	t.Parallel()

	tdir := t.TempDir()
	tdirName := filepath.Base(tdir)

	Convey(`AbsPath works`, t, func() {
		base := filepath.Join(tdir, "..", tdirName, "file.txt")
		So(AbsPath(&base), ShouldBeNil)
		So(base, ShouldEqual, filepath.Join(tdir, "file.txt"))
	})
}

func TestTouch(t *testing.T) {
	t.Parallel()

	Convey(`Testing Touch`, t, func() {
		tdir := t.TempDir()

		thePast := time.Now().Add(-10 * time.Second)

		Convey(`Can update a directory timestamp`, func() {
			path := filepath.Join(tdir, "subdir")

			So(os.Mkdir(path, 0755), ShouldBeNil)
			st, err := os.Lstat(path)
			So(err, ShouldBeNil)
			initialModTime := st.ModTime()

			So(Touch(path, thePast, 0), ShouldBeNil)
			st, err = os.Lstat(path)
			So(err, ShouldBeNil)

			So(st.ModTime(), ShouldHappenBefore, initialModTime)
		})

		Convey(`Can update an empty file timestamp`, func() {
			path := filepath.Join(tdir, "touch")

			So(Touch(path, time.Time{}, 0644), ShouldBeNil)
			st, err := os.Lstat(path)
			So(err, ShouldBeNil)
			initialModTime := st.ModTime()

			So(Touch(path, thePast, 0), ShouldBeNil)
			st, err = os.Lstat(path)
			So(err, ShouldBeNil)
			pastModTime := st.ModTime()
			So(pastModTime, ShouldHappenBefore, initialModTime)

			// Touch back to "now".
			So(Touch(path, time.Time{}, 0644), ShouldBeNil)
			st, err = os.Lstat(path)
			So(err, ShouldBeNil)
			So(st.ModTime(), ShouldHappenOnOrAfter, initialModTime)
		})

		Convey(`Can update a populated file timestamp`, func() {
			path := filepath.Join(tdir, "touch")

			So(os.WriteFile(path, []byte("sup"), 0644), ShouldBeNil)
			st, err := os.Lstat(path)
			So(err, ShouldBeNil)
			initialModTime := st.ModTime()

			So(Touch(path, thePast, 0), ShouldBeNil)
			st, err = os.Lstat(path)
			So(err, ShouldBeNil)

			So(st.ModTime(), ShouldHappenBefore, initialModTime)

			content, err := os.ReadFile(path)
			So(err, ShouldBeNil)
			So(content, ShouldResemble, []byte("sup"))
		})
	})
}

func TestMakeReadOnly(t *testing.T) {
	t.Parallel()

	Convey(`Can RemoveAll`, t, func() {
		tdir := t.TempDir()
		defer func() {
			So(recursiveChmod(tdir, nil, func(mode os.FileMode) os.FileMode { return mode | 0222 }), ShouldBeNil)
		}()
		for _, path := range []string{
			filepath.Join(tdir, "foo", "bar"),
			filepath.Join(tdir, "foo", "baz"),
			filepath.Join(tdir, "qux"),
		} {
			base := filepath.Dir(path)
			if err := os.MkdirAll(base, 0755); err != nil {
				t.Fatalf("failed to populate directory [%s]: %s", base, err)
			}
			if err := os.WriteFile(path, []byte("junk"), 0644); err != nil {
				t.Fatalf("failed to create file [%s]: %s", path, err)
			}
		}

		Convey(`Can mark the entire subdirectory read-only`, func() {
			So(MakeReadOnly(tdir, nil), ShouldBeNil)
			filepath.Walk(tdir, func(path string, info os.FileInfo, err error) error {
				So(err, ShouldBeNil)
				So(info.Mode()&0222, ShouldEqual, 0)
				return nil
			})
		})

		Convey(`Can selectively mark files read-only`, func() {
			// Don't mark <TMP>/foo read-only.
			fooPath := filepath.Join(tdir, "foo")

			So(MakeReadOnly(tdir, func(path string) bool {
				return path != fooPath
			}), ShouldBeNil)

			filepath.Walk(tdir, func(path string, info os.FileInfo, err error) error {
				So(err, ShouldBeNil)
				if path == fooPath {
					So(info.Mode()&0222, ShouldNotEqual, 0)
				} else {
					So(info.Mode()&0222, ShouldEqual, 0)
				}
				return nil
			})
		})
	})
}

func TestRemoveAll(t *testing.T) {
	t.Parallel()

	Convey(`Can RemoveAll`, t, func() {
		tdir := t.TempDir()
		defer func() {
			So(recursiveChmod(tdir, nil, func(mode os.FileMode) os.FileMode { return mode | 0222 }), ShouldBeNil)
		}()
		subDir := filepath.Join(tdir, "sub")

		Convey(`With directory contents...`, func() {
			for _, path := range []string{
				filepath.Join(subDir, "foo", "bar"),
				filepath.Join(subDir, "foo", "baz"),
				filepath.Join(subDir, "qux"),
				filepath.Join(subDir, "dummy"),
			} {
				base := filepath.Dir(path)
				if err := os.MkdirAll(base, 0755); err != nil {
					t.Fatalf("failed to populate directory [%s]: %s", base, err)
				}
				if err := os.WriteFile(path, []byte("junk"), 0644); err != nil {
					t.Fatalf("failed to create file [%s]: %s", path, err)
				}
			}

			// Make invalid symlink here.
			So(os.Symlink("dummy", filepath.Join(subDir, "invalid_symlink")), ShouldBeNil)
			So(os.Remove(filepath.Join(subDir, "dummy")), ShouldBeNil)

			Convey(`Can remove the directory`, func() {
				So(RemoveAll(subDir), ShouldBeNil)
				So(isGone(subDir), ShouldBeTrue)
			})

			Convey(`Removing inside a read-only dir...`, func() {
				So(MakeReadOnly(subDir, nil), ShouldBeNil)
				qux := filepath.Join(subDir, "qux")
				if runtime.GOOS == "windows" {
					Convey(`... is fine on Windows`, func() {
						So(RemoveAll(qux), ShouldBeNil)
						So(isGone(qux), ShouldBeTrue)
					})
				} else {
					Convey(`... fails on POSIX`, func() {
						So(RemoveAll(qux), ShouldNotBeNil)
						So(isGone(qux), ShouldBeFalse)
					})
				}
			})

			Convey(`Can remove the directory when it is read-only`, func() {
				// Make the directory read-only, and assert that classic os.RemoveAll
				// fails. Except on Windows it doesn't, see:
				// https://github.com/golang/go/issues/26295.
				So(MakeReadOnly(subDir, nil), ShouldBeNil)
				if runtime.GOOS != "windows" {
					So(os.RemoveAll(subDir), ShouldNotBeNil)
				}

				So(RemoveAll(subDir), ShouldBeNil)
				So(isGone(subDir), ShouldBeTrue)
			})

			Convey(`Can remove the directory when it has read-only files`, func() {
				readOnly := filepath.Join(subDir, "foo", "bar")
				So(MakeReadOnly(readOnly, nil), ShouldBeNil)
				So(RemoveAll(subDir), ShouldBeNil)
				So(isGone(subDir), ShouldBeTrue)
			})
		})

		Convey(`Will return nil if the target does not exist`, func() {
			So(RemoveAll(filepath.Join(tdir, "dne")), ShouldBeNil)
		})
	})
}

func TestRenamingRemoveAll(t *testing.T) {
	t.Parallel()

	Convey(`Can RenamingRemoveAll`, t, func() {
		tdir := t.TempDir()
		subDir := filepath.Join(tdir, "sub")
		fooDir := filepath.Join(subDir, "foo")
		fooBarFile := filepath.Join(fooDir, "bar")
		if err := os.MkdirAll(fooDir, 0755); err != nil {
			t.Fatalf("failed to populate directory [%s]: %s", fooDir, err)
		}
		if err := os.WriteFile(fooBarFile, []byte("junk"), 0644); err != nil {
			t.Fatalf("failed to create file [%s]: %s", fooBarFile, err)
		}

		Convey(`No dir to rename into specified`, func() {
			renamedTo, err := RenamingRemoveAll(fooDir, "")
			So(err, ShouldBeNil)
			So(isGone(fooDir), ShouldBeTrue)
			So(renamedTo, ShouldStartWith, filepath.Join(subDir, ".trash-"))
		})

		Convey(`Renaming to specified dir works`, func() {
			renamedTo, err := RenamingRemoveAll(fooDir, tdir)
			So(err, ShouldBeNil)
			So(isGone(fooDir), ShouldBeTrue)
			So(renamedTo, ShouldStartWith, filepath.Join(tdir, ".trash-"))
		})

		Convey(`Renaming to specified dir fails due to not exist error`, func() {
			renamedTo, err := RenamingRemoveAll(fooDir, filepath.Join(tdir, "not-exists"))
			So(err, ShouldBeNil)
			So(isGone(fooDir), ShouldBeTrue)
			So(renamedTo, ShouldEqual, "")
		})

		Convey(`Will return nil if the target does not exist`, func() {
			_, err := RenamingRemoveAll(filepath.Join(tdir, "not-exists"), "")
			So(err, ShouldBeNil)
		})
	})
}

func TestReadableCopy(t *testing.T) {
	Convey("ReadableCopy", t, func() {
		dir := t.TempDir()
		out := filepath.Join(dir, "out")
		in := filepath.Join(dir, "in")
		content := []byte("test")
		So(os.WriteFile(in, content, 0644), ShouldBeNil)

		// Change umask on unix so that test is not affected by default umask.
		old := umask(022)
		So(ReadableCopy(out, in), ShouldBeNil)
		umask(old)

		buf, err := os.ReadFile(out)
		So(err, ShouldBeNil)
		So(buf, ShouldResemble, content)

		ostat, err := os.Stat(out)
		So(err, ShouldBeNil)
		istat, err := os.Stat(in)
		So(err, ShouldBeNil)

		So(ostat.Mode(), ShouldEqual, addReadMode(istat.Mode()))
	})
}

func TestCopy(t *testing.T) {
	Convey("Copy", t, func() {
		dir := t.TempDir()
		out := filepath.Join(dir, "out")
		in := filepath.Join(dir, "in")
		content := []byte("test")
		So(os.WriteFile(in, content, 0o644), ShouldBeNil)

		// Change umask on unix so that test is not affected by default umask.
		old := umask(0o22)
		So(Copy(out, in, 0o644), ShouldBeNil)
		umask(old)

		buf, err := os.ReadFile(out)
		So(err, ShouldBeNil)
		So(buf, ShouldResemble, content)

		ostat, err := os.Stat(out)
		So(err, ShouldBeNil)
		_, err = os.Stat(in)
		So(err, ShouldBeNil)

		if runtime.GOOS != "windows" {
			So(ostat.Mode(), ShouldEqual, 0o644)
		}
	})
}

func TestHardlinkRecursively(t *testing.T) {
	t.Parallel()

	checkFile := func(path, content string) {
		buf, err := os.ReadFile(path)
		So(err, ShouldBeNil)
		So(string(buf), ShouldResemble, content)
	}

	Convey("HardlinkRecursively", t, func() {
		dir := t.TempDir()
		src1 := filepath.Join(dir, "src1")
		src2 := filepath.Join(src1, "src2")
		So(os.MkdirAll(src2, 0755), ShouldBeNil)

		So(os.WriteFile(filepath.Join(src1, "file1"), []byte("test1"), 0644), ShouldBeNil)
		So(os.WriteFile(filepath.Join(src2, "file2"), []byte("test2"), 0644), ShouldBeNil)

		So(os.Symlink(filepath.Join("src2", "file2"), filepath.Join(src1, "link")), ShouldBeNil)

		dst := filepath.Join(dir, "dst")
		So(HardlinkRecursively(src1, dst), ShouldBeNil)

		checkFile(filepath.Join(dst, "file1"), "test1")
		checkFile(filepath.Join(dst, "src2", "file2"), "test2")
		checkFile(filepath.Join(dst, "link"), "test2")

		stat, err := os.Stat(filepath.Join(dst, "link"))
		So(err, ShouldBeNil)
		So(stat.Mode()&os.ModeSymlink, ShouldEqual, 0)
	})

	Convey("HardlinkRecursively nested", t, func() {
		dir := t.TempDir()
		top := filepath.Join(dir, "top")
		nested := filepath.Join(top, "nested")
		So(os.MkdirAll(nested, 0755), ShouldBeNil)

		So(os.WriteFile(filepath.Join(nested, "file"), []byte("test"), 0644), ShouldBeNil)

		dstTop := filepath.Join(dir, "dst")
		So(HardlinkRecursively(top, dstTop), ShouldBeNil)
		dstDested := filepath.Join(dstTop, "nested")
		So(HardlinkRecursively(nested, dstDested), ShouldNotBeNil)
	})
}

func TestCreateDirectories(t *testing.T) {
	t.Parallel()
	Convey("CreateDirectories", t, func() {
		dir := t.TempDir()
		So(CreateDirectories(dir, []string{}), ShouldBeNil)

		var paths []string
		correctFiles := func(path string, info os.FileInfo, err error) error {
			paths = append(paths, path)
			return nil
		}

		So(filepath.Walk(dir, correctFiles), ShouldBeNil)
		So(paths, ShouldResemble, []string{dir})

		So(CreateDirectories(dir, []string{
			filepath.Join("a", "b"),
			filepath.Join("c", "d", "e"),
			filepath.Join("c", "f"),
			filepath.Join("g"),
		}), ShouldBeNil)

		paths = nil
		So(filepath.Walk(dir, correctFiles), ShouldBeNil)
		So(paths, ShouldResemble, []string{
			dir,
			filepath.Join(dir, "a"),
			filepath.Join(dir, "c"),
			filepath.Join(dir, "c", "d"),
		})
	})
}

func TestIsEmptyDir(t *testing.T) {
	Convey("IsEmptyDir", t, func() {
		dir := t.TempDir()
		emptyDir := filepath.Join(dir, "empty")
		So(MakeDirs(emptyDir), ShouldBeNil)

		isEmpty, err := IsEmptyDir(emptyDir)
		So(err, ShouldBeNil)
		So(isEmpty, ShouldBeTrue)

		nonEmptyDir := filepath.Join(dir, "non-empty")
		So(MakeDirs(nonEmptyDir), ShouldBeNil)
		file1 := filepath.Join(nonEmptyDir, "file1")
		So(os.WriteFile(file1, []byte("test1"), 0644), ShouldBeNil)

		isEmpty, err = IsEmptyDir(nonEmptyDir)
		So(err, ShouldBeNil)
		So(isEmpty, ShouldBeFalse)

		_, err = IsEmptyDir(file1)
		So(err, ShouldNotBeNil)

		_, err = IsEmptyDir(filepath.Join(dir, "not-existing"))
		So(err, ShouldNotBeNil)
	})
}

func TestIsDir(t *testing.T) {
	Convey("IsDir", t, func() {
		dir := t.TempDir()
		b, err := IsDir(dir)
		So(err, ShouldBeNil)
		So(b, ShouldBeTrue)

		b, err = IsDir(filepath.Join(dir, "not_exists"))
		So(err, ShouldBeNil)
		So(b, ShouldBeFalse)

		file := filepath.Join(dir, "file")
		So(os.WriteFile(file, []byte(""), 0644), ShouldBeNil)
		b, err = IsDir(file)
		So(err, ShouldBeNil)
		So(b, ShouldBeFalse)
	})
}

func TestGetFreeSpace(t *testing.T) {
	t.Parallel()

	Convey("GetFreeSpace", t, func() {
		size, err := GetFreeSpace(".")
		So(err, ShouldBeNil)
		So(size, ShouldBeGreaterThan, 4096)
	})
}

func isGone(path string) bool {
	if _, err := os.Lstat(path); err != nil {
		return os.IsNotExist(err)
	}
	return false
}

func TestGetCommonAncestor(t *testing.T) {
	t.Parallel()

	tdir := t.TempDir()
	if err := Touch(filepath.Join(tdir, "A"), time.Now(), 0o666); err != nil {
		t.Error(err)
	}
	if err := Touch(filepath.Join(tdir, "a"), time.Now(), 0o666); err != nil {
		t.Error(err)
	}
	big, err := os.Stat(filepath.Join(tdir, "A"))
	if err != nil {
		t.Error(err)
	}
	small, err := os.Stat(filepath.Join(tdir, "a"))
	if err != nil {
		t.Error(err)
	}
	fsCaseSensitive := !os.SameFile(big, small)

	const sep = string(filepath.Separator)

	Convey(`GetCommonAncestor`, t, func() {
		Convey(`common ancestor logic`, func() {
			Convey(`common`, func() {
				tdir := t.TempDir()
				So(os.MkdirAll(filepath.Join(tdir, "a", "b", "c", "d"), 0o777), ShouldBeNil)
				if fsCaseSensitive {
					So(os.MkdirAll(filepath.Join(tdir, "a", "B", "c"), 0o777), ShouldBeNil)
					So(os.MkdirAll(filepath.Join(tdir, "A", "b", "c", "d"), 0o777), ShouldBeNil)
				}
				So(os.MkdirAll(filepath.Join(tdir, "a", "1", "2", "3"), 0o777), ShouldBeNil)
				So(os.MkdirAll(filepath.Join(tdir, "r", "o", "o", "t"), 0o777), ShouldBeNil)

				So(Touch(filepath.Join(tdir, "a", "B", "c", "something"), time.Now(), 0o666), ShouldBeNil)
				So(Touch(filepath.Join(tdir, "a", "b", "else"), time.Now(), 0o666), ShouldBeNil)

				Convey(`stops at 'local' common ancestor`, func() {
					common, err := GetCommonAncestor([]string{
						filepath.Join(tdir, "a", "b", "c", "d"),
						filepath.Join(tdir, "a", "B", "c", "something"),
						filepath.Join(tdir, "A", "b", "c", "d"),
						filepath.Join(tdir, "a", "b", "else"),
					}, []string{".git"})
					So(err, ShouldBeNil)
					if fsCaseSensitive {
						So(common, ShouldResemble, tdir+sep)
					} else {
						So(common, ShouldResemble, filepath.Join(tdir, "a", "b")+sep)
					}
				})

				Convey(`walks up to fs root`, func() {
					common, err := GetCommonAncestor([]string{
						filepath.VolumeName(tdir) + sep,
						filepath.Join(tdir, "a", "B", "c", "something"),
						filepath.Join(tdir, "A", "b", "c", "d"),
						filepath.Join(tdir, "a", "b", "else"),
					}, []string{".git"})
					So(err, ShouldBeNil)
					So(common, ShouldResemble, filepath.VolumeName(tdir)+sep)
				})
			})

			Convey(`slash comparison`, func() {
				tdir := t.TempDir()
				So(os.MkdirAll(filepath.Join(tdir, "longpath", "a", "c", "d"), 0o777), ShouldBeNil)
				So(os.MkdirAll(filepath.Join(tdir, "a", "longpath", "c", "d"), 0o777), ShouldBeNil)

				common, err := GetCommonAncestor([]string{
					filepath.Join(tdir, "longpath", "a", "c", "d"),
					filepath.Join(tdir, "a", "longpath", "c", "d"),
				}, []string{".git"})
				So(err, ShouldBeNil)
				So(common, ShouldResemble, tdir+sep)
			})

			Convey(`non exist`, func() {
				_, err := GetCommonAncestor([]string{
					filepath.Join("i", "dont", "exist"),
				}, []string{".git"})
				So(err, ShouldErrLike, "reading path[0]")
			})

			Convey(`sentinel`, func() {
				tdir := t.TempDir()
				So(os.MkdirAll(filepath.Join(tdir, "repoA", ".git"), 0o777), ShouldBeNil)
				So(os.MkdirAll(filepath.Join(tdir, "repoA", "something"), 0o777), ShouldBeNil)
				So(os.MkdirAll(filepath.Join(tdir, "repoB", ".git"), 0o777), ShouldBeNil)
				So(os.MkdirAll(filepath.Join(tdir, "repoB", "else"), 0o777), ShouldBeNil)

				_, err := GetCommonAncestor([]string{
					filepath.Join(tdir, "repoA", "something"),
					filepath.Join(tdir, "repoB", "else"),
				}, []string{".git"})
				So(err, ShouldErrLike, "hit root sentinel")
				So(errors.Is(err, ErrRootSentinel), ShouldBeTrue)
			})

			if runtime.GOOS == "windows" {
				Convey(`different volumes`, func() {
					tdir := t.TempDir()
					_, err := GetCommonAncestor([]string{
						tdir,
						`\\fake\network\share`,
					}, nil)
					So(err, ShouldErrLike, "originate on different volumes")
				})
			}
		})

		Convey(`helpers`, func() {
			if runtime.GOOS == "windows" {
				Convey(`windows paths`, func() {
					So(findPathSeparators(`D:\something\`), ShouldResemble, []int{2, 12})
					So(findPathSeparators(`D:\`), ShouldResemble, []int{2})
					So(findPathSeparators(`\\some\host\something\`),
						ShouldResemble, []int{11, 21})
					So(findPathSeparators(`\\some\host\`),
						ShouldResemble, []int{11})
					So(findPathSeparators(`\\?\C:\Test\`),
						ShouldResemble, []int{6, 11})
				})
			} else {
				Convey(`*nix paths`, func() {
					So(findPathSeparators(`/something/`),
						ShouldResemble, []int{0, 10})
					So(findPathSeparators(`/`),
						ShouldResemble, []int{0})
				})
			}
		})
	})
}
