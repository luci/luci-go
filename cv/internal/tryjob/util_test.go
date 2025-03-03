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

package tryjob

import (
	"slices"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestQueryTryjobIDsUpdatedBefore(t *testing.T) {
	t.Parallel()
	ct := cvtesting.Test{}
	ctx := ct.SetUp(t)

	nextBuildID := int64(1)
	createNTryjobs := func(n int) []*Tryjob {
		tryjobs := make([]*Tryjob, n)
		for i := range tryjobs {
			eid := MustBuildbucketID("example.com", nextBuildID)
			nextBuildID++
			tryjobs[i] = eid.MustCreateIfNotExists(ctx)
		}
		return tryjobs
	}

	var allTryjobs []*Tryjob
	allTryjobs = append(allTryjobs, createNTryjobs(1000)...)
	ct.Clock.Add(1 * time.Minute)
	allTryjobs = append(allTryjobs, createNTryjobs(1000)...)
	ct.Clock.Add(1 * time.Minute)
	allTryjobs = append(allTryjobs, createNTryjobs(1000)...)

	before := ct.Clock.Now().Add(-30 * time.Second)
	var expected common.TryjobIDs
	for _, tj := range allTryjobs {
		if tj.EntityUpdateTime.Before(before) {
			expected = append(expected, tj.ID)
		}
	}
	slices.Sort(expected)

	actual, err := QueryTryjobIDsUpdatedBefore(ctx, before)
	assert.NoErr(t, err)
	assert.That(t, actual, should.Match(expected))
}

func TestIsEnded(t *testing.T) {
	t.Parallel()

	knownStatuses := map[Status]comparison.Func[bool]{
		Status_STATUS_UNSPECIFIED: should.BeFalse,
		Status_PENDING:            should.BeFalse,
		Status_TRIGGERED:          should.BeFalse,
		Status_ENDED:              should.BeTrue,
		Status_CANCELLED:          should.BeTrue,
		Status_UNTRIGGERED:        should.BeTrue,
	}

	ftt.Run("IsEnded works", t, func(t *ftt.Test) {
		for st, exp := range knownStatuses {
			assert.That(t, IsEnded(st), exp)
		}
	})

	ftt.Run("This test is aware of all the statuses", t, func(t *ftt.Test) {
		for st := range Status_name {
			assert.Loosely(t, knownStatuses, should.ContainKey(st))
		}
	})
}
