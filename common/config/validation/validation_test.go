// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package validation

import (
	"testing"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"

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
		So(errors.ExtractData(singleErr, "file"), ShouldEqual, "file.cfg")
		So(errors.ExtractData(singleErr, "element"), ShouldResemble, []string{"ctx 123"})
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
