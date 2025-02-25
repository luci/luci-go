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

// Package testfailuredetection analyses recent test failures with
// the changepoint analysis from LUCI analysis, and select test failures to bisect.
package testfailuredetection

import (
	"context"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/rerun"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

func TestRedundancyScore(t *testing.T) {
	t.Parallel()

	ftt.Run("Same test variant exist", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("no overlap regression range", func(t *ftt.Test) {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "testvarianthash")
			failureInDB.RegressionStartPosition = 101
			failureInDB.RegressionEndPosition = 102
			assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, score, should.BeZero)
		})
		t.Run("overlap regression range", func(t *ftt.Test) {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "testvarianthash")
			failureInDB.RegressionStartPosition = 100
			failureInDB.RegressionEndPosition = 1000
			assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, score, should.Equal(1.0))
		})
	})

	ftt.Run("Same test exist, same test variant not exist", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		t.Run("no overlap regression range", func(t *ftt.Test) {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "othervarianthash")
			failureInDB.RegressionStartPosition = 101
			failureInDB.RegressionEndPosition = 102
			assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, score, should.BeZero)
		})
		t.Run("overlap regression range", func(t *ftt.Test) {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "othervarianthash")
			failureInDB.RegressionStartPosition = 100
			failureInDB.RegressionEndPosition = 102
			assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, score, should.Equal(float64(1)/103))
		})
	})

	ftt.Run("No test failure with same test or test variant exists", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		// Existing test failure.
		failureInDB := fakeTestFailure(102, "othertestID", "varianthash")
		failureInDB.RegressionStartPosition = 1
		failureInDB.RegressionEndPosition = 100
		assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		// New test failure.
		failure := fakeTestFailure(101, "testID", "testvarianthash")
		failure.RegressionStartPosition = 1
		failure.RegressionEndPosition = 100

		score, err := redundancyScore(ctx, failure)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, score, should.BeZero)
	})
}

