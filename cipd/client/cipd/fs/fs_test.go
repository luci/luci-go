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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCaseSensitive(t *testing.T) {
	t.Parallel()

	ftt.Run("CaseSensitive doesn't crash", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)

		_, err := fs.CaseSensitive()
		assert.Loosely(c, err, should.BeNil)

		// No garbage left around.
		entries, err := os.ReadDir(fs.join(""))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, entries, should.HaveLength(0))
	})
}

func TestCwdRelToAbs(t *testing.T) {
	t.Parallel()

	ftt.Run("CwdRelToAbs works", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		p, err := fs.CwdRelToAbs(fs.Root())
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, p, should.Equal(fs.Root()))

		p, err = fs.CwdRelToAbs(fs.join("already_abs/def"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, p, should.Equal(fs.join("already_abs/def")))

		_, err = fs.CwdRelToAbs(fs.join(".."))
		assert.Loosely(c, err, should.NotBeNil)

		_, err = fs.CwdRelToAbs(fs.join("../.."))
		assert.Loosely(c, err, should.NotBeNil)

		_, err = fs.CwdRelToAbs(fs.join("../abc"))
		assert.Loosely(c, err, should.NotBeNil)
	})
}

func TestRootRelToAbs(t *testing.T) {
	t.Parallel()

	ftt.Run("RootRelToAbs works", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)

		p, err := fs.RootRelToAbs(".")
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, p, should.Equal(fs.Root()))

		p, err = fs.RootRelToAbs(fs.join("already_abs/def"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, p, should.Equal(fs.join("already_abs/def")))

		_, err = fs.RootRelToAbs(filepath.FromSlash("abc/../../def"))
		assert.Loosely(c, err, should.NotBeNil)
	})
}

func TestEnsureDirectory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("EnsureDirectory checks root", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		_, err := fs.EnsureDirectory(ctx, fs.join(".."))
		assert.Loosely(c, err, should.NotBeNil)
	})

	ftt.Run("EnsureDirectory works", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		p, err := fs.EnsureDirectory(ctx, fs.join("x/../y/z"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, p, should.Equal(fs.join("y/z")))
		assert.Loosely(c, fs.isDir("y/z"), should.BeTrue)
		// Same one.
		_, err = fs.EnsureDirectory(ctx, fs.join("x/../y/z"))
		assert.Loosely(c, err, should.BeNil)
	})

	ftt.Run("EnsureDirectory replaces files with directories", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a", "xxx")
		_, err := fs.EnsureDirectory(ctx, fs.join("a/b/c"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, fs.isDir("a/b/c"), should.BeTrue)
	})

	if runtime.GOOS != "windows" {
		ftt.Run("EnsureDirectory replaces file symlinks with directories", t, func(c *ftt.Test) {
			fs := tempFileSystem(t)
			fs.write("target", "xxx")
			fs.symlink("target", "a")
			_, err := fs.EnsureDirectory(ctx, fs.join("a"))
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, fs.isDir("a"), should.BeTrue)
		})

		ftt.Run("EnsureDirectory replaces broken symlinks with directories", t, func(c *ftt.Test) {
			fs := tempFileSystem(t)
			fs.symlink("broken", "a")
			_, err := fs.EnsureDirectory(ctx, fs.join("a"))
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, fs.isDir("a"), should.BeTrue)
		})
	}
}

func TestEnsureSymlink(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows: no symlinks")
	}

	ctx := context.Background()

	ftt.Run("EnsureSymlink checks root", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join(".."), fs.Root()), should.NotBeNil)
	})

	ftt.Run("EnsureSymlink creates new symlink", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("symlink"), "target"), should.BeNil)
		assert.Loosely(c, fs.readLink("symlink"), should.Equal("target"))
	})

	ftt.Run("EnsureSymlink builds full path", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("a/b/c"), "target"), should.BeNil)
		assert.Loosely(c, fs.readLink("a/b/c"), should.Equal("target"))
	})

	ftt.Run("EnsureSymlink replaces existing symlink", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		// Replace with same one, then with another one.
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("symlink"), "target"), should.BeNil)
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("symlink"), "target"), should.BeNil)
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("symlink"), "another"), should.BeNil)
		assert.Loosely(c, fs.readLink("symlink"), should.Equal("another"))
	})

	ftt.Run("EnsureSymlink replaces existing file", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("path", "blah")
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("path"), "target"), should.BeNil)
		assert.Loosely(c, fs.readLink("path"), should.Equal("target"))
	})

	ftt.Run("EnsureSymlink replaces existing directory", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/b/c", "something")
		assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("a"), "target"), should.BeNil)
		assert.Loosely(c, fs.readLink("a"), should.Equal("target"))
	})
}

