// Copyright 2015 The LUCI Authors.
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

package assertions

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type customError struct{}

func (customError) Error() string { return "customError noob" }

func TestShouldErrLike(t *testing.T) {
	t.Parallel()

	ce := customError{}
	e := errors.New("e is for error")

	Convey("Test ShouldErrLike", t, func() {
		Convey("too many params", func() {
			So(ShouldErrLike(nil, nil, nil), ShouldContainSubstring, "requires 0 or 1")
		})
		Convey("no expectation", func() {
			So(ShouldErrLike(nil), ShouldEqual, "")
			So(ShouldErrLike(e), ShouldContainSubstring, "Expected: nil")
			So(ShouldErrLike(ce), ShouldContainSubstring, "Expected: nil")
		})
		Convey("nil expectation", func() {
			So(ShouldErrLike(nil, nil), ShouldEqual, "")
			So(ShouldErrLike(e, nil), ShouldContainSubstring, "Expected: nil")
			So(ShouldErrLike(ce, nil), ShouldContainSubstring, "Expected: nil")
		})
		Convey("nil actual", func() {
			So(ShouldErrLike(nil, "wut"), ShouldContainSubstring, "Expected '<nil>' to NOT be nil")
		})
		Convey("not an err", func() {
			So(ShouldErrLike(100, "wut"), ShouldContainSubstring, "Expected: 'error interface support'")
		})
		Convey("string actual", func() {
			So(ShouldErrLike(e, "is for error"), ShouldEqual, "")
			So(ShouldErrLike(ce, "customError"), ShouldEqual, "")
		})
		Convey("error actual", func() {
			So(ShouldErrLike(e, e), ShouldEqual, "")
			So(ShouldErrLike(ce, ce), ShouldEqual, "")
		})
		Convey("bad expected type", func() {
			So(ShouldErrLike(e, 20), ShouldContainSubstring, "unknown argument type int")
		})
	})
}