func TestFailureDetection(t *testing.T) {
	t.Parallel()

	ftt.Run("Have bisection task to send", t, func(t *ftt.Test) {
		ctx, skdr := setupTestingContext(t)
		analysisClient := &fakeLUCIAnalysisClient{
			testFailuresByProject: map[string][]*lucianalysis.BuilderRegressionGroup{},
			buildInfoByProject: map[string]lucianalysis.BuildInfo{
				"testProject": {
					BuildID:         1,
					StartCommitHash: "startCommitHash",
					EndCommitHash:   "endCommitHash",
				},
			},
		}
		task := &tpb.TestFailureDetectionTask{
			Project: "testProject",
		}
		verify := func(selectedGroup *lucianalysis.BuilderRegressionGroup, redundancyScore float64) {
			err := Run(ctx, analysisClient, task)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1))
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureBisectionTask)
			analysisID := resultsTask.AnalysisId
			// Verify TestFailures are saved.
			var primaryFailureKey *datastore.Key
			for i, tv := range selectedGroup.TestVariants {
				testFailureDB, err := datastoreutil.GetTestFailures(ctx, "testProject", tv.TestID.String(), selectedGroup.RefHash.String(), tv.VariantHash.String())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, testFailureDB, should.HaveLength(1))
				if i == 0 {
					primaryFailureKey = datastore.KeyForObj(ctx, testFailureDB[0])
					assert.Loosely(t, testFailureDB[0].IsPrimary, should.Equal(true))
					assert.Loosely(t, testFailureDB[0].RedundancyScore, should.Equal(redundancyScore))
				} else {
					assert.Loosely(t, testFailureDB[0].IsPrimary, should.Equal(false))
				}
				assert.Loosely(t, testFailureDB[0].RegressionStartPosition, should.Equal(selectedGroup.RegressionStartPosition.Int64))
				assert.Loosely(t, testFailureDB[0].RegressionEndPosition, should.Equal(selectedGroup.RegressionEndPosition.Int64))
				assert.Loosely(t, testFailureDB[0].AnalysisKey, should.Match(datastore.NewKey(ctx, "TestFailureAnalysis", "", analysisID, nil)))
			}
			// Verify TestFailureAnalysis is saved.
			analysis, err := datastoreutil.GetTestFailureAnalysis(ctx, resultsTask.AnalysisId)
			assert.Loosely(t, err, should.BeNil)
			expected := &model.TestFailureAnalysis{
				ID:               resultsTask.AnalysisId,
				Project:          "testProject",
				Bucket:           "bucket",
				Builder:          "builder",
				TestFailure:      primaryFailureKey,
				CreateTime:       clock.Now(ctx),
				Status:           pb.AnalysisStatus_CREATED,
				Priority:         rerun.PriorityTestFailure,
				StartCommitHash:  "startCommitHash",
				EndCommitHash:    "endCommitHash",
				FailedBuildID:    1,
				SheriffRotations: []string{"chromium"},
			}
			assert.Loosely(t, analysis, should.Match(expected))
		}
		t.Run("send the most recent test failure", func(t *ftt.Test) {
			selectedGroup := fakeBuilderRegressionGroup("testID", "varianthash3", 200, 201, time.Unix(3600*24*100, 0))
			analysisClient.testFailuresByProject["testProject"] = []*lucianalysis.BuilderRegressionGroup{
				fakeBuilderRegressionGroup("testID", "varianthash", 100, 101, time.Unix(3600*24*99, 0)),
				selectedGroup,
				fakeBuilderRegressionGroup("testID", "varianthash2", 99, 101, time.Unix(3600*24*99, 0)),
			}
			// Existing test failure.
			failureInDB := fakeTestFailure(101, "testID", "varianthash4")
			failureInDB.RegressionStartPosition = 201
			failureInDB.RegressionEndPosition = 202
			assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			verify(selectedGroup, 0.25)
		})

		t.Run("send the least redundant test failure when recency is the same", func(t *ftt.Test) {
			selectedGroup := fakeBuilderRegressionGroup("testID", "varianthash3", 200, 201, time.Unix(3600*24*100, 0))
			analysisClient.testFailuresByProject["testProject"] = []*lucianalysis.BuilderRegressionGroup{
				fakeBuilderRegressionGroup("testID", "varianthash", 100, 101, time.Unix(3600*24*100, 0)),
				selectedGroup,
				fakeBuilderRegressionGroup("testID", "varianthash2", 99, 101, time.Unix(3600*24*100, 0)),
			}
			// Existing test failure.
			failureInDB := fakeTestFailure(101, "testID", "varianthash4")
			failureInDB.RegressionStartPosition = 99
			failureInDB.RegressionEndPosition = 101
			assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			verify(selectedGroup, 0)
		})
	})

	ftt.Run("No bisection task to send", t, func(t *ftt.Test) {
		ctx, skdr := setupTestingContext(t)
		analysisClient := &fakeLUCIAnalysisClient{
			testFailuresByProject: map[string][]*lucianalysis.BuilderRegressionGroup{},
			buildInfoByProject: map[string]lucianalysis.BuildInfo{
				"testProject": {
					BuildID:         1,
					StartCommitHash: "startCommitHash",
					EndCommitHash:   "endCommitHash",
				},
			},
		}
		task := &tpb.TestFailureDetectionTask{
			Project: "testProject",
		}
		t.Run("no builder regression group", func(t *ftt.Test) {
			err := Run(ctx, analysisClient, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
		})

		t.Run("all groups are redundant", func(t *ftt.Test) {
			analysisClient.testFailuresByProject["testProject"] = []*lucianalysis.BuilderRegressionGroup{
				fakeBuilderRegressionGroup("testID", "varianthash", 99, 100, time.Unix(0, 0)),
			}
			// Existing test failure.
			failureInDB := fakeTestFailure(101, "testID", "varianthash")
			failureInDB.RegressionStartPosition = 100
			failureInDB.RegressionEndPosition = 102
			assert.Loosely(t, datastore.Put(ctx, failureInDB), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := Run(ctx, analysisClient, task)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
		})

		t.Run("insufficient data", func(t *ftt.Test) {
			analysisClient.testFailuresByProject["testProject"] = []*lucianalysis.BuilderRegressionGroup{
				fakeBuilderRegressionGroup(insufficientDataTestID, "varianthash", 99, 100, time.Unix(1689343797, 0)),
			}

			err := Run(ctx, analysisClient, task)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.BeZero)
			// Check test analysis is saved.
			q := datastore.NewQuery("TestFailureAnalysis")
			analyses := []*model.TestFailureAnalysis{}
			assert.Loosely(t, datastore.GetAll(ctx, q, &analyses), should.BeNil)
			assert.Loosely(t, len(analyses), should.Equal(1))

			q = datastore.NewQuery("TestFailure").Eq("test_id", insufficientDataTestID)
			tfs := []*model.TestFailure{}
			assert.Loosely(t, datastore.GetAll(ctx, q, &tfs), should.BeNil)
			assert.Loosely(t, len(tfs), should.Equal(1))

			assert.Loosely(t, analyses[0], should.Match(&model.TestFailureAnalysis{
				ID:               analyses[0].ID,
				Project:          "testProject",
				CreateTime:       time.Unix(10000, 0).UTC(),
				Status:           pb.AnalysisStatus_INSUFFICENTDATA,
				RunStatus:        pb.AnalysisRunStatus_ENDED,
				EndTime:          time.Unix(10000, 0).UTC(),
				TestFailure:      datastore.KeyForObj(ctx, tfs[0]),
				SheriffRotations: []string{"chromium"},
			}))

			assert.Loosely(t, tfs[0], should.Match(&model.TestFailure{
				ID:          tfs[0].ID,
				Project:     "testProject",
				TestID:      insufficientDataTestID,
				VariantHash: "varianthash",
				Variant: &pb.Variant{
					Def: map[string]string{
						"builder": "testbuilder",
					},
				},
				RefHash: "testRefHash",
				Bucket:  "bucket",
				Builder: "builder",
				Ref: &pb.SourceRef{
					System: &pb.SourceRef_Gitiles{
						Gitiles: &pb.GitilesRef{
							Host:    "testHost",
							Project: "testProject",
							Ref:     "testRef",
						},
					},
				},
				IsPrimary:               true,
				AnalysisKey:             datastore.KeyForObj(ctx, analyses[0]),
				RegressionStartPosition: 99,
				RegressionEndPosition:   100,
				EndPositionFailureRate:  1,
				StartHour:               time.Unix(1689343797, 0).UTC(),
				EndHour:                 time.Unix(1689343798, 0).UTC(),
			}))
		})
	})
}

