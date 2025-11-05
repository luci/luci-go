// Copyright 2025 The LUCI Authors.
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

package gitilessource

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"slices"
	"testing"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testFile struct {
	hdr     *tar.Header
	content string
}

func file(path string, content string) testFile {
	return testFile{
		&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     path,
			Size:     int64(len(content)),
			Format:   tar.FormatPAX,
		},
		content,
	}
}

func symlink(path string, target string) testFile {
	return testFile{
		&tar.Header{
			Typeflag: tar.TypeSymlink,
			Name:     path,
			Linkname: target,
			Format:   tar.FormatPAX,
		},
		"",
	}
}

func makeTestTarball(t *testing.T, files ...testFile) []byte {
	t.Helper()

	slices.SortFunc(files, func(a, b testFile) int {
		return zipFileNameCompare(a.hdr.Name, b.hdr.Name)
	})

	var tempTarball bytes.Buffer

	gzWriter := gzip.NewWriter(&tempTarball)
	tarWriter := tar.NewWriter(gzWriter)

	addedDirs := stringset.New(0)
	for _, testFile := range files {
		// insert any missing directories
		for _, dir := range dirsOf(testFile.hdr.Name) {
			if addedDirs.Has(dir) {
				continue
			}
			assert.NoErr(t, tarWriter.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     dir + "/",
				Format:   tar.FormatPAX,
			}), truth.LineContext())
			addedDirs.Add(dir)
		}

		assert.NoErr(t, tarWriter.WriteHeader(testFile.hdr), truth.LineContext())
		if len(testFile.content) > 0 {
			_, err := io.WriteString(tarWriter, testFile.content)
			assert.NoErr(t, err, truth.LineContext())
		}
	}

	assert.NoErr(t, tarWriter.Close(), truth.LineContext())
	assert.NoErr(t, gzWriter.Close(), truth.LineContext())

	return tempTarball.Bytes()
}

func readZip(t *testing.T, zfile *zip.Reader, path string) (content string, mode fs.FileMode) {
	t.Helper()

	reader, err := zfile.Open(path)
	assert.NoErr(t, err, truth.LineContext())
	defer reader.Close()

	dat, err := io.ReadAll(reader)
	assert.NoErr(t, err, truth.LineContext())

	st, err := reader.Stat()
	assert.NoErr(t, err, truth.LineContext())

	return string(dat), st.Mode()
}

func TestDirsOf(t *testing.T) {
	assert.Loosely(t, dirsOf(""), should.BeNil)

	assert.Loosely(t, dirsOf("hey"), should.BeNil)

	assert.Loosely(t, dirsOf("some/stuff/thats/cool"), should.Match([]string{
		"some/",
		"some/stuff/",
		"some/stuff/thats/",
	}))
}

func TestTarballToZip(t *testing.T) {
	t.Run(`success`, func(t *testing.T) {
		tball := makeTestTarball(
			t,
			file("some/file", "stuff"),
			file("some/other/file", "more stuff"),
			symlink("some/symup", "../Things"),
			file("Things", "additional things"),
			symlink("symdown", "some/file"),
		)

		var zout bytes.Buffer
		assert.NoErr(t, tgzToZip(tball, "some/prefix", &zout))

		zreader, err := zip.NewReader(bytes.NewReader(zout.Bytes()), int64(zout.Len()))
		assert.NoErr(t, err)

		names := make([]string, 0, len(zreader.File))
		for _, zfile := range zreader.File {
			names = append(names, zfile.Name)
		}
		slices.Sort(names)

		assert.Loosely(t, names, should.Match([]string{
			"some/",
			"some/prefix/",
			"some/prefix/Things",
			"some/prefix/some/",
			"some/prefix/some/file",
			"some/prefix/some/other/",
			"some/prefix/some/other/file",
			"some/prefix/some/symup",
			"some/prefix/symdown",
		}))

		content, mode := readZip(t, zreader, "some/prefix/Things")
		assert.That(t, content, should.Equal("additional things"))
		assert.Loosely(t, mode&fs.ModeSymlink, should.Equal(0))

		content, mode = readZip(t, zreader, "some/prefix/some/symup")
		assert.That(t, content, should.Equal("../Things"))
		assert.Loosely(t, mode&fs.ModeSymlink, should.NotEqual(0))

		content, mode = readZip(t, zreader, "some/prefix/symdown")
		assert.That(t, content, should.Equal("some/file"))
		assert.Loosely(t, mode&fs.ModeSymlink, should.NotEqual(0))

		var someDir *zip.File
		for _, file := range zreader.File {
			if file.Name == "some/prefix/some/" {
				someDir = file
				break
			}
		}
		assert.Loosely(t, someDir, should.NotBeNil)
		assert.That(t, someDir.Mode().IsDir(), should.BeTrue)
	})

	t.Run(`weird file`, func(t *testing.T) {
		tball := makeTestTarball(
			t,
			testFile{
				&tar.Header{
					Typeflag: tar.TypeBlock,
					Name:     "afile",
					Devmajor: 10,
					Devminor: 11,
				},
				"",
			},
		)

		var zout bytes.Buffer
		assert.ErrIsLike(t, tgzToZip(tball, "some/prefix", &zout), "unknown tarball entry type '4'")
	})
}
