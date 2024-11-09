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

package bundler

import (
	"fmt"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// A proto.Message implementation with test fields.
type testMessage struct {
	U64 uint64 `protobuf:"varint,1,opt,name=u64"`
}

func (t *testMessage) Reset()         {}
func (t *testMessage) String() string { return "" }
func (t *testMessage) ProtoMessage()  {}

func TestFastSizerVarintLength(t *testing.T) {
	ftt.Run(`A test message`, t, func(t *ftt.Test) {
		for _, threshold := range []uint64{
			0,
			0x80,
			0x4000,
			0x200000,
			0x100000000,
			0x800000000,
			0x40000000000,
			0x2000000000000,
			0x100000000000000,
			0x8000000000000000,
		} {

			for _, delta := range []int64{
				-2,
				-1,
				0,
				1,
				2,
			} {
				// Add "delta" to "threshold" in a uint64-aware manner.
				u64 := threshold
				if delta >= 0 {
					u64 += uint64(delta)
				} else {
					if u64 < uint64(-delta) {
						continue
					}
					u64 -= uint64(-delta)
				}

				expected := varintLength(u64)
				t.Run(fmt.Sprintf(`Testing threshold 0x%x should encode to varint size %d`, u64, expected), func(t *ftt.Test) {
					m := &testMessage{
						U64: u64,
					}

					expectedSize := proto.Size(m)
					if u64 == 0 {
						// Proto3 doesn't encode default values (0), so the expected size of
						// the number zero is zero.
						expectedSize = 0
					} else {
						// Accommodate the tag ("1").
						expectedSize -= varintLength(1)
					}
					assert.Loosely(t, expected, should.Equal(expectedSize))
				})
			}
		}
	})

	ftt.Run(`Calculates protobuf size.`, t, func(t *ftt.Test) {
		pbuf := &testMessage{
			U64: 0x600dd065,
		}

		assert.Loosely(t, protoSize(pbuf), should.Equal(proto.Size(pbuf)))
	})
}
