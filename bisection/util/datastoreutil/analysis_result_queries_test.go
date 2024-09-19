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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetBuild(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	Convey("No build found", t, func() {
		buildModel, err := GetBuild(c, 100)
		So(err, ShouldBeNil)
		So(buildModel, ShouldBeNil)
	})

	Convey("Build found", t, func() {
		// Prepare datastore
		failed_build := &model.LuciFailedBuild{
			Id: 101,
		}
		So(datastore.Put(c, failed_build), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		buildModel, err := GetBuild(c, 101)
		So(err, ShouldBeNil)
		So(buildModel, ShouldNotBeNil)
		So(buildModel.Id, ShouldEqual, 101)
	})
}

func TestGetAnalysisForBuild(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	Convey("No build found", t, func() {
		analysis, err := GetAnalysisForBuild(c, 100)
		So(err, ShouldBeNil)
		So(analysis, ShouldBeNil)
	})

	Convey("No analysis found", t, func() {
		// Prepare datastore
		failedBuild := &model.LuciFailedBuild{
			Id: 101,
		}
		So(datastore.Put(c, failedBuild), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		analysis, err := GetAnalysisForBuild(c, 101)
		So(err, ShouldBeNil)
		So(analysis, ShouldBeNil)
	})

	Convey("Analysis found", t, func() {
		// Prepare datastore
		failedBuild := &model.LuciFailedBuild{
			Id: 101,
		}
		So(datastore.Put(c, failedBuild), ShouldBeNil)

		compileFailure := &model.CompileFailure{
			Id:    101,
			Build: datastore.KeyForObj(c, failedBuild),
		}
		So(datastore.Put(c, compileFailure), ShouldBeNil)

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id:                 1230001,
			CompileFailure:     datastore.KeyForObj(c, compileFailure),
			FirstFailedBuildId: 101,
		}
		So(datastore.Put(c, compileFailureAnalysis), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		analysis, err := GetAnalysisForBuild(c, 101)
		So(err, ShouldBeNil)
		So(analysis, ShouldNotBeNil)
		So(analysis.Id, ShouldEqual, 1230001)
		So(analysis.FirstFailedBuildId, ShouldEqual, 101)
	})

	Convey("Related analysis found", t, func() {
		// Prepare datastore
		firstFailedBuild := &model.LuciFailedBuild{
			Id: 200,
		}
		So(datastore.Put(c, firstFailedBuild), ShouldBeNil)

		firstCompileFailure := &model.CompileFailure{
			Id:    200,
			Build: datastore.KeyForObj(c, firstFailedBuild),
		}
		So(datastore.Put(c, firstCompileFailure), ShouldBeNil)

		failedBuild := &model.LuciFailedBuild{
			Id: 201,
		}
		So(datastore.Put(c, failedBuild), ShouldBeNil)

		compileFailure := &model.CompileFailure{
			Id:               201,
			Build:            datastore.KeyForObj(c, failedBuild),
			MergedFailureKey: datastore.KeyForObj(c, firstCompileFailure),
		}
		So(datastore.Put(c, compileFailure), ShouldBeNil)

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id:             1230002,
			CompileFailure: datastore.KeyForObj(c, firstCompileFailure),
		}
		So(datastore.Put(c, compileFailureAnalysis), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		analysis, err := GetAnalysisForBuild(c, 201)
		So(err, ShouldBeNil)
		So(analysis, ShouldNotBeNil)
		So(analysis.Id, ShouldEqual, 1230002)
	})
}

func TestGetHeuristicAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	Convey("No heuristic analysis found", t, func() {
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 1230003,
		}
		heuristicAnalysis, err := GetHeuristicAnalysis(c, compileFailureAnalysis)
		So(err, ShouldBeNil)
		So(heuristicAnalysis, ShouldBeNil)
	})

	Convey("Heuristic analysis found", t, func() {
		// Prepare datastore
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 1230003,
		}
		So(datastore.Put(c, compileFailureAnalysis), ShouldBeNil)

		compileHeuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id:             4560001,
			ParentAnalysis: datastore.KeyForObj(c, compileFailureAnalysis),
		}
		So(datastore.Put(c, compileHeuristicAnalysis), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		heuristicAnalysis, err := GetHeuristicAnalysis(c, compileFailureAnalysis)
		So(err, ShouldBeNil)
		So(heuristicAnalysis, ShouldNotBeNil)
		So(heuristicAnalysis.Id, ShouldEqual, 4560001)
	})
}

func TestGetSuspectsForHeuristicAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	datastore.GetTestable(c).AutoIndex(true)

	Convey("No suspects found", t, func() {
		// Prepare datastore
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id: 700,
		}
		So(datastore.Put(c, heuristicAnalysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspects, err := GetSuspectsForHeuristicAnalysis(c, heuristicAnalysis)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 0)
	})

	Convey("All suspects found", t, func() {
		// Prepare datastore
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id: 701,
		}
		So(datastore.Put(c, heuristicAnalysis), ShouldBeNil)

		suspect1 := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, heuristicAnalysis),
			Score:          1,
		}
		suspect2 := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, heuristicAnalysis),
			Score:          3,
		}
		suspect3 := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, heuristicAnalysis),
			Score:          4,
		}
		suspect4 := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, heuristicAnalysis),
			Score:          2,
		}
		So(datastore.Put(c, suspect1), ShouldBeNil)
		So(datastore.Put(c, suspect2), ShouldBeNil)
		So(datastore.Put(c, suspect3), ShouldBeNil)
		So(datastore.Put(c, suspect4), ShouldBeNil)

		// Add a different heuristic analysis with its own suspect
		otherHeuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id: 702,
		}
		So(datastore.Put(c, heuristicAnalysis), ShouldBeNil)
		otherSuspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, otherHeuristicAnalysis),
			Score:          5,
		}
		So(datastore.Put(c, otherSuspect), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		suspects, err := GetSuspectsForHeuristicAnalysis(c, heuristicAnalysis)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 4)
		So(suspects[0].Score, ShouldEqual, 4)
		So(suspects[1].Score, ShouldEqual, 3)
		So(suspects[2].Score, ShouldEqual, 2)
		So(suspects[3].Score, ShouldEqual, 1)
	})
}

func TestGetSuspectForNthSectionAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	Convey("No suspects found", t, func() {
		nthSectionAnalysis := &model.CompileNthSectionAnalysis{}
		So(datastore.Put(c, nthSectionAnalysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect, err := GetSuspectForNthSectionAnalysis(c, nthSectionAnalysis)
		So(err, ShouldBeNil)
		So(suspect, ShouldBeNil)
	})

	Convey("Suspect found", t, func() {
		nthSectionAnalysis := &model.CompileNthSectionAnalysis{}
		So(datastore.Put(c, nthSectionAnalysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, nthSectionAnalysis),
			Id:             123,
		}
		So(datastore.Put(c, suspect), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect, err := GetSuspectForNthSectionAnalysis(c, nthSectionAnalysis)
		So(err, ShouldBeNil)
		So(suspect, ShouldNotBeNil)
		So(suspect.Id, ShouldEqual, 123)
	})
}

func TestGetCompileFailureForAnalysisID(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	datastore.GetTestable(c).AutoIndex(true)

	Convey("No analysis found", t, func() {
		_, err := GetCompileFailureForAnalysisID(c, 100)
		So(err, ShouldNotBeNil)
	})

	Convey("Have analysis for compile failure", t, func() {
		build := &model.LuciFailedBuild{
			Id: 111,
		}
		So(datastore.Put(c, build), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailure := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, build),
		}
		So(datastore.Put(c, compileFailure), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		analysis := &model.CompileFailureAnalysis{
			Id:             456,
			CompileFailure: datastore.KeyForObj(c, compileFailure),
		}
		So(datastore.Put(c, analysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf, err := GetCompileFailureForAnalysisID(c, 456)
		So(err, ShouldBeNil)
		So(cf.Id, ShouldEqual, 123)
	})
}

func TestGetRerunsForRerunBuild(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("GetRerunsForRerunBuild", t, func() {
		// Set up reruns
		rerunBuildModel := &model.CompileRerunBuild{
			Id: 8800,
		}
		So(datastore.Put(c, rerunBuildModel), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		reruns, err := GetRerunsForRerunBuild(c, rerunBuildModel)
		So(err, ShouldBeNil)
		So(len(reruns), ShouldEqual, 0)
		_, err = GetLastRerunForRerunBuild(c, rerunBuildModel)
		So(err, ShouldNotBeNil)

		singleRerun1 := &model.SingleRerun{
			Id:         1,
			RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
			StartTime:  clock.Now(c),
		}

		singleRerun2 := &model.SingleRerun{
			Id:         2,
			RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
			StartTime:  clock.Now(c).Add(time.Hour),
		}

		singleRerun3 := &model.SingleRerun{
			Id:         3,
			RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
			StartTime:  clock.Now(c).Add(time.Minute),
		}

		So(datastore.Put(c, singleRerun1), ShouldBeNil)
		So(datastore.Put(c, singleRerun2), ShouldBeNil)
		So(datastore.Put(c, singleRerun3), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		reruns, err = GetRerunsForRerunBuild(c, rerunBuildModel)
		So(err, ShouldBeNil)
		So(len(reruns), ShouldEqual, 3)
		r, err := GetLastRerunForRerunBuild(c, rerunBuildModel)
		So(err, ShouldBeNil)
		So(r.Id, ShouldEqual, 2)
	})
}

func TestGetNthSectionAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	Convey("No nthsection analysis found", t, func() {
		cfa := &model.CompileFailureAnalysis{
			Id: 123,
		}
		nsa, err := GetNthSectionAnalysis(c, cfa)
		So(err, ShouldBeNil)
		So(nsa, ShouldBeNil)
	})

	Convey("More than one nthsection analysis found", t, func() {
		cfa := &model.CompileFailureAnalysis{
			Id: 456,
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		nsa1 := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		nsa2 := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		So(datastore.Put(c, nsa1), ShouldBeNil)
		So(datastore.Put(c, nsa2), ShouldBeNil)

		_, err := GetNthSectionAnalysis(c, cfa)
		So(err, ShouldNotBeNil)
	})

	Convey("Nthsection analysis found", t, func() {
		cfa := &model.CompileFailureAnalysis{
			Id: 789,
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		nsa1 := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
			Id:             345,
		}
		So(datastore.Put(c, nsa1), ShouldBeNil)
		nsa, err := GetNthSectionAnalysis(c, cfa)
		So(err, ShouldBeNil)
		So(nsa.Id, ShouldEqual, 345)
	})
}

func TestGetCompileFailureAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	Convey("GetCompileFailureAnalysis", t, func() {
		_, err := GetCompileFailureAnalysis(c, 123)
		So(err, ShouldNotBeNil)
		cfa := &model.CompileFailureAnalysis{
			Id: 123,
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		analysis, err := GetCompileFailureAnalysis(c, 123)
		So(err, ShouldBeNil)
		So(analysis.Id, ShouldEqual, 123)
	})
}

func TestGetOtherSuspectsWithSameCL(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	Convey("GetOtherSuspectsWithSameCL", t, func() {
		suspect := &model.Suspect{
			ReviewUrl: "https://this/is/review/url",
			Id:        123,
		}
		So(datastore.Put(c, suspect), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		suspects, err := GetOtherSuspectsWithSameCL(c, suspect)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 0)

		suspect1 := &model.Suspect{
			ReviewUrl: "https://this/is/review/url",
			Id:        124,
		}
		So(datastore.Put(c, suspect1), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		suspects, err = GetOtherSuspectsWithSameCL(c, suspect)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 1)
		So(suspects[0].Id, ShouldEqual, 124)
	})
}

func TestGetLatestBuildFailureAndAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("GetLatestBuildFailureAndAnalysis", t, func() {
		build, err := GetLatestBuildFailureForBuilder(c, "project", "bucket", "builder")
		So(err, ShouldBeNil)
		So(build, ShouldBeNil)
		analysis, err := GetLatestAnalysisForBuilder(c, "project", "bucket", "builder")
		So(err, ShouldBeNil)
		So(analysis, ShouldBeNil)

		bf1 := &model.LuciFailedBuild{
			Id: 123,
			LuciBuild: model.LuciBuild{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
				EndTime: clock.Now(c),
			},
		}

		bf2 := &model.LuciFailedBuild{
			Id: 456,
			LuciBuild: model.LuciBuild{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
				EndTime: clock.Now(c).Add(time.Hour),
			},
		}

		So(datastore.Put(c, bf1), ShouldBeNil)
		So(datastore.Put(c, bf2), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		build, err = GetLatestBuildFailureForBuilder(c, "project", "bucket", "builder")
		So(err, ShouldBeNil)
		So(build.Id, ShouldEqual, 456)

		cf1 := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, bf1),
		}
		So(datastore.Put(c, cf1), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf2 := &model.CompileFailure{
			Id:               456,
			Build:            datastore.KeyForObj(c, bf2),
			MergedFailureKey: datastore.KeyForObj(c, cf1),
		}
		So(datastore.Put(c, cf2), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			Id:             789,
			CompileFailure: datastore.KeyForObj(c, cf1),
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		analysis, err = GetLatestAnalysisForBuilder(c, "project", "bucket", "builder")
		So(err, ShouldBeNil)
		So(analysis.Id, ShouldEqual, 789)
	})
}

func TestGetRerunsForAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("GetRerunsForAnalysis", t, func() {
		cfa := &model.CompileFailureAnalysis{
			Id: 123,
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		So(datastore.Put(c, nsa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		rr1 := &model.SingleRerun{
			Type:     model.RerunBuildType_CulpritVerification,
			Analysis: datastore.KeyForObj(c, cfa),
		}

		rr2 := &model.SingleRerun{
			Id:       333,
			Type:     model.RerunBuildType_NthSection,
			Analysis: datastore.KeyForObj(c, cfa),
		}

		So(datastore.Put(c, rr1), ShouldBeNil)
		So(datastore.Put(c, rr2), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		reruns, err := GetRerunsForAnalysis(c, cfa)
		So(err, ShouldBeNil)
		So(len(reruns), ShouldEqual, 2)

		nsectionReruns, err := GetRerunsForNthSectionAnalysis(c, nsa)
		So(err, ShouldBeNil)
		So(len(nsectionReruns), ShouldEqual, 1)
		So(nsectionReruns[0].Id, ShouldEqual, 333)
	})
}

func TestGetTestFailureAnalysis(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get test failure analysis", t, func() {
		testutil.CreateTestFailureAnalysis(ctx, nil)
		tfa, err := GetTestFailureAnalysis(ctx, 1000)
		So(err, ShouldBeNil)
		So(tfa.ID, ShouldEqual, 1000)
	})
}

func TestGetPrimaryTestFailure(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("No primary failure", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, nil)
		_, err := GetPrimaryTestFailure(ctx, tfa)
		So(err, ShouldNotBeNil)
	})

	Convey("Have primary failure", t, func() {
		tf := testutil.CreateTestFailure(ctx, nil)
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:             1002,
			TestFailureKey: datastore.KeyForObj(ctx, tf),
		})
		tf, err := GetPrimaryTestFailure(ctx, tfa)
		So(err, ShouldBeNil)
		So(tf.ID, ShouldEqual, 100)
	})
}

func TestGetTestFailureBundle(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("No test failures", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, nil)
		_, err := GetTestFailureBundle(ctx, tfa)
		So(err, ShouldNotBeNil)
	})

	Convey("Have test failure", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID: 1002,
		})
		tf1 := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID: 100,
		})
		tf1.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		tf2 := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID: 101,
		})
		tf2.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		So(datastore.Put(ctx, tf1), ShouldBeNil)
		So(datastore.Put(ctx, tf2), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// No primary test failure.
		_, err := GetTestFailureBundle(ctx, tfa)
		So(err, ShouldNotBeNil)

		// Have primary test failure.
		tf2.IsPrimary = true
		So(datastore.Put(ctx, tf2), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		bundle, err := GetTestFailureBundle(ctx, tfa)
		So(err, ShouldBeNil)
		So(bundle.Primary().ID, ShouldEqual, 101)
		others := bundle.Others()
		So(len(others), ShouldEqual, 1)
		So(others[0].ID, ShouldEqual, 100)
	})
}

