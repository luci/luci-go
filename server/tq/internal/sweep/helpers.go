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
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

// onlyLeased shrinks the given slice of Reminders sorted by their ID to contain
// only ones that belong to any of the leased partitions.
func onlyLeased(sorted []*reminder.Reminder, leased partition.SortedPartitions, keySpaceBytes int) []*reminder.Reminder {
	reuse := sorted[:]
	l := 0
	keyOf := func(i int) string {
		return sorted[i].ID
	}
	use := func(i, j int) {
		l += copy(reuse[l:], sorted[i:j])
	}
	leased.OnlyIn(len(sorted), keyOf, use, keySpaceBytes)
	return reuse[:l]
}
