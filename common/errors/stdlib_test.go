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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// TestErrorIs makes a new fake value implementing the Error interface and wraps it using Annotate().
// Along the way, we test whether Is returns what's expected.
func TestErrorIs(t *testing.T) {
	t.Parallel()
	ftt.Run("test is", t, func(t *ftt.Test) {
		newFakeError := &fakeError{}
		wrappedError := Annotate(newFakeError, "8ed2d02c-a8c0-4b8e-b734-bf30cb88d0c6").Err()
		assert.Loosely(t, newFakeError.Error(), should.Equal("f9bd822e-6568-46ab-ba7d-419ef5f64b3b"))
		assert.Loosely(t, errors.Is(newFakeError, &fakeError{}), should.BeTrue)
		assert.Loosely(t, Is(newFakeError, &fakeError{}), should.BeTrue)
		assert.Loosely(t, errors.Is(wrappedError, &fakeError{}), should.BeTrue)
		assert.Loosely(t, Is(wrappedError, &fakeError{}), should.BeTrue)
	})
}

// TestErrorAs makes a new fake value implementing the Error interface and wraps it using Annotate().
// Along the way, we test whether As succeeds for the raw error and the annotated error.
func TestErrorAs(t *testing.T) {
	t.Parallel()
	ftt.Run("test as", t, func(t *ftt.Test) {
		t.Run("raw error", func(t *ftt.Test) {
			var dst *fakeError
			newFakeError := &fakeError{}
			assert.Loosely(t, As(newFakeError, &dst), should.BeTrue)
			assert.Loosely(t, newFakeError == dst, should.BeTrue)
		})
		t.Run("wrapped error", func(t *ftt.Test) {
			var dst *fakeError
			newFakeError := &fakeError{}
			wrappedError := Annotate(newFakeError, "8ed2d02c-a8c0-4b8e-b734-bf30cb88d0c6").Err()
			assert.Loosely(t, As(wrappedError, &dst), should.BeTrue)
			assert.Loosely(t, newFakeError == dst, should.BeTrue)
		})
	})
}

// TestErrorJoin tests using the builtin error.Join.
func TestErrorJoin(t *testing.T) {
	t.Parallel()
	ftt.Run("test join", t, func(t *ftt.Test) {
		errorA := errors.New("a")
		errorB := errors.New("b")
		combined := Join(errorA, errorB)
		assert.Loosely(t, combined.Error(), should.Equal(`a
b`))
	})
}

type fakeError struct{}

func (w *fakeError) Error() string {
	return "f9bd822e-6568-46ab-ba7d-419ef5f64b3b"
}

// Check that fakeError is an error.
var _ error = &fakeError{}
