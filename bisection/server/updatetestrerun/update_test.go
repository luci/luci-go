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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestUpdate(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	ftt.Run("Invalid request", t, func(t *ftt.Test) {
		req := &pb.UpdateTestAnalysisProgressRequest{}
		err := Update(ctx, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))

		req.Bbid = 888
		err = Update(ctx, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))

		req.Bbid = 0
		req.BotId = ""
		err = Update(ctx, req)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))
	})

	ftt.Run("Update", t, func(t *ftt.Test) {
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

		ctx = hosts.UseHosts(ctx, hosts.ModuleOptions{
			APIHost: "test-bisection-host",
		})

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(ctx, ctl)
		ctx = mc.Ctx
		mockBuildBucket(mc, false)

		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID: 100,
		})

		t.Run("No rerun", func(t *ftt.Test) {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  800,
				BotId: "bot",
			}
			err := Update(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.NotFound))
		})

		t.Run("Rerun ended", func(t *ftt.Test) {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  801,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
				ID:     801,
				Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			})
			err := Update(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.Internal))
			assert.Loosely(t, err.Error(), should.ContainSubstring("rerun has ended"))
		})

		t.Run("Invalid rerun type", func(t *ftt.Test) {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  802,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
				ID:          802,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
			})

			err := Update(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.Internal))
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid rerun type"))
		})

		t.Run("No analysis", func(t *ftt.Test) {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:  803,
				BotId: "bot",
			}
			testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
				ID:          803,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.MakeKey(ctx, "TestFailureAnalysis", 1),
				Type:        model.RerunBuildType_CulpritVerification,
			})

			err := Update(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.Internal))
			assert.Loosely(t, err.Error(), should.ContainSubstring("get test failure analysis"))
		})

		t.Run("Tests did not run", func(t *ftt.Test) {
			tfa, _, rerun, _ := setupTestAnalysisForTesting(ctx, t, 1)
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         8000,
				BotId:        "bot",
				RunSucceeded: false,
			}
			enableBisection(ctx, t, true, tfa.Project)

			err := Update(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rerun.ReportTime, should.Match(time.Unix(10000, 0).UTC()))
			assert.Loosely(t, rerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_INFRA_FAILED))
		})

		t.Run("No primary test failure", func(t *ftt.Test) {
			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         805,
				BotId:        "bot",
				RunSucceeded: true,
			}
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
				ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			})
			rerun := testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
				ID:          805,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
				Type:        model.RerunBuildType_NthSection,
			})
			err := Update(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.Internal))
			assert.Loosely(t, err.Error(), should.ContainSubstring("get primary test failure"))

			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rerun.ReportTime, should.Match(time.Unix(10000, 0).UTC()))
			assert.Loosely(t, rerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_INFRA_FAILED))

			assert.Loosely(t, datastore.Get(ctx, nsa), should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_ERROR))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.That(t, nsa.EndTime, should.Match(time.Unix(10000, 0).UTC()))

			assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_ERROR))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.That(t, tfa.EndTime, should.Match(time.Unix(10000, 0).UTC()))
		})

		t.Run("No result for primary failure", func(t *ftt.Test) {
			tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
				ID:             106,
				TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 1061),
			})
			nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
				ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			})

			req := &pb.UpdateTestAnalysisProgressRequest{
				Bbid:         806,
				BotId:        "bot",
				RunSucceeded: true,
			}
			rerun := testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
				ID:          806,
				Status:      pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				AnalysisKey: datastore.KeyForObj(ctx, tfa),
				Type:        model.RerunBuildType_NthSection,
			})

			// Set up test failures
			testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
				ID:          1061,
				IsPrimary:   true,
				TestID:      "test1",
				VariantHash: "hash1",
				Analysis:    tfa,
			})
			testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
				ID:          1062,
				TestID:      "test2",
				VariantHash: "hash2",
				Analysis:    tfa,
			})

			err := Update(ctx, req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, status.Convert(err).Code(), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err.Error(), should.ContainSubstring("no result for primary failure"))
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rerun.ReportTime, should.Match(time.Unix(10000, 0).UTC()))
			assert.Loosely(t, rerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_INFRA_FAILED))

			assert.Loosely(t, datastore.Get(ctx, nsa), should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_ERROR))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.That(t, nsa.EndTime, should.Match(time.Unix(10000, 0).UTC()))

			assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_ERROR))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.That(t, tfa.EndTime, should.Match(time.Unix(10000, 0).UTC()))
		})

		t.Run("Primary test failure skipped", func(t *ftt.Test) {
			tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, t, 2)
			enableBisection(ctx, t, false, tfa.Project)

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
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rerun.ReportTime, should.Match(time.Unix(10000, 0).UTC()))
			assert.Loosely(t, rerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED))
			assert.Loosely(t, rerun.TestResults, should.Resemble(model.RerunTestResults{
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
			}))
			err = datastore.Get(ctx, tfs[1])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfs[1].IsDiverged, should.BeFalse)
			// Check that a new rerun is not scheduled, because primary test was skipped.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			assert.Loosely(t, datastore.GetAll(ctx, q, &reruns), should.BeNil)
			assert.Loosely(t, len(reruns), should.Equal(1))
		})

		t.Run("Primary test failure expected", func(t *ftt.Test) {
			tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, t, 4)
			enableBisection(ctx, t, true, tfa.Project)

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
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rerun.ReportTime, should.Match(time.Unix(10000, 0).UTC()))
			assert.Loosely(t, rerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_PASSED))
			assert.Loosely(t, rerun.TestResults, should.Resemble(model.RerunTestResults{
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
			}))
			err = datastore.Get(ctx, tfs[1])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfs[1].IsDiverged, should.BeFalse)
			err = datastore.Get(ctx, tfs[2])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfs[2].IsDiverged, should.BeTrue)
			err = datastore.Get(ctx, tfs[3])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfs[3].IsDiverged, should.BeTrue)
			// Check that a new rerun is scheduled.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			assert.Loosely(t, datastore.GetAll(ctx, q, &reruns), should.BeNil)
			assert.Loosely(t, len(reruns), should.Equal(2))
		})

		t.Run("Primary test failure unexpected", func(t *ftt.Test) {
			tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, t, 4)
			enableBisection(ctx, t, true, tfa.Project)

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
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			err = datastore.Get(ctx, rerun)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, rerun.ReportTime, should.Match(time.Unix(10000, 0).UTC()))
			assert.Loosely(t, rerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_FAILED))
			assert.Loosely(t, rerun.TestResults, should.Resemble(model.RerunTestResults{
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
			}))
			err = datastore.Get(ctx, tfs[1])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfs[1].IsDiverged, should.BeTrue)
			err = datastore.Get(ctx, tfs[2])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfs[2].IsDiverged, should.BeFalse)
			err = datastore.Get(ctx, tfs[3])
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tfs[3].IsDiverged, should.BeTrue)
			// Check that a new rerun is scheduled.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			assert.Loosely(t, datastore.GetAll(ctx, q, &reruns), should.BeNil)
			assert.Loosely(t, len(reruns), should.Equal(2))
		})

		t.Run("Ended nthsection should not get updated", func(t *ftt.Test) {
			tfa, _, _, nsa := setupTestAnalysisForTesting(ctx, t, 1)
			enableBisection(ctx, t, true, tfa.Project)

			// Set nthsection to end.
			nsa.RunStatus = pb.AnalysisRunStatus_ENDED
			assert.Loosely(t, datastore.Put(ctx, nsa), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Check that a new rerun is not scheduled.
			q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
			reruns := []*model.TestSingleRerun{}
			assert.Loosely(t, datastore.GetAll(ctx, q, &reruns), should.BeNil)
			// 1 because of the rerun created in setupTestAnalysisForTesting.
			assert.Loosely(t, len(reruns), should.Equal(1))

			// Check that no suspect is created.
			q = datastore.NewQuery("Suspect")
			suspects := []*model.Suspect{}
			assert.Loosely(t, datastore.GetAll(ctx, q, &suspects), should.BeNil)
			assert.Loosely(t, len(suspects), should.BeZero)
		})
	})

	ftt.Run("process culprit verification update", t, func(t *ftt.Test) {
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		// set up tfa, rerun, suspect
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:             100,
			TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 1000),
			Status:         pb.AnalysisStatus_SUSPECTFOUND,
			RunStatus:      pb.AnalysisRunStatus_STARTED,
		})
		testFailure := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:          1000,
			IsPrimary:   true,
			TestID:      "test0",
			VariantHash: "hash0",
			Analysis:    tfa,
		})
		suspect := testutil.CreateSuspect(ctx, t, nil)
		suspectRerun := testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
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
		parentRerun := testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
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
		assert.Loosely(t, datastore.Put(ctx, suspect), should.BeNil)
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
		t.Run("suspect under verification", func(t *ftt.Test) {
			err := Update(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			assert.Loosely(t, datastore.Get(ctx, suspectRerun), should.BeNil)
			assert.Loosely(t, suspectRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_FAILED))
			// Check suspect status.
			assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_UnderVerification))
			// Check analysis - no update.
			assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))
			assert.Loosely(t, tfa.VerifiedCulpritKey, should.BeNil)
		})

		t.Run("suspect verified", func(t *ftt.Test) {
			// ParentSuspect finished running.
			parentRerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
			assert.Loosely(t, datastore.Put(ctx, parentRerun), should.BeNil)

			err := Update(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			assert.Loosely(t, datastore.Get(ctx, suspectRerun), should.BeNil)
			assert.Loosely(t, suspectRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_FAILED))
			// Check suspect status.
			assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_ConfirmedCulprit))
			// Check analysis.
			assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, tfa.VerifiedCulpritKey, should.Match(datastore.KeyForObj(ctx, suspect)))
		})

		t.Run("suspect not verified", func(t *ftt.Test) {
			// ParentSuspect finished running.
			parentRerun.Status = pb.RerunStatus_RERUN_STATUS_FAILED
			assert.Loosely(t, datastore.Put(ctx, parentRerun), should.BeNil)

			err := Update(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			assert.Loosely(t, datastore.Get(ctx, suspectRerun), should.BeNil)
			assert.Loosely(t, suspectRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_FAILED))
			// Check suspect status.
			assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_Vindicated))
			// Check analysis.
			assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
			assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, tfa.VerifiedCulpritKey, should.BeNil)
		})
	})
}

