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

package updatetestrerun

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/util/testutil"

	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestUpdate(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)

	Convey("Invalid request", t, func() {
		req := &pb.UpdateTestAnalysisProgressRequest{}
		err := Update(ctx, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)

		req.Bbid = 888
		err = Update(ctx, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)

		req.Bbid = 0
		req.BotId = ""
		err = Update(ctx, req)
		So(err, ShouldNotBeNil)
		So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)
	})

	Convey("Update", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID: 100,
		})

		Convey("No rerun", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  800,
				BotId: "bot",
			}
			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.NotFound)
		})

		Convey("Rerun ended", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  801,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:     801,
				Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			})
			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "rerun has ended")
		})

		Convey("No analysis", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  802,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:          802,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.MakeKey(ctx, "TestFailureAnalysis", 1),
			})

			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "get test failure analysis")
		})

		Convey("Invalid rerun type", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  803,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:          803,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
			})

			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "invalid rerun type")
		})

		Convey("Tests did not run", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  804,
				BotId: "bot",
			}
			rerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:          804,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
				Type:        model.RerunBuildType_NthSection,
			})

			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			So(err, ShouldBeNil)
			So(rerun.ReportTime, ShouldEqual, time.Unix(10000, 0).UTC())
			So(rerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_INFRA_FAILED)
		})

		Convey("No primary test failure", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         805,
				BotId:        "bot",
				RunSucceeded: true,
			}
			rerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:          805,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
				Type:        model.RerunBuildType_NthSection,
			})
			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "get primary test failure")

			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			So(err, ShouldBeNil)
			So(rerun.ReportTime, ShouldEqual, time.Unix(10000, 0).UTC())
			So(rerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_INFRA_FAILED)
		})

		Convey("No result for primary failure", func() {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				ID:             106,
				TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 1061),
			})

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         806,
				BotId:        "bot",
				RunSucceeded: true,
			}
			rerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:          806,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
				Type:        model.RerunBuildType_NthSection,
			})

			// Set up test failures
			testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
				ID:          1061,
				IsPrimary:   true,
				TestID:      "test1",
				VariantHash: "hash1",
				Analysis:    tfa,
			})
			testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
				ID:          1062,
				TestID:      "test2",
				VariantHash: "hash2",
				Analysis:    tfa,
			})

			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "no result for primary failure")
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			So(err, ShouldBeNil)
			So(rerun.ReportTime, ShouldEqual, time.Unix(10000, 0).UTC())
			So(rerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_INFRA_FAILED)
		})

		Convey("Primary test failure skipped", func() {
			_, tfs, rerun := setupTestAnalysisForTesting(ctx, 107, 1071, 807, 2)

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         807,
				BotId:        "bot",
				RunSucceeded: true,
				Results: []*pb.TestResult{
					{
						TestId:      "test0",
						VariantHash: "hash0",
						IsExpected:  true,
						Status:      pb.TestResultStatus_SKIP,
					},
					{
						TestId:      "test1",
						VariantHash: "hash1",
						IsExpected:  true,
						Status:      pb.TestResultStatus_PASS,
					},
				},
			}
			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			So(err, ShouldBeNil)
			So(rerun.ReportTime, ShouldEqual, time.Unix(10000, 0).UTC())
			So(rerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED)
			So(rerun.TestResults, ShouldResembleProto, model.RerunTestResults{
				IsFinalized: true,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[0]),
					},
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[1]),
						ExpectedCount:  1,
					},
				},
			})
			err = datastore.Get(ctx, tfs[1])
			So(err, ShouldBeNil)
			So(tfs[1].IsDiverged, ShouldBeFalse)
		})

		Convey("Primary test failure expected", func() {
			_, tfs, rerun := setupTestAnalysisForTesting(ctx, 108, 1081, 808, 4)

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         808,
				BotId:        "bot",
				RunSucceeded: true,
				Results: []*pb.TestResult{
					{
						TestId:      "test0",
						VariantHash: "hash0",
						IsExpected:  true,
						Status:      pb.TestResultStatus_PASS,
					},
					{
						TestId:      "test1",
						VariantHash: "hash1",
						IsExpected:  true,
						Status:      pb.TestResultStatus_PASS,
					},
					{
						TestId:      "test2",
						VariantHash: "hash2",
						IsExpected:  false,
						Status:      pb.TestResultStatus_FAIL,
					},
					{
						TestId:      "test3",
						VariantHash: "hash3",
						IsExpected:  false,
						Status:      pb.TestResultStatus_SKIP,
					},
				},
			}

			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			So(err, ShouldBeNil)
			So(rerun.ReportTime, ShouldEqual, time.Unix(10000, 0).UTC())
			So(rerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_PASSED)
			So(rerun.TestResults, ShouldResembleProto, model.RerunTestResults{
				IsFinalized: true,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[0]),
						ExpectedCount:  1,
					},
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[1]),
						ExpectedCount:  1,
					},
					{
						TestFailureKey:  datastore.KeyForObj(ctx, tfs[2]),
						UnexpectedCount: 1,
					},
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[3]),
					},
				},
			})
			err = datastore.Get(ctx, tfs[1])
			So(err, ShouldBeNil)
			So(tfs[1].IsDiverged, ShouldBeFalse)
			err = datastore.Get(ctx, tfs[2])
			So(err, ShouldBeNil)
			So(tfs[2].IsDiverged, ShouldBeTrue)
			err = datastore.Get(ctx, tfs[3])
			So(err, ShouldBeNil)
			So(tfs[3].IsDiverged, ShouldBeTrue)
		})

		Convey("Primary test failure unexpected", func() {
			_, tfs, rerun := setupTestAnalysisForTesting(ctx, 109, 1091, 809, 4)

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         809,
				BotId:        "bot",
				RunSucceeded: true,
				Results: []*pb.TestResult{
					{
						TestId:      "test0",
						VariantHash: "hash0",
						IsExpected:  false,
						Status:      pb.TestResultStatus_FAIL,
					},
					{
						TestId:      "test1",
						VariantHash: "hash1",
						IsExpected:  true,
						Status:      pb.TestResultStatus_PASS,
					},
					{
						TestId:      "test2",
						VariantHash: "hash2",
						IsExpected:  false,
						Status:      pb.TestResultStatus_FAIL,
					},
					{
						TestId:      "test3",
						VariantHash: "hash3",
						IsExpected:  false,
						Status:      pb.TestResultStatus_SKIP,
					},
				},
			}

			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			So(err, ShouldBeNil)
			So(rerun.ReportTime, ShouldEqual, time.Unix(10000, 0).UTC())
			So(rerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_FAILED)
			So(rerun.TestResults, ShouldResembleProto, model.RerunTestResults{
				IsFinalized: true,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey:  datastore.KeyForObj(ctx, tfs[0]),
						UnexpectedCount: 1,
					},
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[1]),
						ExpectedCount:  1,
					},
					{
						TestFailureKey:  datastore.KeyForObj(ctx, tfs[2]),
						UnexpectedCount: 1,
					},
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[3]),
					},
				},
			})
			err = datastore.Get(ctx, tfs[1])
			So(err, ShouldBeNil)
			So(tfs[1].IsDiverged, ShouldBeTrue)
			err = datastore.Get(ctx, tfs[2])
			So(err, ShouldBeNil)
			So(tfs[2].IsDiverged, ShouldBeFalse)
			err = datastore.Get(ctx, tfs[3])
			So(err, ShouldBeNil)
			So(tfs[3].IsDiverged, ShouldBeTrue)
		})
	})
}

