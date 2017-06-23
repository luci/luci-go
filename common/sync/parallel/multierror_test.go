// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package parallel

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type otherMEType []error

func (o otherMEType) Error() string { return "FAIL" }

func TestMultiError(t *testing.T) {
	t.Parallel()

	Convey("MultiError works", t, func() {
		Convey("multiErrorFromErrors with errors works", func() {
			mec := make(chan error, 4)
			mec <- nil
			mec <- fmt.Errorf("first error")
			mec <- nil
			mec <- fmt.Errorf("what")
			close(mec)

			err := multiErrorFromErrors(mec)
			So(err.Error(), ShouldEqual, `first error (and 1 other error)`)
		})

		Convey("multiErrorFromErrors with nil works", func() {
			So(multiErrorFromErrors(nil), ShouldBeNil)

			c := make(chan error)
			close(c)
			So(multiErrorFromErrors(c), ShouldBeNil)

			c = make(chan error, 2)
			c <- nil
			c <- nil
			close(c)
			So(multiErrorFromErrors(c), ShouldBeNil)
		})
	})
}
