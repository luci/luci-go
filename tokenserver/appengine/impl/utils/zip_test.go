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

package utils

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestZip(t *testing.T) {
	ftt.Run("ZlibCompress/ZlibDecompress roundtrip", t, func(t *ftt.Test) {
		data := "blah-blah"
		for i := 0; i < 10; i++ {
			data += data
		}

		blob, err := ZlibCompress([]byte(data))
		assert.Loosely(t, err, should.BeNil)

		back, err := ZlibDecompress(blob)
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, back, should.Resemble([]byte(data)))
	})

	ftt.Run("ZlibDecompress garbage", t, func(t *ftt.Test) {
		_, err := ZlibDecompress([]byte("garbage"))
		assert.Loosely(t, err, should.NotBeNil)
	})
}