const insufficientDataTestID = "insufficientDataTestID"

type fakeLUCIAnalysisClient struct {
	testFailuresByProject map[string][]*lucianalysis.BuilderRegressionGroup
	buildInfoByProject    map[string]lucianalysis.BuildInfo
}

func (f *fakeLUCIAnalysisClient) ReadTestFailures(ctx context.Context, task *tpb.TestFailureDetectionTask, filter *configpb.FailureIngestionFilter) ([]*lucianalysis.BuilderRegressionGroup, error) {
	return f.testFailuresByProject[task.Project], nil
}

func (f *fakeLUCIAnalysisClient) ReadBuildInfo(ctx context.Context, tf *model.TestFailure) (lucianalysis.BuildInfo, error) {
	if tf.TestID == insufficientDataTestID {
		return lucianalysis.BuildInfo{}, errors.New("Insufficient data")
	}
	return f.buildInfoByProject[tf.Project], nil
}

func fakeBuilderRegressionGroup(primaryTestID, primaryVariantHash string, start, end int64, startHour time.Time) *lucianalysis.BuilderRegressionGroup {
	return &lucianalysis.BuilderRegressionGroup{
		Bucket:  bqString("bucket"),
		Builder: bqString("builder"),
		RefHash: bqString("testRefHash"),
		Ref: &lucianalysis.Ref{
			Gitiles: &lucianalysis.Gitiles{
				Host:    bqString("testHost"),
				Project: bqString("testProject"),
				Ref:     bqString("testRef"),
			},
		},
		RegressionStartPosition: bigquery.NullInt64{Int64: start, Valid: true},
		RegressionEndPosition:   bigquery.NullInt64{Int64: end, Valid: true},
		TestVariants: []*lucianalysis.TestVariant{
			{TestID: bqString(primaryTestID), VariantHash: bqString(primaryVariantHash), Variant: bigquery.NullJSON{JSONVal: `{"builder":"testbuilder"}`, Valid: true}, StartPositionUnexpectedResultRate: 0, EndPositionUnexpectedResultRate: 1},
			{TestID: bqString(primaryTestID + "1"), VariantHash: bqString(primaryVariantHash), Variant: bigquery.NullJSON{JSONVal: `{"builder":"testbuilder"}`, Valid: true}, StartPositionUnexpectedResultRate: 0, EndPositionUnexpectedResultRate: 1},
			{TestID: bqString(primaryTestID + "2"), VariantHash: bqString(primaryVariantHash), Variant: bigquery.NullJSON{JSONVal: `{"builder":"testbuilder"}`, Valid: true}, StartPositionUnexpectedResultRate: 0, EndPositionUnexpectedResultRate: 1},
		},
		StartHour: bigquery.NullTimestamp{Timestamp: startHour, Valid: true},
		EndHour:   bigquery.NullTimestamp{Timestamp: time.Unix(1689343798, 0), Valid: true},
		SheriffRotations: []bigquery.NullString{
			{
				StringVal: "chromium",
				Valid:     true,
			},
		},
	}
}

func bqString(value string) bigquery.NullString {
	return bigquery.NullString{StringVal: value, Valid: true}
}

func fakeTestFailure(ID int64, testID, variantHash string) *model.TestFailure {
	return &model.TestFailure{
		ID:                       ID,
		Project:                  "testProject",
		TestID:                   testID,
		VariantHash:              variantHash,
		RefHash:                  "testRefHash",
		Bucket:                   "bucket",
		Builder:                  "builder",
		RegressionStartPosition:  0,
		RegressionEndPosition:    0,
		StartPositionFailureRate: 0,
		EndPositionFailureRate:   1,
		IsPrimary:                false,
		IsDiverged:               false,
		RedundancyScore:          0,
		StartHour:                time.Unix(1689343797, 0).UTC(),
		EndHour:                  time.Unix(1689343798, 0).UTC(),
	}
}

func setupTestingContext(t testing.TB) (context.Context, *tqtesting.Scheduler) {
	t.Helper()
	ctx := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.TestAnalysisConfig.DetectorEnabled = true

	projectCfg.TestAnalysisConfig.FailureIngestionFilter = &configpb.FailureIngestionFilter{
		ExcludedBuckets: []string{"try", "findit", "reviver"},
	}
	cfg := map[string]*configpb.ProjectConfig{"testProject": projectCfg}
	assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil, truth.LineContext())
	datastore.GetTestable(ctx).Consistent(true)
	return tq.TestingContext(txndefer.FilterRDS(ctx), nil)
}
