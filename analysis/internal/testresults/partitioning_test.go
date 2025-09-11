// Copyright 2025 The LUCI Authors.
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

package testresults

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTimeRangePartitioner(t *testing.T) {
	t.Parallel()

	getPartitions := func(tr TimeRange, days int) []TimeRange {
		p := tr.Partition()
		var gotPartitions []TimeRange
		for {
			partition, ok := p.Next(days)
			if !ok {
				break
			}
			gotPartitions = append(gotPartitions, partition)
			if len(gotPartitions) > 100 {
				panic("too many partitions produced")
			}
		}
		return gotPartitions
	}

	ftt.Run("Full days", t, func(t *ftt.Test) {
		tr := TimeRange{
			Latest:   time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
			Earliest: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		}

		expectedPartitions := []TimeRange{
			{
				Latest:   time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
			},
			{
				Latest:   time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			},
		}
		assert.That(t, getPartitions(tr, 2), should.Match(expectedPartitions))
	})
	ftt.Run("Partial days", t, func(t *ftt.Test) {
		tr := TimeRange{
			Latest:   time.Date(2024, 1, 5, 18, 2, 0, 0, time.UTC),
			Earliest: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC),
		}

		expectedPartitions := []TimeRange{
			{
				Latest:   time.Date(2024, 1, 5, 18, 2, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
			},
			{
				Latest:   time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			{
				Latest:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC),
			},
		}
		assert.That(t, getPartitions(tr, 2), should.Match(expectedPartitions))
	})
	ftt.Run("Partial days (non-UTC)", t, func(t *ftt.Test) {
		tzSydney, _ := time.LoadLocation("Australia/Sydney") // UTC+11
		tr := TimeRange{
			Latest:   time.Date(2024, 1, 5, 18, 2, 0, 0, tzSydney), // This still falls on the same day in UTC.
			Earliest: time.Date(2024, 1, 1, 12, 1, 0, 0, tzSydney), // This still falls on the same day in UTC.
		}

		expectedPartitions := []TimeRange{
			{
				Latest:   time.Date(2024, 1, 5, 7, 2, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
			},
			{
				Latest:   time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
			},
			{
				Latest:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
				Earliest: time.Date(2024, 1, 1, 1, 1, 0, 0, time.UTC),
			},
		}
		assert.That(t, getPartitions(tr, 2), should.Match(expectedPartitions))
	})
}