func TestGetTestSingleRerun(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get test single rerun", t, func() {
		_, err := GetTestSingleRerun(ctx, 123)
		So(err, ShouldNotBeNil)
		testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID: 123,
		})
		rerun, err := GetTestSingleRerun(ctx, 123)
		So(err, ShouldBeNil)
		So(rerun.ID, ShouldEqual, 123)
	})
}

func TestGetTestFailure(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get test failure", t, func() {
		_, err := GetTestFailure(ctx, 123)
		So(err, ShouldNotBeNil)
		testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID: 123,
		})
		rerun, err := GetTestFailure(ctx, 123)
		So(err, ShouldBeNil)
		So(rerun.ID, ShouldEqual, 123)
	})
}

func TestGetTestNthSectionAnalysis(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get test nthsection analysis", t, func() {
		_, err := GetTestNthSectionAnalysis(ctx, 123)
		So(err, ShouldNotBeNil)
		testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ID: 123,
		})
		rerun, err := GetTestNthSectionAnalysis(ctx, 123)
		So(err, ShouldBeNil)
		So(rerun.ID, ShouldEqual, 123)
	})
}

func TestGetTestNthSectionForAnalysis(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get test nthsection analysis", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{})
		nsa, err := GetTestNthSectionForAnalysis(ctx, tfa)
		So(err, ShouldBeNil)
		So(nsa, ShouldBeNil)
		testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ID:                123,
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		nsa, err = GetTestNthSectionForAnalysis(ctx, tfa)
		So(err, ShouldBeNil)
		So(nsa.ID, ShouldEqual, 123)
	})
}

