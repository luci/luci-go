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

package builder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"
	"time"

	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/cipd/common"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBuildInstance(t *testing.T) {
	ctx := context.Background()

	ftt.Run("Building empty package", t, func(t *ftt.Test) {
		out := bytes.Buffer{}
		pin, err := BuildInstance(ctx, Options{
			Input:            []fs.File{},
			Output:           &out,
			PackageName:      "testing",
			CompressionLevel: 5,
		})
		assert.Loosely(t, err, should.BeNil)

		// BuildInstance builds deterministic zip. It MUST NOT depend on
		// the platform, or a time of day, or anything else, only on the input data,
		// CIPD code and compress/deflat and archive/zip code.
		assert.Loosely(t, getSHA256(&out), should.Equal("3cf335f34fb98be575ab9e79e1ec0a18ee23ab871607e70ec13bb28680afde3a"))
		assert.Loosely(t, pin, should.Resemble(common.Pin{
			PackageName: "testing",
			InstanceID:  "PPM180-5i-V1q5554ewKGO4jq4cWB-cOwTuyhoCv3joC",
		}))

		// There should be a single file: the manifest.
		goodManifest := `{
  "format_version": "1.1",
  "package_name": "testing"
}`
		files := readZip(out.Bytes())
		assert.Loosely(t, files, should.Resemble([]zippedFile{
			{
				name: pkg.ManifestName,
				size: uint64(len(goodManifest)),
				mode: 0400,
				body: []byte(goodManifest),
			},
		}))
	})

	ftt.Run("Building package with a bunch of files at different deflate levels", t, func(t *ftt.Test) {
		testMTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		makeOpts := func(out io.Writer, level int) Options {
			return Options{
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
				InstallMode:      pkg.InstallModeCopy,
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
				name: pkg.ManifestName,
				size: uint64(len(goodManifest)),
				mode: 0400,
				body: []byte(goodManifest),
			},
		}

		for lvl := 0; lvl <= 9; lvl++ {
			out := bytes.Buffer{}
			_, err := BuildInstance(ctx, makeOpts(&out, lvl))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, readZip(out.Bytes()), should.Resemble(goodFiles))
		}
	})

	ftt.Run("Building package with a bunch of files preserving mtime and u+w", t, func(t *ftt.Test) {
		testMTime := time.Date(2018, 1, 1, 0, 0, 0, 0, time.UTC)
		makeOpts := func(out io.Writer, level int) Options {
			return Options{
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
				InstallMode:      pkg.InstallModeCopy,
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
				name: pkg.ManifestName,
				size: uint64(len(goodManifest)),
				mode: 0400,
				body: []byte(goodManifest),
			},
		}

		for lvl := 0; lvl <= 9; lvl++ {
			out := bytes.Buffer{}
			_, err := BuildInstance(ctx, makeOpts(&out, lvl))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, readZip(out.Bytes()), should.Resemble(goodFiles))
		}
	})

	ftt.Run("Duplicate files fail", t, func(t *ftt.Test) {
		_, err := BuildInstance(ctx, Options{
			Input: []fs.File{
				fs.NewTestFile("a", "12345", fs.TestFileOpts{}),
				fs.NewTestFile("a", "12345", fs.TestFileOpts{}),
			},
			Output:      &bytes.Buffer{},
			PackageName: "testing",
		})
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Writing to service dir fails", t, func(t *ftt.Test) {
		_, err := BuildInstance(ctx, Options{
			Input: []fs.File{
				fs.NewTestFile(".cipdpkg/stuff", "12345", fs.TestFileOpts{}),
			},
			Output:      &bytes.Buffer{},
			PackageName: "testing",
		})
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Bad name fails", t, func(t *ftt.Test) {
		_, err := BuildInstance(ctx, Options{
			Output:      &bytes.Buffer{},
			PackageName: "../../asdad",
		})
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Bad version file fails", t, func(t *ftt.Test) {
		_, err := BuildInstance(ctx, Options{
			Output:      &bytes.Buffer{},
			PackageName: "abc",
			VersionFile: "../bad/path",
		})
		assert.Loosely(t, err, should.NotBeNil)
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
		body, err := io.ReadAll(reader)
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
