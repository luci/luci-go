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

package throttle

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	_ "go.chromium.org/luci/bisection/testfailuredetection"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/testutil"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

func TestCronHandler(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	baseTime := time.Unix(10000, 0).UTC()
	cl.Set(baseTime)
	ctx = clock.Set(ctx, cl)
	testutil.UpdateIndices(ctx)

	Convey("Check daily limit", t, func() {
		setDailyLimit(ctx, 1)
		Convey("no analysis", func() {
			ctx, skdr := tq.TestingContext(ctx, nil)
			err := CronHandler(ctx)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 2)
		})
		Convey("analysis is disabled", func() {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:     pb.AnalysisStatus_DISABLED,
				CreateTime: clock.Now(ctx),
			})
			err := CronHandler(ctx)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 2)
		})
		Convey("analysis is not supported", func() {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:     pb.AnalysisStatus_UNSUPPORTED,
				CreateTime: clock.Now(ctx),
			})
			err := CronHandler(ctx)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 2)
		})
		Convey("analysis is not recent", func() {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Status:     pb.AnalysisStatus_FOUND,
				CreateTime: clock.Now(ctx).Add(-25 * time.Hour),
			})
			err := CronHandler(ctx)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 2)
		})
		Convey("analysis is recent", func() {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				Project:    "chrome",
				Status:     pb.AnalysisStatus_FOUND,
				CreateTime: clock.Now(ctx).Add(-23 * time.Hour),
			})
			err := CronHandler(ctx)
			So(err, ShouldBeNil)
			// Should not schedule a task.
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
		})
	})

	Convey("schedule test failure detection task", t, func() {
		ctx, skdr := tq.TestingContext(ctx, nil)
		setDailyLimit(ctx, 100)
		Convey("no exclude dimensions", func() {
			testRerun := createTestSingleRerun(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "")
			// No OS dimension.
			testRerun.Dimensions = &pb.Dimensions{
				Dimensions: []*pb.Dimension{{
					Key:   "cpu",
					Value: "8",
				}},
			}
			So(datastore.Put(ctx, testRerun), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := CronHandler(ctx)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 2)
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureDetectionTask)
			So(resultsTask, ShouldResembleProto, &tpb.TestFailureDetectionTask{
				Project:           "chrome",
				DimensionExcludes: []*pb.Dimension{},
			})
		})
		Convey("exclude dimensions", func() {
			setDailyLimit(ctx, 100)
			// Scheduled test rerun - os excluded.
			createTestSingleRerun(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "test_os1")
			// Started test rerun - os not excluded.
			createTestSingleRerun(ctx, buildbucketpb.Status_STARTED, baseTime.Add(-6*time.Minute), "test_os2")
			// Scheduled test rerun older than 7 days - os not excluded.
			createTestSingleRerun(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-time.Hour*7*24), "test_os3")
			// Scheduled test rerun created less than 5 minutes - os not excluded.
			createTestSingleRerun(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-4*time.Minute), "test_os4")
			// Scheduled test rerun belongs to a different project - os not excluded.
			testRerun := createTestSingleRerun(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "test_os5")
			testRerun.Project = "otherproject"
			So(datastore.Put(ctx, testRerun), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Scheduled compile rerun - os excluded.
			createCompileReruns(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "compile_os1")
			// Started compile rerun - os not excluded.
			createCompileReruns(ctx, buildbucketpb.Status_STARTED, baseTime.Add(-6*time.Minute), "compile_os2")
			// Scheduled compile rerun older than 7 days - os not excluded.
			createCompileReruns(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-time.Hour*7*24), "compile_os3")
			// Scheduled compile rerun created less than 5 minutes - os not excluded.
			createCompileReruns(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-4*time.Minute), "compile_os4")
			// Scheduled compile rerun belongs to a different project - os not excluded.
			rerunBuild, _ := createCompileReruns(ctx, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "compile_os5")
			rerunBuild.Project = "otherproject"
			So(datastore.Put(ctx, rerunBuild), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := CronHandler(ctx)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 2)
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureDetectionTask)
			expectedDimensionExcludes := []*pb.Dimension{{Key: "os", Value: "test_os1"}, {Key: "os", Value: "compile_os1"}}
			util.SortDimension(expectedDimensionExcludes)
			So(resultsTask, ShouldResembleProto, &tpb.TestFailureDetectionTask{
				Project:           "chrome",
				DimensionExcludes: expectedDimensionExcludes,
			})
		})
	})
}

func createTestSingleRerun(ctx context.Context, status buildbucketpb.Status, createdTime time.Time, os string) *model.TestSingleRerun {
	tsr := &model.TestSingleRerun{
		ID: 0,
		LUCIBuild: model.LUCIBuild{
			BuildID:    0,
			Project:    "chrome",
			CreateTime: createdTime,
			Status:     status,
		},
		Dimensions: &pb.Dimensions{
			Dimensions: []*pb.Dimension{{
				Key:   "os",
				Value: os,
			}},
		},
	}
	So(datastore.Put(ctx, tsr), ShouldBeNil)
	datastore.GetTestable(ctx).CatchupIndexes()
	return tsr
}

func createCompileReruns(ctx context.Context, status buildbucketpb.Status, createdTime time.Time, os string) (*model.CompileRerunBuild, *model.SingleRerun) {
	crb := &model.CompileRerunBuild{
		LuciBuild: model.LuciBuild{
			Project:    "chrome",
			CreateTime: createdTime,
			Status:     status,
		},
	}
	So(datastore.Put(ctx, crb), ShouldBeNil)
	sr := &model.SingleRerun{
		RerunBuild: datastore.KeyForObj(ctx, crb),
		Dimensions: &pb.Dimensions{Dimensions: []*pb.Dimension{{Key: "os", Value: os}}},
	}
	So(datastore.Put(ctx, sr), ShouldBeNil)
	datastore.GetTestable(ctx).CatchupIndexes()
	return crb, sr
}

func setDailyLimit(ctx context.Context, limit int) {
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.TestAnalysisConfig.DailyLimit = uint32(limit)
	cfg := map[string]*configpb.ProjectConfig{
		"chromium": projectCfg,
		"chrome":   projectCfg,
	}
	So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)
}
