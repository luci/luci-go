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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	aerr := New("test error A")
	berr := New("test error B")

	Convey(`Filter works.`, t, func() {
		So(Filter(nil, nil), ShouldEqual, nil)
		So(Filter(nil, nil, aerr, berr), ShouldEqual, nil)
		So(Filter(aerr, nil), ShouldEqual, aerr)
		So(Filter(aerr, nil, aerr, berr), ShouldEqual, nil)
		So(Filter(aerr, berr), ShouldEqual, aerr)
		So(Filter(MultiError{aerr, berr}, berr), ShouldResemble, MultiError{aerr, nil})
		So(Filter(MultiError{aerr, aerr}, aerr), ShouldEqual, nil)
		So(Filter(MultiError{MultiError{aerr, aerr}, aerr}, aerr), ShouldEqual, nil)
	})

	Convey(`FilterFunc works.`, t, func() {
		So(FilterFunc(nil, func(error) bool { return false }), ShouldEqual, nil)
		So(FilterFunc(aerr, func(error) bool { return true }), ShouldEqual, nil)
		So(FilterFunc(aerr, func(error) bool { return false }), ShouldEqual, aerr)

		// Make sure MultiError gets evaluated before its contents do.
		So(FilterFunc(MultiError{aerr, berr}, func(e error) bool {
			if me, ok := e.(MultiError); ok {
				return len(me) == 2
			}
			return false
		}), ShouldEqual, nil)
	})
}
