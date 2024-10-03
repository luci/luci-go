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

package metrics

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMetrics(t *testing.T) {
	ftt.Run(`Metrics`, t, func(t *ftt.Test) {
		t.Run(`Metrics have unique sort orders`, func(t *ftt.Test) {
			usedSortOrders := make(map[int]bool)
			for _, m := range ComputedMetrics {
				unique := !usedSortOrders[m.DefaultConfig.SortPriority]
				assert.Loosely(t, unique, should.BeTrue)
				usedSortOrders[m.DefaultConfig.SortPriority] = true
			}
		})
		t.Run(`Metrics have unique IDs`, func(t *ftt.Test) {
			usedIDs := make(map[string]bool)
			for _, m := range ComputedMetrics {
				unique := !usedIDs[m.ID.String()]
				assert.Loosely(t, unique, should.BeTrue)
				usedIDs[m.ID.String()] = true
			}
		})
	})
}
