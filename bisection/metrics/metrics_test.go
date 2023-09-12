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
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestCollectGlobalMetrics(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	c, _ = tsmon.WithDummyInMemory(c)

	Convey("running compile analyses", t, func() {
		createRunningAnalysis(c, 123, "chromium", model.PlatformLinux)
		createRunningAnalysis(c, 456, "chromeos", model.PlatformLinux)
		createRunningAnalysis(c, 789, "chromium", model.PlatformLinux)
		err := collectMetricsForRunningAnalyses(c)
		So(err, ShouldBeNil)
		So(runningAnalysesGauge.Get(c, "chromium", "compile"), ShouldEqual, 2)
		So(runningAnalysesGauge.Get(c, "chromeos", "compile"), ShouldEqual, 1)

		m, err := retrieveRunningAnalyses(c)
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string]int{
			"chromium": 2,
			"chromeos": 1,
		})
	})

	Convey("running test analyses", t, func() {
		testutil.CreateTestFailureAnalysis(c, &testutil.TestFailureAnalysisCreationOption{
			ID:        1000,
			Project:   "chromium",
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		testutil.CreateTestFailureAnalysis(c, &testutil.TestFailureAnalysisCreationOption{
			ID:        1001,
			Project:   "chromium",
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		testutil.CreateTestFailureAnalysis(c, &testutil.TestFailureAnalysisCreationOption{
			ID:        1002,
			Project:   "chromeos",
			RunStatus: pb.AnalysisRunStatus_STARTED,
		})
		testutil.CreateTestFailureAnalysis(c, &testutil.TestFailureAnalysisCreationOption{
			ID:        1003,
			Project:   "chromeos",
			RunStatus: pb.AnalysisRunStatus_ENDED,
		})
		err := collectMetricsForRunningAnalyses(c)
		So(err, ShouldBeNil)
		So(runningAnalysesGauge.Get(c, "chromium", "test"), ShouldEqual, 2)
		So(runningAnalysesGauge.Get(c, "chromeos", "test"), ShouldEqual, 1)

		m, err := retrieveRunningAnalyses(c)
		So(err, ShouldBeNil)
		So(m, ShouldResemble, map[string]int{
			"chromium": 2,
			"chromeos": 1,
		})
	})

	Convey("For running reruns", t, func() {
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)
		testutil.UpdateIndices(c)

		// Create a rerun for chromium
		cfa1 := createRunningAnalysis(c, 123, "chromium", model.PlatformLinux)

		rrBuild1 := &model.CompileRerunBuild{
			LuciBuild: model.LuciBuild{
				Status: buildbucketpb.Status_STATUS_UNSPECIFIED,
			},
		}
		So(datastore.Put(c, rrBuild1), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		rerun1 := &model.SingleRerun{
			Analysis:   datastore.KeyForObj(c, cfa1),
			RerunBuild: datastore.KeyForObj(c, rrBuild1),
			CreateTime: clock.Now(c).Add(-10 * time.Second),
			Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		}
		So(datastore.Put(c, rerun1), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Create another rerun for chromeos
		cfa2 := createRunningAnalysis(c, 456, "chromeos", model.PlatformMac)

		rrBuild2 := &model.CompileRerunBuild{
			LuciBuild: model.LuciBuild{
				Status: buildbucketpb.Status_STARTED,
			},
		}
		So(datastore.Put(c, rrBuild2), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		rerun2 := &model.SingleRerun{
			Analysis:   datastore.KeyForObj(c, cfa2),
			RerunBuild: datastore.KeyForObj(c, rrBuild2),
			CreateTime: clock.Now(c).Add(time.Minute),
			Status:     pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		}
		So(datastore.Put(c, rerun2), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		err := collectMetricsForRunningReruns(c)
		So(err, ShouldBeNil)
		So(runningRerunGauge.Get(c, "chromium", "pending", "linux", "compile"), ShouldEqual, 1)
		So(runningRerunGauge.Get(c, "chromium", "running", "linux", "compile"), ShouldEqual, 0)
		So(runningRerunGauge.Get(c, "chromeos", "pending", "mac", "compile"), ShouldEqual, 0)
		So(runningRerunGauge.Get(c, "chromeos", "running", "mac", "compile"), ShouldEqual, 1)
		dist := rerunAgeMetric.Get(c, "chromium", "pending", "linux", "compile")
		So(dist.Count(), ShouldEqual, 1)
		dist = rerunAgeMetric.Get(c, "chromeos", "running", "mac", "compile")
		So(dist.Count(), ShouldEqual, 1)
	})

	Convey("running test reruns", t, func() {
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)
		createRerun := func(ID int64, project, OS string, status buildbucketpb.Status) {
			rerun := &model.TestSingleRerun{
				ID:     ID,
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				LUCIBuild: model.LUCIBuild{
					Project:    project,
					Status:     status,
					CreateTime: clock.Now(c).Add(-10 * time.Second),
				},
				Dimensions: &pb.Dimensions{Dimensions: []*pb.Dimension{{
					Key:   "os",
					Value: OS,
				}}},
			}
			So(datastore.Put(c, rerun), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
		}
		createRerun(100, "chromium", "Ubuntu-22.04", buildbucketpb.Status_SCHEDULED)
		createRerun(101, "chromium", "Ubuntu-22.04", buildbucketpb.Status_STARTED)
		createRerun(102, "chromium", "Ubuntu-22.04", buildbucketpb.Status_STARTED)
		createRerun(103, "chromium", "Mac-12", buildbucketpb.Status_STARTED)
		createRerun(104, "chromeos", "Ubuntu-22.04", buildbucketpb.Status_STARTED)

		err := collectMetricsForRunningTestReruns(c)
		So(err, ShouldBeNil)
		So(runningRerunGauge.Get(c, "chromium", "running", "linux", "test"), ShouldEqual, 2)
		So(runningRerunGauge.Get(c, "chromium", "pending", "linux", "test"), ShouldEqual, 1)
		So(runningRerunGauge.Get(c, "chromium", "running", "mac", "test"), ShouldEqual, 1)
		So(runningRerunGauge.Get(c, "chromeos", "running", "linux", "test"), ShouldEqual, 1)
		dist := rerunAgeMetric.Get(c, "chromium", "pending", "linux", "test")
		So(dist.Count(), ShouldEqual, 1)
		dist = rerunAgeMetric.Get(c, "chromeos", "running", "linux", "test")
		So(dist.Count(), ShouldEqual, 1)
	})
}

func createRunningAnalysis(c context.Context, id int64, proj string, platform model.Platform) *model.CompileFailureAnalysis {
	fb := &model.LuciFailedBuild{
		Id: id,
		LuciBuild: model.LuciBuild{
			Project: proj,
		},
		Platform: platform,
	}
	So(datastore.Put(c, fb), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()

	cf := testutil.CreateCompileFailure(c, fb)
	cfa := &model.CompileFailureAnalysis{
		Id:             id,
		CompileFailure: datastore.KeyForObj(c, cf),
		RunStatus:      pb.AnalysisRunStatus_STARTED,
	}
	So(datastore.Put(c, cfa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}
