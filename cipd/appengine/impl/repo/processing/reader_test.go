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
	"io/ioutil"
	"strings"
	"testing"

	"go.chromium.org/luci/cipd/appengine/impl/testutil"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Happy path", t, func() {
		pkg, err := NewPackageReader(reader, size)
		So(err, ShouldBeNil)

		fr, size, err := pkg.Open("file2")
		So(err, ShouldBeNil)
		So(size, ShouldEqual, 4)
		blob, err := ioutil.ReadAll(fr)
		So(err, ShouldBeNil)
		So(string(blob), ShouldEqual, "blah")
	})

	Convey("No such file", t, func() {
		pkg, err := NewPackageReader(reader, size)
		So(err, ShouldBeNil)

		_, _, err = pkg.Open("zzz")
		So(err.Error(), ShouldEqual, `no file "zzz" inside the package`)
	})

	Convey("Propagates errors when opening", t, func() {
		calls := 0
		r := &cbReaderAt{
			readAt: func(p []byte, off int64) (int, error) {
				// Fail the second read call, it makes more interesting test case.
				calls += 1
				if calls == 2 {
					return 0, readErr
				}
				return reader.ReadAt(p, off)
			},
		}

		_, err := NewPackageReader(r, size)
		So(err, ShouldEqual, readErr) // exact same error object
	})

	Convey("Propagates errors when reading", t, func() {
		r := &cbReaderAt{readAt: reader.ReadAt}

		// Let the directory be read successfully.
		pkg, err := NewPackageReader(r, size)
		So(err, ShouldBeNil)

		// Now inject errors.
		r.readAt = func([]byte, int64) (int, error) { return 0, readErr }
		_, _, err = pkg.Open("file1")
		So(err, ShouldEqual, readErr) // exact same error object
	})
}
