// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package errors

import (
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMultiError(t *testing.T) {
	t.Parallel()

	Convey("MultiError works", t, func() {
		var me error = MultiError{fmt.Errorf("hello"), fmt.Errorf("bob")}

		So(me.Error(), ShouldEqual, `hello (and 1 other error)`)
	})
}

func TestUpstreamErrors(t *testing.T) {
	t.Parallel()

	Convey("Test MultiError", t, func() {
		Convey("nil", func() {
			me := MultiError(nil)
			So(me.Error(), ShouldEqual, "(0 errors)")
			Convey("single", func() {
				So(SingleError(error(me)), ShouldBeNil)
			})
		})
		Convey("one", func() {
			me := MultiError{errors.New("sup")}
			So(me.Error(), ShouldEqual, "sup")
		})
		Convey("two", func() {
			me := MultiError{errors.New("sup"), errors.New("what")}
			So(me.Error(), ShouldEqual, "sup (and 1 other error)")
		})
		Convey("more", func() {
			me := MultiError{errors.New("sup"), errors.New("what"), errors.New("nerds")}
			So(me.Error(), ShouldEqual, "sup (and 2 other errors)")

			Convey("single", func() {
				So(SingleError(error(me)), ShouldResemble, errors.New("sup"))
			})
		})
	})

	Convey("SingleError passes through", t, func() {
		e := errors.New("unique")
		So(SingleError(e), ShouldEqual, e)
	})
}
