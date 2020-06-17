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
	"errors"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalk(t *testing.T) {
	t.Parallel()

	Convey(`Testing the Walk function`, t, func() {
		count := 0
		keepWalking := true
		walkFn := func(err error) bool {
			count++
			return keepWalking
		}

		Convey(`Will not walk at all for a nil error.`, func() {
			Walk(nil, walkFn)
			So(count, ShouldEqual, 0)
		})

		Convey(`Will fully traverse a wrapped MultiError.`, func() {
			Walk(MultiError{nil, testWrap(New("sup")), nil}, walkFn)
			So(count, ShouldEqual, 3)
		})

		Convey(`Will unwrap a Wrapped error.`, func() {
			Walk(testWrap(New("sup")), walkFn)
			So(count, ShouldEqual, 2)
		})

		Convey(`Will short-circuit if the walk function returns false.`, func() {
			keepWalking = false
			Walk(testWrap(New("sup")), walkFn)
			So(count, ShouldEqual, 1)
		})
	})

	Convey(`Testing the WalkLeaves function`, t, func() {
		count := 0
		keepWalking := true
		walkFn := func(err error) bool {
			count++
			return keepWalking
		}

		Convey(`Will not walk at all for a nil error.`, func() {
			WalkLeaves(nil, walkFn)
			So(count, ShouldEqual, 0)
		})

		Convey(`Will traverse leaves of a wrapped MultiError.`, func() {
			WalkLeaves(MultiError{nil, testWrap(New("sup")), New("sup")}, walkFn)
			So(count, ShouldEqual, 2)
		})

		Convey(`Will unwrap a Wrapped error.`, func() {
			WalkLeaves(testWrap(New("sup")), walkFn)
			So(count, ShouldEqual, 1)
		})

		Convey(`Will short-circuit if the walk function returns false.`, func() {
			keepWalking = false
			WalkLeaves(MultiError{testWrap(New("sup")), New("foo")}, walkFn)
			So(count, ShouldEqual, 1)
		})
	})
}

type intError int

func (i *intError) Is(err error) bool {
	if e, ok := err.(*intError); ok {
		return int(*i)/2 == int(*e)/2
	}
	return false
}

func (i *intError) Error() string {
	return fmt.Sprintf("%d", int(*i))
}

func TestAny(t *testing.T) {
	t.Parallel()

	Convey(`Testing the Any function`, t, func() {
		testErr := errors.New("test error")
		filter := func(err error) bool { return err == testErr }

		for _, err := range []error{
			nil,
			Reason("error test: foo").Err(),
			errors.New("other error"),
		} {
			Convey(fmt.Sprintf(`Registers false for %T %v`, err, err), func() {
				So(Any(err, filter), ShouldBeFalse)
			})
		}

		for _, err := range []error{
			testErr,
			MultiError{errors.New("other error"), MultiError{testErr, nil}},
			Annotate(testErr, "error test").Err(),
		} {
			Convey(fmt.Sprintf(`Registers true for %T %v`, err, err), func() {
				So(Any(err, filter), ShouldBeTrue)
			})
		}
	})
}

func TestContains(t *testing.T) {
	t.Parallel()

	Convey(`Testing the Contains function`, t, func() {
		testErr := errors.New("test error")

		for _, err := range []error{
			nil,
			Reason("error test: foo").Err(),
			errors.New("other error"),
		} {
			Convey(fmt.Sprintf(`Registers false for %T %v`, err, err), func() {
				So(Contains(err, testErr), ShouldBeFalse)
			})
		}

		for _, err := range []error{
			testErr,
			MultiError{errors.New("other error"), MultiError{testErr, nil}},
			Annotate(testErr, "error test").Err(),
		} {
			Convey(fmt.Sprintf(`Registers true for %T %v`, err, err), func() {
				So(Contains(err, testErr), ShouldBeTrue)
			})
		}

		Convey(`Support Is`, func() {
			e0 := intError(0)
			e1 := intError(1)
			e2 := intError(2)
			wrapped0 := testWrap(&e0)
			So(Contains(wrapped0, &e0), ShouldBeTrue)
			So(Contains(wrapped0, &e1), ShouldBeTrue)
			So(Contains(wrapped0, &e2), ShouldBeFalse)
		})
	})
}
