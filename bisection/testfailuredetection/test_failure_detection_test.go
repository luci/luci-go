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
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/rerun"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

func TestRedundancyScore(t *testing.T) {
	t.Parallel()

	Convey("Same test variant exist", t, func() {
		ctx := memory.Use(context.Background())

		Convey("no overlap regression range", func() {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "testvarianthash")
			failureInDB.RegressionStartPosition = 101
			failureInDB.RegressionEndPosition = 102
			So(datastore.Put(ctx, failureInDB), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			So(err, ShouldBeNil)
			So(score, ShouldEqual, 0)
		})
		Convey("overlap regression range", func() {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "testvarianthash")
			failureInDB.RegressionStartPosition = 100
			failureInDB.RegressionEndPosition = 1000
			So(datastore.Put(ctx, failureInDB), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			So(err, ShouldBeNil)
			So(score, ShouldEqual, 1)
		})
	})

	Convey("Same test exist, same test variant not exist", t, func() {
		ctx := memory.Use(context.Background())
		Convey("no overlap regression range", func() {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "othervarianthash")
			failureInDB.RegressionStartPosition = 101
			failureInDB.RegressionEndPosition = 102
			So(datastore.Put(ctx, failureInDB), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			So(err, ShouldBeNil)
			So(score, ShouldEqual, 0)
		})
		Convey("overlap regression range", func() {
			// Existing test failure.
			failureInDB := fakeTestFailure(102, "testID", "othervarianthash")
			failureInDB.RegressionStartPosition = 100
			failureInDB.RegressionEndPosition = 102
			So(datastore.Put(ctx, failureInDB), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// New test failure.
			failure := fakeTestFailure(101, "testID", "testvarianthash")
			failure.RegressionStartPosition = 1
			failure.RegressionEndPosition = 100

			score, err := redundancyScore(ctx, failure)
			So(err, ShouldBeNil)
			So(score, ShouldEqual, float64(1)/103)
		})
	})

	Convey("No test failure with same test or test variant exists", t, func() {
		ctx := memory.Use(context.Background())
		// Existing test failure.
		failureInDB := fakeTestFailure(102, "othertestID", "varianthash")
		failureInDB.RegressionStartPosition = 1
		failureInDB.RegressionEndPosition = 100
		So(datastore.Put(ctx, failureInDB), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		// New test failure.
		failure := fakeTestFailure(101, "testID", "testvarianthash")
		failure.RegressionStartPosition = 1
		failure.RegressionEndPosition = 100

		score, err := redundancyScore(ctx, failure)
		So(err, ShouldBeNil)
		So(score, ShouldEqual, 0)
	})
}

func TestFailureDetection(t *testing.T) {
	t.Parallel()
	bisection.RegisterTaskClass()

	Convey("Have bisection task to send", t, func() {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		ctx, skdr := tq.TestingContext(txndefer.FilterRDS(ctx), nil)
		analysisClient := &fakeLUCIAnalysisClient{testFailuresByProject: map[string][]*lucianalysis.BuilderRegressionGroup{}}
		d := TestFailureDetector{LUCIAnalysis: analysisClient}
		task := &tpb.TestFailureDetectionTask{
			Project:          "testProject",
			VariantPredicate: &tpb.VariantPredicate{},
		}
		verify := func(selectedGroup *lucianalysis.BuilderRegressionGroup, redundancyScore float64) {
			err := d.Find(ctx, task)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureBisectionTask)
			analysisID := resultsTask.AnalysisId
			// Verify TestFailures are saved.
			var primaryFailureKey *datastore.Key
			for i, tv := range selectedGroup.TestVariants {
				testFailureDB, err := datastoreutil.GetTestFailures(ctx, "testProject", tv.TestID.String(), selectedGroup.RefHash.String(), tv.VariantHash.String())
				So(err, ShouldBeNil)
				So(testFailureDB, ShouldHaveLength, 1)
				if i == 0 {
					primaryFailureKey = datastore.KeyForObj(ctx, testFailureDB[0])
					So(testFailureDB[0].IsPrimary, ShouldEqual, true)
				} else {
					So(testFailureDB[0].IsPrimary, ShouldEqual, false)
				}
				So(testFailureDB[0].RegressionStartPosition, ShouldEqual, selectedGroup.RegressionStartPosition.Int64)
				So(testFailureDB[0].RegressionEndPosition, ShouldEqual, selectedGroup.RegressionEndPosition.Int64)
				So(testFailureDB[0].AnalysisKey, ShouldEqual, datastore.NewKey(ctx, "TestFailureAnalysis", "", analysisID, nil))
			}
			// Verify TestFailureAnalysis is saved.
			analysis, err := datastoreutil.GetTestFailureAnalysis(ctx, resultsTask.AnalysisId)
			So(err, ShouldBeNil)
			expected := &model.TestFailureAnalysis{
				Project:     "testProject",
				ID:          resultsTask.AnalysisId,
				TestFailure: primaryFailureKey,
				Status:      pb.AnalysisStatus_CREATED,
				CreateTime:  clock.Now(ctx),
				Priority:    rerun.PriorityTestFailure,
			}
			So(analysis, ShouldResemble, expected)
		}
		Convey("send the least redundant test failure group", func() {
			selectedGroup := fakeBuilderRegressionGroup("testID", "varianthash3", 200, 201)
			analysisClient.testFailuresByProject["testProject"] = []*lucianalysis.BuilderRegressionGroup{
				fakeBuilderRegressionGroup("testID", "varianthash", 100, 101),
				selectedGroup,
				fakeBuilderRegressionGroup("testID", "varianthash2", 99, 101),
			}
			// Existing test failure.
			failureInDB := fakeTestFailure(101, "testID", "varianthash")
			failureInDB.RegressionStartPosition = 100
			failureInDB.RegressionEndPosition = 102
			So(datastore.Put(ctx, failureInDB), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			verify(selectedGroup, 0)
		})

		Convey("send the most recent test failure group when redundancy score are the same", func() {
			selectedGroup := fakeBuilderRegressionGroup("testID", "variantHash3", 200, 201)
			analysisClient.testFailuresByProject["testProject"] = []*lucianalysis.BuilderRegressionGroup{
				fakeBuilderRegressionGroup("testID", "variantHash", 100, 101),
				selectedGroup,
				fakeBuilderRegressionGroup("testID", "variantHash2", 99, 101),
			}

			verify(selectedGroup, 0)
		})
	})

	Convey("No bisection task to send", t, func() {
		ctx := memory.Use(context.Background())
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		ctx, skdr := tq.TestingContext(txndefer.FilterRDS(ctx), nil)
		analysisClient := &fakeLUCIAnalysisClient{testFailuresByProject: map[string][]*lucianalysis.BuilderRegressionGroup{}}
		d := TestFailureDetector{LUCIAnalysis: analysisClient}
		task := &tpb.TestFailureDetectionTask{
			Project:          "testProject",
			VariantPredicate: &tpb.VariantPredicate{},
		}
		Convey("no builder regression group", func() {
			err := d.Find(ctx, task)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
		})

		Convey("all groups are redundant", func() {
			analysisClient.testFailuresByProject["testProject"] = []*lucianalysis.BuilderRegressionGroup{
				fakeBuilderRegressionGroup("testID", "varianthash", 99, 100),
			}
			// Existing test failure.
			failureInDB := fakeTestFailure(101, "testID", "varianthash")
			failureInDB.RegressionStartPosition = 100
			failureInDB.RegressionEndPosition = 102
			So(datastore.Put(ctx, failureInDB), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := d.Find(ctx, task)
			So(err, ShouldBeNil)
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 0)
		})
	})
}

type fakeLUCIAnalysisClient struct {
	testFailuresByProject map[string][]*lucianalysis.BuilderRegressionGroup
}

func (f *fakeLUCIAnalysisClient) ReadTestFailures(ctx context.Context, opts lucianalysis.ReadTestFailuresOptions) ([]*lucianalysis.BuilderRegressionGroup, error) {
	return f.testFailuresByProject[opts.Project], nil
}

func fakeBuilderRegressionGroup(primaryTestID, primaryVariantHash string, start, end int64) *lucianalysis.BuilderRegressionGroup {
	return &lucianalysis.BuilderRegressionGroup{
		RefHash: bqString("testRefHash"),
		Ref: &lucianalysis.Ref{
			Gitiles: &lucianalysis.Gitiles{
				Host:    bqString("testHost"),
				Project: bqString("testProject"),
				Ref:     bqString("testRef"),
			},
		},
		RegressionStartPosition:  bigquery.NullInt64{Int64: start, Valid: true},
		RegressionEndPosition:    bigquery.NullInt64{Int64: end, Valid: true},
		StartPositionFailureRate: 0,
		EndPositionFailureRate:   1,
		TestVariants: []*lucianalysis.TestVariant{
			{TestID: bqString(primaryTestID), VariantHash: bqString(primaryVariantHash), Variant: bigquery.NullJSON{JSONVal: `{"builder":"testbuilder"}`, Valid: true}},
			{TestID: bqString(primaryTestID + "1"), VariantHash: bqString(primaryVariantHash), Variant: bigquery.NullJSON{JSONVal: `{"builder":"testbuilder"}`, Valid: true}},
			{TestID: bqString(primaryTestID + "2"), VariantHash: bqString(primaryVariantHash), Variant: bigquery.NullJSON{JSONVal: `{"builder":"testbuilder"}`, Valid: true}},
		},
		StartHour: bigquery.NullTimestamp{Timestamp: time.Unix(1689343797, 0), Valid: true},
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
		RegressionStartPosition:  0,
		RegressionEndPosition:    0,
		StartPositionFailureRate: 0,
		EndPositionFailureRate:   1,
		IsPrimary:                false,
		IsDiverged:               false,
		RedundancyScore:          0,
		StartHour:                time.Unix(1689343797, 0).UTC(),
	}
}
