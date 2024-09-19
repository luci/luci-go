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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCountLatestRevertsCreated(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("No suspects at all", t, func() {
		count, err := CountLatestRevertsCreated(c, 24, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 0)
	})

	Convey("Count of recent created reverts", t, func() {
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

		So(datastore.Put(c, suspect1), ShouldBeNil)
		So(datastore.Put(c, suspect2), ShouldBeNil)
		So(datastore.Put(c, suspect3), ShouldBeNil)
		So(datastore.Put(c, suspect4), ShouldBeNil)
		So(datastore.Put(c, suspect5), ShouldBeNil)
		So(datastore.Put(c, suspect6), ShouldBeNil)
		So(datastore.Put(c, suspect7), ShouldBeNil)
		So(datastore.Put(c, suspect8), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		count, err := CountLatestRevertsCreated(c, 24, pb.AnalysisType_COMPILE_FAILURE_ANALYSIS)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 3)
		count, err = CountLatestRevertsCreated(c, 24, pb.AnalysisType_TEST_FAILURE_ANALYSIS)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 1)
	})
}

func TestCountLatestRevertsCommitted(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	// Set test clock
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("No suspects at all", t, func() {
		count, err := CountLatestRevertsCommitted(c, 24)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 0)
	})

	Convey("Count of recent committed reverts", t, func() {
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
		So(datastore.Put(c, suspect1), ShouldBeNil)
		So(datastore.Put(c, suspect2), ShouldBeNil)
		So(datastore.Put(c, suspect3), ShouldBeNil)
		So(datastore.Put(c, suspect4), ShouldBeNil)
		So(datastore.Put(c, suspect5), ShouldBeNil)
		So(datastore.Put(c, suspect6), ShouldBeNil)
		So(datastore.Put(c, suspect7), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		count, err := CountLatestRevertsCommitted(c, 24)
		So(err, ShouldBeNil)
		So(count, ShouldEqual, 3)
	})
}

func TestGetAssociatedBuildID(t *testing.T) {
	ctx := memory.Use(context.Background())

	Convey("Associated failed build ID for heuristic suspect", t, func() {
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
		So(datastore.Put(ctx, failedBuild), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		compileFailure := &model.CompileFailure{
			Build: datastore.KeyForObj(ctx, failedBuild),
		}
		So(datastore.Put(ctx, compileFailure), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		analysis := &model.CompileFailureAnalysis{
			Id:             444,
			CompileFailure: datastore.KeyForObj(ctx, compileFailure),
		}
		So(datastore.Put(ctx, analysis), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(ctx, analysis),
		}
		So(datastore.Put(ctx, heuristicAnalysis), ShouldBeNil)
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
		So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		bbid, err := GetAssociatedBuildID(ctx, heuristicSuspect)
		So(err, ShouldBeNil)
		So(bbid, ShouldEqual, 88128398584903)
	})
}

func TestGetSuspect(t *testing.T) {
	ctx := memory.Use(context.Background())

	Convey("GetSuspect", t, func() {
		// Setup datastore
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 123,
		}
		So(datastore.Put(ctx, compileFailureAnalysis), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		parentAnalysis := datastore.KeyForObj(ctx, compileFailureAnalysis)

		compileHeuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id:             45600001,
			ParentAnalysis: parentAnalysis,
		}
		So(datastore.Put(ctx, compileHeuristicAnalysis), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		Convey("no suspect exists", func() {
			suspect, err := GetSuspect(ctx, 789, parentAnalysis)
			So(err, ShouldNotBeNil)
			So(suspect, ShouldBeNil)
		})

		Convey("suspect exists", func() {
			// Setup suspect in datastore
			s := &model.Suspect{
				Id:             789,
				ParentAnalysis: parentAnalysis,
			}
			So(datastore.Put(ctx, s), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			suspect, err := GetSuspect(ctx, 789, parentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldResemble, s)
		})
	})
}

func TestFetchSuspectsForAnalysis(t *testing.T) {
	c := memory.Use(context.Background())

	Convey("FetchSuspectsForAnalysis", t, func() {
		_, _, cfa := testutil.CreateCompileFailureAnalysisAnalysisChain(c, 8000, "chromium", 555)
		suspects, err := FetchSuspectsForAnalysis(c, cfa)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 0)

		ha := testutil.CreateHeuristicAnalysis(c, cfa)
		testutil.CreateHeuristicSuspect(c, ha, model.SuspectVerificationStatus_Unverified)

		suspects, err = FetchSuspectsForAnalysis(c, cfa)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 1)

		nsa := testutil.CreateNthSectionAnalysis(c, cfa)
		testutil.CreateNthSectionSuspect(c, nsa)

		suspects, err = FetchSuspectsForAnalysis(c, cfa)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 2)
	})
}

