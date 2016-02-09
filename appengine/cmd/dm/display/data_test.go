// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package display

import (
	"testing"

	"github.com/luci/luci-go/appengine/cmd/dm/types"
	"github.com/luci/luci-go/common/clock/testclock"
	. "github.com/smartystreets/goconvey/convey"
)

func TestData(t *testing.T) {
	t.Parallel()

	Convey("Data", t, func() {
		d := &Data{}

		Convey("can merge", func() {
			aid := *types.NewAttemptID("questid|1")

			d2 := &Data{
				DistributorSlice{{"distributor", "url"}},
				QuestSlice{{"questid", "payload", "distributor", testclock.TestTimeUTC}},
				AttemptSlice{{ID: aid}},
				AttemptResultSlice{{aid, "result"}},
				ExecutionsForAttemptSlice{{aid, ExecutionInfoSlice{{1, "dist_token"}}}},
				DepsFromAttemptSlice{{aid, QuestAttemptsSlice{{"otherQuest", types.U32s{1}}}}},
				DepsFromAttemptSlice{{*types.NewAttemptID("otherQuest|1"), QuestAttemptsSlice{{"questid", types.U32s{1}}}}},
				false,
				false,
				false,
			}
			So(d.Merge(d2), ShouldResemble, d2)
			So(d.Merge(d2), ShouldBeNil)
			So(d, ShouldResemble, d2)
		})

	})
}
