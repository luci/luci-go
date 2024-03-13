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

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdate(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

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
		ctx := memory.Use(context.Background())
		// We need this because without this, updates on TestSingleRerun status
		// will not be reflected in subsequent queries, due to the eventual
		// consistency.
		// LUCI Bisection is running on Firestore on Datastore mode in dev and prod,
		// which uses strong consistency, so this should reflect the real world.
		datastore.GetTestable(ctx).Consistent(true)
		testutil.UpdateIndices(ctx)

		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(ctx, ctl)
		ctx = mc.Ctx
		mockBuildBucket(mc, false)

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

		Convey("Invalid rerun type", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  802,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:          802,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
			})

			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "invalid rerun type")
		})

		Convey("No analysis", func() {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  803,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
				ID:          803,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.MakeKey(ctx, "TestFailureAnalysis", 1),
				Type:        model.RerunBuildType_CulpritVerification,
			})

			err := Update(ctx, req)
			So(err, ShouldNotBeNil)
			So(status.Convert(err).Code(), ShouldEqual, codes.Internal)
			So(err.Error(), ShouldContainSubstring, "get test failure analysis")
		})

		Convey("Tests did not run", func() {
			tfa, _, rerun, _ := setupTestAnalysisForTesting(ctx, 1)
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         8000,
				BotId:        "bot",
				RunSucceeded: false,
			}
			enableBisection(ctx, true, tfa.Project)

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
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
				ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			})
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

			So(datastore.Get(ctx, nsa), ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(nsa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())

			So(datastore.Get(ctx, tfa), ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(tfa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())
		})

		Convey("No result for primary failure", func() {
			tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
				ID:             106,
				TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 1061),
			})
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
				ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
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
			So(status.Convert(err).Code(), ShouldEqual, codes.InvalidArgument)
			So(err.Error(), ShouldContainSubstring, "no result for primary failure")
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			So(err, ShouldBeNil)
			So(rerun.ReportTime, ShouldEqual, time.Unix(10000, 0).UTC())
			So(rerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_INFRA_FAILED)

			So(datastore.Get(ctx, nsa), ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(nsa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())

			So(datastore.Get(ctx, tfa), ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(tfa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())
		})

		Convey("Primary test failure skipped", func() {
			tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, 2)
			enableBisection(ctx, false, tfa.Project)

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         8000,
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
			// Check that a new rerun is not scheduled, because primary test was skipped.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			So(datastore.GetAll(ctx, q, &reruns), ShouldBeNil)
			So(len(reruns), ShouldEqual, 1)
		})

		Convey("Primary test failure expected", func() {
			tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, 4)
			enableBisection(ctx, true, tfa.Project)

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         8000,
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
			// Check that a new rerun is scheduled.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			So(datastore.GetAll(ctx, q, &reruns), ShouldBeNil)
			So(len(reruns), ShouldEqual, 2)
		})

		Convey("Primary test failure unexpected", func() {
			tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, 4)
			enableBisection(ctx, true, tfa.Project)

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         8000,
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
			// Check that a new rerun is scheduled.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			So(datastore.GetAll(ctx, q, &reruns), ShouldBeNil)
			So(len(reruns), ShouldEqual, 2)
		})

		Convey("Ended nthsection should not get updated", func() {
			tfa, _, _, nsa := setupTestAnalysisForTesting(ctx, 1)
			enableBisection(ctx, true, tfa.Project)

			// Set nthsection to end.
			nsa.RunStatus = pb.AnalysisRunStatus_ENDED
			So(datastore.Put(ctx, nsa), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         8000,
				BotId:        "bot",
				RunSucceeded: true,
				Results: []*pb.TestResult{
					{
						TestId:      "test0",
						VariantHash: "hash0",
						IsExpected:  false,
						Status:      pb.TestResultStatus_FAIL,
					},
				},
			}

			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Check that a new rerun is not scheduled.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			So(datastore.GetAll(ctx, q, &reruns), ShouldBeNil)
			// 1 because of the rerun created in setupTestAnalysisForTesting.
			So(len(reruns), ShouldEqual, 1)

			// Check that no suspect is created.
			q = datastore.NewQuery("Suspect")
			suspects := []*model.Suspect{}
			So(datastore.GetAll(ctx, q, &suspects), ShouldBeNil)
			So(len(suspects), ShouldEqual, 0)
		})
	})

	Convey("process culprit verification update", t, func() {
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		// set up tfa, rerun, suspect
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:             100,
			TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 1000),
			Status:         pb.AnalysisStatus_SUSPECTFOUND,
			RunStatus:      pb.AnalysisRunStatus_STARTED,
		})
		testFailure := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:          1000,
			IsPrimary:   true,
			TestID:      "test0",
			VariantHash: "hash0",
			Analysis:    tfa,
		})
		suspect := testutil.CreateSuspect(ctx, nil)
		suspectRerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID:          8000,
			Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			Type:        model.RerunBuildType_CulpritVerification,
			TestResult: model.RerunTestResults{
				Results: []model.RerunSingleTestResult{
					{TestFailureKey: datastore.KeyForObj(ctx, testFailure)},
				},
			},
			CulpritKey: datastore.KeyForObj(ctx, suspect),
		})
		parentRerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID:          8001,
			Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			Type:        model.RerunBuildType_CulpritVerification,
			TestResult: model.RerunTestResults{
				Results: []model.RerunSingleTestResult{
					{TestFailureKey: datastore.KeyForObj(ctx, testFailure)},
				},
			},
			CulpritKey: datastore.KeyForObj(ctx, suspect),
		})
		suspect.SuspectRerunBuild = datastore.KeyForObj(ctx, suspectRerun)
		suspect.ParentRerunBuild = datastore.KeyForObj(ctx, parentRerun)
		So(datastore.Put(ctx, suspect), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		req := &pb.UpdateTestAnalysisProgressRequest{
			Bbid:         8000,
			BotId:        "bot",
			RunSucceeded: true,
			Results: []*pb.TestResult{
				{
					TestId:      "test0",
					VariantHash: "hash0",
					IsExpected:  false,
					Status:      pb.TestResultStatus_FAIL,
				},
			},
		}
		Convey("suspect under verification", func() {
			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			So(datastore.Get(ctx, suspectRerun), ShouldBeNil)
			So(suspectRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_FAILED)
			// Check suspect status.
			So(datastore.Get(ctx, suspect), ShouldBeNil)
			So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_UnderVerification)
			// Check analysis - no update.
			So(datastore.Get(ctx, tfa), ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)
			So(tfa.VerifiedCulpritKey, ShouldBeNil)
		})

		Convey("suspect verified", func() {
			// ParentSuspect finished running.
			parentRerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
			So(datastore.Put(ctx, parentRerun), ShouldBeNil)

			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			So(datastore.Get(ctx, suspectRerun), ShouldBeNil)
			So(suspectRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_FAILED)
			// Check suspect status.
			So(datastore.Get(ctx, suspect), ShouldBeNil)
			So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_ConfirmedCulprit)
			// Check analysis.
			So(datastore.Get(ctx, tfa), ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(tfa.VerifiedCulpritKey, ShouldEqual, datastore.KeyForObj(ctx, suspect))
		})

		Convey("suspect not verified", func() {
			// ParentSuspect finished running.
			parentRerun.Status = pb.RerunStatus_RERUN_STATUS_FAILED
			So(datastore.Put(ctx, parentRerun), ShouldBeNil)

			err := Update(ctx, req)
			So(err, ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			So(datastore.Get(ctx, suspectRerun), ShouldBeNil)
			So(suspectRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_FAILED)
			// Check suspect status.
			So(datastore.Get(ctx, suspect), ShouldBeNil)
			So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_Vindicated)
			// Check analysis.
			So(datastore.Get(ctx, tfa), ShouldBeNil)
			So(tfa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(tfa.VerifiedCulpritKey, ShouldBeNil)
		})
	})
}

