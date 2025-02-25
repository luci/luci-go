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
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestScanFileSystem(t *testing.T) {
	t.Parallel()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("Scan empty dir works", func(t *ftt.Test) {
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, files, should.BeEmpty)
		})

		t.Run("Discovering single file works", func(t *ftt.Test) {
			writeFile(tempDir, "single_file", "12345", 0666)
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(files), should.Equal(1))

			file := files[0]
			assert.Loosely(t, file.Name(), should.Equal("single_file"))
			assert.Loosely(t, file.Size(), should.Equal(uint64(5)))
			assert.Loosely(t, file.Executable(), should.BeFalse)

			r, err := file.Open()
			if r != nil {
				defer r.Close()
			}
			assert.Loosely(t, err, should.BeNil)
			buf, err := io.ReadAll(r)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buf, should.Match([]byte("12345")))
		})

		t.Run("Enumerating subdirectories", func(t *ftt.Test) {
			writeFile(tempDir, "a", "", 0666)
			writeFile(tempDir, "b", "", 0666)
			writeFile(tempDir, "1/a", "", 0666)
			writeFile(tempDir, "1/b", "", 0666)
			writeFile(tempDir, "1/2/a", "", 0666)
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			names := make([]string, len(files))
			for i, f := range files {
				names[i] = f.Name()
			}
			// Order matters. Slashes matters.
			assert.Loosely(t, names, should.Match([]string{
				"1/2/a",
				"1/a",
				"1/b",
				"a",
				"b",
			}))
		})

		t.Run("Empty subdirectories are skipped", func(t *ftt.Test) {
			mkDir(tempDir, "a")
			mkDir(tempDir, "1/2/3")
			mkDir(tempDir, "1/c")
			writeFile(tempDir, "1/d/file", "1234", 0666)
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(files), should.Equal(1))
			assert.Loosely(t, files[0].Name(), should.Equal("1/d/file"))
		})

		t.Run("Non root start path works", func(t *ftt.Test) {
			writeFile(tempDir, "a", "", 0666)
			writeFile(tempDir, "b", "", 0666)
			writeFile(tempDir, "1/a", "", 0666)
			writeFile(tempDir, "1/b", "", 0666)
			writeFile(tempDir, "1/2/a", "", 0666)
			files, err := ScanFileSystem(filepath.Join(tempDir, "1"), tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			names := make([]string, len(files))
			for i, f := range files {
				names[i] = f.Name()
			}
			// Order matters. Slashes matters.
			assert.Loosely(t, names, should.Match([]string{
				"1/2/a",
				"1/a",
				"1/b",
			}))
		})

		t.Run("Start path must be under root", func(t *ftt.Test) {
			_, err := ScanFileSystem(filepath.Dir(tempDir), tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Exclude filter works", func(t *ftt.Test) {
			writeFile(tempDir, "a", "", 0666)
			writeFile(tempDir, "b", "", 0666)
			writeFile(tempDir, "c/a", "", 0666)
			writeFile(tempDir, "1/a", "", 0666)
			writeFile(tempDir, "1/b", "", 0666)
			writeFile(tempDir, "1/2/a", "", 0666)

			// Exclude "a" and entire "1/" directory.
			var excluderCalls []string
			excluder := func(rel string) bool {
				excluderCalls = append(excluderCalls, rel)
				return rel == "a" || rel == "1"
			}

			files, err := ScanFileSystem(tempDir, tempDir, excluder, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(files), should.Equal(2))
			assert.Loosely(t, files[0].Name(), should.Equal("b"))
			assert.Loosely(t, files[1].Name(), should.Equal("c/a"))

			// "1/*" subdir should have been skipped completely.
			assert.Loosely(t, excluderCalls, should.Match([]string{
				"1", "a", "b", "c", filepath.FromSlash("c/a"),
			}))
		})

		t.Run(".cipd links turn into real files", func(t *ftt.Test) {
			writeFile(tempDir, ".cipd/pkgs/0/deadbeef/some_file", "hello", 0666)
			writeFile(tempDir, ".cipd/pkgs/0/deadbeef/some_executable", "#!/usr/bin/python", 0777)
			writeSymlink(tempDir, ".cipd/pkgs/0/current", "deadbeef")
			writeSymlink(tempDir, "some_executable", ".cipd/pkgs/0/current/some_executable")
			writeSymlink(tempDir, "some_file", ".cipd/pkgs/0/current/some_file")

			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(files), should.Equal(2))

			if runtime.GOOS != "windows" {
				assert.Loosely(t, files[0].Executable(), should.BeTrue)
			}

			assert.Loosely(t, files[1].Size(), should.Equal[uint64](5))
			assert.Loosely(t, files[1].Name(), should.Equal("some_file"))
			assert.Loosely(t, files[1].Symlink(), should.BeFalse)
			assert.Loosely(t, files[1].Executable(), should.BeFalse)
			rc, err := files[1].Open()
			assert.Loosely(t, err, should.BeNil)
			defer rc.Close()

			data, err := io.ReadAll(rc)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, string(data), should.Match("hello"))
		})

		if runtime.GOOS != "windows" {
			t.Run("Discovering single executable file works", func(t *ftt.Test) {
				writeFile(tempDir, "single_file", "12345", 0766)
				files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(files), should.Equal(1))
				file := files[0]
				assert.Loosely(t, file.Executable(), should.BeTrue)
			})

			t.Run("Relative symlink to outside of package cause error", func(t *ftt.Test) {
				writeSymlink(tempDir, "a/b1/rel_symlink", filepath.FromSlash("../../.."))
				_, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run("Scanning a symlink", func(t *ftt.Test) {
				writeFile(tempDir, "zzz/real_root/file", "12345", 0666)
				writeSymlink(tempDir, "symlink_root", filepath.FromSlash("zzz/real_root"))

				symlinkRootAbs := filepath.Join(tempDir, "symlink_root")

				files, err := ScanFileSystem(symlinkRootAbs, symlinkRootAbs, nil, ScanOptions{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(files), should.Equal(1))
				assert.Loosely(t, files[0].Name(), should.Equal("file"))
			})
		}
	})
}

