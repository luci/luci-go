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

package fs

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCaseSensitive(t *testing.T) {
	t.Parallel()

	Convey("CaseSensitive doesn't crash", t, func(c C) {
		fs := tempFileSystem(c)

		_, err := fs.CaseSensitive()
		So(err, ShouldBeNil)

		// No garbage left around.
		fis, err := ioutil.ReadDir(fs.join(""))
		So(err, ShouldBeNil)
		So(fis, ShouldHaveLength, 0)
	})
}

func TestCwdRelToAbs(t *testing.T) {
	t.Parallel()

	Convey("CwdRelToAbs works", t, func(c C) {
		fs := tempFileSystem(c)
		p, err := fs.CwdRelToAbs(fs.Root())
		So(err, ShouldBeNil)
		So(p, ShouldEqual, fs.Root())

		p, err = fs.CwdRelToAbs(fs.join("already_abs/def"))
		So(err, ShouldBeNil)
		So(p, ShouldEqual, fs.join("already_abs/def"))

		_, err = fs.CwdRelToAbs(fs.join(".."))
		So(err, ShouldNotBeNil)

		_, err = fs.CwdRelToAbs(fs.join("../.."))
		So(err, ShouldNotBeNil)

		_, err = fs.CwdRelToAbs(fs.join("../abc"))
		So(err, ShouldNotBeNil)
	})
}

func TestRootRelToAbs(t *testing.T) {
	t.Parallel()

	Convey("RootRelToAbs works", t, func(c C) {
		fs := tempFileSystem(c)

		p, err := fs.RootRelToAbs(".")
		So(err, ShouldBeNil)
		So(p, ShouldEqual, fs.Root())

		p, err = fs.RootRelToAbs(fs.join("already_abs/def"))
		So(err, ShouldBeNil)
		So(p, ShouldEqual, fs.join("already_abs/def"))

		_, err = fs.RootRelToAbs(filepath.FromSlash("abc/../../def"))
		So(err, ShouldNotBeNil)
	})
}

func TestEnsureDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("EnsureDirectory checks root", t, func(c C) {
		fs := tempFileSystem(c)
		_, err := fs.EnsureDirectory(ctx, fs.join(".."))
		So(err, ShouldNotBeNil)
	})

	Convey("EnsureDirectory works", t, func(c C) {
		fs := tempFileSystem(c)
		p, err := fs.EnsureDirectory(ctx, fs.join("x/../y/z"))
		So(err, ShouldBeNil)
		So(p, ShouldEqual, fs.join("y/z"))
		So(fs.isDir("y/z"), ShouldBeTrue)
		// Same one.
		_, err = fs.EnsureDirectory(ctx, fs.join("x/../y/z"))
		So(err, ShouldBeNil)
	})

	Convey("EnsureDirectory replaces files with directories", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a", "xxx")
		_, err := fs.EnsureDirectory(ctx, fs.join("a/b/c"))
		So(err, ShouldBeNil)
		So(fs.isDir("a/b/c"), ShouldBeTrue)
	})
}

func TestEnsureSymlink(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no symlinks")
	}

	ctx := context.Background()

	Convey("EnsureSymlink checks root", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.EnsureSymlink(ctx, fs.join(".."), fs.Root()), ShouldNotBeNil)
	})

	Convey("EnsureSymlink creates new symlink", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.EnsureSymlink(ctx, fs.join("symlink"), "target"), ShouldBeNil)
		So(fs.readLink("symlink"), ShouldEqual, "target")
	})

	Convey("EnsureSymlink builds full path", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.EnsureSymlink(ctx, fs.join("a/b/c"), "target"), ShouldBeNil)
		So(fs.readLink("a/b/c"), ShouldEqual, "target")
	})

	Convey("EnsureSymlink replaces existing symlink", t, func(c C) {
		fs := tempFileSystem(c)
		// Replace with same one, then with another one.
		So(fs.EnsureSymlink(ctx, fs.join("symlink"), "target"), ShouldBeNil)
		So(fs.EnsureSymlink(ctx, fs.join("symlink"), "target"), ShouldBeNil)
		So(fs.EnsureSymlink(ctx, fs.join("symlink"), "another"), ShouldBeNil)
		So(fs.readLink("symlink"), ShouldEqual, "another")
	})

	Convey("EnsureSymlink replaces existing file", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("path", "blah")
		So(fs.EnsureSymlink(ctx, fs.join("path"), "target"), ShouldBeNil)
		So(fs.readLink("path"), ShouldEqual, "target")
	})

	Convey("EnsureSymlink replaces existing directory", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/b/c", "something")
		So(fs.EnsureSymlink(ctx, fs.join("a"), "target"), ShouldBeNil)
		So(fs.readLink("a"), ShouldEqual, "target")
	})
}

func TestEnsureFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("EnsureFile checks root", t, func(c C) {
		fs := tempFileSystem(c)
		So(EnsureFile(ctx, fs, fs.join(".."), strings.NewReader("blah")), ShouldNotBeNil)
	})

	Convey("EnsureFile creates new file", t, func(c C) {
		fs := tempFileSystem(c)
		So(EnsureFile(ctx, fs, fs.join("name"), strings.NewReader("blah")), ShouldBeNil)
		So(fs.read("name"), ShouldEqual, "blah")
	})

	Convey("EnsureFile builds full path", t, func(c C) {
		fs := tempFileSystem(c)
		So(EnsureFile(ctx, fs, fs.join("a/b/c"), strings.NewReader("blah")), ShouldBeNil)
		So(fs.read("a/b/c"), ShouldEqual, "blah")
	})

	if runtime.GOOS != "windows" {
		Convey("EnsureFile replaces existing symlink", t, func(c C) {
			fs := tempFileSystem(c)
			So(fs.EnsureSymlink(ctx, fs.join("path"), "target"), ShouldBeNil)
			So(EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("blah")), ShouldBeNil)
			So(fs.read("path"), ShouldEqual, "blah")
		})
	}

	Convey("EnsureFile replaces existing file", t, func(c C) {
		fs := tempFileSystem(c)
		So(EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("huh")), ShouldBeNil)
		So(EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("blah")), ShouldBeNil)
		So(fs.read("path"), ShouldEqual, "blah")
	})

	Convey("EnsureFile replaces existing file while it is being read", t, func(c C) {
		fs := tempFileSystem(c)

		// Create an initial file, then open it for reading.
		So(EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("huh")), ShouldBeNil)
		fd, err := fs.OpenFile(fs.join("path"))
		So(err, ShouldBeNil)
		defer fd.Close()

		// Ensure a second file on top of it.
		So(EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("blah")), ShouldBeNil)

		// Read and verify the content of the first file.
		content, err := ioutil.ReadAll(fd)
		So(err, ShouldBeNil)
		So(content, ShouldResemble, []byte("huh"))

		// Verify the content of the second ensured file.
		So(fs.read("path"), ShouldEqual, "blah")

		// The first file should successfully close.
		So(fd.Close(), ShouldBeNil)
	})

	Convey("EnsureFile replaces existing file while it is being written", t, func(c C) {
		fs := tempFileSystem(c)

		// Start one EnsureFile, pausing in the middle so another can start
		// alongside it.
		firstEnsureStartedC := make(chan struct{})
		finishFirstWriteC := make(chan struct{})
		errC := make(chan error)
		go func() {
			errC <- fs.EnsureFile(ctx, fs.join("path"), func(fd *os.File) error {
				close(firstEnsureStartedC)
				<-finishFirstWriteC
				_, err := fd.Write([]byte("foo"))
				return err
			})
		}()

		// Wait for the first to signal that it's begin, then perform a second
		// overlapping EnsureFile.
		<-firstEnsureStartedC
		So(EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("overwrite me")), ShouldBeNil)
		close(finishFirstWriteC)
		So(<-errC, ShouldBeNil)

		// The first EnsureFile content should be the one that is present.
		So(fs.read("path"), ShouldEqual, "foo")
	})

	Convey("EnsureFile replaces existing directory", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/b/c", "something")
		So(EnsureFile(ctx, fs, fs.join("a"), strings.NewReader("blah")), ShouldBeNil)
		So(fs.read("a"), ShouldEqual, "blah")
	})
}