func TestScheduleNewRerun(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)

	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(ctx, ctl)
	ctx = mc.Ctx
	mockBuildBucket(mc, false)

	Convey("Nth section found culprit", t, func() {
		culpritverification.RegisterTaskClass()
		ctx, skdr := tq.TestingContext(ctx, nil)
		tfa, _, rerun, nsa := setupTestAnalysisForTesting(ctx, 1)
		enableBisection(ctx, true, tfa.Project)
		// Commit 1 pass -> commit 0 is the culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit1",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
		So(datastore.Put(ctx, rerun), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err := processNthSectionUpdate(ctx, rerun, tfa, &pb.UpdateTestAnalysisProgressRequest{})
		So(err, ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check suspect being stored.
		q := datastore.NewQuery("Suspect")
		suspects := []*model.Suspect{}
		So(datastore.GetAll(ctx, q, &suspects), ShouldBeNil)
		So(len(suspects), ShouldEqual, 1)
		// Check the field individually because ShouldResembleProto does not work here.
		So(&suspects[0].GitilesCommit, ShouldResembleProto, &bbpb.GitilesCommit{
			Id:      "commit0",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		})
		So(suspects[0].ParentAnalysis, ShouldEqual, datastore.KeyForObj(ctx, nsa))
		So(suspects[0].Type, ShouldEqual, model.SuspectType_NthSection)
		So(suspects[0].AnalysisType, ShouldEqual, pb.AnalysisType_TEST_FAILURE_ANALYSIS)

		// Check nsa.
		So(datastore.Get(ctx, nsa), ShouldBeNil)
		So(nsa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
		So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		So(nsa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())
		So(nsa.CulpritKey, ShouldEqual, datastore.KeyForObj(ctx, suspects[0]))

		// Check tfa.
		So(datastore.Get(ctx, tfa), ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)

		// Culprit verification task scheduled.
		So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
		resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureCulpritVerificationTask)
		So(resultsTask, ShouldResembleProto, &tpb.TestFailureCulpritVerificationTask{
			AnalysisId: tfa.ID,
		})
	})

	Convey("Nth section not found", t, func() {
		tfa, _, rerun, nsa := setupTestAnalysisForTesting(ctx, 1)
		enableBisection(ctx, true, tfa.Project)
		// Commit 1 pass -> commit 0 is the culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit1",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED
		So(datastore.Put(ctx, rerun), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err := processNthSectionUpdate(ctx, rerun, tfa, &pb.UpdateTestAnalysisProgressRequest{})
		So(err, ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check nsa.
		So(datastore.Get(ctx, nsa), ShouldBeNil)
		So(nsa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
		So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		So(nsa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())

		// Check tfa.
		So(datastore.Get(ctx, tfa), ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		So(tfa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())
	})

	Convey("regression range conflicts", t, func() {
		tfa, _, rerun, nsa := setupTestAnalysisForTesting(ctx, 1)
		enableBisection(ctx, true, tfa.Project)
		// Commit 0 pass -> no culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit0",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
		So(datastore.Put(ctx, rerun), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err := processNthSectionUpdate(ctx, rerun, tfa, &pb.UpdateTestAnalysisProgressRequest{})
		So(err, ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check nsa.
		So(datastore.Get(ctx, nsa), ShouldBeNil)
		So(nsa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
		So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		So(nsa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())

		// Check tfa.
		So(datastore.Get(ctx, tfa), ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		So(tfa.EndTime, ShouldEqual, time.Unix(10000, 0).UTC())
	})

	Convey("Nth section should schedule another run", t, func() {
		mc := buildbucket.NewMockedClient(ctx, ctl)
		ctx = mc.Ctx
		mockBuildBucket(mc, true)
		tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, 1)
		enableBisection(ctx, true, tfa.Project)
		// Commit 1 pass -> commit 0 is the culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit2",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
		So(datastore.Put(ctx, rerun), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		req := &pb.UpdateTestAnalysisProgressRequest{
			BotId: "bot",
		}
		err := processNthSectionUpdate(ctx, rerun, tfa, req)
		So(err, ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check that a new rerun is scheduled.
		q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa)).Eq("status", pb.RerunStatus_RERUN_STATUS_IN_PROGRESS)
		reruns := []*model.TestSingleRerun{}
		So(datastore.GetAll(ctx, q, &reruns), ShouldBeNil)
		So(len(reruns), ShouldEqual, 1)
		So(reruns[0], ShouldResembleProto, &model.TestSingleRerun{
			ID:                    reruns[0].ID,
			Type:                  model.RerunBuildType_NthSection,
			AnalysisKey:           datastore.KeyForObj(ctx, tfa),
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			LUCIBuild: model.LUCIBuild{
				BuildID:     8765,
				Project:     "chromium",
				Bucket:      "findit",
				Builder:     "test-single-revision",
				BuildNumber: 10,
				Status:      bbpb.Status_SCHEDULED,
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "refs/heads/main",
					Id:      "hash",
				},
				CreateTime: time.Unix(1000, 0),
			},
			Dimensions: &pb.Dimensions{
				Dimensions: []*pb.Dimension{
					{
						Key:   "key",
						Value: "val",
					},
				},
			},
			TestResults: model.RerunTestResults{
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey: datastore.KeyForObj(ctx, tfs[0]),
					},
				},
			},
		})
	})
}

func setupTestAnalysisForTesting(ctx context.Context, numTest int) (*model.TestFailureAnalysis, []*model.TestFailure, *model.TestSingleRerun, *model.TestNthSectionAnalysis) {
	tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
		ID:             100,
		TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 1000),
	})

	nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
		ID:                200,
		ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		BlameList:         testutil.CreateBlamelist(4),
	})

	// Set up test failures
	tfs := make([]*model.TestFailure, numTest)
	results := make([]model.RerunSingleTestResult, numTest)
	for i := 0; i < numTest; i++ {
		isPrimary := (i == 0)
		tfs[i] = testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:          1000 + int64(i),
			IsPrimary:   isPrimary,
			TestID:      fmt.Sprintf("test%d", i),
			VariantHash: fmt.Sprintf("hash%d", i),
			Analysis:    tfa,
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "ref",
					},
				},
			},
		})
		results[i] = model.RerunSingleTestResult{
			TestFailureKey: datastore.KeyForObj(ctx, tfs[i]),
		}
	}
	rerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
		ID:          8000,
		Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		AnalysisKey: datastore.KeyForObj(ctx, tfa),
		Type:        model.RerunBuildType_NthSection,
		TestResult: model.RerunTestResults{
			Results: results,
		},
		NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
	})
	return tfa, tfs, rerun, nsa
}

