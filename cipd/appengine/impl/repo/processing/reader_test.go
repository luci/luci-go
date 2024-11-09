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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

type cbReaderAt struct {
	readAt func(p []byte, off int64) (int, error)
}

func (c *cbReaderAt) ReadAt(b []byte, off int64) (int, error) {
	return c.readAt(b, off)
}

func TestPackageReader(t *testing.T) {
	t.Parallel()

	testZip := testutil.MakeZip(map[string]string{
		"file1": strings.Repeat("hello", 50),
		"file2": "blah",
	})
	reader := bytes.NewReader(testZip)
	size := int64(reader.Len())

	readErr := fmt.Errorf("some read error")

	ftt.Run("Happy path", t, func(t *ftt.Test) {
		pkg, err := NewPackageReader(reader, size)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, pkg.Files(), should.Resemble([]string{"file1", "file2"}))

		fr, actualSize, err := pkg.Open("file2")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualSize, should.Equal(4))
		blob, err := io.ReadAll(fr)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(blob), should.Equal("blah"))
	})

	ftt.Run("No such file", t, func(t *ftt.Test) {
		pkg, err := NewPackageReader(reader, size)
		assert.Loosely(t, err, should.BeNil)

		_, _, err = pkg.Open("zzz")
		assert.Loosely(t, err.Error(), should.Equal(`no file "zzz" inside the package`))
	})

	ftt.Run("Propagates errors when opening", t, func(t *ftt.Test) {
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

	ftt.Run("Propagates errors when reading", t, func(t *ftt.Test) {
		r := &cbReaderAt{readAt: reader.ReadAt}

		// Let the directory be read successfully.
		pkg, err := NewPackageReader(r, size)
		assert.Loosely(t, err, should.BeNil)

		// Now inject errors.
		r.readAt = func([]byte, int64) (int, error) { return 0, readErr }
		_, _, err = pkg.Open("file1")
		assert.Loosely(t, err, should.Equal(readErr)) // exact same error object
	})
}
