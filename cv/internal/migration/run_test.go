// Copyright 2021 The LUCI Authors.
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

package migration

import (
	"testing"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFinalizeRun(t *testing.T) {
	t.Parallel()
	Convey("FinalizeRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		rid := common.RunID("chromium/111-2-deadbeef")

		Convey("Error when no finishedRun found", func() {
			So(FinalizeRun(ctx, &run.Run{ID: rid}), ShouldErrLike, "Run \"chromium/111-2-deadbeef\" hasn't been reported finished by CQDaemon")
		})

		Convey("Update Run with finshedRun", func() {
			endTime := clock.Now(ctx).UTC()
			So(datastore.Put(ctx, &finishedRun{
				ID:      rid,
				Status:  run.Status_FAILED,
				EndTime: endTime,
			}), ShouldBeNil)
			r := &run.Run{ID: rid}
			err := FinalizeRun(ctx, r)
			So(err, ShouldBeNil)
			So(*r, ShouldResemble, run.Run{
				ID:      rid,
				Status:  run.Status_FAILED,
				EndTime: endTime,
			})
		})
	})
}
