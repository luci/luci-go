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

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestRead(t *testing.T) {
	Convey(`Invalid`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Not Found`, func() {
			_, err := Read(span.Single(ctx), "chromium", "try:linux-rel")
			So(err, ShouldErrLike, NotFound)
		})

	})

	Convey(`Valid`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		Convey(`Exists`, func() {
			expected := &Baseline{
				Project:    "chromium",
				BaselineID: "try:linux-rel",
			}
			commitTime := testutil.MustApply(ctx, Create(expected.Project, expected.BaselineID))

			res, err := Read(span.Single(ctx), expected.Project, expected.BaselineID)
			So(err, ShouldBeNil)
			So(res.Project, ShouldEqual, expected.Project)
			So(res.BaselineID, ShouldEqual, expected.BaselineID)
			So(res.LastUpdatedTime, ShouldEqual, commitTime)
			So(res.CreationTime, ShouldEqual, commitTime)
		})
	})
}

func TestIsSpinningUp(t *testing.T) {
	Convey(`IsSpinningUp`, t, func() {
		now := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)

		Convey(`Yes`, func() {
			b := &Baseline{
				Project:         "chromium",
				BaselineID:      "try:linux-rel",
				LastUpdatedTime: now.Add(-time.Hour * 1),
				CreationTime:    now.Add(-time.Hour * 2),
			}
			So(b.IsSpinningUp(now), ShouldBeTrue)
		})

		Convey(`No`, func() {
			b := &Baseline{
				Project:         "chromium",
				BaselineID:      "try:linux-rel",
				LastUpdatedTime: now.Add(-time.Hour * 1),
				CreationTime:    now.Add(-time.Hour * 100),
			}
			So(b.IsSpinningUp(now), ShouldBeFalse)
		})
	})
}
