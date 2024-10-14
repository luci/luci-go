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
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDataPool(t *testing.T) {
	ftt.Run(`A data pool registry`, t, func(t *ftt.Test) {
		reg := dataPoolRegistry{}

		t.Run(`Can retrieve a 1024-byte data pool.`, func(t *ftt.Test) {
			pool := reg.getPool(1024)
			assert.Loosely(t, pool.size, should.Equal(1024))

			t.Run(`Subsequent requests return the same pool.`, func(t *ftt.Test) {
				assert.Loosely(t, reg.getPool(1024), should.Equal(pool))
			})
		})
	})
}

func TestData(t *testing.T) {
	ftt.Run(`A 512-byte data pool`, t, func(t *ftt.Test) {
		pool := (&dataPoolRegistry{}).getPool(512)

		t.Run(`Will allocate a clean 512-byte Data`, func(t *ftt.Test) {
			d := pool.getData().(*streamData)
			defer d.Release()

			assert.Loosely(t, d, should.NotBeNil)
			assert.Loosely(t, len(d.Bytes()), should.Equal(512))
			assert.Loosely(t, cap(d.Bytes()), should.Equal(512))

			t.Run(`When bound, adjusts its byte size and retains a timestamp.`, func(t *ftt.Test) {
				now := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
				data := [64]byte{}
				for i := range data {
					data[i] = byte(i)
				}

				copy(d.Bytes(), data[:])
				assert.Loosely(t, d.Bind(len(data), now), should.Equal(d))
				assert.Loosely(t, d.Bytes(), should.Resemble(data[:]))
				assert.Loosely(t, d.Len(), should.Equal(len(data)))
				assert.Loosely(t, d.Timestamp().Equal(now), should.BeTrue)
			})
		})
	})
}