func TestGetSuspectForTestAnalysis(t *testing.T) {
	ctx := memory.Use(context.Background())

	Convey("GetSuspectForTestAnalysis", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, nil)
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		s, err := GetSuspectForTestAnalysis(ctx, tfa)
		So(err, ShouldBeNil)
		So(s, ShouldBeNil)

		testutil.CreateSuspect(ctx, &testutil.SuspectCreationOption{
			ID:        300,
			ParentKey: datastore.KeyForObj(ctx, nsa),
		})
		s, err = GetSuspectForTestAnalysis(ctx, tfa)
		So(err, ShouldBeNil)
		So(s.Id, ShouldEqual, 300)
	})
}

func TestFetchTestFailuresForSuspect(t *testing.T) {
	ctx := memory.Use(context.Background())

	Convey("FetchTestFailuresForSuspect", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			Project:        "chromium",
			TestFailureKey: datastore.NewKey(ctx, "TestFailure", "", 1, nil),
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		suspect := &model.Suspect{
			Id:             100,
			Type:           model.SuspectType_NthSection,
			ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			AnalysisType:   pb.AnalysisType_TEST_FAILURE_ANALYSIS,
		}
		So(datastore.Put(ctx, suspect), ShouldBeNil)
		tf1 := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID: 1, IsPrimary: true, Analysis: tfa,
		})
		tf2 := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{ID: 2, Analysis: tfa})

		bundle, err := FetchTestFailuresForSuspect(ctx, suspect)
		So(err, ShouldBeNil)
		So(len(bundle.All()), ShouldEqual, 2)
		So(bundle.Primary(), ShouldResembleProto, tf1)
		So(bundle.Others()[0], ShouldResembleProto, tf2)
	})
}

func TestGetTestFailureAnalysisForSuspect(t *testing.T) {
	ctx := memory.Use(context.Background())

	Convey("GetTestFailureAnalysisForSuspect", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			Project:        "chromium",
			TestFailureKey: datastore.NewKey(ctx, "TestFailure", "", 1, nil),
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		suspect := &model.Suspect{
			Id:             100,
			Type:           model.SuspectType_NthSection,
			ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			AnalysisType:   pb.AnalysisType_TEST_FAILURE_ANALYSIS,
		}

		res, err := GetTestFailureAnalysisForSuspect(ctx, suspect)
		So(err, ShouldBeNil)
		So(res, ShouldResembleProto, tfa)
	})
}

func TestGetVerifiedCulprit(t *testing.T) {
	ctx := memory.Use(context.Background())

	Convey("Get verified culprit", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, nil)
		culprit, err := GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		So(err, ShouldBeNil)
		So(culprit, ShouldBeNil)

		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})

		suspect := testutil.CreateSuspect(ctx, &testutil.SuspectCreationOption{
			ID:        300,
			ParentKey: datastore.KeyForObj(ctx, nsa),
		})
		tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, suspect)
		So(datastore.Put(ctx, tfa), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		culprit, err = GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		So(err, ShouldBeNil)
		So(culprit.Id, ShouldEqual, 300)
	})
}
