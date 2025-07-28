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

package recordio

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFrameHeaderSize(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing FrameHeaderSize`, t, func(t *ftt.Test) {
		t.Run(`Matches actual written frame size`, func(t *ftt.Test) {
			prev := -1
			for i := range 3 {
				base := 1 << uint64(7*i)
				for _, delta := range []int{-1, 0, 1} {
					base += delta
					if base <= prev {
						// Over/underflow, skip.
						continue
					}
					prev = base

					t.Run(fmt.Sprintf(`Frame size %d.`, base), func(t *ftt.Test) {
						data := bytes.Repeat([]byte{0x55}, int(base))

						amt, err := WriteFrame(io.Discard, data)
						if err != nil {
							panic(err)
						}

						assert.Loosely(t, amt-len(data), should.Equal(FrameHeaderSize(int64(len(data)))))
					})
				}
			}
		})

		t.Run(`Matches written frame header size (no alloc)`, func(t *ftt.Test) {
			prev, first := int64(0), true
			for i := range 9 {
				for _, delta := range []int64{-1, 0, 1} {
					base := int64(1<<uint64(7*i)) + delta
					if (!first) && base <= prev {
						// Repeated value, skip.
						continue
					}
					prev, first = base, false

					t.Run(fmt.Sprintf(`Frame size %d.`, base), func(t *ftt.Test) {
						amt, err := writeFrameHeader(io.Discard, uint64(base))
						if err != nil {
							panic(err)
						}

						assert.Loosely(t, amt, should.Equal(FrameHeaderSize(int64(base))))
					})
				}
			}
		})
	})
}
