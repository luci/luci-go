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

package chunkstream

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBuffer(t *testing.T) {
	ftt.Run(`An empty Buffer instance`, t, func(t *ftt.Test) {
		b := Buffer{}

		t.Run(`Has a length of zero.`, func(t *ftt.Test) {
			assert.Loosely(t, b.Len(), should.BeZero)
		})

		t.Run(`Has a FirstChunk of nil.`, func(t *ftt.Test) {
			assert.Loosely(t, b.FirstChunk(), should.BeNil)
		})

		t.Run(`Will panic if more than zero bytes are consumed.`, func(t *ftt.Test) {
			assert.Loosely(t, func() { b.Consume(1) }, should.Panic)
		})

		t.Run(`When Appending an empty chunk`, func(t *ftt.Test) {
			c := tc()
			b.Append(c)

			t.Run(`Has a FirstChunk of nil.`, func(t *ftt.Test) {
				assert.Loosely(t, b.FirstChunk(), should.BeNil)
			})

			t.Run(`The Chunk is released.`, func(t *ftt.Test) {
				assert.Loosely(t, c.released, should.BeTrue)
			})
		})

		for _, chunks := range [][]*testChunk{
			{},
			{tc()},
			{tc(0, 1, 2), tc(), tc(3, 4, 5)},
		} {
			t.Run(fmt.Sprintf(`With chunks %v, can append.`, chunks), func(t *ftt.Test) {
				size := int64(0)
				coalesced := []byte(nil)
				for _, c := range chunks {
					b.Append(c)
					coalesced = append(coalesced, c.Bytes()...)
					size += int64(c.Len())
					assert.Loosely(t, b.Len(), should.Equal(size))
				}
				assert.Loosely(t, b.Bytes(), should.Resemble(coalesced))

				t.Run(`Can consume chunk-at-a-time.`, func(t *ftt.Test) {
					for i, c := range chunks {
						if c.Len() > 0 {
							assert.Loosely(t, b.FirstChunk(), should.Equal(chunks[i]))
						}
						assert.Loosely(t, b.Len(), should.Equal(size))
						b.Consume(int64(c.Len()))
						size -= int64(c.Len())
					}
					assert.Loosely(t, b.Len(), should.BeZero)

					t.Run(`All chunks are released.`, func(t *ftt.Test) {
						for _, c := range chunks {
							assert.Loosely(t, c.released, should.BeTrue)
						}
					})
				})

				t.Run(`Can consume byte-at-a-time.`, func(t *ftt.Test) {
					for i := int64(0); i < size; i++ {
						assert.Loosely(t, b.Len(), should.Equal((size - i)))
						assert.Loosely(t, b.Bytes(), should.Resemble(coalesced[i:]))
						b.Consume(1)
					}
					assert.Loosely(t, b.Len(), should.BeZero)

					t.Run(`All chunks are released.`, func(t *ftt.Test) {
						for _, c := range chunks {
							assert.Loosely(t, c.released, should.BeTrue)
						}
					})
				})

				t.Run(`Can consume two bytes at a time.`, func(t *ftt.Test) {
					for i := int64(0); i < size; i += 2 {
						// Final byte(s), make sure we don't over-consume.
						if b.Len() < 2 {
							i = b.Len()
						}

						assert.Loosely(t, b.Len(), should.Equal((size - i)))
						assert.Loosely(t, b.Bytes(), should.Resemble(coalesced[i:]))
						b.Consume(2)
					}
					assert.Loosely(t, b.Len(), should.BeZero)

					t.Run(`All chunks are released.`, func(t *ftt.Test) {
						for _, c := range chunks {
							assert.Loosely(t, c.released, should.BeTrue)
						}
					})
				})

				t.Run(`Can consume all at once.`, func(t *ftt.Test) {
					b.Consume(size)
					assert.Loosely(t, b.Len(), should.BeZero)

					t.Run(`All chunks are released.`, func(t *ftt.Test) {
						for _, c := range chunks {
							assert.Loosely(t, c.released, should.BeTrue)
						}
					})
				})
			})
		}
	})
}