func TestEnsureFile(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("EnsureFile checks root", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join(".."), strings.NewReader("blah")), should.NotBeNil)
	})

	ftt.Run("EnsureFile creates new file", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("name"), strings.NewReader("blah")), should.BeNil)
		assert.Loosely(c, fs.read("name"), should.Equal("blah"))
	})

	ftt.Run("EnsureFile builds full path", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("a/b/c"), strings.NewReader("blah")), should.BeNil)
		assert.Loosely(c, fs.read("a/b/c"), should.Equal("blah"))
	})

	if runtime.GOOS != "windows" {
		ftt.Run("EnsureFile replaces existing symlink", t, func(c *ftt.Test) {
			fs := tempFileSystem(t)
			assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("path"), "target"), should.BeNil)
			assert.Loosely(c, EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("blah")), should.BeNil)
			assert.Loosely(c, fs.read("path"), should.Equal("blah"))
		})
	}

	ftt.Run("EnsureFile replaces existing file", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("huh")), should.BeNil)
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("blah")), should.BeNil)
		assert.Loosely(c, fs.read("path"), should.Equal("blah"))
	})

	ftt.Run("EnsureFile replaces existing file while it is being read", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)

		// Create an initial file, then open it for reading.
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("huh")), should.BeNil)
		fd, err := fs.OpenFile(fs.join("path"))
		assert.Loosely(c, err, should.BeNil)
		defer fd.Close()

		// Ensure a second file on top of it.
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("blah")), should.BeNil)

		// Read and verify the content of the first file.
		content, err := io.ReadAll(fd)
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, content, should.Resemble([]byte("huh")))

		// Verify the content of the second ensured file.
		assert.Loosely(c, fs.read("path"), should.Equal("blah"))

		// The first file should successfully close.
		assert.Loosely(c, fd.Close(), should.BeNil)
	})

	ftt.Run("EnsureFile replaces existing file while it is being written", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)

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
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("path"), strings.NewReader("overwrite me")), should.BeNil)
		close(finishFirstWriteC)
		assert.Loosely(c, <-errC, should.BeNil)

		// The first EnsureFile content should be the one that is present.
		assert.Loosely(c, fs.read("path"), should.Equal("foo"))
	})

	ftt.Run("EnsureFile replaces existing directory", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/b/c", "something")
		assert.Loosely(c, EnsureFile(ctx, fs, fs.join("a"), strings.NewReader("blah")), should.BeNil)
		assert.Loosely(c, fs.read("a"), should.Equal("blah"))
	})
}

func TestEnsureFileGone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("EnsureFileGone checks root", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.EnsureFileGone(ctx, fs.join("../abc")), should.NotBeNil)
	})

	ftt.Run("EnsureFileGone works", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("abc", "")
		assert.Loosely(c, fs.EnsureFileGone(ctx, fs.join("abc")), should.BeNil)
		assert.Loosely(c, fs.isMissing("abc"), should.BeTrue)
	})

	ftt.Run("EnsureFileGone works with missing file", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.EnsureFileGone(ctx, fs.join("abc")), should.BeNil)
	})

	ftt.Run("EnsureFileGone works with empty directory", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.mkdir("abc")
		assert.Loosely(c, fs.EnsureFileGone(ctx, fs.join("abc")), should.BeNil)
		assert.Loosely(c, fs.isMissing("abc"), should.BeTrue)
	})

	ftt.Run("EnsureFileGone fails with non-empty directory", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("abc/file", "zzz")
		assert.Loosely(c, fs.EnsureFileGone(ctx, fs.join("abc")), should.NotBeNil)
	})

	ftt.Run("EnsureFileGone skips paths that use files as if they are dirs", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("abc/file", "zzz")
		assert.Loosely(c, fs.EnsureFileGone(ctx, fs.join("abc/file/huh")), should.BeNil)
	})

	if runtime.GOOS != "windows" {
		ftt.Run("EnsureFileGone works with symlink", t, func(c *ftt.Test) {
			fs := tempFileSystem(t)
			assert.Loosely(c, fs.EnsureSymlink(ctx, fs.join("abc"), "target"), should.BeNil)
			assert.Loosely(c, fs.EnsureFileGone(ctx, fs.join("abc")), should.BeNil)
			assert.Loosely(c, fs.isMissing("abc"), should.BeTrue)
		})
	}

	ftt.Run("crbug.com/936911: EnsureFileGone can delete files opened with OpenFile", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("abc/file", "zzz")
		f, err := fs.OpenFile(fs.join("abc/file"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, f.Close(), should.BeNil)
		assert.Loosely(c, fs.EnsureFileGone(ctx, f.Name()), should.BeNil)
	})
}

