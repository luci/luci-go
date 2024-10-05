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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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

	ftt.Run("Check daily limit", t, func(t *ftt.Test) {
		setDailyLimit(ctx, t, 1)
		t.Run("no analysis", func(t *ftt.Test) {
			ctx, skdr := tq.TestingContext(ctx, nil)
			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(2))
		})
		t.Run("analysis is disabled", func(t *ftt.Test) {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:     pb.AnalysisStatus_DISABLED,
				CreateTime: clock.Now(ctx),
			})
			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(2))
		})
		t.Run("analysis is not supported", func(t *ftt.Test) {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:     pb.AnalysisStatus_UNSUPPORTED,
				CreateTime: clock.Now(ctx),
			})
			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(2))
		})
		t.Run("analysis is not recent", func(t *ftt.Test) {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Status:     pb.AnalysisStatus_FOUND,
				CreateTime: clock.Now(ctx).Add(-25 * time.Hour),
			})
			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(2))
		})
		t.Run("analysis is recent", func(t *ftt.Test) {
			ctx, skdr := tq.TestingContext(ctx, nil)
			testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				Project:    "chrome",
				Status:     pb.AnalysisStatus_FOUND,
				CreateTime: clock.Now(ctx).Add(-23 * time.Hour),
			})
			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1))
			// Chromium
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureDetectionTask)
			assert.Loosely(t, resultsTask, should.Resemble(&tpb.TestFailureDetectionTask{
				Project:           "chromium",
				DimensionExcludes: []*pb.Dimension{},
			}))
		})
	})

	ftt.Run("schedule test failure detection task", t, func(t *ftt.Test) {
		ctx, skdr := tq.TestingContext(ctx, nil)
		setDailyLimit(ctx, t, 100)
		t.Run("no exclude dimensions", func(t *ftt.Test) {
			testRerun := createTestSingleRerun(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "")
			// No OS dimension.
			testRerun.Dimensions = &pb.Dimensions{
				Dimensions: []*pb.Dimension{{
					Key:   "cpu",
					Value: "8",
				}},
			}
			assert.Loosely(t, datastore.Put(ctx, testRerun), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(2))
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureDetectionTask)
			assert.Loosely(t, resultsTask, should.Resemble(&tpb.TestFailureDetectionTask{
				Project:           "chrome",
				DimensionExcludes: []*pb.Dimension{},
			}))
		})
		t.Run("exclude dimensions", func(t *ftt.Test) {
			setDailyLimit(ctx, t, 100)
			// Scheduled test rerun - os excluded.
			createTestSingleRerun(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "test_os1")
			// Started test rerun - os not excluded.
			createTestSingleRerun(ctx, t, buildbucketpb.Status_STARTED, baseTime.Add(-6*time.Minute), "test_os2")
			// Scheduled test rerun older than 7 days - os not excluded.
			createTestSingleRerun(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-time.Hour*7*24), "test_os3")
			// Scheduled test rerun created less than 5 minutes - os not excluded.
			createTestSingleRerun(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-4*time.Minute), "test_os4")
			// Scheduled test rerun belongs to a different project - os not excluded.
			testRerun := createTestSingleRerun(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "test_os5")
			testRerun.Project = "otherproject"
			assert.Loosely(t, datastore.Put(ctx, testRerun), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Scheduled compile rerun - os excluded.
			createCompileReruns(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "compile_os1")
			// Started compile rerun - os not excluded.
			createCompileReruns(ctx, t, buildbucketpb.Status_STARTED, baseTime.Add(-6*time.Minute), "compile_os2")
			// Scheduled compile rerun older than 7 days - os not excluded.
			createCompileReruns(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-time.Hour*7*24), "compile_os3")
			// Scheduled compile rerun created less than 5 minutes - os not excluded.
			createCompileReruns(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-4*time.Minute), "compile_os4")
			// Scheduled compile rerun belongs to a different project - os not excluded.
			rerunBuild, _ := createCompileReruns(ctx, t, buildbucketpb.Status_SCHEDULED, baseTime.Add(-6*time.Minute), "compile_os5")
			rerunBuild.Project = "otherproject"
			assert.Loosely(t, datastore.Put(ctx, rerunBuild), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := CronHandler(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(2))
			resultsTask := skdr.Tasks().Payloads()[1].(*tpb.TestFailureDetectionTask)
			expectedDimensionExcludes := []*pb.Dimension{{Key: "os", Value: "test_os1"}, {Key: "os", Value: "compile_os1"}}
			util.SortDimension(expectedDimensionExcludes)
			assert.Loosely(t, resultsTask, should.Resemble(&tpb.TestFailureDetectionTask{
				Project:           "chromium",
				DimensionExcludes: expectedDimensionExcludes,
			}))
		})
	})
}

func createTestSingleRerun(ctx context.Context, t testing.TB, status buildbucketpb.Status, createdTime time.Time, os string) *model.TestSingleRerun {
	t.Helper()
	tsr := &model.TestSingleRerun{
		ID: 0,
		LUCIBuild: model.LUCIBuild{
			BuildID:    0,
			Project:    "chromium",
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
	assert.Loosely(t, datastore.Put(ctx, tsr), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	return tsr
}

func createCompileReruns(ctx context.Context, t testing.TB, status buildbucketpb.Status, createdTime time.Time, os string) (*model.CompileRerunBuild, *model.SingleRerun) {
	t.Helper()
	crb := &model.CompileRerunBuild{
		LuciBuild: model.LuciBuild{
			Project:    "chromium",
			CreateTime: createdTime,
			Status:     status,
		},
	}
	assert.Loosely(t, datastore.Put(ctx, crb), should.BeNil, truth.LineContext())
	sr := &model.SingleRerun{
		RerunBuild: datastore.KeyForObj(ctx, crb),
		Dimensions: &pb.Dimensions{Dimensions: []*pb.Dimension{{Key: "os", Value: os}}},
	}
	assert.Loosely(t, datastore.Put(ctx, sr), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).CatchupIndexes()
	return crb, sr
}

func setDailyLimit(ctx context.Context, t testing.TB, limit int) {
	t.Helper()
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.TestAnalysisConfig.DailyLimit = uint32(limit)
	cfg := map[string]*configpb.ProjectConfig{
		"chromium": projectCfg,
		"chrome":   projectCfg,
	}
	assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil, truth.LineContext())
}