func TestWrapFile(t *testing.T) {
	t.Parallel()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()

		t.Run("WrapFile simple file works", func(t *ftt.Test) {
			writeFile(tempDir, "dir/a/b", "12345", 0666)
			out, err := WrapFile(filepath.Join(tempDir, "dir", "a", "b"), tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, out.Name(), should.Equal("dir/a/b"))
		})

		t.Run("WrapFile directory fails", func(t *ftt.Test) {
			mkDir(tempDir, "dir")
			_, err := WrapFile(filepath.Join(tempDir, "dir"), tempDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("WrapFile outside of root fails", func(t *ftt.Test) {
			mkDir(tempDir, "a")
			writeFile(tempDir, "b", "body", 0666)
			_, err := WrapFile(filepath.Join(tempDir, "b"), filepath.Join(tempDir, "a"), nil, ScanOptions{})
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("WrapFile outside of root fails (tricky path)", func(t *ftt.Test) {
			mkDir(tempDir, "a")
			// "abc" starts with "a", it tricks naive string.HasPrefix subpath check.
			writeFile(tempDir, "abc", "body", 0666)
			_, err := WrapFile(filepath.Join(tempDir, "abc"), filepath.Join(tempDir, "a"), nil, ScanOptions{})
			assert.Loosely(t, err, should.NotBeNil)
		})

		if runtime.GOOS != "windows" {
			t.Run("WrapFile executable file works", func(t *ftt.Test) {
				writeFile(tempDir, "single_file", "12345", 0766)
				out, err := WrapFile(filepath.Join(tempDir, "single_file"), tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, out.Executable(), should.BeTrue)
			})

			t.Run("WrapFile rel symlink in root", func(t *ftt.Test) {
				writeSymlink(tempDir, "a/b/c", filepath.FromSlash("../../d"))
				mkDir(tempDir, "d")
				out, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.BeNil)
				ensureSymlinkTarget(t, out, "../../d")
			})

			t.Run("WrapFile .cipd symlink", func(t *ftt.Test) {
				writeFile(tempDir, ".cipd/pkgs/0/deadbeef/some_executable", "#!/usr/bin/python", 0777)
				writeSymlink(tempDir, ".cipd/pkgs/0/current", "deadbeef")
				writeSymlink(tempDir, "some_executable", ".cipd/pkgs/0/current/some_executable")

				out, err := WrapFile(filepath.Join(tempDir, "some_executable"), tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.BeNil)
				if runtime.GOOS != "windows" {
					assert.Loosely(t, out.Executable(), should.BeTrue)
				}
				assert.Loosely(t, out.Symlink(), should.BeFalse)
				assert.That(t, out.Size(), should.Equal[uint64](17))
			})

			t.Run("WrapFile rel symlink outside root", func(t *ftt.Test) {
				writeSymlink(tempDir, "a/b/c", filepath.FromSlash("../../../d"))
				_, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run("WrapFile abs symlink in root", func(t *ftt.Test) {
				writeSymlink(tempDir, "a/b/c", filepath.Join(tempDir, "a", "d"))
				out, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.BeNil)
				ensureSymlinkTarget(t, out, "../d")
			})

			t.Run("WrapFile abs symlink outside root", func(t *ftt.Test) {
				writeSymlink(tempDir, "a/b/c", filepath.Dir(tempDir))
				out, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				assert.Loosely(t, err, should.BeNil)
				ensureSymlinkTarget(t, out, filepath.ToSlash(filepath.Dir(tempDir)))
			})
		}
	})
}

func mkDir(root string, path string) {
	abs := filepath.Join(root, filepath.FromSlash(path))
	err := os.MkdirAll(abs, 0777)
	if err != nil {
		panic("Failed to create a directory under temp directory")
	}
}

func writeFile(root string, path string, data string, mode os.FileMode) {
	abs := filepath.Join(root, filepath.FromSlash(path))
	os.MkdirAll(filepath.Dir(abs), 0777)
	err := os.WriteFile(abs, []byte(data), mode)
	if err != nil {
		panic("Failed to write a temp file")
	}
}

func writeSymlink(root string, path string, target string) {
	abs := filepath.Join(root, filepath.FromSlash(path))
	os.MkdirAll(filepath.Dir(abs), 0777)
	err := os.Symlink(target, abs)
	if err != nil {
		panic("Failed to create symlink")
	}
}

func writeFileToDest(t testing.TB, dest Destination, name string, opts CreateFileOptions, data string) {
	writer, err := dest.CreateFile(context.Background(), name, opts)
	assert.Loosely(t, err, should.BeNil)
	_, err = writer.Write([]byte(data))
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, writer.Close(), should.BeNil)
}

func writeAttrFileToDest(t testing.TB, dest Destination, name string, attr WinAttrs, data string) {
	writer, err := dest.CreateFile(context.Background(), name, CreateFileOptions{WinAttrs: attr})
	assert.Loosely(t, err, should.BeNil)
	_, err = writer.Write([]byte(data))
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, writer.Close(), should.BeNil)
}

func writeSymlinkToDest(t testing.TB, dest Destination, name string, target string) {
	assert.Loosely(t, dest.CreateSymlink(context.Background(), name, target), should.BeNil)
}

func ensureSymlinkTarget(t testing.TB, file File, target string) {
	assert.Loosely(t, file.Symlink(), should.BeTrue)
	discoveredTarget, err := file.SymlinkTarget()
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, discoveredTarget, should.Equal(target))
}