func TestEnsureDirectoryGone(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("EnsureDirectoryGone checks root", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.EnsureDirectoryGone(ctx, fs.join("../abc")), should.NotBeNil)
	})

	ftt.Run("EnsureDirectoryGone works", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("dir/a/1", "")
		fs.write("dir/a/2", "")
		fs.write("dir/b/1", "")
		assert.Loosely(c, fs.EnsureDirectoryGone(ctx, fs.join("dir")), should.BeNil)
		assert.Loosely(c, fs.isMissing("dir"), should.BeTrue)
	})

	ftt.Run("EnsureDirectoryGone works with missing dir", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.EnsureDirectoryGone(ctx, fs.join("missing")), should.BeNil)
	})

}

func TestReplace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Replace checks root", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.Replace(ctx, fs.join("../abc"), fs.join("def")), should.NotBeNil)
		fs.write("def", "")
		assert.Loosely(c, fs.Replace(ctx, fs.join("def"), fs.join("../abc")), should.NotBeNil)
	})

	ftt.Run("Replace does nothing if oldpath == newpath", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.Replace(ctx, fs.join("abc"), fs.join("abc/d/..")), should.BeNil)
	})

	ftt.Run("Replace recognizes missing oldpath", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		assert.Loosely(c, fs.Replace(ctx, fs.join("missing"), fs.join("abc")), should.NotBeNil)
	})

	ftt.Run("Replace creates file and dir if missing", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123", "")
		assert.Loosely(c, fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), should.BeNil)
		assert.Loosely(c, fs.isMissing("a/123"), should.BeTrue)
		assert.Loosely(c, fs.isFile("b/c/d/"), should.BeTrue)
	})

	ftt.Run("Replace replaces regular file with a file", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123", "xxx")
		fs.write("b/c/d", "yyy")
		assert.Loosely(c, fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), should.BeNil)
		assert.Loosely(c, fs.isMissing("a/123"), should.BeTrue)
		assert.Loosely(c, fs.read("b/c/d"), should.Equal("xxx"))
	})

	ftt.Run("Replace replaces regular file with a dir", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123/456", "xxx")
		fs.write("b/c/d", "yyy")
		assert.Loosely(c, fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), should.BeNil)
		assert.Loosely(c, fs.isMissing("a/123"), should.BeTrue)
		assert.Loosely(c, fs.read("b/c/d/456"), should.Equal("xxx"))
	})

	ftt.Run("Replace replaces empty dir with a file", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123", "xxx")
		_, err := fs.EnsureDirectory(ctx, fs.join("b/c/d"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), should.BeNil)
		assert.Loosely(c, fs.isMissing("a/123"), should.BeTrue)
		assert.Loosely(c, fs.read("b/c/d"), should.Equal("xxx"))
	})

	ftt.Run("Replace replaces empty dir with a dir", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123/456", "xxx")
		_, err := fs.EnsureDirectory(ctx, fs.join("b/c/d"))
		assert.Loosely(c, err, should.BeNil)
		assert.Loosely(c, fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), should.BeNil)
		assert.Loosely(c, fs.isMissing("a/123"), should.BeTrue)
		assert.Loosely(c, fs.read("b/c/d/456"), should.Equal("xxx"))
	})

	ftt.Run("Replace replaces dir with a file", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123", "xxx")
		fs.write("b/c/d/456", "yyy")
		assert.Loosely(c, fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), should.BeNil)
		assert.Loosely(c, fs.isMissing("a/123"), should.BeTrue)
		assert.Loosely(c, fs.read("b/c/d"), should.Equal("xxx"))
	})

	ftt.Run("Replace replaces dir with a dir", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123/456", "xxx")
		fs.write("b/c/d/456", "yyy")
		assert.Loosely(c, fs.Replace(ctx, fs.join("a/123"), fs.join("b/c/d")), should.BeNil)
		assert.Loosely(c, fs.isMissing("a/123"), should.BeTrue)
		assert.Loosely(c, fs.read("b/c/d/456"), should.Equal("xxx"))
	})

	ftt.Run("Replace replaces file in dir path with a directory", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		fs.write("a/123", "xxx")
		fs.write("f", "yyy")
		assert.Loosely(c, fs.Replace(ctx, fs.join("f"), fs.join("a/123/456")), should.BeNil)
		assert.Loosely(c, fs.read("a/123/456"), should.Equal("yyy"))
	})

	ftt.Run("Replace works for very long paths", t, func(c *ftt.Test) {
		fs := tempFileSystem(t)
		// Windows runs into trouble with paths > 260 chars
		longPath1 := strings.Repeat("abc/", 260/4) + "file"
		longPath2 := strings.Repeat("xyz/", 260/4) + "file"
		assert.Loosely(c, len(longPath1), should.BeGreaterThan(260))
		fs.write(longPath1, "hi")
		fs.write(longPath2, "there")
		assert.Loosely(c, fs.Replace(ctx, fs.join(longPath1), fs.join(longPath2)), should.BeNil)
		assert.Loosely(c, fs.read(longPath2), should.Equal("hi"))
	})

	ftt.Run("Concurrency test", t, func(t *ftt.Test) {
		t.Skip("TODO(crbug/1442427): Test failed on windows.")

		const threads = 50
		fs := tempFileSystem(t)
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
func tempFileSystem(t testing.TB) *tempFileSystemImpl {
	return &tempFileSystemImpl{NewFileSystem(t.TempDir(), ""), t}
}

type tempFileSystemImpl struct {
	FileSystem
	c testing.TB
}

// join returns absolute path given a slash separated path relative to Root().
func (f *tempFileSystemImpl) join(path string) string {
	return filepath.Join(f.Root(), filepath.FromSlash(path))
}

// write creates a file at a given slash separated path relative to Root().
func (f *tempFileSystemImpl) write(rel string, data string) {
	abs := f.join(rel)
	err := os.MkdirAll(filepath.Dir(abs), 0777)
	assert.That(f.c, err, should.ErrLike(nil))
	file, err := os.Create(abs)
	assert.That(f.c, err, should.ErrLike(nil))
	file.WriteString(data)
	assert.That(f.c, file.Close(), should.ErrLike(nil))
}

// mkdir creates an empty directory.
func (f *tempFileSystemImpl) mkdir(rel string) {
	assert.That(f.c, os.MkdirAll(f.join(rel), 0777), should.ErrLike(nil))
}

// symlink creates a symlink.
func (f *tempFileSystemImpl) symlink(oldname, newname string) {
	assert.That(f.c, os.Symlink(f.join(oldname), f.join(newname)), should.ErrLike(nil))
}

// read reads an existing file at a given slash separated path relative to Root().
func (f *tempFileSystemImpl) read(rel string) string {
	fd, err := f.OpenFile(f.join(rel))
	assert.That(f.c, err, should.ErrLike(nil))
	defer fd.Close()

	data, err := io.ReadAll(fd)
	assert.That(f.c, err, should.ErrLike(nil))
	return string(data)
}

// readLink reads a symlink at a given slash separated path relative to Root().
func (f *tempFileSystemImpl) readLink(rel string) string {
	val, err := os.Readlink(f.join(rel))
	assert.That(f.c, err, should.ErrLike(nil))
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
