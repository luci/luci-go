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

package sink

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func createFile(s string) string {
	f, err := ioutil.TempFile("", "test_foo")
	So(err, ShouldBeNil)
	defer f.Close()

	if len(s) > 0 {
		l, err := f.WriteString(s)
		So(err, ShouldBeNil)
		So(l, ShouldEqual, len(s))
	}
	return f.Name()
}

func encode(v interface{}) string {
	b, err := json.Marshal(v)
	So(err, ShouldBeNil)
	return string(b)
}

func TestValidate(t *testing.T) {
	Convey("validateUploadTestResult", t, func() {
		Convey("Success", func() {
			Convey("With valid artifacts", func() {
			})

			Convey("Without artifacts", func() {
			})
		})

		Convey("Fails with invalid artifacts", func() {
		})

	})

	Convey("validateUploadTestResultFile", t, func() {
		Convey("Success", func() {
			Convey("With valid artifacts", func() {
			})

			Convey("Without artifacts", func() {
			})
		})

		Convey("Fails with invalid artifacts", func() {
		})
	})

	Convey("validateArtifact", t, func() {
		Convey("Success with a valid artifact", func() {
		})

		Convey("Fails", func() {
			Convey("With invalid name", func() {
			})

			Convey("With inaccessible file", func() {
			})
		})
	})

	Convey("checkFileAccess", t, func() {
		Convey("Success", func() {
			Convey("With a valid file", func() {
				p := createFile(`"this is an artifact."`)
				defer os.Remove(p)
				So(checkFileAccess(p), ShouldBeNil)
			})
			Convey("With an empty file", func() {
				p := createFile("")
				defer os.Remove(p)
				So(checkFileAccess(p), ShouldBeNil)
			})
		})

		Convey("Fails", func() {
			Convey("With a non-existing file", func() {
				p := "this fie doesnt existttt"
				_, err := os.Stat(p)
				So(err, ShouldErrLike, "no such file")
				So(checkFileAccess(p), ShouldErrLike, "no such file")
			})
			Convey("With an existing directory", func() {
				p, err := ioutil.TempDir("", "test_foo")
				So(err, ShouldBeNil)
				defer os.RemoveAll(p)
				So(checkFileAccess(p), ShouldErrLike, "not a regular file")
			})
			Convey("With a not-readable file", func() {
				p := createFile(`"this is an artifact."`)
				defer os.Remove(p)
				So(os.Chmod(p, 0111), ShouldBeNil)
				So(checkFileAccess(p), ShouldErrLike, "permission denied")
			})
		})
	})
}
