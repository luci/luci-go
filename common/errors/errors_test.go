// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package errors

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMark(t *testing.T) {
	Convey("MakeMarkFn works", t, func() {
		f := MakeMarkFn("errorsTest")

		type Cat struct{ name string }

		// unfortunately... the line numbers for these tests matter
		c := &Cat{"dude"}
		var err error
		err = f(c)

		So(err.Error(), ShouldEqual, "errorsTest - errors_test.go:23 - &{name:dude}")
		So(f("hello").Error(), ShouldEqual, "errorsTest - errors_test.go:26 - hello")

		So(err.(*MarkedError).Orig, ShouldEqual, c)

		So(f(nil), ShouldBeNil)
	})
}

func TestMultiError(t *testing.T) {
	Convey("MultiError works", t, func() {
		var me error = MultiError{fmt.Errorf("hello"), fmt.Errorf("bob")}

		So(me.Error(), ShouldEqual, `["hello" "bob"]`)

		Convey("MultiErrorFromErrors with errors works", func() {
			mec := make(chan error, 5)
			mec <- fmt.Errorf("what")
			mec <- nil
			mec <- MakeMarkFn("multiErr")("one-off")
			close(mec)

			err := MultiErrorFromErrors(mec)
			So(err.Error(), ShouldEqual, `["what" "multiErr - errors_test.go:44 - one-off"]`)
		})

		Convey("MultiErrorFromErrors with nil works", func() {
			So(MultiErrorFromErrors(nil), ShouldBeNil)

			c := make(chan error)
			close(c)
			So(MultiErrorFromErrors(c), ShouldBeNil)

			c = make(chan error, 4)
			c <- nil
			c <- nil
			close(c)
			So(MultiErrorFromErrors(c), ShouldBeNil)
		})
	})
}

func ExampleMakeMarkFn() {
	// usually this would be in some global area of your library
	mark := MakeMarkFn("cool_package")

	data := 100 // Data can be anything!
	err := mark(data)
	fmt.Printf("got: %q\n", err)

	marked := err.(*MarkedError)
	fmt.Printf("original: %d", marked.Orig)

	// Output:
	// got: "cool_package - errors_test.go:72 - 100"
	// original: 100
}

func ExampleMultiError() {
	errCh := make(chan error, 10)
	errCh <- nil // nils are ignored
	errCh <- fmt.Errorf("what")
	close(errCh)

	err := MultiErrorFromErrors(errCh)
	fmt.Printf("got: %s len=%d\n", err, len(err.(MultiError)))

	errCh = make(chan error, 10)
	errCh <- nil // and if the channel only has nils
	close(errCh)

	err = MultiErrorFromErrors(errCh) // then it returns nil
	fmt.Printf("got: %v\n", err)

	// Output:
	// got: ["what"] len=1
	// got: <nil>
}