func createBunchOfFiles(t testing.TB, dest Destination, tempDir string) {
	testMTime := time.Date(2018, 1, 1, 1, 0, 0, 0, time.UTC)
	writeFileToDest(t, dest, "a", CreateFileOptions{}, "a data")
	writeFileToDest(t, dest, "exe", CreateFileOptions{Executable: true}, "exe data")
	writeFileToDest(t, dest, "dir/c", CreateFileOptions{}, "dir/c data")
	writeFileToDest(t, dest, "dir/dir/d", CreateFileOptions{}, "dir/dir/d data")
	writeFileToDest(t, dest, "ts", CreateFileOptions{ModTime: testMTime}, "ts data")
	writeFileToDest(t, dest, "wr", CreateFileOptions{Writable: true}, "wr data")
	if runtime.GOOS != "windows" {
		writeSymlinkToDest(t, dest, "abs_symlink", filepath.FromSlash(tempDir))
		writeSymlinkToDest(t, dest, "dir/dir/rel_symlink", "../../a")
	} else {
		writeAttrFileToDest(t, dest, "secret_file", WinAttrHidden, "ninja")
		writeAttrFileToDest(t, dest, "system_file", WinAttrSystem, "system")
	}
}

func checkBunchOfFiles(t testing.TB, destDir, tempDir string) {
	testMTime := time.Date(2018, 1, 1, 1, 0, 0, 0, time.UTC)

	// Ensure everything is there.
	files, err := ScanFileSystem(destDir, destDir, nil, ScanOptions{
		PreserveModTime:  true,
		PreserveWritable: true,
	})
	assert.Loosely(t, err, should.BeNil)
	names := make([]string, len(files))
	mapping := make(map[string]File, len(files))
	for i, f := range files {
		names[i] = f.Name()
		mapping[f.Name()] = f
	}

	if runtime.GOOS == "windows" {
		assert.Loosely(t, names, should.Match([]string{
			"a",
			"dir/c",
			"dir/dir/d",
			"exe",
			"secret_file",
			"system_file",
			"ts",
			"wr",
		}))
	} else {
		assert.Loosely(t, names, should.Match([]string{
			"a",
			"abs_symlink",
			"dir/c",
			"dir/dir/d",
			"dir/dir/rel_symlink",
			"exe",
			"ts",
			"wr",
		}))
	}

	// Ensure data is valid (check first file only).
	r, err := mapping["a"].Open()
	if r != nil {
		defer r.Close()
	}
	assert.Loosely(t, err, should.BeNil)
	data, err := io.ReadAll(r)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, data, should.Match([]byte("a data")))

	// File mode and symlinks are valid.
	if runtime.GOOS != "windows" {
		assert.Loosely(t, mapping["a"].Writable(), should.BeFalse)
		assert.Loosely(t, mapping["exe"].Executable(), should.BeTrue)
		assert.Loosely(t, mapping["ts"].ModTime(), should.Match(testMTime))
		assert.Loosely(t, mapping["wr"].Writable(), should.BeTrue)
		ensureSymlinkTarget(t, mapping["abs_symlink"], filepath.FromSlash(tempDir))
		ensureSymlinkTarget(t, mapping["dir/dir/rel_symlink"], "../../a")
	} else {
		assert.Loosely(t, mapping["secret_file"].WinAttrs()&WinAttrHidden, should.Equal(WinAttrHidden))
		assert.Loosely(t, mapping["system_file"].WinAttrs()&WinAttrSystem, should.Equal(WinAttrSystem))
	}

	// Ensure no temp files left.
	allFiles, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
	assert.Loosely(t, len(allFiles), should.Equal(len(files)))
}

func TestExistingDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		destDir := filepath.Join(tempDir, "dest")
		dest := ExistingDestination(destDir, nil)

		readFromDest := func(name string) string {
			b, err := os.ReadFile(filepath.Join(destDir, name))
			assert.Loosely(t, err, should.BeNil)
			return string(b)
		}

		t.Run("Creates files", func(t *ftt.Test) {
			writeFileToDest(t, dest, "a/b/c", CreateFileOptions{}, "123")
			assert.Loosely(t, readFromDest("a/b/c"), should.Equal("123"))
		})

		t.Run("CreateFile can override read-only files, atomic mode", func(t *ftt.Test) {
			writeFileToDest(t, dest, "a/b/c", CreateFileOptions{}, "123")
			writeFileToDest(t, dest, "a/b/c", CreateFileOptions{}, "456")
			assert.Loosely(t, readFromDest("a/b/c"), should.Equal("456"))
		})

		t.Run("CreateFile can override read-only files, non-atomic mode", func(t *ftt.Test) {
			dest.(*fsDest).atomic = false
			writeFileToDest(t, dest, "a/b/c", CreateFileOptions{}, "123")
			writeFileToDest(t, dest, "a/b/c", CreateFileOptions{}, "456")
			assert.Loosely(t, readFromDest("a/b/c"), should.Equal("456"))
		})

		t.Run("Can't have two instances of a file open at the same time", func(t *ftt.Test) {
			wr, err := dest.CreateFile(ctx, "a/b/c", CreateFileOptions{})
			assert.Loosely(t, err, should.BeNil)

			// Open same file (perhaps via lexically different path).
			_, err = dest.CreateFile(ctx, "a/b/d/../c", CreateFileOptions{})
			assert.Loosely(t, err.Error(), should.Equal(`"a/b/d/../c": already open`))

			assert.Loosely(t, wr.Close(), should.BeNil)

			// Can be opened now.
			wr, err = dest.CreateFile(ctx, "a/b/d/../c", CreateFileOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, wr.Close(), should.BeNil)
		})

		t.Run("CreateFile rejects invalid relative paths", func(t *ftt.Test) {
			// Rel path that is still inside the package is ok.
			wr, err := dest.CreateFile(ctx, "a/b/c/../../../d", CreateFileOptions{})
			assert.Loosely(t, err, should.BeNil)
			wr.Close()
			// Rel path pointing outside is forbidden.
			_, err = dest.CreateFile(ctx, "a/b/c/../../../../d", CreateFileOptions{})
			assert.Loosely(t, err, should.NotBeNil)
		})

		if runtime.GOOS != "windows" {
			t.Run("CreateSymlink rejects invalid relative paths", func(t *ftt.Test) {
				// Rel symlink to a file inside the destination is OK.
				assert.Loosely(t, dest.CreateSymlink(ctx, "a/b/c", "../.."), should.BeNil)
				// Rel symlink to a file outside -> error.
				assert.Loosely(t, dest.CreateSymlink(ctx, "a/b/c", "../../.."), should.NotBeNil)
			})
		}

		t.Run("Creating a bunch of files", func(t *ftt.Test) {
			createBunchOfFiles(t, dest, tempDir)
			checkBunchOfFiles(t, destDir, tempDir)
		})
	})
}

func TestNewDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("Given a temp directory", t, func(t *ftt.Test) {
		tempDir := t.TempDir()
		destDir := filepath.Join(tempDir, "dest")
		dest := NewDestination(destDir, nil)

		t.Run("Empty success write works", func(t *ftt.Test) {
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			assert.Loosely(t, dest.End(ctx, true), should.BeNil)

			// Should create a new directory.
			stat, err := os.Stat(destDir)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, stat.IsDir(), should.BeTrue)

			// And it should be empty.
			files, err := ScanFileSystem(destDir, destDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(files), should.BeZero)
		})

		t.Run("Empty failed write works", func(t *ftt.Test) {
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			assert.Loosely(t, dest.End(ctx, false), should.BeNil)

			// Doesn't create a directory.
			_, err := os.Stat(destDir)
			assert.Loosely(t, os.IsNotExist(err), should.BeTrue)
		})

		t.Run("Double begin or double end fails", func(t *ftt.Test) {
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			assert.Loosely(t, dest.Begin(ctx), should.NotBeNil)
			assert.Loosely(t, dest.End(ctx, true), should.BeNil)
			assert.Loosely(t, dest.End(ctx, true), should.NotBeNil)
		})

		t.Run("CreateFile works only when destination is open", func(t *ftt.Test) {
			wr, err := dest.CreateFile(ctx, "testing", CreateFileOptions{Executable: true})
			assert.Loosely(t, wr, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("Committing bunch of files works", func(t *ftt.Test) {
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			createBunchOfFiles(t, dest, tempDir)
			assert.Loosely(t, dest.End(ctx, true), should.BeNil)
			checkBunchOfFiles(t, destDir, tempDir)
		})

		t.Run("Rolling back bunch of files works", func(t *ftt.Test) {
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			createBunchOfFiles(t, dest, tempDir)
			assert.Loosely(t, dest.End(ctx, false), should.BeNil)

			// No dest directory.
			_, err := os.Stat(destDir)
			assert.Loosely(t, os.IsNotExist(err), should.BeTrue)

			// Ensure no temp files left.
			allFiles, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			assert.Loosely(t, len(allFiles), should.BeZero)
		})

		t.Run("Overwriting a directory works", func(t *ftt.Test) {
			// Create dest directory manually with some stuff.
			err := os.Mkdir(destDir, 0777)
			assert.Loosely(t, err, should.BeNil)
			err = os.WriteFile(filepath.Join(destDir, "data"), []byte("data"), 0666)
			assert.Loosely(t, err, should.BeNil)

			// Now deploy something to it.
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			createBunchOfFiles(t, dest, tempDir)
			assert.Loosely(t, dest.End(ctx, true), should.BeNil)

			// Overwritten.
			checkBunchOfFiles(t, destDir, tempDir)
		})

		t.Run("Not overwriting a directory works", func(t *ftt.Test) {
			// Create dest directory manually with some stuff.
			err := os.Mkdir(destDir, 0777)
			assert.Loosely(t, err, should.BeNil)
			err = os.WriteFile(filepath.Join(destDir, "data"), []byte("data"), 0666)
			assert.Loosely(t, err, should.BeNil)

			// Now attempt deploy something to it, but roll back.
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			createBunchOfFiles(t, dest, tempDir)
			assert.Loosely(t, dest.End(ctx, false), should.BeNil)

			// Kept as is.
			files, err := ScanFileSystem(destDir, destDir, nil, ScanOptions{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(files), should.Equal(1))
			assert.Loosely(t, files[0].Name(), should.Equal("data"))
		})

		t.Run("End with opened files fail", func(t *ftt.Test) {
			assert.Loosely(t, dest.Begin(ctx), should.BeNil)
			w, err := dest.CreateFile(ctx, "a", CreateFileOptions{})
			assert.Loosely(t, w, should.NotBeNil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, dest.End(ctx, true), should.NotBeNil)
			w.Close()
			assert.Loosely(t, dest.End(ctx, true), should.BeNil)
		})
	})
}
