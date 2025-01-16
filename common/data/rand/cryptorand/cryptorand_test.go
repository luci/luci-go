// Copyright 2016 The LUCI Authors.
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

package cryptorand

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCryptoRand(t *testing.T) {
	t.Parallel()

	ftt.Run("test unmocked real rand", t, func(t *ftt.Test) {
		// Read bytes from real rand, make sure we don't crash.
		n, err := Read(context.Background(), make([]byte, 16))
		assert.Loosely(t, n, should.Equal(16))
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("test mocked rand", t, func(t *ftt.Test) {
		ctx := MockForTest(context.Background(), 0)
		buf := make([]byte, 16)
		n, err := Read(ctx, buf)
		assert.Loosely(t, n, should.Equal(16))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buf, should.Match([]byte{0xfa, 0x12, 0xf9, 0x2a, 0xfb, 0xe0, 0xf,
			0x85, 0x8, 0xd0, 0xe8, 0x3b, 0xab, 0x9c, 0xf8, 0xce}))
	})

	ftt.Run("test mocked rand with reader", t, func(t *ftt.Test) {
		ov := &oneValue{v: "always the same value"}
		ctx := MockForTestWithIOReader(context.Background(), ov)
		buf := make([]byte, 21)
		n, err := Read(ctx, buf)
		assert.That(t, n, should.Equal(len(ov.v)))
		assert.NoErr(t, err)
		assert.That(t, buf, should.Match([]byte("always the same value")))

		newBuf := make([]byte, 21)
		_, err = Read(ctx, newBuf)
		assert.NoErr(t, err)
		assert.That(t, newBuf, should.Match([]byte("always the same value")))
	})
}

type oneValue struct {
	v string
}

func (o *oneValue) Read(p []byte) (n int, err error) {
	for i := range p {
		p[i] = byte(o.v[i])
	}
	return len(p), nil
}
