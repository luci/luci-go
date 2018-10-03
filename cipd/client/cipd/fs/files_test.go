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
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestScanFileSystem(t *testing.T) {
	t.Parallel()

	Convey("Given a temp directory", t, func() {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		Convey("Scan empty dir works", func() {
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			So(files, ShouldBeEmpty)
			So(err, ShouldBeNil)
		})

		Convey("Discovering single file works", func() {
			writeFile(tempDir, "single_file", "12345", 0666)
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			So(len(files), ShouldEqual, 1)
			So(err, ShouldBeNil)

			file := files[0]
			So(file.Name(), ShouldEqual, "single_file")
			So(file.Size(), ShouldEqual, uint64(5))
			So(file.Executable(), ShouldBeFalse)

			r, err := file.Open()
			if r != nil {
				defer r.Close()
			}
			So(err, ShouldBeNil)
			buf, err := ioutil.ReadAll(r)
			So(buf, ShouldResemble, []byte("12345"))
			So(err, ShouldBeNil)
		})

		Convey("Enumerating subdirectories", func() {
			writeFile(tempDir, "a", "", 0666)
			writeFile(tempDir, "b", "", 0666)
			writeFile(tempDir, "1/a", "", 0666)
			writeFile(tempDir, "1/b", "", 0666)
			writeFile(tempDir, "1/2/a", "", 0666)
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			So(err, ShouldBeNil)
			names := make([]string, len(files))
			for i, f := range files {
				names[i] = f.Name()
			}
			// Order matters. Slashes matters.
			So(names, ShouldResemble, []string{
				"1/2/a",
				"1/a",
				"1/b",
				"a",
				"b",
			})
		})

		Convey("Empty subdirectories are skipped", func() {
			mkDir(tempDir, "a")
			mkDir(tempDir, "1/2/3")
			mkDir(tempDir, "1/c")
			writeFile(tempDir, "1/d/file", "1234", 0666)
			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			So(len(files), ShouldEqual, 1)
			So(err, ShouldBeNil)
			So(files[0].Name(), ShouldEqual, "1/d/file")
		})

		Convey("Non root start path works", func() {
			writeFile(tempDir, "a", "", 0666)
			writeFile(tempDir, "b", "", 0666)
			writeFile(tempDir, "1/a", "", 0666)
			writeFile(tempDir, "1/b", "", 0666)
			writeFile(tempDir, "1/2/a", "", 0666)
			files, err := ScanFileSystem(filepath.Join(tempDir, "1"), tempDir, nil, ScanOptions{})
			So(err, ShouldBeNil)
			names := make([]string, len(files))
			for i, f := range files {
				names[i] = f.Name()
			}
			// Order matters. Slashes matters.
			So(names, ShouldResemble, []string{
				"1/2/a",
				"1/a",
				"1/b",
			})
		})

		Convey("Start path must be under root", func() {
			_, err := ScanFileSystem(filepath.Dir(tempDir), tempDir, nil, ScanOptions{})
			So(err, ShouldNotBeNil)
		})

		Convey("Exclude filter works", func() {
			writeFile(tempDir, "a", "", 0666)
			writeFile(tempDir, "b", "", 0666)
			writeFile(tempDir, "1/a", "", 0666)
			writeFile(tempDir, "1/b", "", 0666)
			writeFile(tempDir, "1/2/a", "", 0666)

			// Exclude "a" and entire "1/" directory.
			var excluderCalls []string
			excluder := func(abs string) bool {
				excluderCalls = append(excluderCalls, abs)
				if abs == filepath.Join(tempDir, "a") {
					return true
				}
				if abs == filepath.Join(tempDir, "1") {
					return true
				}
				return false
			}

			files, err := ScanFileSystem(tempDir, tempDir, excluder, ScanOptions{})
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 1)
			So(files[0].Name(), ShouldEqual, "b")

			// "1/*" subdir should have been skipped completely.
			So(excluderCalls, ShouldResemble, []string{
				filepath.Join(tempDir, "1"),
				filepath.Join(tempDir, "a"),
				filepath.Join(tempDir, "b"),
			})
		})

		Convey(".cipd links turn into real files", func() {
			writeFile(tempDir, ".cipd/pkgs/0/deadbeef/some_file", "hello", 0666)
			writeFile(tempDir, ".cipd/pkgs/0/deadbeef/some_executable", "#!/usr/bin/python", 0777)
			writeSymlink(tempDir, ".cipd/pkgs/0/current", "deadbeef")
			writeSymlink(tempDir, "some_executable", ".cipd/pkgs/0/current/some_executable")
			writeSymlink(tempDir, "some_file", ".cipd/pkgs/0/current/some_file")

			files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 2)

			if runtime.GOOS != "windows" {
				So(files[0].Executable(), ShouldBeTrue)
			}

			So(files[1].Size(), ShouldEqual, 5)
			So(files[1].Name(), ShouldEqual, "some_file")
			So(files[1].Symlink(), ShouldBeFalse)
			So(files[1].Executable(), ShouldBeFalse)
			rc, err := files[1].Open()
			So(err, ShouldBeNil)
			defer rc.Close()

			data, err := ioutil.ReadAll(rc)
			So(err, ShouldBeNil)

			So(string(data), ShouldResemble, "hello")
		})

		if runtime.GOOS != "windows" {
			Convey("Discovering single executable file works", func() {
				writeFile(tempDir, "single_file", "12345", 0766)
				files, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
				So(len(files), ShouldEqual, 1)
				So(err, ShouldBeNil)
				file := files[0]
				So(file.Executable(), ShouldBeTrue)
			})

			Convey("Relative symlink to outside of package cause error", func() {
				writeSymlink(tempDir, "a/b1/rel_symlink", filepath.FromSlash("../../.."))
				_, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
				So(err, ShouldNotBeNil)
			})
		}
	})
}

