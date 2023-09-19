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
	"go.chromium.org/luci/bisection/util/testutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestUpdateAnalysisStatus(t *testing.T) {
	t.Parallel()

	Convey("UpdateStatus", t, func() {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		Convey("Update status ended", func() {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_RUNNING,
				RunStatus: pb.AnalysisRunStatus_STARTED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(tfa.EndTime.Unix(), ShouldEqual, 10000)
		})

		Convey("Update status started", func() {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_CREATED,
				RunStatus: pb.AnalysisRunStatus_ANALYSIS_RUN_STATUS_UNSPECIFIED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_CREATED, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_CREATED)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)
			So(tfa.StartTime.Unix(), ShouldEqual, 10000)
		})

		Convey("Do not update the start time of a started analysis", func() {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_RUNNING,
				RunStatus: pb.AnalysisRunStatus_STARTED,
				StartTime: time.Unix(5000, 0).UTC(),
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)
			So(tfa.StartTime.Unix(), ShouldEqual, 5000)
		})

		Convey("Ended analysis will not update", func() {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_FOUND,
				RunStatus: pb.AnalysisRunStatus_ENDED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Canceled analysis will not update", func() {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:    pb.AnalysisStatus_NOTFOUND,
				RunStatus: pb.AnalysisRunStatus_CANCELED,
			})
			err := UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_CANCELED)
		})
	})
}

func TestUpdateNthSectionAnalysisStatus(t *testing.T) {
	t.Parallel()

	Convey("Update Nthsection Status", t, func() {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		Convey("Update status ended", func() {
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
				Status:    pb.AnalysisStatus_RUNNING,
				RunStatus: pb.AnalysisRunStatus_STARTED,
			})
			err := UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			So(err, ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(nsa.EndTime.Unix(), ShouldEqual, 10000)
		})

		Convey("Ended analysis will not update", func() {
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
				Status:    pb.AnalysisStatus_FOUND,
				RunStatus: pb.AnalysisRunStatus_ENDED,
			})
			err := UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			So(err, ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Canceled analysis will not update", func() {
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
				Status:    pb.AnalysisStatus_NOTFOUND,
				RunStatus: pb.AnalysisRunStatus_CANCELED,
			})
			err := UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_CANCELED)
		})
	})
}

func TestUpdateStatusWhenError(t *testing.T) {
	t.Parallel()

	Convey("Update Status When Error", t, func() {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			Status:    pb.AnalysisStatus_CREATED,
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			Status:            pb.AnalysisStatus_CREATED,
			RunStatus:         pb.AnalysisRunStatus_STARTED,
		})

		Convey("No in progress rerun", func() {
			err := UpdateAnalysisStatusWhenError(ctx, tfa)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, nsa), ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(nsa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())

			So(datastore.Get(ctx, tfa), ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(tfa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())
		})

		Convey("Have in progress rerun", func() {
			testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
				Type:        model.RerunBuildType_NthSection,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			})
			err := UpdateAnalysisStatusWhenError(ctx, tfa)
			So(err, ShouldBeNil)
			So(datastore.Get(ctx, nsa), ShouldBeNil)
			// No change.
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_CREATED)
			So(datastore.Get(ctx, tfa), ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_CREATED)
		})
	})
}
