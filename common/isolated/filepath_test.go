// Copyright 2019 The LUCI Authors.
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

package isolated

import (
	"io/ioutil"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSetReadOnly(t *testing.T) {
	t.Parallel()

	Convey("Simple tests for SetReadOnly", t, func() {
		tmpdir, err := ioutil.TempDir("", "set_read_only_")
		So(err, ShouldBeNil)
		defer func() {
			So(os.RemoveAll(tmpdir), ShouldBeNil)
		}()

		f, err := ioutil.TempFile(tmpdir, "tmp")
		So(err, ShouldBeNil)
		tmpfile := f.Name()
		So(f.Close(), ShouldBeNil)

		// make tmpfile readonly
		So(SetReadOnly(tmpfile, true), ShouldBeNil)

		_, err = os.Create(tmpfile)
		So(err.Error(), ShouldEqual, "open "+tmpfile+": permission denied")

		f, err = os.Open(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		// make tmpfile writable
		So(SetReadOnly(tmpfile, false), ShouldBeNil)

		f, err = os.Create(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)

		f, err = os.Open(tmpfile)
		So(err, ShouldBeNil)
		So(f.Close(), ShouldBeNil)
	})

}
