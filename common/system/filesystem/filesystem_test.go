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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestIsNotExist(t *testing.T) {
	t.Parallel()

	tdir := t.TempDir()
	dnePath := filepath.Join(tdir, "anything")
	_, err := os.Open(dnePath)
	if !os.IsNotExist(err) {
		t.Fatalf("failed to get IsNotExist error: %s", err)
	}

	ftt.Run(`IsNotExist works`, t, func(t *ftt.Test) {
		assert.Loosely(t, IsNotExist(nil), should.BeFalse)
		assert.Loosely(t, IsNotExist(errors.New("something")), should.BeFalse)

		assert.Loosely(t, IsNotExist(err), should.BeTrue)
		assert.Loosely(t, IsNotExist(errors.Annotate(err, "annotated").Err()), should.BeTrue)
	})
}

func TestAbsPath(t *testing.T) {
	t.Parallel()

	tdir := t.TempDir()
	tdirName := filepath.Base(tdir)

	ftt.Run(`AbsPath works`, t, func(t *ftt.Test) {
		base := filepath.Join(tdir, "..", tdirName, "file.txt")
		assert.Loosely(t, AbsPath(&base), should.BeNil)
		assert.Loosely(t, base, should.Equal(filepath.Join(tdir, "file.txt")))
	})
}

