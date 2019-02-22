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

package lucicfg

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFindTrackedFiles(t *testing.T) {
	t.Parallel()

	Convey("With a bunch of files", t, func() {
		tmp, err := ioutil.TempDir("", "lucicfg")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tmp)

		touch := func(p string) {
			p = filepath.Join(tmp, filepath.FromSlash(p))
			So(os.MkdirAll(filepath.Dir(p), 0700), ShouldBeNil)
			So(ioutil.WriteFile(p, nil, 0600), ShouldBeNil)
		}

		files := []string{
			"a-dev.cfg",
			"a.cfg",
			"sub/a-dev.cfg",
			"sub/a.cfg",
		}
		for _, f := range files {
			touch(f)
		}

		Convey("Works", func() {
			files, err := FindTrackedFiles(tmp, []string{"*.cfg", "!*-dev.cfg"})
			So(err, ShouldBeNil)
			So(files, ShouldResemble, []string{"a.cfg", "sub/a.cfg"})
		})

		Convey("No negative", func() {
			files, err := FindTrackedFiles(tmp, []string{"*.cfg"})
			So(err, ShouldBeNil)
			So(files, ShouldResemble, []string{
				"a-dev.cfg",
				"a.cfg",
				"sub/a-dev.cfg",
				"sub/a.cfg",
			})
		})

		Convey("No positive", func() {
			files, err := FindTrackedFiles(tmp, []string{"!*.cfg"})
			So(err, ShouldBeNil)
			So(files, ShouldHaveLength, 0)
		})

		Convey("Implied *", func() {
			files, err := FindTrackedFiles(tmp, []string{"!*-dev.cfg"})
			So(err, ShouldBeNil)
			So(files, ShouldResemble, []string{
				"a.cfg",
				"sub/a.cfg",
			})
		})

		Convey("Missing directory", func() {
			files, err := FindTrackedFiles(filepath.Join(tmp, "missing"), []string{"*.cfg"})
			So(err, ShouldBeNil)
			So(files, ShouldHaveLength, 0)
		})

		Convey("Empty patterns", func() {
			files, err := FindTrackedFiles(tmp, nil)
			So(err, ShouldBeNil)
			So(files, ShouldHaveLength, 0)
		})

		Convey("Bad positive pattern", func() {
			_, err := FindTrackedFiles(tmp, []string{"["})
			So(err, ShouldErrLike, `bad pattern "["`)
		})

		Convey("Bad negative pattern", func() {
			_, err := FindTrackedFiles(tmp, []string{"*", "!["})
			So(err, ShouldErrLike, `bad pattern "["`)
		})
	})
}
