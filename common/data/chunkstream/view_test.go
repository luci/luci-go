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
	"bytes"
	"fmt"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestView(t *testing.T) {
	ftt.Run(`An empty Buffer with generation 42`, t, func(t *ftt.Test) {
		b := Buffer{}

		for _, chunks := range [][]*testChunk{
			[]*testChunk(nil),
			{tc()},
			{tc(0)},
			{tc(0, 0, 1)},
			{tc(0), tc(0), tc(0)},
			{tc(1, 2, 3, 0)},
			{tc(1, 2), tc(3, 0, 4, 0), tc(0, 5)},
		} {
			t.Run(fmt.Sprintf(`With Chunks %v`, chunks), func(t *ftt.Test) {
				aggregate := []byte{}
				size := int64(0)
				for _, c := range chunks {
					aggregate = append(aggregate, c.Bytes()...)
					size += int64(c.Len())
					b.Append(c)
				}

				// This is pretty stupid to do, but for testing it's a nice edge case
				// to hammer.
				t.Run(`A View with limit 0`, func(t *ftt.Test) {
					br := b.ViewLimit(0)

					assert.Loosely(t, br.Remaining(), should.BeZero)
					assert.Loosely(t, br.Consumed(), should.BeZero)

					t.Run(`Read() returns EOF.`, func(t *ftt.Test) {
						buf := make([]byte, 16)
						a, err := br.Read(buf)
						assert.Loosely(t, err, should.Equal(io.EOF))
						assert.Loosely(t, a, should.BeZero)
					})

					t.Run(`ReadByte() returns EOF.`, func(t *ftt.Test) {
						_, err := br.ReadByte()
						assert.Loosely(t, err, should.Equal(io.EOF))
					})

					t.Run(`Skip() panics.`, func(t *ftt.Test) {
						assert.Loosely(t, func() { br.Skip(1) }, should.Panic)
					})

					t.Run(`Index() returns -1.`, func(t *ftt.Test) {
						assert.Loosely(t, br.Index([]byte{0}), should.Equal(-1))
					})
				})

				t.Run(`An unlimited View`, func(t *ftt.Test) {
					br := b.View()
					assert.Loosely(t, br.Remaining(), should.Equal(b.Len()))
					assert.Loosely(t, br.Consumed(), should.BeZero)

					t.Run(`Can Read() the full block of data.`, func(t *ftt.Test) {
						buf := make([]byte, len(aggregate))
						amt, err := br.Read(buf)
						assert.Loosely(t, amt, should.Equal(len(aggregate)))
						assert.Loosely(t, err, should.Equal(io.EOF))
						assert.Loosely(t, buf[:amt], should.Match(aggregate))

						t.Run(`Subsequent Read() will return io.EOF.`, func(t *ftt.Test) {
							amt, err := br.Read(buf)
							assert.Loosely(t, amt, should.BeZero)
							assert.Loosely(t, err, should.Equal(io.EOF))
						})
					})

					t.Run(`Can Read() the full block of data byte-by-byte.`, func(t *ftt.Test) {
						buf := make([]byte, 1)
						for i, d := range aggregate {
							amt, err := br.Read(buf)
							if i == len(aggregate)-1 {
								assert.Loosely(t, err, should.Equal(io.EOF))
							} else {
								assert.Loosely(t, err, should.BeNil)
							}

							assert.Loosely(t, amt, should.Equal(1))
							assert.Loosely(t, buf[0], should.Equal(d))
						}

						t.Run(`Subsequent Read() will return io.EOF.`, func(t *ftt.Test) {
							amt, err := br.Read(buf)
							assert.Loosely(t, amt, should.BeZero)
							assert.Loosely(t, err, should.Equal(io.EOF))
						})
					})

					t.Run(`Can ReadByte() the full block of data.`, func(t *ftt.Test) {
						for _, d := range aggregate {
							b, err := br.ReadByte()
							assert.Loosely(t, err, should.BeNil)
							assert.Loosely(t, b, should.Equal(d))
						}

						t.Run(`Subsequent ReadByte() will return io.EOF.`, func(t *ftt.Test) {
							_, err := br.ReadByte()
							assert.Loosely(t, err, should.Equal(io.EOF))
						})
					})

					for _, needle := range [][]byte{
						{0},
						{0, 0},
						{0, 0, 0, 0},
					} {
						expected := bytes.Index(aggregate, needle)
						t.Run(fmt.Sprintf(`Index() of %v returns %v.`, needle, expected), func(t *ftt.Test) {
							assert.Loosely(t, br.Index(needle), should.Equal(expected))
						})
					}
				})
			})
		}

		t.Run(`An unlimited View`, func(t *ftt.Test) {
			br := b.View()

			t.Run(`Has chunksRemaining() value of 0.`, func(t *ftt.Test) {
				assert.Loosely(t, br.chunkRemaining(), should.BeZero)
			})
		})

		t.Run(`With chunks [{0x01, 0x02, 0x00}, {0x00, 0x03}, {0x00}, {0x00, 0x00}, {0x04}]`, func(t *ftt.Test) {
			for _, c := range []Chunk{tc(1, 2, 0), tc(0, 3), tc(0), tc(0, 0), tc(4)} {
				b.Append(c)
			}

			t.Run(`An unlimited View`, func(t *ftt.Test) {
				br := b.View()

				t.Run(`Should have Remaining() value of 9.`, func(t *ftt.Test) {
					assert.Loosely(t, br.Remaining(), should.Equal(9))
				})

				t.Run(`Can spawn a limited clone.`, func(t *ftt.Test) {
					buf := bytes.Buffer{}
					_, err := buf.ReadFrom(br.CloneLimit(7))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, buf.Bytes(), should.Match([]byte{0x01, 0x02, 0x00, 0x00, 0x03, 0x00, 0x00}))
				})

				for _, s := range []struct {
					needle []byte
					index  int64
				}{
					{[]byte(nil), 0},
					{[]byte{0x01}, 0},
					{[]byte{0x01, 0x02}, 0},
					{[]byte{0x02, 0x00}, 1},
					{[]byte{0x00}, 2},
					{[]byte{0x00, 0x00}, 2},
					{[]byte{0x00, 0x00, 0x00}, 5},
					{[]byte{0x03, 0x00, 0x00}, 4},
				} {
					t.Run(fmt.Sprintf(`Has Index %v for needle %v`, s.index, s.needle), func(t *ftt.Test) {
						assert.Loosely(t, br.Index(s.needle), should.Equal(s.index))
					})
				}
			})

			t.Run(`A View with a limit of 6`, func(t *ftt.Test) {
				br := b.ViewLimit(6)

				t.Run(`Should have Remaining() value of 6.`, func(t *ftt.Test) {
					assert.Loosely(t, br.Remaining(), should.Equal(6))
				})

				t.Run(`Has index of -1 for needle [0x00, 0x04]`, func(t *ftt.Test) {
					assert.Loosely(t, br.Index([]byte{0x00, 0x04}), should.Equal(-1))
				})
			})

			t.Run(`A View with a limit of 20`, func(t *ftt.Test) {
				br := b.ViewLimit(20)

				t.Run(`Should have Remaining() value of 9.`, func(t *ftt.Test) {
					assert.Loosely(t, br.Remaining(), should.Equal(9))
				})
			})
		})

		t.Run(`With chunks [{0x0F}..{0x00}]`, func(t *ftt.Test) {
			for i := 0x0F; i >= 0x00; i-- {
				b.Append(tc(byte(i)))
			}
			br := b.View()

			t.Run(`Has index of -1 for needle [0x0f, 0x10]`, func(t *ftt.Test) {
				assert.Loosely(t, br.Index([]byte{0x0f, 0x10}), should.Equal(-1))
			})

			for _, s := range []struct {
				needle []byte
				index  int64
			}{
				{[]byte{0x0F}, 0},
				{[]byte{0x04, 0x03, 0x02}, 11},
				{[]byte{0x01, 0x00}, 14},
				{[]byte{0x00}, 15},
				{[]byte{0x00, 0xFF}, -1},
			} {
				t.Run(fmt.Sprintf(`Has Index %v for needle %v`, s.index, s.needle), func(t *ftt.Test) {
					assert.Loosely(t, br.Index(s.needle), should.Equal(s.index))
				})
			}
		})
	})
}
