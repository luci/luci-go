// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type testMarkError struct{ name string }

func (c *testMarkError) Error() string {
	return c.name
}

func TestMark(t *testing.T) {
	t.Parallel()

	Convey("MakeMarkFn works", t, func() {
		f := MakeMarkFn("errorsTest")

		// unfortunately... the line numbers for these tests matter
		c := &testMarkError{"dude"}
		var err error
		err = f(c)

		So(err.Error(), ShouldEqual, "errorsTest: markederror_test.go:29: dude")
		So(f(New("hello")).Error(), ShouldEqual, "errorsTest: markederror_test.go:32: hello")

		So(Unwrap(err), ShouldEqual, c)

		So(f(nil), ShouldBeNil)
	})
}

func ExampleMakeMarkFn() {
	// usually this would be in some global area of your library
	mark := MakeMarkFn("cool_package")

	err := mark(New("my error"))
	fmt.Printf("got: %q\n", err)
	fmt.Printf("original: %s", Unwrap(err))

	// Output:
	// got: "cool_package: markederror_test.go:44: my error"
	// original: my error
}
