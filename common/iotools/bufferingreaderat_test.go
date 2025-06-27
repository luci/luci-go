// Copyright 2018 The LUCI Authors.
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

package iotools

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBlocksLRU(t *testing.T) {
	t.Parallel()

	makeBlock := func(offset int) block {
		return block{int64(offset), []byte(fmt.Sprintf("%d", offset)), false}
	}

	ftt.Run("With LRU", t, func(t *ftt.Test) {
		evicted := []block{}

		lru := blocksLRU{
			evicted: func(b block) { evicted = append(evicted, b) },
		}
		lru.init(3)

		t.Run("Basic add/get works", func(t *ftt.Test) {
			b, ok := lru.get(0)
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, b, should.Resemble(block{}))

			lru.add(makeBlock(0))

			b, ok = lru.get(0)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, b, should.Resemble(makeBlock(0)))
		})

		t.Run("Basic eviction works", func(t *ftt.Test) {
			for i := range 5 {
				lru.add(makeBlock(i))
			}
			// Evicted two oldest ones.
			assert.Loosely(t, evicted, should.Resemble([]block{makeBlock(0), makeBlock(1)}))
			// The rest are still there.
			for i := 2; i < 5; i++ {
				b, ok := lru.get(int64(i))
				assert.Loosely(t, ok, should.BeTrue)
				assert.Loosely(t, b, should.Resemble(makeBlock(i)))
			}
		})

		t.Run("LRU logic works", func(t *ftt.Test) {
			lru.add(makeBlock(0))
			lru.add(makeBlock(1))
			lru.add(makeBlock(2))

			// 0 is oldest one, but we read it to bump its access time.
			lru.get(0)
			// This call now evicts 1 as the oldest.
			lru.add(makeBlock(3))
			// Yep.
			assert.Loosely(t, evicted, should.Resemble([]block{makeBlock(1)}))
		})
	})
}

// trackingReaderAt tracks calls to ReadAt and can inject errors.
type trackingReaderAt struct {
	r     io.ReaderAt
	calls []readAtCall

	err         error // error to return
	partialRead int   // how many bytes to read when simulating an error
}

type readAtCall struct {
	offset int64
	length int
}

func (r *trackingReaderAt) ReadAt(p []byte, offset int64) (int, error) {
	r.calls = append(r.calls, readAtCall{offset, len(p)})

	if r.err != nil {
		p = p[:r.partialRead]
		n, err := r.r.ReadAt(p, offset)
		if err != nil {
			return n, err
		}
		return n, r.err
	}

	return r.r.ReadAt(p, offset)
}

func TestBufferingReaderAt(t *testing.T) {
	t.Parallel()

	ftt.Run("One byte block size", t, func(t *ftt.Test) {
		data := &trackingReaderAt{r: strings.NewReader("0123456789")}
		r := NewBufferingReaderAt(data, 1, 4)

		buf := make([]byte, 4)
		n, err := r.ReadAt(buf, 0)
		assert.Loosely(t, n, should.Equal(4))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal("0123"))

		// Fetched the data block-by-block sequentially.
		assert.Loosely(t, data.calls, should.Resemble([]readAtCall{
			{0, 1},
			{1, 1},
			{2, 1},
			{3, 1},
		}))
		data.calls = nil

		// Read from the middle of already read range, should make no new calls.
		buf = make([]byte, 2)
		n, err = r.ReadAt(buf, 1)
		assert.Loosely(t, n, should.Equal(2))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal("12"))
		assert.Loosely(t, data.calls, should.HaveLength(0))

		// Read few bytes more, it should reading new data.
		buf = make([]byte, 5)
		n, err = r.ReadAt(buf, 1)
		assert.Loosely(t, n, should.Equal(5))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal("12345"))
		assert.Loosely(t, data.calls, should.Resemble([]readAtCall{
			{4, 1},
			{5, 1},
		}))
		data.calls = nil

		// Hit EOF.
		buf = make([]byte, 11)
		n, err = r.ReadAt(buf, 0)
		assert.Loosely(t, n, should.Equal(10))
		assert.Loosely(t, err, should.Equal(io.EOF))
		assert.Loosely(t, string(buf[:10]), should.Equal("0123456789"))

		// Try to read past EOF.
		n, err = r.ReadAt(buf, 10)
		assert.Loosely(t, n, should.BeZero)
		assert.Loosely(t, err, should.Equal(io.EOF))

		// 0 buffer read just "probes" for EOF.
		n, err = r.ReadAt(nil, 0)
		assert.Loosely(t, n, should.BeZero)
		assert.Loosely(t, err, should.BeNil)
		n, err = r.ReadAt(nil, 10)
		assert.Loosely(t, n, should.BeZero)
		assert.Loosely(t, err, should.Equal(io.EOF))
	})

	ftt.Run("Big block size", t, func(t *ftt.Test) {
		data := &trackingReaderAt{r: strings.NewReader("0123456789")}
		r := NewBufferingReaderAt(data, 6, 2)

		// Read a small chunk of the first block from the middle.
		buf := make([]byte, 2)
		n, err := r.ReadAt(buf, 2)
		assert.Loosely(t, n, should.Equal(2))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal("23"))

		assert.Loosely(t, data.calls, should.Resemble([]readAtCall{{0, 6}}))
		data.calls = nil

		// Read a large chunk of the first block and a bit of the second.
		buf = make([]byte, 7)
		n, err = r.ReadAt(buf, 2)
		assert.Loosely(t, n, should.Equal(7))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal("2345678"))

		assert.Loosely(t, data.calls, should.Resemble([]readAtCall{{6, 6}}))
		data.calls = nil

		// Partially read the last chunk.
		n, err = r.ReadAt(buf, 8)
		assert.Loosely(t, n, should.Equal(2))
		assert.Loosely(t, err, should.Equal(io.EOF))
		assert.Loosely(t, string(buf[:2]), should.Equal("89"))

		// Try to partially read a block past EOF.
		n, err = r.ReadAt(buf, 23)
		assert.Loosely(t, n, should.BeZero)
		assert.Loosely(t, err, should.Equal(io.EOF))
	})

	ftt.Run("Handle unexpected errors", t, func(t *ftt.Test) {
		data := &trackingReaderAt{r: strings.NewReader("0123456789")}
		r := NewBufferingReaderAt(data, 5, 2)

		// Simulate partial failed read that returns 4 bytes.
		data.err = fmt.Errorf("boo")
		data.partialRead = 4

		// But read only 3 bytes. It should succeed, we don't care about 4th byte.
		buf := make([]byte, 3)
		n, err := r.ReadAt(buf, 0)
		assert.Loosely(t, n, should.Equal(3))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(buf), should.Equal("012"))

		// But when reading the full page, it actually returns the error.
		buf = make([]byte, 5)
		n, err = r.ReadAt(buf, 0)
		assert.Loosely(t, n, should.Equal(4))
		assert.Loosely(t, err, should.Equal(data.err))
		assert.Loosely(t, string(buf[:4]), should.Equal("0123"))
	})
}
