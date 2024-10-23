// Copyright 2023 The LUCI Authors.
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

package baselines

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestRead(t *testing.T) {
	ftt.Run(`Invalid`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Not Found`, func(t *ftt.Test) {
			_, err := Read(span.Single(ctx), "chromium", "try:linux-rel")
			assert.Loosely(t, err, should.ErrLike(NotFound))
		})

	})

	ftt.Run(`Valid`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		t.Run(`Exists`, func(t *ftt.Test) {
			expected := &Baseline{
				Project:    "chromium",
				BaselineID: "try:linux-rel",
			}
			commitTime := testutil.MustApply(ctx, t, Create(expected.Project, expected.BaselineID))

			res, err := Read(span.Single(ctx), expected.Project, expected.BaselineID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res.Project, should.Equal(expected.Project))
			assert.Loosely(t, res.BaselineID, should.Equal(expected.BaselineID))
			assert.Loosely(t, res.LastUpdatedTime, should.Match(commitTime))
			assert.Loosely(t, res.CreationTime, should.Match(commitTime))
		})
	})
}

func TestIsSpinningUp(t *testing.T) {
	ftt.Run(`IsSpinningUp`, t, func(t *ftt.Test) {
		now := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)

		t.Run(`Yes`, func(t *ftt.Test) {
			b := &Baseline{
				Project:         "chromium",
				BaselineID:      "try:linux-rel",
				LastUpdatedTime: now.Add(-time.Hour * 1),
				CreationTime:    now.Add(-time.Hour * 2),
			}
			assert.Loosely(t, b.IsSpinningUp(now), should.BeTrue)
		})

		t.Run(`No`, func(t *ftt.Test) {
			b := &Baseline{
				Project:         "chromium",
				BaselineID:      "try:linux-rel",
				LastUpdatedTime: now.Add(-time.Hour * 1),
				CreationTime:    now.Add(-time.Hour * 100),
			}
			assert.Loosely(t, b.IsSpinningUp(now), should.BeFalse)
		})
	})
}