func TestScheduleNewRerun(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ctx = hosts.UseHosts(ctx, hosts.ModuleOptions{
		APIHost: "test-bisection-host",
	})

	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)

	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(ctx, ctl)
	ctx = mc.Ctx
	mockBuildBucket(mc, false)

	ftt.Run("Nth section found culprit", t, func(t *ftt.Test) {
		culpritverification.RegisterTaskClass()
		ctx, skdr := tq.TestingContext(ctx, nil)
		tfa, _, rerun, nsa := setupTestAnalysisForTesting(ctx, t, 1)
		enableBisection(ctx, t, true, tfa.Project)
		// Commit 1 pass -> commit 0 is the culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit1",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
		assert.Loosely(t, datastore.Put(ctx, rerun), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err := processNthSectionUpdate(ctx, rerun, tfa, &pb.UpdateTestAnalysisProgressRequest{})
		assert.Loosely(t, err, should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check suspect being stored.
		q := datastore.NewQuery("Suspect")
		suspects := []*model.Suspect{}
		assert.Loosely(t, datastore.GetAll(ctx, q, &suspects), should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(1))
		// Check the field individually because ShouldResembleProto does not work here.
		assert.Loosely(t, &suspects[0].GitilesCommit, should.Resemble(&bbpb.GitilesCommit{
			Id:      "commit0",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}))
		assert.Loosely(t, suspects[0].ParentAnalysis, should.Match(datastore.KeyForObj(ctx, nsa)))
		assert.Loosely(t, suspects[0].Type, should.Equal(model.SuspectType_NthSection))
		assert.Loosely(t, suspects[0].AnalysisType, should.Equal(pb.AnalysisType_TEST_FAILURE_ANALYSIS))

		// Check nsa.
		assert.Loosely(t, datastore.Get(ctx, nsa), should.BeNil)
		assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
		assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		assert.That(t, nsa.EndTime, should.Match(time.Unix(10000, 0).UTC()))
		assert.That(t, nsa.CulpritKey, should.Match(datastore.KeyForObj(ctx, suspects[0])))

		// Check tfa.
		assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))

		// Culprit verification task scheduled.
		assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1))
		resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureCulpritVerificationTask)
		assert.Loosely(t, resultsTask, should.Resemble(&tpb.TestFailureCulpritVerificationTask{
			AnalysisId: tfa.ID,
		}))
	})

	ftt.Run("Nth section not found", t, func(t *ftt.Test) {
		tfa, _, rerun, nsa := setupTestAnalysisForTesting(ctx, t, 1)
		enableBisection(ctx, t, true, tfa.Project)
		// Commit 1 pass -> commit 0 is the culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit1",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED
		assert.Loosely(t, datastore.Put(ctx, rerun), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err := processNthSectionUpdate(ctx, rerun, tfa, &pb.UpdateTestAnalysisProgressRequest{})
		assert.Loosely(t, err, should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check nsa.
		assert.Loosely(t, datastore.Get(ctx, nsa), should.BeNil)
		assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
		assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		assert.That(t, nsa.EndTime, should.Match(time.Unix(10000, 0).UTC()))

		// Check tfa.
		assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		assert.That(t, tfa.EndTime, should.Match(time.Unix(10000, 0).UTC()))
	})

	ftt.Run("regression range conflicts", t, func(t *ftt.Test) {
		tfa, _, rerun, nsa := setupTestAnalysisForTesting(ctx, t, 1)
		enableBisection(ctx, t, true, tfa.Project)
		// Commit 0 pass -> no culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit0",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
		assert.Loosely(t, datastore.Put(ctx, rerun), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err := processNthSectionUpdate(ctx, rerun, tfa, &pb.UpdateTestAnalysisProgressRequest{})
		assert.Loosely(t, err, should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check nsa.
		assert.Loosely(t, datastore.Get(ctx, nsa), should.BeNil)
		assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
		assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		assert.That(t, nsa.EndTime, should.Match(time.Unix(10000, 0).UTC()))

		// Check tfa.
		assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		assert.That(t, tfa.EndTime, should.Match(time.Unix(10000, 0).UTC()))
	})

	ftt.Run("Nth section should schedule another run", t, func(t *ftt.Test) {
		mc := buildbucket.NewMockedClient(ctx, ctl)
		ctx = mc.Ctx
		mockBuildBucket(mc, true)
		tfa, tfs, rerun, nsa := setupTestAnalysisForTesting(ctx, t, 1)
		enableBisection(ctx, t, true, tfa.Project)
		// Commit 1 pass -> commit 0 is the culprit.
		rerun.LUCIBuild.GitilesCommit = &bbpb.GitilesCommit{
			Id:      "commit2",
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "ref",
		}
		rerun.Status = pb.RerunStatus_RERUN_STATUS_PASSED
		assert.Loosely(t, datastore.Put(ctx, rerun), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		req := &pb.UpdateTestAnalysisProgressRequest{
			BotId: "bot",
		}
		err := processNthSectionUpdate(ctx, rerun, tfa, req)
		assert.Loosely(t, err, should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Check that a new rerun is scheduled.
		q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa)).Eq("status", pb.RerunStatus_RERUN_STATUS_IN_PROGRESS)
		reruns := []*model.TestSingleRerun{}
		assert.Loosely(t, datastore.GetAll(ctx, q, &reruns), should.BeNil)
		assert.Loosely(t, len(reruns), should.Equal(1))
		assert.Loosely(t, reruns[0], should.Resemble(&model.TestSingleRerun{
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
		}))
	})
}

func setupTestAnalysisForTesting(ctx context.Context, t testing.TB, numTest int) (*model.TestFailureAnalysis, []*model.TestFailure, *model.TestSingleRerun, *model.TestNthSectionAnalysis) {
	t.Helper()
	tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
		ID:             100,
		TestFailureKey: datastore.MakeKey(ctx, "TestFailure", 1000),
	})

	nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
		ID:                200,
		ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		BlameList:         testutil.CreateBlamelist(4),
	})

	// Set up test failures
	tfs := make([]*model.TestFailure, numTest)
	results := make([]model.RerunSingleTestResult, numTest)
	for i := 0; i < numTest; i++ {
		isPrimary := (i == 0)
		tfs[i] = testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
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
	rerun := testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
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

func enableBisection(ctx context.Context, t testing.TB, enabled bool, project string) {
	t.Helper()
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.TestAnalysisConfig.BisectorEnabled = enabled
	cfg := map[string]*configpb.ProjectConfig{project: projectCfg}
	assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil, truth.LineContext())
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
					"bisection_host":        structpb.NewStringValue("test-bisection-host"),
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