func setupTestAnalysisForTesting(ctx context.Context, analysisID int64, startTestFailureID int64, rerunID int64, numTest int) (*model.TestFailureAnalysis, []*model.TestFailure, *model.TestSingleRerun) {
	tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
		ID:             analysisID,
		TestFailureKey: datastore.MakeKey(ctx, "TestFailure", startTestFailureID),
	})

	// Set up test failures
	tfs := make([]*model.TestFailure, numTest)
	results := make([]model.RerunSingleTestResult, numTest)
	for i := 0; i < numTest; i++ {
		isPrimary := (i == 0)
		tfs[i] = testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:          startTestFailureID + int64(i),
			IsPrimary:   isPrimary,
			TestID:      fmt.Sprintf("test%d", i),
			VariantHash: fmt.Sprintf("hash%d", i),
			Analysis:    tfa,
		})
		results[i] = model.RerunSingleTestResult{
			TestFailureKey: datastore.KeyForObj(ctx, tfs[i]),
		}
	}
	rerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
		ID:          rerunID,
		Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		AnalysisKey: datastore.KeyForObj(ctx, tfa),
		Type:        model.RerunBuildType_NthSection,
		TestResult: model.RerunTestResults{
			Results: results,
		},
	})
	return tfa, tfs, rerun
}
