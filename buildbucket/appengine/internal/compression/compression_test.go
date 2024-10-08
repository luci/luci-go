// Copyright 2022 The LUCI Authors.
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

package compression

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestCompression(t *testing.T) {
	t.Parallel()

	ftt.Run("zlib", t, func(t *ftt.Test) {
		raw := []byte("abc")
		compressed, err := ZlibCompress(raw)
		assert.Loosely(t, err, should.BeNil)
		decompressed, err := ZlibDecompress(compressed)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(decompressed), should.Equal("abc"))
	})

	ftt.Run("zstd", t, func(t *ftt.Test) {
		raw := []byte("abc")
		compressed := make([]byte, 0, len(raw))
		compressed = ZstdCompress(raw, compressed)
		decompressed, err := ZstdDecompress(compressed, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(decompressed), should.Equal("abc"))
	})
}
