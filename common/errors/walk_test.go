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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWalk(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing the Walk function`, t, func(t *ftt.Test) {
		count := 0
		keepWalking := true
		walkFn := func(err error) bool {
			count++
			return keepWalking
		}

		t.Run(`Will not walk at all for a nil error.`, func(t *ftt.Test) {
			Walk(nil, walkFn)
			assert.Loosely(t, count, should.BeZero)
		})

		t.Run(`Will fully traverse a wrapped MultiError.`, func(t *ftt.Test) {
			Walk(MultiError{nil, testWrap(New("sup")), nil}, walkFn)
			assert.Loosely(t, count, should.Equal(3))
		})

		t.Run(`Will unwrap a Wrapped error.`, func(t *ftt.Test) {
			Walk(testWrap(New("sup")), walkFn)
			assert.Loosely(t, count, should.Equal(2))
		})

		t.Run(`Will short-circuit if the walk function returns false.`, func(t *ftt.Test) {
			keepWalking = false
			Walk(testWrap(New("sup")), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})
	})

	ftt.Run(`Testing the WalkLeaves function`, t, func(t *ftt.Test) {
		count := 0
		keepWalking := true
		walkFn := func(err error) bool {
			count++
			return keepWalking
		}

		t.Run(`Will not walk at all for a nil error.`, func(t *ftt.Test) {
			WalkLeaves(nil, walkFn)
			assert.Loosely(t, count, should.BeZero)
		})

		t.Run(`Will visit a simple annotator error.`, func(t *ftt.Test) {
			WalkLeaves(Reason("sup").Err(), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})

		t.Run(`Will visit a wrapped annotator error.`, func(t *ftt.Test) {
			WalkLeaves(Annotate(Reason("sup").Err(), "boo").Err(), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})

		t.Run(`Will traverse leaves of a wrapped MultiError.`, func(t *ftt.Test) {
			WalkLeaves(MultiError{nil, testWrap(New("sup")), New("sup")}, walkFn)
			assert.Loosely(t, count, should.Equal(2))
		})

		t.Run(`Will unwrap a Wrapped error.`, func(t *ftt.Test) {
			WalkLeaves(testWrap(New("sup")), walkFn)
			assert.Loosely(t, count, should.Equal(1))
		})

		t.Run(`Will short-circuit if the walk function returns false.`, func(t *ftt.Test) {
			keepWalking = false
			WalkLeaves(MultiError{testWrap(New("sup")), New("foo")}, walkFn)
			assert.Loosely(t, count, should.Equal(1))
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

	ftt.Run(`Testing the Any function`, t, func(t *ftt.Test) {
		testErr := errors.New("test error")
		filter := func(err error) bool { return err == testErr }

		for _, err := range []error{
			nil,
			Reason("error test: foo").Err(),
			errors.New("other error"),
		} {
			t.Run(fmt.Sprintf(`Registers false for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, Any(err, filter), should.BeFalse)
			})
		}

		for _, err := range []error{
			testErr,
			MultiError{errors.New("other error"), MultiError{testErr, nil}},
			Annotate(testErr, "error test").Err(),
		} {
			t.Run(fmt.Sprintf(`Registers true for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, Any(err, filter), should.BeTrue)
			})
		}
	})
}

func TestContains(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing the Contains function`, t, func(t *ftt.Test) {
		testErr := errors.New("test error")

		for _, err := range []error{
			nil,
			Reason("error test: foo").Err(),
			errors.New("other error"),
		} {
			t.Run(fmt.Sprintf(`Registers false for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, Contains(err, testErr), should.BeFalse)
			})
		}

		for _, err := range []error{
			testErr,
			MultiError{errors.New("other error"), MultiError{testErr, nil}},
			Annotate(testErr, "error test").Err(),
		} {
			t.Run(fmt.Sprintf(`Registers true for %T %v`, err, err), func(t *ftt.Test) {
				assert.Loosely(t, Contains(err, testErr), should.BeTrue)
			})
		}

		t.Run(`Support Is`, func(t *ftt.Test) {
			e0 := intError(0)
			e1 := intError(1)
			e2 := intError(2)
			wrapped0 := testWrap(&e0)
			assert.Loosely(t, Contains(wrapped0, &e0), should.BeTrue)
			assert.Loosely(t, Contains(wrapped0, &e1), should.BeTrue)
			assert.Loosely(t, Contains(wrapped0, &e2), should.BeFalse)
		})
	})
}