func enableBisection(ctx context.Context, enabled bool, project string) {
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.TestAnalysisConfig.BisectorEnabled = enabled
	cfg := map[string]*configpb.ProjectConfig{project: projectCfg}
	So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)
}

func mockBuildBucket(mc *buildbucket.MockedClient, withBotID bool) {
	bootstrapProperties := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"bs_key_1": structpb.NewStringValue("bs_val_1"),
		},
	}

	getBuildRes := &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "ci",
			Builder: "linux-test",
		},
		Input: &bbpb.Build_Input{
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"builder_group":         structpb.NewStringValue("buildergroup1"),
					"$bootstrap/properties": structpb.NewStructValue(bootstrapProperties),
					"another_prop":          structpb.NewStringValue("another_val"),
				},
			},
		},
		Infra: &bbpb.BuildInfra{
			Swarming: &bbpb.BuildInfra_Swarming{
				TaskDimensions: []*bbpb.RequestedDimension{
					{
						Key:   "key",
						Value: "val",
					},
				},
			},
		},
	}

	scheduleBuildRes := &bbpb.Build{
		Id: 8765,
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "test-single-revision",
		},
		Number:     10,
		Status:     bbpb.Status_SCHEDULED,
		CreateTime: timestamppb.New(time.Unix(1000, 0)),
		Input: &bbpb.Build_Input{
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "refs/heads/main",
				Id:      "hash",
			},
		},
	}

	mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(getBuildRes, nil).AnyTimes()
	if !withBotID {
		mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(scheduleBuildRes, nil).AnyTimes()
	} else {
		mc.Client.EXPECT().ScheduleBuild(gomock.Any(), proto.MatcherEqual(&bbpb.ScheduleBuildRequest{
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "findit",
				Builder: "test-single-revision",
			},
			Dimensions: []*bbpb.RequestedDimension{
				{
					Key:   "id",
					Value: "bot",
				},
			},
			Tags: []*bbpb.StringPair{
				{
					Key:   "analyzed_build_id",
					Value: "8000",
				},
			},
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "commit1",
				Ref:     "ref",
			},
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"$bootstrap/properties": structpb.NewStructValue(bootstrapProperties),
					"analysis_id":           structpb.NewNumberValue(100),
					"bisection_host":        structpb.NewStringValue("app.appspot.com"),
					"builder_group":         structpb.NewStringValue("buildergroup1"),
					"target_builder": structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"builder": structpb.NewStringValue("linux-test"),
							"group":   structpb.NewStringValue("buildergroup1"),
						},
					}),
					"tests_to_run": structpb.NewListValue(&structpb.ListValue{
						Values: []*structpb.Value{
							structpb.NewStructValue(&structpb.Struct{
								Fields: map[string]*structpb.Value{
									"test_id":         structpb.NewStringValue("test0"),
									"test_name":       structpb.NewStringValue(""),
									"test_suite_name": structpb.NewStringValue(""),
									"variant_hash":    structpb.NewStringValue("hash0"),
								},
							}),
						},
					}),
				},
			},
		}), gomock.Any()).Return(scheduleBuildRes, nil).Times(1)
	}
}
