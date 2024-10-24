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

package sweep

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

func TestHelpers(t *testing.T) {
	t.Parallel()

	const keySpaceBytes = 16

	ftt.Run("OnlyLeased", t, func(t *ftt.Test) {
		reminders := []*reminder.Reminder{
			// Each key be exactly 2*keySpaceBytes chars long.
			{ID: "00000000000000000000000000000001"},
			{ID: "00000000000000000000000000000005"},
			{ID: "00000000000000000000000000000009"},
			{ID: "0000000000000000000000000000000f"}, // ie 15
		}
		leased := partition.SortedPartitions{partition.FromInts(5, 9)}
		assert.Loosely(t, onlyLeased(reminders, leased, keySpaceBytes), should.Resemble([]*reminder.Reminder{
			{ID: "00000000000000000000000000000005"},
		}))
	})
}
