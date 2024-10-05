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

package datastoreutil

import (
	"context"
	"testing"
	"time"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestCountLatestRevertsCreated(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("No suspects at all", t, func(t *ftt.Test) {
		count, err := CountLatestRevertsCreated(c, 24, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.BeZero)
	})

	ftt.Run("Count of recent created reverts", t, func(t *ftt.Test) {
		// Set up suspects
		suspect1 := &model.Suspect{}
		suspect2 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100000",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c),
			},
			AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		suspect3 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100001",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Hour * 72),
			},
			AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		suspect4 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100002",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Hour * 4),
			},
			AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		suspect5 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100003",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Minute * 10),
			},
			AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		suspect6 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				RevertURL:        "https://chromium-test-review.googlesource.com/c/test/project/+/100004",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Hour * 24),
			},
			AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		suspect7 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				RevertURL:        "",
				IsRevertCreated:  false,
				RevertCreateTime: clock.Now(c).Add(-time.Minute * 10),
			},
			AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		suspect8 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				RevertURL:        "",
				IsRevertCreated:  true,
				RevertCreateTime: clock.Now(c).Add(-time.Minute * 10),
			},
			AnalysisType: pb.AnalysisType_TEST_FAILURE_ANALYSIS,
		}

		assert.Loosely(t, datastore.Put(c, suspect1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect2), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect3), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect4), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect5), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect6), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect7), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect8), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		count, err := CountLatestRevertsCreated(c, 24, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.Equal(3))
		count, err = CountLatestRevertsCreated(c, 24, pb.AnalysisType_TEST_FAILURE_ANALYSIS)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.Equal(1))
	})
}

func TestCountLatestRevertsCommitted(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("No suspects at all", t, func(t *ftt.Test) {
		count, err := CountLatestRevertsCommitted(c, 24)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.BeZero)
	})

	ftt.Run("Count of recent committed reverts", t, func(t *ftt.Test) {
		// Set up suspects
		suspect1 := &model.Suspect{}
		suspect2 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c),
			},
		}
		suspect3 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Hour * 72),
			},
		}
		suspect4 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Hour * 4),
			},
		}
		suspect5 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Minute * 10),
			},
		}
		suspect6 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				IsRevertCommitted: true,
				RevertCommitTime:  clock.Now(c).Add(-time.Hour * 24),
			},
		}
		suspect7 := &model.Suspect{
			ActionDetails: model.ActionDetails{
				IsRevertCommitted: false,
				RevertCommitTime:  clock.Now(c).Add(-time.Minute * 10),
			},
		}
		assert.Loosely(t, datastore.Put(c, suspect1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect2), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect3), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect4), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect5), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect6), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect7), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		count, err := CountLatestRevertsCommitted(c, 24)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, count, should.Equal(3))
	})
}