func TestTouch(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing Touch`, t, func(t *ftt.Test) {
		tdir := t.TempDir()

		thePast := time.Now().Add(-10 * time.Second)

		t.Run(`Can update a directory timestamp`, func(t *ftt.Test) {
			path := filepath.Join(tdir, "subdir")

			assert.Loosely(t, os.Mkdir(path, 0755), should.BeNil)
			st, err := os.Lstat(path)
			assert.Loosely(t, err, should.BeNil)
			initialModTime := st.ModTime()

			assert.Loosely(t, Touch(path, thePast, 0), should.BeNil)
			st, err = os.Lstat(path)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, st.ModTime(), should.HappenBefore(initialModTime))
		})

		t.Run(`Can update an empty file timestamp`, func(t *ftt.Test) {
			path := filepath.Join(tdir, "touch")

			assert.Loosely(t, Touch(path, time.Time{}, 0644), should.BeNil)
			st, err := os.Lstat(path)
			assert.Loosely(t, err, should.BeNil)
			initialModTime := st.ModTime()

			assert.Loosely(t, Touch(path, thePast, 0), should.BeNil)
			st, err = os.Lstat(path)
			assert.Loosely(t, err, should.BeNil)
			pastModTime := st.ModTime()
			assert.Loosely(t, pastModTime, should.HappenBefore(initialModTime))

			// Touch back to "now".
			assert.Loosely(t, Touch(path, time.Time{}, 0644), should.BeNil)
			st, err = os.Lstat(path)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, st.ModTime(), should.HappenOnOrAfter(initialModTime))
		})

		t.Run(`Can update a populated file timestamp`, func(t *ftt.Test) {
			path := filepath.Join(tdir, "touch")

			assert.Loosely(t, os.WriteFile(path, []byte("sup"), 0644), should.BeNil)
			st, err := os.Lstat(path)
			assert.Loosely(t, err, should.BeNil)
			initialModTime := st.ModTime()

			assert.Loosely(t, Touch(path, thePast, 0), should.BeNil)
			st, err = os.Lstat(path)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, st.ModTime(), should.HappenBefore(initialModTime))

			content, err := os.ReadFile(path)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, content, should.Match([]byte("sup")))
		})
	})
}

func TestMakeReadOnly(t *testing.T) {
	t.Parallel()

	ftt.Run(`Can RemoveAll`, t, func(t *ftt.Test) {
		tdir := t.TempDir()
		defer func() {
			assert.Loosely(t, recursiveChmod(tdir, nil, func(mode os.FileMode) os.FileMode { return mode | 0222 }), should.BeNil)
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

		t.Run(`Can mark the entire subdirectory read-only`, func(t *ftt.Test) {
			assert.Loosely(t, MakeReadOnly(tdir, nil), should.BeNil)
			filepath.Walk(tdir, func(path string, info os.FileInfo, err error) error {
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, info.Mode()&0222, should.BeZero)
				return nil
			})
		})

		t.Run(`Can selectively mark files read-only`, func(t *ftt.Test) {
			// Don't mark <TMP>/foo read-only.
			fooPath := filepath.Join(tdir, "foo")

			assert.Loosely(t, MakeReadOnly(tdir, func(path string) bool {
				return path != fooPath
			}), should.BeNil)

			filepath.Walk(tdir, func(path string, info os.FileInfo, err error) error {
				assert.Loosely(t, err, should.BeNil)
				if path == fooPath {
					assert.Loosely(t, info.Mode()&0222, should.NotEqual(0))
				} else {
					assert.Loosely(t, info.Mode()&0222, should.BeZero)
				}
				return nil
			})
		})
	})
}

func TestRemoveAll(t *testing.T) {
	t.Parallel()

	ftt.Run(`Can RemoveAll`, t, func(t *ftt.Test) {
		tdir := t.TempDir()
		defer func() {
			assert.Loosely(t, recursiveChmod(tdir, nil, func(mode os.FileMode) os.FileMode { return mode | 0222 }), should.BeNil)
		}()
		subDir := filepath.Join(tdir, "sub")

		t.Run(`With directory contents...`, func(t *ftt.Test) {
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
			assert.Loosely(t, os.Symlink("dummy", filepath.Join(subDir, "invalid_symlink")), should.BeNil)
			assert.Loosely(t, os.Remove(filepath.Join(subDir, "dummy")), should.BeNil)

			t.Run(`Can remove the directory`, func(t *ftt.Test) {
				assert.Loosely(t, RemoveAll(subDir), should.BeNil)
				assert.Loosely(t, isGone(subDir), should.BeTrue)
			})

			t.Run(`Removing inside a read-only dir...`, func(t *ftt.Test) {
				assert.Loosely(t, MakeReadOnly(subDir, nil), should.BeNil)
				qux := filepath.Join(subDir, "qux")
				if runtime.GOOS == "windows" {
					t.Run(`... is fine on Windows`, func(t *ftt.Test) {
						assert.Loosely(t, RemoveAll(qux), should.BeNil)
						assert.Loosely(t, isGone(qux), should.BeTrue)
					})
				} else {
					t.Run(`... fails on POSIX`, func(t *ftt.Test) {
						assert.Loosely(t, RemoveAll(qux), should.NotBeNil)
						assert.Loosely(t, isGone(qux), should.BeFalse)
					})
				}
			})

			t.Run(`Can remove the directory when it is read-only`, func(t *ftt.Test) {
				// Make the directory read-only, and assert that classic os.RemoveAll
				// fails. Except on Windows it doesn't, see:
				// https://github.com/golang/go/issues/26295.
				assert.Loosely(t, MakeReadOnly(subDir, nil), should.BeNil)
				if runtime.GOOS != "windows" {
					assert.Loosely(t, os.RemoveAll(subDir), should.NotBeNil)
				}

				assert.Loosely(t, RemoveAll(subDir), should.BeNil)
				assert.Loosely(t, isGone(subDir), should.BeTrue)
			})

			t.Run(`Can remove the directory when it has read-only files`, func(t *ftt.Test) {
				readOnly := filepath.Join(subDir, "foo", "bar")
				assert.Loosely(t, MakeReadOnly(readOnly, nil), should.BeNil)
				assert.Loosely(t, RemoveAll(subDir), should.BeNil)
				assert.Loosely(t, isGone(subDir), should.BeTrue)
			})
		})

		t.Run(`Will return nil if the target does not exist`, func(t *ftt.Test) {
			assert.Loosely(t, RemoveAll(filepath.Join(tdir, "dne")), should.BeNil)
		})
	})
}

func TestRenamingRemoveAll(t *testing.T) {
	t.Parallel()

	ftt.Run(`Can RenamingRemoveAll`, t, func(t *ftt.Test) {
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

		t.Run(`No dir to rename into specified`, func(t *ftt.Test) {
			renamedTo, err := RenamingRemoveAll(fooDir, "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isGone(fooDir), should.BeTrue)
			assert.Loosely(t, renamedTo, should.HavePrefix(filepath.Join(subDir, ".trash-")))
		})

		t.Run(`Renaming to specified dir works`, func(t *ftt.Test) {
			renamedTo, err := RenamingRemoveAll(fooDir, tdir)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isGone(fooDir), should.BeTrue)
			assert.Loosely(t, renamedTo, should.HavePrefix(filepath.Join(tdir, ".trash-")))
		})

		t.Run(`Renaming to specified dir fails due to not exist error`, func(t *ftt.Test) {
			renamedTo, err := RenamingRemoveAll(fooDir, filepath.Join(tdir, "not-exists"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isGone(fooDir), should.BeTrue)
			assert.Loosely(t, renamedTo, should.BeEmpty)
		})

		t.Run(`Will return nil if the target does not exist`, func(t *ftt.Test) {
			_, err := RenamingRemoveAll(filepath.Join(tdir, "not-exists"), "")
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestReadableCopy(t *testing.T) {
	ftt.Run("ReadableCopy", t, func(t *ftt.Test) {
		dir := t.TempDir()
		out := filepath.Join(dir, "out")
		in := filepath.Join(dir, "in")
		content := []byte("test")
		assert.Loosely(t, os.WriteFile(in, content, 0644), should.BeNil)

		// Change umask on unix so that test is not affected by default umask.
		old := umask(022)
		assert.Loosely(t, ReadableCopy(out, in), should.BeNil)
		umask(old)

		buf, err := os.ReadFile(out)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buf, should.Match(content))

		ostat, err := os.Stat(out)
		assert.Loosely(t, err, should.BeNil)
		istat, err := os.Stat(in)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, ostat.Mode(), should.Equal(addReadMode(istat.Mode())))
	})
}

func TestCopy(t *testing.T) {
	ftt.Run("Copy", t, func(t *ftt.Test) {
		dir := t.TempDir()
		out := filepath.Join(dir, "out")
		in := filepath.Join(dir, "in")
		content := []byte("test")
		assert.Loosely(t, os.WriteFile(in, content, 0o644), should.BeNil)

		// Change umask on unix so that test is not affected by default umask.
		old := umask(0o22)
		assert.Loosely(t, Copy(out, in, 0o644), should.BeNil)
		umask(old)

		buf, err := os.ReadFile(out)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buf, should.Match(content))

		ostat, err := os.Stat(out)
		assert.Loosely(t, err, should.BeNil)
		_, err = os.Stat(in)
		assert.Loosely(t, err, should.BeNil)

		if runtime.GOOS != "windows" {
			assert.Loosely(t, ostat.Mode(), should.Equal(0o644))
		}
	})
}

func TestHardlinkRecursively(t *testing.T) {
	t.Parallel()

	checkFile := func(path, content string) {
		buf, err := os.ReadFile(path)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Match(content))
	}

	ftt.Run("HardlinkRecursively", t, func(t *ftt.Test) {
		dir := t.TempDir()
		src1 := filepath.Join(dir, "src1")
		src2 := filepath.Join(src1, "src2")
		assert.Loosely(t, os.MkdirAll(src2, 0755), should.BeNil)

		assert.Loosely(t, os.WriteFile(filepath.Join(src1, "file1"), []byte("test1"), 0644), should.BeNil)
		assert.Loosely(t, os.WriteFile(filepath.Join(src2, "file2"), []byte("test2"), 0644), should.BeNil)

		assert.Loosely(t, os.Symlink(filepath.Join("src2", "file2"), filepath.Join(src1, "link")), should.BeNil)

		dst := filepath.Join(dir, "dst")
		assert.Loosely(t, HardlinkRecursively(src1, dst), should.BeNil)

		checkFile(filepath.Join(dst, "file1"), "test1")
		checkFile(filepath.Join(dst, "src2", "file2"), "test2")
		checkFile(filepath.Join(dst, "link"), "test2")

		stat, err := os.Stat(filepath.Join(dst, "link"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, stat.Mode()&os.ModeSymlink, should.BeZero)
	})

	ftt.Run("HardlinkRecursively nested", t, func(t *ftt.Test) {
		dir := t.TempDir()
		top := filepath.Join(dir, "top")
		nested := filepath.Join(top, "nested")
		assert.Loosely(t, os.MkdirAll(nested, 0755), should.BeNil)

		assert.Loosely(t, os.WriteFile(filepath.Join(nested, "file"), []byte("test"), 0644), should.BeNil)

		dstTop := filepath.Join(dir, "dst")
		assert.Loosely(t, HardlinkRecursively(top, dstTop), should.BeNil)
		dstDested := filepath.Join(dstTop, "nested")
		assert.Loosely(t, HardlinkRecursively(nested, dstDested), should.NotBeNil)
	})
}

func TestCreateDirectories(t *testing.T) {
	t.Parallel()
	ftt.Run("CreateDirectories", t, func(t *ftt.Test) {
		dir := t.TempDir()
		assert.Loosely(t, CreateDirectories(dir, []string{}), should.BeNil)

		var paths []string
		correctFiles := func(path string, info os.FileInfo, err error) error {
			paths = append(paths, path)
			return nil
		}

		assert.Loosely(t, filepath.Walk(dir, correctFiles), should.BeNil)
		assert.Loosely(t, paths, should.Match([]string{dir}))

		assert.Loosely(t, CreateDirectories(dir, []string{
			filepath.Join("a", "b"),
			filepath.Join("c", "d", "e"),
			filepath.Join("c", "f"),
			filepath.Join("g"),
		}), should.BeNil)

		paths = nil
		assert.Loosely(t, filepath.Walk(dir, correctFiles), should.BeNil)
		assert.Loosely(t, paths, should.Match([]string{
			dir,
			filepath.Join(dir, "a"),
			filepath.Join(dir, "c"),
			filepath.Join(dir, "c", "d"),
		}))
	})
}

func TestIsEmptyDir(t *testing.T) {
	ftt.Run("IsEmptyDir", t, func(t *ftt.Test) {
		dir := t.TempDir()
		emptyDir := filepath.Join(dir, "empty")
		assert.Loosely(t, MakeDirs(emptyDir), should.BeNil)

		isEmpty, err := IsEmptyDir(emptyDir)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, isEmpty, should.BeTrue)

		nonEmptyDir := filepath.Join(dir, "non-empty")
		assert.Loosely(t, MakeDirs(nonEmptyDir), should.BeNil)
		file1 := filepath.Join(nonEmptyDir, "file1")
		assert.Loosely(t, os.WriteFile(file1, []byte("test1"), 0644), should.BeNil)

		isEmpty, err = IsEmptyDir(nonEmptyDir)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, isEmpty, should.BeFalse)

		_, err = IsEmptyDir(file1)
		assert.Loosely(t, err, should.NotBeNil)

		_, err = IsEmptyDir(filepath.Join(dir, "not-existing"))
		assert.Loosely(t, err, should.NotBeNil)
	})
}

func TestIsDir(t *testing.T) {
	ftt.Run("IsDir", t, func(t *ftt.Test) {
		dir := t.TempDir()
		b, err := IsDir(dir)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, b, should.BeTrue)

		b, err = IsDir(filepath.Join(dir, "not_exists"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, b, should.BeFalse)

		file := filepath.Join(dir, "file")
		assert.Loosely(t, os.WriteFile(file, []byte(""), 0644), should.BeNil)
		b, err = IsDir(file)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, b, should.BeFalse)
	})
}

func TestGetFreeSpace(t *testing.T) {
	t.Parallel()

	ftt.Run("GetFreeSpace", t, func(t *ftt.Test) {
		size, err := GetFreeSpace(".")
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, size, should.BeGreaterThan[uint64](4096))
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

	ftt.Run(`GetCommonAncestor`, t, func(t *ftt.Test) {
		t.Run(`common ancestor logic`, func(t *ftt.Test) {
			t.Run(`common`, func(t *ftt.Test) {
				tdir := t.TempDir()
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "a", "b", "c", "d"), 0o777), should.BeNil)
				if fsCaseSensitive {
					assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "a", "B", "c"), 0o777), should.BeNil)
					assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "A", "b", "c", "d"), 0o777), should.BeNil)
				}
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "a", "1", "2", "3"), 0o777), should.BeNil)
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "r", "o", "o", "t"), 0o777), should.BeNil)

				assert.Loosely(t, Touch(filepath.Join(tdir, "a", "B", "c", "something"), time.Now(), 0o666), should.BeNil)
				assert.Loosely(t, Touch(filepath.Join(tdir, "a", "b", "else"), time.Now(), 0o666), should.BeNil)

				t.Run(`stops at 'local' common ancestor`, func(t *ftt.Test) {
					common, err := GetCommonAncestor([]string{
						filepath.Join(tdir, "a", "b", "c", "d"),
						filepath.Join(tdir, "a", "B", "c", "something"),
						filepath.Join(tdir, "A", "b", "c", "d"),
						filepath.Join(tdir, "a", "b", "else"),
					}, []string{".git"})
					assert.Loosely(t, err, should.BeNil)
					if fsCaseSensitive {
						assert.Loosely(t, common, should.Match(tdir+sep))
					} else {
						assert.Loosely(t, common, should.Match(filepath.Join(tdir, "a", "b")+sep))
					}
				})

				t.Run(`walks up to fs root`, func(t *ftt.Test) {
					common, err := GetCommonAncestor([]string{
						filepath.VolumeName(tdir) + sep,
						filepath.Join(tdir, "a", "B", "c", "something"),
						filepath.Join(tdir, "A", "b", "c", "d"),
						filepath.Join(tdir, "a", "b", "else"),
					}, []string{".git"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, common, should.Match(filepath.VolumeName(tdir)+sep))
				})
			})

			t.Run(`slash comparison`, func(t *ftt.Test) {
				tdir := t.TempDir()
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "longpath", "a", "c", "d"), 0o777), should.BeNil)
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "a", "longpath", "c", "d"), 0o777), should.BeNil)

				common, err := GetCommonAncestor([]string{
					filepath.Join(tdir, "longpath", "a", "c", "d"),
					filepath.Join(tdir, "a", "longpath", "c", "d"),
				}, []string{".git"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, common, should.Match(tdir+sep))
			})

			t.Run(`non exist`, func(t *ftt.Test) {
				_, err := GetCommonAncestor([]string{
					filepath.Join("i", "dont", "exist"),
				}, []string{".git"})
				assert.Loosely(t, err, should.ErrLike("reading path[0]"))
			})

			t.Run(`sentinel`, func(t *ftt.Test) {
				tdir := t.TempDir()
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "repoA", ".git"), 0o777), should.BeNil)
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "repoA", "something"), 0o777), should.BeNil)
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "repoB", ".git"), 0o777), should.BeNil)
				assert.Loosely(t, os.MkdirAll(filepath.Join(tdir, "repoB", "else"), 0o777), should.BeNil)

				_, err := GetCommonAncestor([]string{
					filepath.Join(tdir, "repoA", "something"),
					filepath.Join(tdir, "repoB", "else"),
				}, []string{".git"})
				assert.Loosely(t, err, should.ErrLike("hit root sentinel"))
				assert.Loosely(t, errors.Is(err, ErrRootSentinel), should.BeTrue)
			})

			if runtime.GOOS == "windows" {
				t.Run(`different volumes`, func(t *ftt.Test) {
					tdir := t.TempDir()
					_, err := GetCommonAncestor([]string{
						tdir,
						`\\fake\network\share`,
					}, nil)
					assert.Loosely(t, err, should.ErrLike("originate on different volumes"))
				})
			}
		})

		t.Run(`helpers`, func(t *ftt.Test) {
			if runtime.GOOS == "windows" {
				t.Run(`windows paths`, func(t *ftt.Test) {
					assert.Loosely(t, findPathSeparators(`D:\something\`), should.Match([]int{2, 12}))
					assert.Loosely(t, findPathSeparators(`D:\`), should.Match([]int{2}))
					assert.Loosely(t, findPathSeparators(`\\some\host\something\`),
						should.Match([]int{11, 21}))
					assert.Loosely(t, findPathSeparators(`\\some\host\`),
						should.Match([]int{11}))
					assert.Loosely(t, findPathSeparators(`\\?\C:\Test\`),
						should.Match([]int{6, 11}))
				})
			} else {
				t.Run(`*nix paths`, func(t *ftt.Test) {
					assert.Loosely(t, findPathSeparators(`/something/`),
						should.Match([]int{0, 10}))
					assert.Loosely(t, findPathSeparators(`/`),
						should.Match([]int{0}))
				})
			}
		})
	})
}
