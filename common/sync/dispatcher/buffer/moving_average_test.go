// Copyright 2019 The LUCI Authors.
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

package buffer

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMovingAverage(t *testing.T) {
	ftt.Run(`movingAverage`, t, func(t *ftt.Test) {

		t.Run(`construction`, func(t *ftt.Test) {
			t.Run(`panics on bad window`, func(t *ftt.Test) {
				assert.Loosely(t, func() {
					newMovingAverage(0, 100)
				}, should.PanicLike("window must be"))
			})
		})

		t.Run(`usage`, func(t *ftt.Test) {
			ma := newMovingAverage(10, 17)

			t.Run(`avg matches seed`, func(t *ftt.Test) {
				assert.Loosely(t, ma.get(), should.Equal(17.0))
			})

			t.Run(`adding new data changes the average`, func(t *ftt.Test) {
				ma.record(100)
				assert.Loosely(t, ma.get(), should.Equal(25.3))
			})

			t.Run(`adding a lot of data is fine`, func(t *ftt.Test) {
				for range 100 {
					ma.record(100)
				}
				assert.Loosely(t, ma.get(), should.Equal(100.0))
			})
		})

	})
}
