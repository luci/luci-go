// Copyright 2021 The LUCI Authors.
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

package pkg

import (
	"bytes"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"io"
	"testing"
)

func TestReadSeekerSource(t *testing.T) {
	t.Parallel()

	ftt.Run("With buffer", t, func(t *ftt.Test) {
		src, err := NewReadSeekerSource(bytes.NewReader([]byte("12345")))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, src.Size(), should.Equal(5))

		buf := make([]byte, 2)

		t.Run("Sequential reads", func(t *ftt.Test) {
			n, err := src.ReadAt(buf, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
			assert.Loosely(t, buf, should.Resemble([]byte("12")))

			n, err = src.ReadAt(buf, 2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
			assert.Loosely(t, buf, should.Resemble([]byte("34")))

			n, err = src.ReadAt(buf, 4)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))
			assert.Loosely(t, buf[:1], should.Resemble([]byte("5")))

			n, err = src.ReadAt(buf, 5)
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, n, should.BeZero)
		})

		t.Run("Random reads", func(t *ftt.Test) {
			n, err := src.ReadAt(buf, 4)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(1))
			assert.Loosely(t, buf[:1], should.Resemble([]byte("5")))

			n, err = src.ReadAt(buf, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
			assert.Loosely(t, buf, should.Resemble([]byte("12")))

			// Again.
			n, err = src.ReadAt(buf, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, n, should.Equal(2))
			assert.Loosely(t, buf, should.Resemble([]byte("12")))

			n, err = src.ReadAt(buf, 5)
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, n, should.BeZero)

			n, err = src.ReadAt(buf, 6)
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, n, should.BeZero)
		})

		t.Run("Negative offset", func(t *ftt.Test) {
			n, err := src.ReadAt(buf, -1)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, n, should.BeZero)
		})
	})
}
