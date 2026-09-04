// Copyright 2018 The LUCI Authors.
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

package processing

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/cipd/appengine/impl/testutil"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
)

type cbReaderAt struct {
	readAt func(p []byte, off int64) (int, error)
}

func (c *cbReaderAt) ReadAt(b []byte, off int64) (int, error) {
	return c.readAt(b, off)
}

func TestPackageReader(t *testing.T) {
	t.Parallel()

	manifestName := pkg.ManifestName
	testZip := testutil.MakeZip(map[string]string{
		"file1":      strings.Repeat("hello", 50),
		"file2":      "blah",
		manifestName: `{"package_name": "some/package"}`,
	})
	reader := bytes.NewReader(testZip)
	size := int64(reader.Len())

	readErr := fmt.Errorf("some read error")

	t.Run("Happy path", func(t *testing.T) {
		pkg, err := NewPackageReader(reader, size)
		assert.NoErr(t, err)

		assert.Loosely(t, pkg.Files(), should.Match([]string{manifestName, "file1", "file2"}))

		fr, actualSize, err := pkg.Open("file2")
		assert.NoErr(t, err)
		assert.Loosely(t, actualSize, should.Equal(4))
		blob, err := io.ReadAll(fr)
		assert.NoErr(t, err)
		assert.Loosely(t, string(blob), should.Equal("blah"))
	})

	t.Run("No such file", func(t *testing.T) {
		pkg, err := NewPackageReader(reader, size)
		assert.NoErr(t, err)

		_, _, err = pkg.Open("zzz")
		assert.Loosely(t, err.Error(), should.Equal(`no file "zzz" inside the package`))
	})

	t.Run("Propagates errors when opening", func(t *testing.T) {
		calls := 0
		r := &cbReaderAt{
			readAt: func(p []byte, off int64) (int, error) {
				// Fail the second read call, it makes more interesting test case.
				calls++
				if calls == 2 {
					return 0, readErr
				}
				return reader.ReadAt(p, off)
			},
		}

		_, err := NewPackageReader(r, size)
		assert.Loosely(t, err, should.Equal(readErr)) // exact same error object
	})

	t.Run("Propagates errors when reading", func(t *testing.T) {
		r := &cbReaderAt{readAt: reader.ReadAt}

		// Let the directory be read successfully.
		pkg, err := NewPackageReader(r, size)
		assert.NoErr(t, err)

		// Now inject errors.
		r.readAt = func([]byte, int64) (int, error) { return 0, readErr }
		_, _, err = pkg.Open("file1")
		assert.Loosely(t, err, should.Equal(readErr)) // exact same error object
	})

	t.Run("Can extract manifest", func(t *testing.T) {
		r := &cbReaderAt{readAt: reader.ReadAt}

		pkg, err := NewPackageReader(r, size)
		assert.NoErr(t, err)

		manifest, err := pkg.Manifest()
		assert.NoErr(t, err)

		assert.That(t, manifest.PackageName, should.Equal("some/package"))
	})

	t.Run("Manifest too large", func(t *testing.T) {
		testZip := testutil.MakeZip(map[string]string{
			"file1": strings.Repeat("hello", 50),
			"file2": "blah",
			manifestName: fmt.Sprintf(
				`{"package_name": "some/package", "other": %q}`,
				strings.Repeat("spam", 3000),
			),
		})
		reader := bytes.NewReader(testZip)
		r := &cbReaderAt{readAt: reader.ReadAt}

		pkg, err := NewPackageReader(r, int64(reader.Len()))
		assert.NoErr(t, err)

		_, err = pkg.Manifest()
		assert.ErrIsLike(t, err, "manifest file is too large")
		assert.That(t, status.Code(err), should.Equal(codes.InvalidArgument))
	})
}
