// Copyright 2017 The LUCI Authors.
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

package validation

import (
	"testing"

	"go.chromium.org/luci/common/logging"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	Convey("No errors", t, func() {
		v := Context{}

		v.SetFile("zz")
		v.Enter("zzz")
		v.Exit()

		So(v.Finalize(), ShouldBeNil)
	})

	Convey("One simple error", t, func() {
		v := Context{
			Logger: logging.Null, // for code coverage
		}

		v.SetFile("file.cfg")
		v.Enter("ctx %d", 123)
		v.Error("blah %s", "zzz")
		err := v.Finalize()
		So(err, ShouldHaveSameTypeAs, &Error{})
		So(err.Error(), ShouldEqual, `in "file.cfg" (ctx 123): blah zzz`)

		singleErr := err.(*Error).Errors[0]
		So(singleErr.Error(), ShouldEqual, `in "file.cfg" (ctx 123): blah zzz`)
		d, ok := fileTag.In(singleErr)
		So(ok, ShouldBeTrue)
		So(d, ShouldEqual, "file.cfg")

		elts, ok := elementTag.In(singleErr)
		So(ok, ShouldBeTrue)
		So(elts, ShouldResemble, []string{"ctx 123"})
	})

	Convey("Regular usage", t, func() {
		v := Context{}

		v.Error("top %d", 1)
		v.Error("top %d", 2)

		v.SetFile("file_1.cfg")
		v.Error("f1")
		v.Error("f2")

		v.Enter("p %d", 1)
		v.Error("zzz 1")

		v.Enter("p %d", 2)
		v.Error("zzz 2")
		v.Exit()

		v.Error("zzz 3")

		v.SetFile("file_2.cfg")
		v.Error("zzz 4")

		err := v.Finalize()
		So(err, ShouldHaveSameTypeAs, &Error{})
		So(err.Error(), ShouldEqual, `in <unspecified file>: top 1 (and 7 other errors)`)

		errs := []string{}
		for _, e := range err.(*Error).Errors {
			errs = append(errs, e.Error())
		}
		So(errs, ShouldResemble, []string{
			`in <unspecified file>: top 1`,
			`in <unspecified file>: top 2`,
			`in "file_1.cfg": f1`,
			`in "file_1.cfg": f2`,
			`in "file_1.cfg" (p 1): zzz 1`,
			`in "file_1.cfg" (p 1 / p 2): zzz 2`,
			`in "file_1.cfg" (p 1): zzz 3`,
			`in "file_2.cfg": zzz 4`,
		})
	})
}
