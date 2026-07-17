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

package decompress

import (
	"bytes"
	"testing"

	"github.com/klauspost/compress/gzip"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGzip(t *testing.T) {
	t.Parallel()

	ftt.Run("Gzip", t, func(t *ftt.Test) {
		t.Run("valid gzip", func(t *ftt.Test) {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			_, err := gw.Write([]byte("hello world"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, gw.Close(), should.BeNil)

			decompressed, err := Gzip(buf.Bytes())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, string(decompressed), should.Equal("hello world"))
		})

		t.Run("invalid gzip", func(t *ftt.Test) {
			_, err := Gzip([]byte("not gzip"))
			assert.Loosely(t, err, should.ErrLike("failed to create gzip reader"))
		})

		t.Run("exceeds max size", func(t *ftt.Test) {
			var buf bytes.Buffer
			gw := gzip.NewWriter(&buf)
			chunk := make([]byte, 1024*1024)
			for i := 0; i <= ConfigMaxSize/(1024*1024); i++ {
				_, err := gw.Write(chunk)
				assert.Loosely(t, err, should.BeNil)
			}
			assert.Loosely(t, gw.Close(), should.BeNil)

			_, err := Gzip(buf.Bytes())
			assert.Loosely(t, err, should.ErrLike("decompressed data exceeds max size"))
		})
	})
}
