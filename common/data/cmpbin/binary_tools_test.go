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

package cmpbin

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBinaryTools(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Join", t, func(t *ftt.Test) {
		t.Run("returns bytes with nil separator", func(t *ftt.Test) {
			join := ConcatBytes([]byte("hello"), []byte("world"))
			assert.Loosely(t, join, should.Match([]byte("helloworld")))
		})
	})

	ftt.Run("Test Invert", t, func(t *ftt.Test) {
		t.Run("returns nil for nil input", func(t *ftt.Test) {
			inv := InvertBytes(nil)
			assert.Loosely(t, inv, should.BeNil)
		})

		t.Run("returns nil for empty input", func(t *ftt.Test) {
			inv := InvertBytes([]byte{})
			assert.Loosely(t, inv, should.BeNil)
		})

		t.Run("returns byte slice of same length as input", func(t *ftt.Test) {
			input := []byte("こんにちは, world")
			inv := InvertBytes(input)
			assert.Loosely(t, len(input), should.Equal(len(inv)))
		})

		t.Run("returns byte slice with each byte inverted", func(t *ftt.Test) {
			inv := InvertBytes([]byte("foo"))
			assert.Loosely(t, inv, should.Match([]byte{153, 144, 144}))
		})
	})

	ftt.Run("Test Increment", t, func(t *ftt.Test) {
		t.Run("returns empty slice and overflow true when input is nil", func(t *ftt.Test) {
			incr, overflow := IncrementBytes(nil)
			assert.Loosely(t, incr, should.BeNil)
			assert.Loosely(t, overflow, should.BeTrue)
		})

		t.Run("returns empty slice and overflow true when input is empty", func(t *ftt.Test) {
			incr, overflow := IncrementBytes([]byte{})
			assert.Loosely(t, incr, should.BeNil)
			assert.Loosely(t, overflow, should.BeTrue)
		})

		t.Run("handles overflow", func(t *ftt.Test) {
			incr, overflow := IncrementBytes([]byte{0xFF, 0xFF})
			assert.Loosely(t, incr, should.Match([]byte{0, 0}))
			assert.Loosely(t, overflow, should.BeTrue)
		})

		t.Run("increments with overflow false when there is no overflow", func(t *ftt.Test) {
			incr, overflow := IncrementBytes([]byte{0xCA, 0xFF, 0xFF})
			assert.Loosely(t, incr, should.Match([]byte{0xCB, 0, 0}))
			assert.Loosely(t, overflow, should.BeFalse)
		})
	})
}