func TestEnsureFileGone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("EnsureFileGone checks root", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.EnsureFileGone(ctx, fs.join("../abc")), ShouldNotBeNil)
	})

	Convey("EnsureFileGone works", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("abc", "")
		So(fs.EnsureFileGone(ctx, fs.join("abc")), ShouldBeNil)
		So(fs.isMissing("abc"), ShouldBeTrue)
	})

	Convey("EnsureFileGone works with missing file", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.EnsureFileGone(ctx, fs.join("abc")), ShouldBeNil)
	})

	Convey("EnsureFileGone works with empty directory", t, func(c C) {
		fs := tempFileSystem(c)
		fs.mkdir("abc")
		So(fs.EnsureFileGone(ctx, fs.join("abc")), ShouldBeNil)
		So(fs.isMissing("abc"), ShouldBeTrue)
	})

	Convey("EnsureFileGone fails with non-empty directory", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("abc/file", "zzz")
		So(fs.EnsureFileGone(ctx, fs.join("abc")), ShouldNotBeNil)
	})

	Convey("EnsureFileGone skips paths that use files as if they are dirs", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("abc/file", "zzz")
		So(fs.EnsureFileGone(ctx, fs.join("abc/file/huh")), ShouldBeNil)
	})

	if runtime.GOOS != "windows" {
		Convey("EnsureFileGone works with symlink", t, func(c C) {
			fs := tempFileSystem(c)
			So(fs.EnsureSymlink(ctx, fs.join("abc"), "target"), ShouldBeNil)
			So(fs.EnsureFileGone(ctx, fs.join("abc")), ShouldBeNil)
			So(fs.isMissing("abc"), ShouldBeTrue)
		})
	}

	Convey("crbug.com/936911: EnsureFileGone can delete files opened with OpenFile", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("abc/file", "zzz")
		f, err := fs.OpenFile(fs.join("abc/file"))
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)
		So(fs.EnsureFileGone(ctx, f.Name()), ShouldBeNil)
	})
}

func TestEnsureDirectoryGone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("EnsureDirectoryGone checks root", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.EnsureDirectoryGone(ctx, fs.join("../abc")), ShouldNotBeNil)
	})

	Convey("EnsureDirectoryGone works", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("dir/a/1", "")
		fs.write("dir/a/2", "")
		fs.write("dir/b/1", "")
		So(fs.EnsureDirectoryGone(ctx, fs.join("dir")), ShouldBeNil)
		So(fs.isMissing("dir"), ShouldBeTrue)
	})

	Convey("EnsureDirectoryGone works with missing dir", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.EnsureDirectoryGone(ctx, fs.join("missing")), ShouldBeNil)
	})

}

func TestReplace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Replace checks root", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.Replace(ctx, fs.join("../abc"), fs.join("def")), ShouldNotBeNil)
		fs.write("def", "")
		So(fs.Replace(ctx, fs.join("def"), fs.join("../abc")), ShouldNotBeNil)
	})

	Convey("Replace does nothing if oldpath == newpath", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.Replace(ctx, fs.join("abc"), fs.join("abc/d/..")), ShouldBeNil)
	})

	Convey("Replace recognizes missing oldpath", t, func(c C) {
		fs := tempFileSystem(c)
		So(fs.Replace(ctx, fs.join("missing"), fs.join("abc")), ShouldNotBeNil)
	})

	Convey("Replace creates file and dir if missing", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123", "")
		So(fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), ShouldBeNil)
		So(fs.isMissing("a/123"), ShouldBeTrue)
		So(fs.isFile("b/c/d/"), ShouldBeTrue)
	})

	Convey("Replace replaces regular file with a file", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123", "xxx")
		fs.write("b/c/d", "yyy")
		So(fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), ShouldBeNil)
		So(fs.isMissing("a/123"), ShouldBeTrue)
		So(fs.read("b/c/d"), ShouldEqual, "xxx")
	})

	Convey("Replace replaces regular file with a dir", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123/456", "xxx")
		fs.write("b/c/d", "yyy")
		So(fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), ShouldBeNil)
		So(fs.isMissing("a/123"), ShouldBeTrue)
		So(fs.read("b/c/d/456"), ShouldEqual, "xxx")
	})

	Convey("Replace replaces empty dir with a file", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123", "xxx")
		_, err := fs.EnsureDirectory(ctx, fs.join("b/c/d"))
		So(err, ShouldBeNil)
		So(fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), ShouldBeNil)
		So(fs.isMissing("a/123"), ShouldBeTrue)
		So(fs.read("b/c/d"), ShouldEqual, "xxx")
	})

	Convey("Replace replaces empty dir with a dir", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123/456", "xxx")
		_, err := fs.EnsureDirectory(ctx, fs.join("b/c/d"))
		So(err, ShouldBeNil)
		So(fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), ShouldBeNil)
		So(fs.isMissing("a/123"), ShouldBeTrue)
		So(fs.read("b/c/d/456"), ShouldEqual, "xxx")
	})

	Convey("Replace replaces dir with a file", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123", "xxx")
		fs.write("b/c/d/456", "yyy")
		So(fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), ShouldBeNil)
		So(fs.isMissing("a/123"), ShouldBeTrue)
		So(fs.read("b/c/d"), ShouldEqual, "xxx")
	})

	Convey("Replace replaces dir with a dir", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123/456", "xxx")
		fs.write("b/c/d/456", "yyy")
		So(fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), ShouldBeNil)
		So(fs.isMissing("a/123"), ShouldBeTrue)
		So(fs.read("b/c/d/456"), ShouldEqual, "xxx")
	})

	Convey("Replace replaces file in dir path with a directory", t, func(c C) {
		fs := tempFileSystem(c)
		fs.write("a/123", "xxx")
		fs.write("f", "yyy")
		So(fs.Replace(ctx, fs.join("f"), fs.join("a/123/456")), ShouldBeNil)
		So(fs.read("a/123/456"), ShouldEqual, "yyy")
	})

	Convey("Replace works for very long paths", t, func(c C) {
		fs := tempFileSystem(c)
		// Windows runs into trouble with paths > 260 chars
		longPath1 := strings.Repeat("abc/", 260/4) + "file"
		longPath2 := strings.Repeat("xyz/", 260/4) + "file"
		So(len(longPath1), ShouldBeGreaterThan, 260)
		fs.write(longPath1, "hi")
		fs.write(longPath2, "there")
		So(fs.Replace(ctx, fs.join(longPath1), fs.join(longPath2)), ShouldBeNil)
		So(fs.read(longPath2), ShouldEqual, "hi")
	})

	Convey("Concurrency test", t, func(c C) {
		const threads = 50
		fs := tempFileSystem(c)
		wg := sync.WaitGroup{}
		wg.Add(threads)
		for th := 0; th < threads; th++ {
			th := th
			go func() {
				defer wg.Done()
				var oldpath string
				switch th % 3 {
				case 0: // replacing with a file
					oldpath = fmt.Sprintf("file_%d", th)
					fs.write(oldpath, "...")
				case 1: // replacing with an empty directory
					oldpath = fmt.Sprintf("empty_%d", th)
					fs.mkdir(oldpath)
				case 2: // replacing with a non-empty directory
					oldpath = fmt.Sprintf("dir_%d", th)
					fs.write(oldpath+"/file", "...")
				}
				if err := fs.Replace(ctx, fs.join(oldpath), fs.join("new/path")); err != nil {
					panic(err)
				}
			}()
		}
		wg.Wait()
	})
}

