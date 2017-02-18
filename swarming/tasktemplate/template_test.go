// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package tasktemplate

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	Convey(`Testing substitution resolution`, t, func() {
		var p Params

		Convey(`With no parameters`, func() {
			v, err := p.Resolve("foo")
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "foo")

			_, err = p.Resolve("${swarming_run_id}")
			So(err, ShouldNotBeNil)

			_, err = p.Resolve("${undefined}")
			So(err, ShouldNotBeNil)
		})

		Convey(`With a "swarming_run_id" parameter defined.`, func() {
			p.SwarmingRunID = "12345"

			v, err := p.Resolve("foo")
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "foo")

			v, err = p.Resolve("${swarming_run_id}")
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "12345")

			_, err = p.Resolve("${undefined}")
			So(err, ShouldNotBeNil)
		})
	})
}
