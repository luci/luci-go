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

	. "github.com/smartystreets/goconvey/convey"
)

func TestBlocksLRU(t *testing.T) {
	t.Parallel()

	makeBlock := func(offset int) block {
		return block{int64(offset), []byte(fmt.Sprintf("%d", offset)), false}
	}

	Convey("With LRU", t, func() {
		evicted := []block{}

		lru := blocksLRU{
			evicted: func(b block) { evicted = append(evicted, b) },
		}
		lru.init(3)

		Convey("Basic add/get works", func() {
			b, ok := lru.get(0)
			So(ok, ShouldBeFalse)
			So(b, ShouldResemble, block{})

			lru.add(makeBlock(0))

			b, ok = lru.get(0)
			So(ok, ShouldBeTrue)
			So(b, ShouldResemble, makeBlock(0))
		})

		Convey("Basic eviction works", func() {
			for i := 0; i < 5; i++ {
				lru.add(makeBlock(i))
			}
			// Evicted two oldest ones.
			So(evicted, ShouldResemble, []block{makeBlock(0), makeBlock(1)})
			// The rest are still there.
			for i := 2; i < 5; i++ {
				b, ok := lru.get(int64(i))
				So(ok, ShouldBeTrue)
				So(b, ShouldResemble, makeBlock(i))
			}
		})

		Convey("LRU logic works", func() {
			lru.add(makeBlock(0))
			lru.add(makeBlock(1))
			lru.add(makeBlock(2))

			// 0 is oldest one, but we read it to bump its access time.
			lru.get(0)
			// This call now evicts 1 as the oldest.
			lru.add(makeBlock(3))
			// Yep.
			So(evicted, ShouldResemble, []block{makeBlock(1)})
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

	Convey("One byte block size", t, func() {
		data := &trackingReaderAt{r: strings.NewReader("0123456789")}
		r := NewBufferingReaderAt(data, 1, 4)

		buf := make([]byte, 4)
		n, err := r.ReadAt(buf, 0)
		So(n, ShouldEqual, 4)
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, "0123")

		// Fetched the data block-by-block sequentially.
		So(data.calls, ShouldResemble, []readAtCall{
			{0, 1},
			{1, 1},
			{2, 1},
			{3, 1},
		})
		data.calls = nil

		// Read from the middle of already read range, should make no new calls.
		buf = make([]byte, 2)
		n, err = r.ReadAt(buf, 1)
		So(n, ShouldEqual, 2)
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, "12")
		So(data.calls, ShouldHaveLength, 0)

		// Read few bytes more, it should reading new data.
		buf = make([]byte, 5)
		n, err = r.ReadAt(buf, 1)
		So(n, ShouldEqual, 5)
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, "12345")
		So(data.calls, ShouldResemble, []readAtCall{
			{4, 1},
			{5, 1},
		})
		data.calls = nil

		// Hit EOF.
		buf = make([]byte, 11)
		n, err = r.ReadAt(buf, 0)
		So(n, ShouldEqual, 10)
		So(err, ShouldEqual, io.EOF)
		So(string(buf[:10]), ShouldEqual, "0123456789")

		// Try to read past EOF.
		n, err = r.ReadAt(buf, 10)
		So(n, ShouldEqual, 0)
		So(err, ShouldEqual, io.EOF)

		// 0 buffer read just "probes" for EOF.
		n, err = r.ReadAt(nil, 0)
		So(n, ShouldEqual, 0)
		So(err, ShouldBeNil)
		n, err = r.ReadAt(nil, 10)
		So(n, ShouldEqual, 0)
		So(err, ShouldEqual, io.EOF)
	})

	Convey("Big block size", t, func() {
		data := &trackingReaderAt{r: strings.NewReader("0123456789")}
		r := NewBufferingReaderAt(data, 6, 2)

		// Read a small chunk of the first block from the middle.
		buf := make([]byte, 2)
		n, err := r.ReadAt(buf, 2)
		So(n, ShouldEqual, 2)
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, "23")

		So(data.calls, ShouldResemble, []readAtCall{{0, 6}})
		data.calls = nil

		// Read a large chunk of the first block and a bit of the second.
		buf = make([]byte, 7)
		n, err = r.ReadAt(buf, 2)
		So(n, ShouldEqual, 7)
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, "2345678")

		So(data.calls, ShouldResemble, []readAtCall{{6, 6}})
		data.calls = nil

		// Partially read the last chunk.
		n, err = r.ReadAt(buf, 8)
		So(n, ShouldEqual, 2)
		So(err, ShouldEqual, io.EOF)
		So(string(buf[:2]), ShouldEqual, "89")

		// Try to partially read a block past EOF.
		n, err = r.ReadAt(buf, 23)
		So(n, ShouldEqual, 0)
		So(err, ShouldEqual, io.EOF)
	})

	Convey("Handle unexpected errors", t, func() {
		data := &trackingReaderAt{r: strings.NewReader("0123456789")}
		r := NewBufferingReaderAt(data, 5, 2)

		// Simulate partial failed read that returns 4 bytes.
		data.err = fmt.Errorf("boo")
		data.partialRead = 4

		// But read only 3 bytes. It should succeed, we don't care about 4th byte.
		buf := make([]byte, 3)
		n, err := r.ReadAt(buf, 0)
		So(n, ShouldEqual, 3)
		So(err, ShouldBeNil)
		So(string(buf), ShouldEqual, "012")

		// But when reading the full page, it actually returns the error.
		buf = make([]byte, 5)
		n, err = r.ReadAt(buf, 0)
		So(n, ShouldEqual, 4)
		So(err, ShouldEqual, data.err)
		So(string(buf[:4]), ShouldEqual, "0123")
	})
}