///////

// tempFileSystem returns FileSystem for tests built over a temp directory.
func tempFileSystem(c C) *tempFileSystemImpl {
	tempDir, err := ioutil.TempDir("", "cipd_test")
	So(err, ShouldBeNil)
	Reset(func() { os.RemoveAll(tempDir) })
	return &tempFileSystemImpl{NewFileSystem(tempDir, ""), c}
}

type tempFileSystemImpl struct {
	FileSystem
	c C
}

// join returns absolute path given a slash separated path relative to Root().
func (f *tempFileSystemImpl) join(path string) string {
	return filepath.Join(f.Root(), filepath.FromSlash(path))
}

// write creates a file at a given slash separated path relative to Root().
func (f *tempFileSystemImpl) write(rel string, data string) {
	abs := f.join(rel)
	err := os.MkdirAll(filepath.Dir(abs), 0777)
	f.c.So(err, ShouldBeNil)
	file, err := os.Create(abs)
	f.c.So(err, ShouldBeNil)
	file.WriteString(data)
	f.c.So(file.Close(), ShouldBeNil)
}

// mkdir creates an empty directory.
func (f *tempFileSystemImpl) mkdir(rel string) {
	f.c.So(os.MkdirAll(f.join(rel), 0777), ShouldBeNil)
}

// read reads an existing file at a given slash separated path relative to Root().
func (f *tempFileSystemImpl) read(rel string) string {
	fd, err := f.OpenFile(f.join(rel))
	f.c.So(err, ShouldBeNil)
	defer fd.Close()

	data, err := ioutil.ReadAll(fd)
	f.c.So(err, ShouldBeNil)
	return string(data)
}

// readLink reads a symlink at a given slash separated path relative to Root().
func (f *tempFileSystemImpl) readLink(rel string) string {
	val, err := os.Readlink(f.join(rel))
	f.c.So(err, ShouldBeNil)
	return val
}

// isMissing returns true if there's no file at a given slash separated path
// relative to Root().
func (f *tempFileSystemImpl) isMissing(rel string) bool {
	_, err := os.Stat(f.join(rel))
	return os.IsNotExist(err)
}

// isDir returns true if a file at a given slash separated path relative to
// Root() is a directory.
func (f *tempFileSystemImpl) isDir(rel string) bool {
	stat, err := os.Stat(f.join(rel))
	if err != nil {
		return false
	}
	return stat.IsDir()
}

// isFile returns true if a file at a given slash separated path relative to
// Root() is a file (not a directory).
func (f *tempFileSystemImpl) isFile(rel string) bool {
	stat, err := os.Stat(f.join(rel))
	if err != nil {
		return false
	}
	return !stat.IsDir()
}
