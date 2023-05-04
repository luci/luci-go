// Copyright 2022 The LUCI Authors.
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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

// TestErrorIs makes a new fake value implementing the Error interface and wraps it using Annotate().
// Along the way, we test whether Is returns what's expected.
func TestErrorIs(t *testing.T) {
	t.Parallel()
	Convey("test is", t, func() {
		newFakeError := &fakeError{}
		wrappedError := Annotate(newFakeError, "8ed2d02c-a8c0-4b8e-b734-bf30cb88d0c6").Err()
		So(newFakeError.Error(), ShouldEqual, "f9bd822e-6568-46ab-ba7d-419ef5f64b3b")
		So(errors.Is(newFakeError, &fakeError{}), ShouldBeTrue)
		So(Is(newFakeError, &fakeError{}), ShouldBeTrue)
		So(errors.Is(wrappedError, &fakeError{}), ShouldBeTrue)
		So(Is(wrappedError, &fakeError{}), ShouldBeTrue)
	})
}

// TestErrorAs makes a new fake value implementing the Error interface and wraps it using Annotate().
// Along the way, we test whether As succeeds for the raw error and the annotated error.
func TestErrorAs(t *testing.T) {
	t.Parallel()
	Convey("test as", t, func() {
		Convey("raw error", func() {
			var dst *fakeError
			newFakeError := &fakeError{}
			So(As(newFakeError, &dst), ShouldBeTrue)
			So(newFakeError == dst, ShouldBeTrue)
		})
		Convey("wrapped error", func() {
			var dst *fakeError
			newFakeError := &fakeError{}
			wrappedError := Annotate(newFakeError, "8ed2d02c-a8c0-4b8e-b734-bf30cb88d0c6").Err()
			So(As(wrappedError, &dst), ShouldBeTrue)
			So(newFakeError == dst, ShouldBeTrue)
		})
	})
}

// TestErrorJoin tests using the builtin error.Join.
func TestErrorJoin(t *testing.T) {
	t.Parallel()
	Convey("test join", t, func() {
		errorA := errors.New("a")
		errorB := errors.New("b")
		combined := Join(errorA, errorB)
		So(combined.Error(), ShouldEqual, `a
b`)
	})
}

type fakeError struct{}

func (w *fakeError) Error() string {
	return "f9bd822e-6568-46ab-ba7d-419ef5f64b3b"
}

// Check that fakeError is an error.
var _ error = &fakeError{}
