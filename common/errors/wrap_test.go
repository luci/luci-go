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

package errors

import (
	stderrors "errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

type testWrapped struct {
	error
}

func (w *testWrapped) Error() string {
	if w.error == nil {
		return "wrapped: nil"
	}
	return fmt.Sprintf("wrapped: %v", w.error.Error())
}

func (w *testWrapped) InnerError() error {
	return w.error
}

func testWrap(err error) error {
	return &testWrapped{err}
}

type testUnwrappable struct {
	error
}

func (u *testUnwrappable) Unwrap() error {
	return u.error
}

func testNewUnwrappable(err error) error {
	return &testUnwrappable{err}
}

func TestWrapped(t *testing.T) {
	t.Parallel()

	Convey(`Test Wrapped`, t, func() {
		Convey(`A nil error`, func() {
			var err error

			Convey(`Unwraps to nil.`, func() {
				So(Unwrap(err), ShouldBeNil)
			})

			Convey(`When wrapped, does not unwrap to nil.`, func() {
				So(Unwrap(testWrap(err)), ShouldNotBeNil)
			})

			Convey(`When unwrappalbe, does not unwrap to nil.`, func() {
				So(Unwrap(testNewUnwrappable(err)), ShouldNotBeNil)
				So(stderrors.Unwrap(testNewUnwrappable(err)), ShouldBeNil)
			})
		})

		Convey(`A non-wrapped error.`, func() {
			err := New("test error")

			Convey(`Unwraps to itself.`, func() {
				So(Unwrap(err), ShouldEqual, err)
			})

			Convey(`When wrapped, unwraps to itself.`, func() {
				So(Unwrap(testWrap(err)), ShouldEqual, err)
			})

			Convey(`When unwrappable, unwraps to itself.`, func() {
				So(Unwrap(testNewUnwrappable(err)), ShouldEqual, err)
			})

			Convey(`When double-wrapped, unwraps to itself.`, func() {
				So(Unwrap(testWrap(testWrap(err))), ShouldEqual, err)
			})

			Convey(`When double-unwrappable, unwraps to itself.`, func() {
				So(Unwrap(testNewUnwrappable(testNewUnwrappable(err))), ShouldEqual, err)
			})
		})
	})
}
