// Copyright 2020 The LUCI Authors.
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

package common

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
)

func TestOffset(t *testing.T) {
	t.Parallel()

	ftt.Run("DistributeOffset forms uniformish distribution", t, func(t *ftt.Test) {
		testIntervalOf100x := func(d time.Duration) {
			t.Run((100 * d).String(), func(t *ftt.Test) {
				offsets := make([]time.Duration, 101)
				for i := 0; i < 101; i++ {
					cs := config.MustProjectSet(fmt.Sprintf("example-%d", i))
					offsets[i] = DistributeOffset(100*d, "config_set", string(cs))
				}
				sort.Slice(offsets, func(i, j int) bool { return offsets[i] < offsets[j] })
				assert.Loosely(t, offsets[0], should.BeGreaterThanOrEqual(time.Duration(0)))
				for i, o := range offsets {
					min := time.Duration(i-10) * d
					max := time.Duration(i+10) * d
					assert.Loosely(t, o, should.BeBetweenOrEqual(min, max))
				}
				assert.Loosely(t, offsets[100], should.BeLessThan(100*d))
			})
		}

		testIntervalOf100x(time.Nanosecond)
		testIntervalOf100x(time.Millisecond)
		testIntervalOf100x(10 * time.Millisecond)
		testIntervalOf100x(100 * time.Millisecond)
		testIntervalOf100x(time.Second)
		testIntervalOf100x(time.Minute)
		testIntervalOf100x(time.Hour)
		testIntervalOf100x(7 * 24 * time.Hour)
	})
}