func TestWrapFile(t *testing.T) {
	t.Parallel()

	Convey("Given a temp directory", t, func() {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		Convey("WrapFile simple file works", func() {
			writeFile(tempDir, "dir/a/b", "12345", 0666)
			out, err := WrapFile(filepath.Join(tempDir, "dir", "a", "b"), tempDir, nil, ScanOptions{})
			So(err, ShouldBeNil)
			So(out.Name(), ShouldEqual, "dir/a/b")
		})

		Convey("WrapFile directory fails", func() {
			mkDir(tempDir, "dir")
			_, err := WrapFile(filepath.Join(tempDir, "dir"), tempDir, nil, ScanOptions{})
			So(err, ShouldNotBeNil)
		})

		Convey("WrapFile outside of root fails", func() {
			mkDir(tempDir, "a")
			writeFile(tempDir, "b", "body", 0666)
			_, err := WrapFile(filepath.Join(tempDir, "b"), filepath.Join(tempDir, "a"), nil, ScanOptions{})
			So(err, ShouldNotBeNil)
		})

		Convey("WrapFile outside of root fails (tricky path)", func() {
			mkDir(tempDir, "a")
			// "abc" starts with "a", it tricks naive string.HasPrefix subpath check.
			writeFile(tempDir, "abc", "body", 0666)
			_, err := WrapFile(filepath.Join(tempDir, "abc"), filepath.Join(tempDir, "a"), nil, ScanOptions{})
			So(err, ShouldNotBeNil)
		})

		if runtime.GOOS != "windows" {
			Convey("WrapFile executable file works", func() {
				writeFile(tempDir, "single_file", "12345", 0766)
				out, err := WrapFile(filepath.Join(tempDir, "single_file"), tempDir, nil, ScanOptions{})
				So(err, ShouldBeNil)
				So(out.Executable(), ShouldBeTrue)
			})

			Convey("WrapFile rel symlink in root", func() {
				writeSymlink(tempDir, "a/b/c", filepath.FromSlash("../../d"))
				mkDir(tempDir, "d")
				out, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				So(err, ShouldBeNil)
				ensureSymlinkTarget(out, "../../d")
			})

			Convey("WrapFile .cipd symlink", func() {
				writeFile(tempDir, ".cipd/pkgs/0/deadbeef/some_executable", "#!/usr/bin/python", 0777)
				writeSymlink(tempDir, ".cipd/pkgs/0/current", "deadbeef")
				writeSymlink(tempDir, "some_executable", ".cipd/pkgs/0/current/some_executable")

				out, err := WrapFile(filepath.Join(tempDir, "some_executable"), tempDir, nil, ScanOptions{})
				So(err, ShouldBeNil)
				if runtime.GOOS != "windows" {
					So(out.Executable(), ShouldBeTrue)
				}
				So(out.Symlink(), ShouldBeFalse)
				So(out.Size(), ShouldEqual, 17)
			})

			Convey("WrapFile rel symlink outside root", func() {
				writeSymlink(tempDir, "a/b/c", filepath.FromSlash("../../../d"))
				_, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				So(err, ShouldNotBeNil)
			})

			Convey("WrapFile abs symlink in root", func() {
				writeSymlink(tempDir, "a/b/c", filepath.Join(tempDir, "a", "d"))
				out, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				So(err, ShouldBeNil)
				ensureSymlinkTarget(out, "../d")
			})

			Convey("WrapFile abs symlink outside root", func() {
				writeSymlink(tempDir, "a/b/c", filepath.Dir(tempDir))
				out, err := WrapFile(filepath.Join(tempDir, "a", "b", "c"), tempDir, nil, ScanOptions{})
				So(err, ShouldBeNil)
				ensureSymlinkTarget(out, filepath.ToSlash(filepath.Dir(tempDir)))
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
	err := ioutil.WriteFile(abs, []byte(data), mode)
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

func writeFileToDest(dest Destination, name string, opts CreateFileOptions, data string) {
	writer, err := dest.CreateFile(context.Background(), name, opts)
	So(err, ShouldBeNil)
	_, err = writer.Write([]byte(data))
	So(err, ShouldBeNil)
	So(writer.Close(), ShouldBeNil)
}

func writeAttrFileToDest(dest Destination, name string, attr WinAttrs, data string) {
	writer, err := dest.CreateFile(context.Background(), name, CreateFileOptions{WinAttrs: attr})
	So(err, ShouldBeNil)
	_, err = writer.Write([]byte(data))
	So(err, ShouldBeNil)
	So(writer.Close(), ShouldBeNil)
}

func writeSymlinkToDest(dest Destination, name string, target string) {
	So(dest.CreateSymlink(context.Background(), name, target), ShouldBeNil)
}

func ensureSymlinkTarget(file File, target string) {
	So(file.Symlink(), ShouldBeTrue)
	discoveredTarget, err := file.SymlinkTarget()
	So(err, ShouldBeNil)
	So(discoveredTarget, ShouldEqual, target)
}

func createBunchOfFiles(dest Destination, tempDir string) {
	testMTime := time.Date(2018, 1, 1, 1, 0, 0, 0, time.UTC)
	writeFileToDest(dest, "a", CreateFileOptions{}, "a data")
	writeFileToDest(dest, "exe", CreateFileOptions{Executable: true}, "exe data")
	writeFileToDest(dest, "dir/c", CreateFileOptions{}, "dir/c data")
	writeFileToDest(dest, "dir/dir/d", CreateFileOptions{}, "dir/dir/d data")
	writeFileToDest(dest, "ts", CreateFileOptions{ModTime: testMTime}, "ts data")
	writeFileToDest(dest, "wr", CreateFileOptions{Writable: true}, "wr data")
	if runtime.GOOS != "windows" {
		writeSymlinkToDest(dest, "abs_symlink", filepath.FromSlash(tempDir))
		writeSymlinkToDest(dest, "dir/dir/rel_symlink", "../../a")
	} else {
		writeAttrFileToDest(dest, "secret_file", WinAttrHidden, "ninja")
		writeAttrFileToDest(dest, "system_file", WinAttrSystem, "system")
	}
}

func checkBunchOfFiles(destDir, tempDir string) {
	testMTime := time.Date(2018, 1, 1, 1, 0, 0, 0, time.UTC)

	// Ensure everything is there.
	files, err := ScanFileSystem(destDir, destDir, nil, ScanOptions{
		PreserveModTime:  true,
		PreserveWritable: true,
	})
	So(err, ShouldBeNil)
	names := make([]string, len(files))
	mapping := make(map[string]File, len(files))
	for i, f := range files {
		names[i] = f.Name()
		mapping[f.Name()] = f
	}

	if runtime.GOOS == "windows" {
		So(names, ShouldResemble, []string{
			"a",
			"dir/c",
			"dir/dir/d",
			"exe",
			"secret_file",
			"system_file",
			"ts",
			"wr",
		})
	} else {
		So(names, ShouldResemble, []string{
			"a",
			"abs_symlink",
			"dir/c",
			"dir/dir/d",
			"dir/dir/rel_symlink",
			"exe",
			"ts",
			"wr",
		})
	}

	// Ensure data is valid (check first file only).
	r, err := mapping["a"].Open()
	if r != nil {
		defer r.Close()
	}
	So(err, ShouldBeNil)
	data, err := ioutil.ReadAll(r)
	So(err, ShouldBeNil)
	So(data, ShouldResemble, []byte("a data"))

	// File mode and symlinks are valid.
	if runtime.GOOS != "windows" {
		So(mapping["a"].Writable(), ShouldBeFalse)
		So(mapping["exe"].Executable(), ShouldBeTrue)
		So(mapping["ts"].ModTime(), ShouldEqual, testMTime)
		So(mapping["wr"].Writable(), ShouldBeTrue)
		ensureSymlinkTarget(mapping["abs_symlink"], filepath.FromSlash(tempDir))
		ensureSymlinkTarget(mapping["dir/dir/rel_symlink"], "../../a")
	} else {
		So(mapping["secret_file"].WinAttrs()&WinAttrHidden, ShouldEqual, WinAttrHidden)
		So(mapping["system_file"].WinAttrs()&WinAttrSystem, ShouldEqual, WinAttrSystem)
	}

	// Ensure no temp files left.
	allFiles, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
	So(len(allFiles), ShouldEqual, len(files))
}

func TestExistingDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		destDir := filepath.Join(tempDir, "dest")
		So(err, ShouldBeNil)
		dest := ExistingDestination(destDir, nil)
		defer os.RemoveAll(tempDir)

		readFromDest := func(name string) string {
			b, err := ioutil.ReadFile(filepath.Join(destDir, name))
			So(err, ShouldBeNil)
			return string(b)
		}

		Convey("Creates files", func() {
			writeFileToDest(dest, "a/b/c", CreateFileOptions{}, "123")
			So(readFromDest("a/b/c"), ShouldEqual, "123")
		})

		Convey("CreateFile can override read-only files, atomic mode", func() {
			writeFileToDest(dest, "a/b/c", CreateFileOptions{}, "123")
			writeFileToDest(dest, "a/b/c", CreateFileOptions{}, "456")
			So(readFromDest("a/b/c"), ShouldEqual, "456")
		})

		Convey("CreateFile can override read-only files, non-atomic mode", func() {
			dest.(*fsDest).atomic = false
			writeFileToDest(dest, "a/b/c", CreateFileOptions{}, "123")
			writeFileToDest(dest, "a/b/c", CreateFileOptions{}, "456")
			So(readFromDest("a/b/c"), ShouldEqual, "456")
		})

		Convey("Can't have two instances of a file open at the same time", func() {
			wr, err := dest.CreateFile(ctx, "a/b/c", CreateFileOptions{})
			So(err, ShouldBeNil)

			// Open same file (perhaps via lexically different path).
			_, err = dest.CreateFile(ctx, "a/b/d/../c", CreateFileOptions{})
			So(err.Error(), ShouldEqual, "file a/b/d/../c is already open")

			So(wr.Close(), ShouldBeNil)

			// Can be opened now.
			wr, err = dest.CreateFile(ctx, "a/b/d/../c", CreateFileOptions{})
			So(err, ShouldBeNil)
			So(wr.Close(), ShouldBeNil)
		})

		Convey("CreateFile rejects invalid relative paths", func() {
			// Rel path that is still inside the package is ok.
			wr, err := dest.CreateFile(ctx, "a/b/c/../../../d", CreateFileOptions{})
			So(err, ShouldBeNil)
			wr.Close()
			// Rel path pointing outside is forbidden.
			_, err = dest.CreateFile(ctx, "a/b/c/../../../../d", CreateFileOptions{})
			So(err, ShouldNotBeNil)
		})

		if runtime.GOOS != "windows" {
			Convey("CreateSymlink rejects invalid relative paths", func() {
				// Rel symlink to a file inside the destination is OK.
				So(dest.CreateSymlink(ctx, "a/b/c", "../.."), ShouldBeNil)
				// Rel symlink to a file outside -> error.
				So(dest.CreateSymlink(ctx, "a/b/c", "../../.."), ShouldNotBeNil)
			})
		}

		Convey("Creating a bunch of files", func() {
			createBunchOfFiles(dest, tempDir)
			checkBunchOfFiles(destDir, tempDir)
		})
	})
}

func TestNewDestination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("Given a temp directory", t, func() {
		tempDir, err := ioutil.TempDir("", "cipd_test")
		destDir := filepath.Join(tempDir, "dest")
		So(err, ShouldBeNil)
		dest := NewDestination(destDir, nil)
		defer os.RemoveAll(tempDir)

		Convey("Empty success write works", func() {
			So(dest.Begin(ctx), ShouldBeNil)
			So(dest.End(ctx, true), ShouldBeNil)

			// Should create a new directory.
			stat, err := os.Stat(destDir)
			So(err, ShouldBeNil)
			So(stat.IsDir(), ShouldBeTrue)

			// And it should be empty.
			files, err := ScanFileSystem(destDir, destDir, nil, ScanOptions{})
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 0)
		})

		Convey("Empty failed write works", func() {
			So(dest.Begin(ctx), ShouldBeNil)
			So(dest.End(ctx, false), ShouldBeNil)

			// Doesn't create a directory.
			_, err := os.Stat(destDir)
			So(os.IsNotExist(err), ShouldBeTrue)
		})

		Convey("Double begin or double end fails", func() {
			So(dest.Begin(ctx), ShouldBeNil)
			So(dest.Begin(ctx), ShouldNotBeNil)
			So(dest.End(ctx, true), ShouldBeNil)
			So(dest.End(ctx, true), ShouldNotBeNil)
		})

		Convey("CreateFile works only when destination is open", func() {
			wr, err := dest.CreateFile(ctx, "testing", CreateFileOptions{Executable: true})
			So(wr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("Committing bunch of files works", func() {
			So(dest.Begin(ctx), ShouldBeNil)
			createBunchOfFiles(dest, tempDir)
			So(dest.End(ctx, true), ShouldBeNil)
			checkBunchOfFiles(destDir, tempDir)
		})

		Convey("Rolling back bunch of files works", func() {
			So(dest.Begin(ctx), ShouldBeNil)
			createBunchOfFiles(dest, tempDir)
			So(dest.End(ctx, false), ShouldBeNil)

			// No dest directory.
			_, err := os.Stat(destDir)
			So(os.IsNotExist(err), ShouldBeTrue)

			// Ensure no temp files left.
			allFiles, err := ScanFileSystem(tempDir, tempDir, nil, ScanOptions{})
			So(len(allFiles), ShouldEqual, 0)
		})

		Convey("Overwriting a directory works", func() {
			// Create dest directory manually with some stuff.
			err := os.Mkdir(destDir, 0777)
			So(err, ShouldBeNil)
			err = ioutil.WriteFile(filepath.Join(destDir, "data"), []byte("data"), 0666)
			So(err, ShouldBeNil)

			// Now deploy something to it.
			So(dest.Begin(ctx), ShouldBeNil)
			createBunchOfFiles(dest, tempDir)
			So(dest.End(ctx, true), ShouldBeNil)

			// Overwritten.
			checkBunchOfFiles(destDir, tempDir)
		})

		Convey("Not overwriting a directory works", func() {
			// Create dest directory manually with some stuff.
			err := os.Mkdir(destDir, 0777)
			So(err, ShouldBeNil)
			err = ioutil.WriteFile(filepath.Join(destDir, "data"), []byte("data"), 0666)
			So(err, ShouldBeNil)

			// Now attempt deploy something to it, but roll back.
			So(dest.Begin(ctx), ShouldBeNil)
			createBunchOfFiles(dest, tempDir)
			So(dest.End(ctx, false), ShouldBeNil)

			// Kept as is.
			files, err := ScanFileSystem(destDir, destDir, nil, ScanOptions{})
			So(err, ShouldBeNil)
			So(len(files), ShouldEqual, 1)
			So(files[0].Name(), ShouldEqual, "data")
		})

		Convey("End with opened files fail", func() {
			So(dest.Begin(ctx), ShouldBeNil)
			w, err := dest.CreateFile(ctx, "a", CreateFileOptions{})
			So(w, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(dest.End(ctx, true), ShouldNotBeNil)
			w.Close()
			So(dest.End(ctx, true), ShouldBeNil)
		})
	})
}
