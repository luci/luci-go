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
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidation(t *testing.T) {
	t.Parallel()

	Convey("No errors", t, func() {
		c := Context{Context: context.Background()}

		c.SetFile("zz")
		c.Enter("zzz")
		c.Exit()

		So(c.Finalize(), ShouldBeNil)
	})

	Convey("One simple error", t, func() {
		c := Context{Context: context.Background()}

		c.SetFile("file.cfg")
		c.Enter("ctx %d", 123)
		c.Errorf("blah %s", "zzz")
		err := c.Finalize()
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
		c := Context{Context: context.Background()}

		c.Errorf("top %d", 1)
		c.Errorf("top %d", 2)

		c.SetFile("file_1.cfg")
		c.Errorf("f1")
		c.Errorf("f2")

		c.Enter("p %d", 1)
		c.Errorf("zzz 1")

		c.Enter("p %d", 2)
		c.Errorf("zzz 2")
		c.Exit()

		c.Errorf("zzz 3")

		c.SetFile("file_2.cfg")
		c.Errorf("zzz 4")

		err := c.Finalize()
		So(err, ShouldHaveSameTypeAs, &Error{})
		So(err.Error(), ShouldEqual, `in <unspecified file>: top 1 (and 7 other errors)`)

		var errs []string
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
