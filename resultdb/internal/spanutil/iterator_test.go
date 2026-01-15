// Copyright 2026 The LUCI Authors.
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

package spanutil

import (
	"testing"

	"google.golang.org/api/iterator"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type fakeIterator struct {
	items []int
	index int
}

func (m *fakeIterator) Next() (int, error) {
	if m.index >= len(m.items) {
		return 0, iterator.Done
	}
	item := m.items[m.index]
	m.index++
	return item, nil
}

func (m *fakeIterator) Stop() {}

func TestPeekingIterator(t *testing.T) {
	ftt.Run("PeekingIterator", t, func(t *ftt.Test) {
		fake := &fakeIterator{items: []int{1, 2, 3}}
		it := NewPeekingIterator(fake)

		t.Run("Peek and Next work correctly", func(t *ftt.Test) {
			// Peek first item
			val, err := it.Peek()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(1))

			// Peek again, should be same
			val, err = it.Peek()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(1))

			// Next should return first item
			val, err = it.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(1))

			// Next should return second item
			val, err = it.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(2))

			// Peek third item
			val, err = it.Peek()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(3))

			// Next should return third item
			val, err = it.Next()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, val, should.Equal(3))

			// Peek done
			_, err = it.Peek()
			assert.Loosely(t, err, should.Equal(iterator.Done))

			// Next done
			_, err = it.Next()
			assert.Loosely(t, err, should.Equal(iterator.Done))
		})
	})
}
