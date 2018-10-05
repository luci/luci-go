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

package local

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common/cipdpkg"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuildInstance(t *testing.T) {
	ctx := context.Background()

	Convey("Building empty package", t, func() {
		out := bytes.Buffer{}
		err := BuildInstance(ctx, BuildInstanceOptions{
			Input:            []fs.File{},
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		So(err, ShouldBeNil)

		// BuildInstance builds deterministic zip. It MUST NOT depend on
		// the platform, or a time of day, or anything else, only on the input data.
		So(getSHA256(&out), ShouldEqual, "66b44a6bd1cdfcb9481d952c6654f3a66882b3c02abc0873f5367337de90a71e")

		// There should be a single file: the manifest.
		goodManifest := `{
  "format_version": "1.1",
  "package_name": "testing"
}`
		files := readZip(out.Bytes())
		So(files, ShouldResemble, []zippedFile{
			{
				name: cipdpkg.ManifestName,
				size: uint64(len(goodManifest)),
				mode: 0400,
				body: []byte(goodManifest),
			},
		})
	})

	Convey("Building package with a bunch of files at different deflate levels", t, func() {
		testMTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		makeOpts := func(out io.Writer, level int) BuildInstanceOptions {
			return BuildInstanceOptions{
				Input: []fs.File{
					fs.NewTestFile("testing/qwerty", "12345", fs.TestFileOpts{}),
					fs.NewTestFile("abc", "duh", fs.TestFileOpts{Executable: true}),
					fs.NewTestFile("writable", "write me", fs.TestFileOpts{Writable: true}),
					fs.NewTestFile("timestamped", "I'm old", fs.TestFileOpts{ModTime: testMTime}),
					fs.NewWinTestFile("sneaky_file", "ninja", fs.WinAttrHidden),
					fs.NewWinTestFile("special_file", "special", fs.WinAttrSystem),
					fs.NewTestSymlink("rel_symlink", "abc"),
					fs.NewTestSymlink("abs_symlink", "/abc/def"),
				},
				Output:           out,
				PackageName:      "testing",
				VersionFile:      "version.json",
				InstallMode:      cipdpkg.InstallModeCopy,
				CompressionLevel: level,
			}
		}

		goodManifest := `{
  "format_version": "1.1",
  "package_name": "testing",
  "version_file": "version.json",
  "install_mode": "copy"
}`

		goodFiles := []zippedFile{
			{
				name: "testing/qwerty",
				size: 5,
				mode: 0400,
				body: []byte("12345"),
			},
			{
				name: "abc",
				size: 3,
				mode: 0500,
				body: []byte("duh"),
			},
			{
				name: "writable",
				size: 8,
				mode: 0600,
				body: []byte("write me"),
			},
			{
				name:    "timestamped",
				size:    7,
				mode:    0400,
				body:    []byte("I'm old"),
				modTime: testMTime,
			},
			{
				name:     "sneaky_file",
				size:     5,
				mode:     0400,
				body:     []byte("ninja"),
				winAttrs: fs.WinAttrHidden,
			},
			{
				name:     "special_file",
				size:     7,
				mode:     0400,
				body:     []byte("special"),
				winAttrs: fs.WinAttrSystem,
			},
			{
				name: "rel_symlink",
				size: 3,
				mode: 0400 | os.ModeSymlink,
				body: []byte("abc"),
			},
			{
				name: "abs_symlink",
				size: 8,
				mode: 0400 | os.ModeSymlink,
				body: []byte("/abc/def"),
			},
			{
				name: cipdpkg.ManifestName,
				size: uint64(len(goodManifest)),
				mode: 0400,
				body: []byte(goodManifest),
			},
		}

		for lvl := 0; lvl <= 9; lvl++ {
			out := bytes.Buffer{}
			err := BuildInstance(ctx, makeOpts(&out, lvl))
			So(err, ShouldBeNil)
			So(readZip(out.Bytes()), ShouldResemble, goodFiles)
		}
	})

	Convey("Building package with a bunch of files preserving mtime and u+w", t, func() {
		testMTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		makeOpts := func(out io.Writer, level int) BuildInstanceOptions {
			return BuildInstanceOptions{
				Input: []fs.File{
					fs.NewTestFile("testing/qwerty", "12345", fs.TestFileOpts{}),
					fs.NewTestFile("abc", "duh", fs.TestFileOpts{Executable: true}),
					fs.NewTestFile("writable", "write me", fs.TestFileOpts{Writable: true}),
					fs.NewTestFile("timestamped", "I'm old", fs.TestFileOpts{ModTime: testMTime}),
					fs.NewWinTestFile("sneaky_file", "ninja", fs.WinAttrHidden),
					fs.NewWinTestFile("special_file", "special", fs.WinAttrSystem),
					fs.NewTestSymlink("rel_symlink", "abc"),
					fs.NewTestSymlink("abs_symlink", "/abc/def"),
				},
				Output:           out,
				PackageName:      "testing",
				VersionFile:      "version.json",
				InstallMode:      cipdpkg.InstallModeCopy,
				CompressionLevel: level,
			}
		}

		goodManifest := `{
  "format_version": "1.1",
  "package_name": "testing",
  "version_file": "version.json",
  "install_mode": "copy"
}`

		goodFiles := []zippedFile{
			{
				name: "testing/qwerty",
				size: 5,
				mode: 0400,
				body: []byte("12345"),
			},
			{
				name: "abc",
				size: 3,
				mode: 0500,
				body: []byte("duh"),
			},
			{
				name: "writable",
				size: 8,
				mode: 0600,
				body: []byte("write me"),
			},
			{
				name:    "timestamped",
				size:    7,
				mode:    0400,
				body:    []byte("I'm old"),
				modTime: testMTime,
			},
			{
				name:     "sneaky_file",
				size:     5,
				mode:     0400,
				body:     []byte("ninja"),
				winAttrs: fs.WinAttrHidden,
			},
			{
				name:     "special_file",
				size:     7,
				mode:     0400,
				body:     []byte("special"),
				winAttrs: fs.WinAttrSystem,
			},
			{
				name: "rel_symlink",
				size: 3,
				mode: 0400 | os.ModeSymlink,
				body: []byte("abc"),
			},
			{
				name: "abs_symlink",
				size: 8,
				mode: 0400 | os.ModeSymlink,
				body: []byte("/abc/def"),
			},
			{
				name: cipdpkg.ManifestName,
				size: uint64(len(goodManifest)),
				mode: 0400,
				body: []byte(goodManifest),
			},
		}

		for lvl := 0; lvl <= 9; lvl++ {
			out := bytes.Buffer{}
			err := BuildInstance(ctx, makeOpts(&out, lvl))
			So(err, ShouldBeNil)
			So(readZip(out.Bytes()), ShouldResemble, goodFiles)
		}
	})

	Convey("Duplicate files fail", t, func() {
		err := BuildInstance(ctx, BuildInstanceOptions{
			Input: []fs.File{
				fs.NewTestFile("a", "12345", fs.TestFileOpts{}),
				fs.NewTestFile("a", "12345", fs.TestFileOpts{}),
			},
			Output:      &bytes.Buffer{},
			PackageName: "testing",
		})
		So(err, ShouldNotBeNil)
	})

	Convey("Writing to service dir fails", t, func() {
		err := BuildInstance(ctx, BuildInstanceOptions{
			Input: []fs.File{
				fs.NewTestFile(".cipdpkg/stuff", "12345", fs.TestFileOpts{}),
			},
			Output:      &bytes.Buffer{},
			PackageName: "testing",
		})
		So(err, ShouldNotBeNil)
	})

	Convey("Bad name fails", t, func() {
		err := BuildInstance(ctx, BuildInstanceOptions{
			Output:      &bytes.Buffer{},
			PackageName: "../../asdad",
		})
		So(err, ShouldNotBeNil)
	})

	Convey("Bad version file fails", t, func() {
		err := BuildInstance(ctx, BuildInstanceOptions{
			Output:      &bytes.Buffer{},
			PackageName: "abc",
			VersionFile: "../bad/path",
		})
		So(err, ShouldNotBeNil)
	})
}

////////////////////////////////////////////////////////////////////////////////

// getSHA256 returns SHA256 hex digest of a byte buffer.
func getSHA256(buf *bytes.Buffer) string {
	h := sha256.New()
	h.Write(buf.Bytes())
	return hex.EncodeToString(h.Sum(nil))
}

////////////////////////////////////////////////////////////////////////////////

type zippedFile struct {
	name     string
	size     uint64
	mode     os.FileMode
	modTime  time.Time
	body     []byte
	winAttrs fs.WinAttrs
}

// readZip scans zip directory and returns files it finds.
func readZip(data []byte) []zippedFile {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		panic("Failed to open zip file")
	}
	files := make([]zippedFile, len(z.File))
	for i, zf := range z.File {
		reader, err := zf.Open()
		if err != nil {
			panic("Failed to open file inside zip")
		}
		body, err := ioutil.ReadAll(reader)
		if err != nil {
			panic("Failed to read zipped file")
		}
		var mtime time.Time
		if zf.ModifiedTime != 0 || zf.ModifiedDate != 0 {
			mtime = zf.ModTime()
		}
		files[i] = zippedFile{
			name:     zf.Name,
			size:     zf.FileHeader.UncompressedSize64,
			mode:     zf.Mode(),
			modTime:  mtime,
			body:     body,
			winAttrs: fs.WinAttrs(zf.ExternalAttrs) & fs.WinAttrsAll,
		}
	}
	return files
}