func TestGetInProgressReruns(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get in progress rerun", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{})
		testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID:          100,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			Type:        model.RerunBuildType_CulpritVerification,
			Status:      bisectionpb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		})
		testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID:          101,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			Type:        model.RerunBuildType_NthSection,
			Status:      bisectionpb.RerunStatus_RERUN_STATUS_FAILED,
		})
		reruns, err := GetInProgressReruns(ctx, tfa)
		So(err, ShouldBeNil)
		So(len(reruns), ShouldEqual, 1)
		So(reruns[0].ID, ShouldEqual, 100)
	})
}

func TestGetVerificationRerunsForTestCulprit(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get verification reruns for test culprit", t, func() {
		suspectRerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID: 100,
		})
		parentRerun := testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID: 101,
		})
		suspect := testutil.CreateSuspect(ctx, &testutil.SuspectCreationOption{
			SuspectRerunKey: datastore.KeyForObj(ctx, suspectRerun),
			ParentRerunKey:  datastore.KeyForObj(ctx, parentRerun),
		})
		s, p, err := GetVerificationRerunsForTestCulprit(ctx, suspect)
		So(err, ShouldBeNil)
		So(s.ID, ShouldEqual, 100)
		So(p.ID, ShouldEqual, 101)
	})
}

func TestGetTestNthSectionReruns(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	Convey("Get test nthsection rerun", t, func() {
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ID: 100,
		})
		testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID:                    200,
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			CreateTime:            time.Unix(100, 0).UTC(),
		})
		testutil.CreateTestSingleRerun(ctx, &testutil.TestSingleRerunCreationOption{
			ID:                    201,
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			CreateTime:            time.Unix(50, 0).UTC(),
		})
		reruns, err := GetTestNthSectionReruns(ctx, nsa)
		So(err, ShouldBeNil)
		So(len(reruns), ShouldEqual, 2)
		So(reruns[0].ID, ShouldEqual, 201)
		So(reruns[1].ID, ShouldEqual, 200)
	})
}