func TestGetAssociatedBuildID(t *testing.T) {
	ctx := memory.Use(context.Background())

	ftt.Run("Associated failed build ID for heuristic suspect", t, func(t *ftt.Test) {
		failedBuild := &model.LuciFailedBuild{
			Id: 88128398584903,
			LuciBuild: model.LuciBuild{
				BuildId:     88128398584903,
				Project:     "chromium",
				Bucket:      "ci",
				Builder:     "android",
				BuildNumber: 123,
			},
			BuildFailureType: pb.BuildFailureType_COMPILE,
		}
		assert.Loosely(t, datastore.Put(ctx, failedBuild), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		compileFailure := &model.CompileFailure{
			Build: datastore.KeyForObj(ctx, failedBuild),
		}
		assert.Loosely(t, datastore.Put(ctx, compileFailure), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		analysis := &model.CompileFailureAnalysis{
			Id:             444,
			CompileFailure: datastore.KeyForObj(ctx, compileFailure),
		}
		assert.Loosely(t, datastore.Put(ctx, analysis), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(ctx, analysis),
		}
		assert.Loosely(t, datastore.Put(ctx, heuristicAnalysis), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		heuristicSuspect := &model.Suspect{
			Id:             1,
			Type:           model.SuspectType_Heuristic,
			Score:          10,
			ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "test.googlesource.com",
				Project: "chromium/test",
				Id:      "12ab34cd56ef",
			},
			ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
			VerificationStatus: model.SuspectVerificationStatus_UnderVerification,
			AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		assert.Loosely(t, datastore.Put(ctx, heuristicSuspect), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		bbid, err := GetAssociatedBuildID(ctx, heuristicSuspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, bbid, should.Equal(88128398584903))
	})
}

func TestGetSuspect(t *testing.T) {
	ctx := memory.Use(context.Background())

	ftt.Run("GetSuspect", t, func(t *ftt.Test) {
		// Setup datastore
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 123,
		}
		assert.Loosely(t, datastore.Put(ctx, compileFailureAnalysis), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		parentAnalysis := datastore.KeyForObj(ctx, compileFailureAnalysis)

		compileHeuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id:             45600001,
			ParentAnalysis: parentAnalysis,
		}
		assert.Loosely(t, datastore.Put(ctx, compileHeuristicAnalysis), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		t.Run("no suspect exists", func(t *ftt.Test) {
			suspect, err := GetSuspect(ctx, 789, parentAnalysis)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, suspect, should.BeNil)
		})

		t.Run("suspect exists", func(t *ftt.Test) {
			// Setup suspect in datastore
			s := &model.Suspect{
				Id:             789,
				ParentAnalysis: parentAnalysis,
			}
			assert.Loosely(t, datastore.Put(ctx, s), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			suspect, err := GetSuspect(ctx, 789, parentAnalysis)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, suspect, should.Resemble(s))
		})
	})
}

func TestFetchSuspectsForAnalysis(t *testing.T) {
	c := memory.Use(context.Background())

	ftt.Run("FetchSuspectsForAnalysis", t, func(t *ftt.Test) {
		_, _, cfa := testutil.CreateCompileFailureAnalysisAnalysisChain(c, t, 8000, "chromium", 555)
		suspects, err := FetchSuspectsForAnalysis(c, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.BeZero)

		ha := testutil.CreateHeuristicAnalysis(c, t, cfa)
		testutil.CreateHeuristicSuspect(c, t, ha, model.SuspectVerificationStatus_Unverified)

		suspects, err = FetchSuspectsForAnalysis(c, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(1))

		nsa := testutil.CreateNthSectionAnalysis(c, t, cfa)
		testutil.CreateNthSectionSuspect(c, t, nsa)

		suspects, err = FetchSuspectsForAnalysis(c, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(2))
	})
}

func TestGetSuspectForTestAnalysis(t *testing.T) {
	ctx := memory.Use(context.Background())

	ftt.Run("GetSuspectForTestAnalysis", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, nil)
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		s, err := GetSuspectForTestAnalysis(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.BeNil)

		testutil.CreateSuspect(ctx, t, &testutil.SuspectCreationOption{
			ID:        300,
			ParentKey: datastore.KeyForObj(ctx, nsa),
		})
		s, err = GetSuspectForTestAnalysis(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s.Id, should.Equal(300))
	})
}

func TestFetchTestFailuresForSuspect(t *testing.T) {
	ctx := memory.Use(context.Background())

	ftt.Run("FetchTestFailuresForSuspect", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			Project:        "chromium",
			TestFailureKey: datastore.NewKey(ctx, "TestFailure", "", 1, nil),
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		suspect := &model.Suspect{
			Id:             100,
			Type:           model.SuspectType_NthSection,
			ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			AnalysisType:   pb.AnalysisType_TEST_FAILURE_ANALYSIS,
		}
		assert.Loosely(t, datastore.Put(ctx, suspect), should.BeNil)
		tf1 := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID: 1, IsPrimary: true, Analysis: tfa,
		})
		tf2 := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{ID: 2, Analysis: tfa})

		bundle, err := FetchTestFailuresForSuspect(ctx, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(bundle.All()), should.Equal(2))
		assert.Loosely(t, bundle.Primary(), should.Resemble(tf1))
		assert.Loosely(t, bundle.Others()[0], should.Resemble(tf2))
	})
}

func TestGetTestFailureAnalysisForSuspect(t *testing.T) {
	ctx := memory.Use(context.Background())

	ftt.Run("GetTestFailureAnalysisForSuspect", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			Project:        "chromium",
			TestFailureKey: datastore.NewKey(ctx, "TestFailure", "", 1, nil),
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		suspect := &model.Suspect{
			Id:             100,
			Type:           model.SuspectType_NthSection,
			ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			AnalysisType:   pb.AnalysisType_TEST_FAILURE_ANALYSIS,
		}

		res, err := GetTestFailureAnalysisForSuspect(ctx, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, res, should.Resemble(tfa))
	})
}

func TestGetVerifiedCulprit(t *testing.T) {
	ctx := memory.Use(context.Background())

	ftt.Run("Get verified culprit", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, nil)
		culprit, err := GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, culprit, should.BeNil)

		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})

		suspect := testutil.CreateSuspect(ctx, t, &testutil.SuspectCreationOption{
			ID:        300,
			ParentKey: datastore.KeyForObj(ctx, nsa),
		})
		tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, suspect)
		assert.Loosely(t, datastore.Put(ctx, tfa), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		culprit, err = GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, culprit.Id, should.Equal(300))
	})
}
