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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type testChunk struct {
	data     []byte
	released bool
}

var _ Chunk = (*testChunk)(nil)

func tc(d ...byte) *testChunk {
	return &testChunk{
		data: d,
	}
}

func (c *testChunk) String() string {
	pieces := make([]string, len(c.data))
	for i, d := range c.data {
		pieces[i] = fmt.Sprintf("0x%02x", d)
	}
	return fmt.Sprintf("{%s}", strings.Join(pieces, ", "))
}

func (c *testChunk) Bytes() []byte {
	return c.data
}

func (c *testChunk) Len() int {
	return len(c.data)
}

func (c *testChunk) Release() {
	if c.released {
		panic("double-free")
	}
	c.released = true
}

func TestChunkNode(t *testing.T) {
	ftt.Run(`A chunkNode wrapping a testing Chunk implementation`, t, func(t *ftt.Test) {
		c := tc(0, 1, 2)
		n := newChunkNode(c)

		t.Run(`Should call Chunk methods.`, func(t *ftt.Test) {
			assert.Loosely(t, n.Bytes(), should.Resemble([]byte{0, 1, 2}))
		})

		t.Run(`When released, releases the wrapped Chunk.`, func(t *ftt.Test) {
			n.release()
			assert.Loosely(t, c.released, should.BeTrue)

			t.Run(`If released again, panics.`, func(t *ftt.Test) {
				assert.Loosely(t, func() { n.release() }, should.Panic)
			})
		})
	})
}
