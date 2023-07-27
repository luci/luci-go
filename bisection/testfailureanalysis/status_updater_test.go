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

package testfailureanalysis

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestUpdateStatus(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)

	Convey("UpdateStatus", t, func() {
		Convey("Update status ended", func() {
			tfa := createAnalysisModelWithStatus(ctx, 1000, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			err := UpdateStatus(ctx, tfa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(tfa.EndTime.Unix(), ShouldEqual, 10000)
		})

		Convey("Update status started", func() {
			tfa := createAnalysisModelWithStatus(ctx, 1001, pb.AnalysisStatus_CREATED, pb.AnalysisRunStatus_ANALYSIS_RUN_STATUS_UNSPECIFIED)
			err := UpdateStatus(ctx, tfa, pb.AnalysisStatus_CREATED, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_CREATED)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)
			So(tfa.StartTime.Unix(), ShouldEqual, 10000)
		})

		Convey("Ended analysis will not update", func() {
			tfa := createAnalysisModelWithStatus(ctx, 1002, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			err := UpdateStatus(ctx, tfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Canceled analysis will not update", func() {
			tfa := createAnalysisModelWithStatus(ctx, 1003, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			err := UpdateStatus(ctx, tfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_CANCELED)
		})

	})
}

func createAnalysisModelWithStatus(c context.Context, id int64, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.TestFailureAnalysis {
	tfa := &model.TestFailureAnalysis{
		ID:        id,
		Status:    status,
		RunStatus: runStatus,
	}
	So(datastore.Put(c, tfa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return tfa
}
