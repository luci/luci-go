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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestGetBuild(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("No build found", t, func(t *ftt.Test) {
		buildModel, err := GetBuild(c, 100)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buildModel, should.BeNil)
	})

	ftt.Run("Build found", t, func(t *ftt.Test) {
		// Prepare datastore
		failed_build := &model.LuciFailedBuild{
			Id: 101,
		}
		assert.Loosely(t, datastore.Put(c, failed_build), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		buildModel, err := GetBuild(c, 101)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, buildModel, should.NotBeNil)
		assert.Loosely(t, buildModel.Id, should.Equal(101))
	})
}

func TestGetAnalysisForBuild(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("No build found", t, func(t *ftt.Test) {
		analysis, err := GetAnalysisForBuild(c, 100)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis, should.BeNil)
	})

	ftt.Run("No analysis found", t, func(t *ftt.Test) {
		// Prepare datastore
		failedBuild := &model.LuciFailedBuild{
			Id: 101,
		}
		assert.Loosely(t, datastore.Put(c, failedBuild), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		analysis, err := GetAnalysisForBuild(c, 101)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis, should.BeNil)
	})

	ftt.Run("Analysis found", t, func(t *ftt.Test) {
		// Prepare datastore
		failedBuild := &model.LuciFailedBuild{
			Id: 101,
		}
		assert.Loosely(t, datastore.Put(c, failedBuild), should.BeNil)

		compileFailure := &model.CompileFailure{
			Id:    101,
			Build: datastore.KeyForObj(c, failedBuild),
		}
		assert.Loosely(t, datastore.Put(c, compileFailure), should.BeNil)

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id:                 1230001,
			CompileFailure:     datastore.KeyForObj(c, compileFailure),
			FirstFailedBuildId: 101,
		}
		assert.Loosely(t, datastore.Put(c, compileFailureAnalysis), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		analysis, err := GetAnalysisForBuild(c, 101)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis, should.NotBeNil)
		assert.Loosely(t, analysis.Id, should.Equal(1230001))
		assert.Loosely(t, analysis.FirstFailedBuildId, should.Equal(101))
	})

	ftt.Run("Related analysis found", t, func(t *ftt.Test) {
		// Prepare datastore
		firstFailedBuild := &model.LuciFailedBuild{
			Id: 200,
		}
		assert.Loosely(t, datastore.Put(c, firstFailedBuild), should.BeNil)

		firstCompileFailure := &model.CompileFailure{
			Id:    200,
			Build: datastore.KeyForObj(c, firstFailedBuild),
		}
		assert.Loosely(t, datastore.Put(c, firstCompileFailure), should.BeNil)

		failedBuild := &model.LuciFailedBuild{
			Id: 201,
		}
		assert.Loosely(t, datastore.Put(c, failedBuild), should.BeNil)

		compileFailure := &model.CompileFailure{
			Id:               201,
			Build:            datastore.KeyForObj(c, failedBuild),
			MergedFailureKey: datastore.KeyForObj(c, firstCompileFailure),
		}
		assert.Loosely(t, datastore.Put(c, compileFailure), should.BeNil)

		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id:             1230002,
			CompileFailure: datastore.KeyForObj(c, firstCompileFailure),
		}
		assert.Loosely(t, datastore.Put(c, compileFailureAnalysis), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		analysis, err := GetAnalysisForBuild(c, 201)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis, should.NotBeNil)
		assert.Loosely(t, analysis.Id, should.Equal(1230002))
	})
}

func TestGetHeuristicAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("No heuristic analysis found", t, func(t *ftt.Test) {
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 1230003,
		}
		heuristicAnalysis, err := GetHeuristicAnalysis(c, compileFailureAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, heuristicAnalysis, should.BeNil)
	})

	ftt.Run("Heuristic analysis found", t, func(t *ftt.Test) {
		// Prepare datastore
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 1230003,
		}
		assert.Loosely(t, datastore.Put(c, compileFailureAnalysis), should.BeNil)

		compileHeuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id:             4560001,
			ParentAnalysis: datastore.KeyForObj(c, compileFailureAnalysis),
		}
		assert.Loosely(t, datastore.Put(c, compileHeuristicAnalysis), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		heuristicAnalysis, err := GetHeuristicAnalysis(c, compileFailureAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, heuristicAnalysis, should.NotBeNil)
		assert.Loosely(t, heuristicAnalysis.Id, should.Equal(4560001))
	})
}

func TestGetGenAIAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("No genai analysis found", t, func(t *ftt.Test) {
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 1230004,
		}
		genaiAnalysis, err := GetGenAIAnalysis(c, compileFailureAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, genaiAnalysis, should.BeNil)
	})

	ftt.Run("GenAI analysis found", t, func(t *ftt.Test) {
		// Prepare datastore
		compileFailureAnalysis := &model.CompileFailureAnalysis{
			Id: 1230004,
		}
		assert.Loosely(t, datastore.Put(c, compileFailureAnalysis), should.BeNil)

		compileGenAIAnalysis := &model.CompileGenAIAnalysis{
			Id:             4560002,
			ParentAnalysis: datastore.KeyForObj(c, compileFailureAnalysis),
		}
		assert.Loosely(t, datastore.Put(c, compileGenAIAnalysis), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		genaiAnalysis, err := GetGenAIAnalysis(c, compileFailureAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, genaiAnalysis, should.NotBeNil)
		assert.Loosely(t, genaiAnalysis.Id, should.Equal(4560002))
	})
}

func TestGetSuspectsForHeuristicAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	datastore.GetTestable(c).AutoIndex(true)

	ftt.Run("No suspects found", t, func(t *ftt.Test) {
		// Prepare datastore
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id: 700,
		}
		assert.Loosely(t, datastore.Put(c, heuristicAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspects, err := GetSuspectsForHeuristicAnalysis(c, heuristicAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.BeZero)
	})

	ftt.Run("All suspects found", t, func(t *ftt.Test) {
		// Prepare datastore
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id: 701,
		}
		assert.Loosely(t, datastore.Put(c, heuristicAnalysis), should.BeNil)

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
		assert.Loosely(t, datastore.Put(c, suspect1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect2), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect3), should.BeNil)
		assert.Loosely(t, datastore.Put(c, suspect4), should.BeNil)

		// Add a different heuristic analysis with its own suspect
		otherHeuristicAnalysis := &model.CompileHeuristicAnalysis{
			Id: 702,
		}
		assert.Loosely(t, datastore.Put(c, heuristicAnalysis), should.BeNil)
		otherSuspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, otherHeuristicAnalysis),
			Score:          5,
		}
		assert.Loosely(t, datastore.Put(c, otherSuspect), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		suspects, err := GetSuspectsForHeuristicAnalysis(c, heuristicAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(4))
		assert.Loosely(t, suspects[0].Score, should.Equal(4))
		assert.Loosely(t, suspects[1].Score, should.Equal(3))
		assert.Loosely(t, suspects[2].Score, should.Equal(2))
		assert.Loosely(t, suspects[3].Score, should.Equal(1))
	})
}

func TestGetSuspectsForGenAIAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	ftt.Run("No suspects found", t, func(t *ftt.Test) {
		genaiAnalysis := &model.CompileGenAIAnalysis{}
		assert.Loosely(t, datastore.Put(c, genaiAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect, err := GetSuspectsForGenAIAnalysis(c, genaiAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, suspect, should.BeNil)
	})

	ftt.Run("Suspect found", t, func(t *ftt.Test) {
		genaiAnalysis := &model.CompileGenAIAnalysis{}
		assert.Loosely(t, datastore.Put(c, genaiAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, genaiAnalysis),
			Id:             789,
		}
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect, err := GetSuspectsForGenAIAnalysis(c, genaiAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, suspect, should.NotBeNil)
		assert.Loosely(t, suspect.Id, should.Equal(789))
	})
}

func TestGetSuspectForNthSectionAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	ftt.Run("No suspects found", t, func(t *ftt.Test) {
		nthSectionAnalysis := &model.CompileNthSectionAnalysis{}
		assert.Loosely(t, datastore.Put(c, nthSectionAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect, err := GetSuspectForNthSectionAnalysis(c, nthSectionAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, suspect, should.BeNil)
	})

	ftt.Run("Suspect found", t, func(t *ftt.Test) {
		nthSectionAnalysis := &model.CompileNthSectionAnalysis{}
		assert.Loosely(t, datastore.Put(c, nthSectionAnalysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, nthSectionAnalysis),
			Id:             123,
		}
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect, err := GetSuspectForNthSectionAnalysis(c, nthSectionAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, suspect, should.NotBeNil)
		assert.Loosely(t, suspect.Id, should.Equal(123))
	})
}

func TestGetCompileFailureForAnalysisID(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	datastore.GetTestable(c).AutoIndex(true)

	ftt.Run("No analysis found", t, func(t *ftt.Test) {
		_, err := GetCompileFailureForAnalysisID(c, 100)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Have analysis for compile failure", t, func(t *ftt.Test) {
		build := &model.LuciFailedBuild{
			Id: 111,
		}
		assert.Loosely(t, datastore.Put(c, build), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		compileFailure := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, build),
		}
		assert.Loosely(t, datastore.Put(c, compileFailure), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		analysis := &model.CompileFailureAnalysis{
			Id:             456,
			CompileFailure: datastore.KeyForObj(c, compileFailure),
		}
		assert.Loosely(t, datastore.Put(c, analysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf, err := GetCompileFailureForAnalysisID(c, 456)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cf.Id, should.Equal(123))
	})
}

func TestGetRerunsForRerunBuild(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("GetRerunsForRerunBuild", t, func(t *ftt.Test) {
		// Set up reruns
		rerunBuildModel := &model.CompileRerunBuild{
			Id: 8800,
		}
		assert.Loosely(t, datastore.Put(c, rerunBuildModel), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		reruns, err := GetRerunsForRerunBuild(c, rerunBuildModel)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(reruns), should.BeZero)
		_, err = GetLastRerunForRerunBuild(c, rerunBuildModel)
		assert.Loosely(t, err, should.NotBeNil)

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

		assert.Loosely(t, datastore.Put(c, singleRerun1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, singleRerun2), should.BeNil)
		assert.Loosely(t, datastore.Put(c, singleRerun3), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		reruns, err = GetRerunsForRerunBuild(c, rerunBuildModel)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(reruns), should.Equal(3))
		r, err := GetLastRerunForRerunBuild(c, rerunBuildModel)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, r.Id, should.Equal(2))
	})
}

func TestGetNthSectionAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("No nthsection analysis found", t, func(t *ftt.Test) {
		cfa := &model.CompileFailureAnalysis{
			Id: 123,
		}
		nsa, err := GetNthSectionAnalysis(c, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, nsa, should.BeNil)
	})

	ftt.Run("More than one nthsection analysis found", t, func(t *ftt.Test) {
		cfa := &model.CompileFailureAnalysis{
			Id: 456,
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		nsa1 := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		nsa2 := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		assert.Loosely(t, datastore.Put(c, nsa1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, nsa2), should.BeNil)

		_, err := GetNthSectionAnalysis(c, cfa)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Nthsection analysis found", t, func(t *ftt.Test) {
		cfa := &model.CompileFailureAnalysis{
			Id: 789,
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		nsa1 := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
			Id:             345,
		}
		assert.Loosely(t, datastore.Put(c, nsa1), should.BeNil)
		nsa, err := GetNthSectionAnalysis(c, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, nsa.Id, should.Equal(345))
	})
}

func TestGetCompileFailureAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("GetCompileFailureAnalysis", t, func(t *ftt.Test) {
		_, err := GetCompileFailureAnalysis(c, 123)
		assert.Loosely(t, err, should.NotBeNil)
		cfa := &model.CompileFailureAnalysis{
			Id: 123,
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		analysis, err := GetCompileFailureAnalysis(c, 123)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis.Id, should.Equal(123))
	})
}

func TestGetOtherSuspectsWithSameCL(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())

	ftt.Run("GetOtherSuspectsWithSameCL", t, func(t *ftt.Test) {
		suspect := &model.Suspect{
			ReviewUrl: "https://this/is/review/url",
			Id:        123,
		}
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		suspects, err := GetOtherSuspectsWithSameCL(c, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.BeZero)

		suspect1 := &model.Suspect{
			ReviewUrl: "https://this/is/review/url",
			Id:        124,
		}
		assert.Loosely(t, datastore.Put(c, suspect1), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		suspects, err = GetOtherSuspectsWithSameCL(c, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(1))
		assert.Loosely(t, suspects[0].Id, should.Equal(124))
	})
}

func TestGetLatestBuildFailureAndAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("GetLatestBuildFailureAndAnalysis", t, func(t *ftt.Test) {
		build, err := GetLatestBuildFailureForBuilder(c, "project", "bucket", "builder")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, build, should.BeNil)
		analysis, err := GetLatestAnalysisForBuilder(c, "project", "bucket", "builder")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis, should.BeNil)

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

		assert.Loosely(t, datastore.Put(c, bf1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, bf2), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		build, err = GetLatestBuildFailureForBuilder(c, "project", "bucket", "builder")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, build.Id, should.Equal(456))

		cf1 := &model.CompileFailure{
			Id:    123,
			Build: datastore.KeyForObj(c, bf1),
		}
		assert.Loosely(t, datastore.Put(c, cf1), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf2 := &model.CompileFailure{
			Id:               456,
			Build:            datastore.KeyForObj(c, bf2),
			MergedFailureKey: datastore.KeyForObj(c, cf1),
		}
		assert.Loosely(t, datastore.Put(c, cf2), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			Id:             789,
			CompileFailure: datastore.KeyForObj(c, cf1),
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		analysis, err = GetLatestAnalysisForBuilder(c, "project", "bucket", "builder")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis.Id, should.Equal(789))
	})
}

func TestGetRerunsForAnalysis(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("GetRerunsForAnalysis", t, func(t *ftt.Test) {
		cfa := &model.CompileFailureAnalysis{
			Id: 123,
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)
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

		assert.Loosely(t, datastore.Put(c, rr1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, rr2), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		reruns, err := GetRerunsForAnalysis(c, cfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(reruns), should.Equal(2))

		nsectionReruns, err := GetRerunsForNthSectionAnalysis(c, nsa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(nsectionReruns), should.Equal(1))
		assert.Loosely(t, nsectionReruns[0].Id, should.Equal(333))
	})
}

func TestGetTestFailureAnalysis(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get test failure analysis", t, func(t *ftt.Test) {
		testutil.CreateTestFailureAnalysis(ctx, t, nil)
		tfa, err := GetTestFailureAnalysis(ctx, 1000)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tfa.ID, should.Equal(1000))
	})
}

func TestGetPrimaryTestFailure(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("No primary failure", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, nil)
		_, err := GetPrimaryTestFailure(ctx, tfa)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Have primary failure", t, func(t *ftt.Test) {
		tf := testutil.CreateTestFailure(ctx, t, nil)
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:             1002,
			TestFailureKey: datastore.KeyForObj(ctx, tf),
		})
		tf, err := GetPrimaryTestFailure(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tf.ID, should.Equal(100))
	})
}

func TestGetTestFailureBundle(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("No test failures", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, nil)
		_, err := GetTestFailureBundle(ctx, tfa)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Have test failure", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID: 1002,
		})
		tf1 := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID: 100,
		})
		tf1.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		tf2 := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID: 101,
		})
		tf2.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		assert.Loosely(t, datastore.Put(ctx, tf1), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, tf2), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		// No primary test failure.
		_, err := GetTestFailureBundle(ctx, tfa)
		assert.Loosely(t, err, should.NotBeNil)

		// Have primary test failure.
		tf2.IsPrimary = true
		assert.Loosely(t, datastore.Put(ctx, tf2), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		bundle, err := GetTestFailureBundle(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, bundle.Primary().ID, should.Equal(101))
		others := bundle.Others()
		assert.Loosely(t, len(others), should.Equal(1))
		assert.Loosely(t, others[0].ID, should.Equal(100))
	})
}

func TestGetTestSingleRerun(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get test single rerun", t, func(t *ftt.Test) {
		_, err := GetTestSingleRerun(ctx, 123)
		assert.Loosely(t, err, should.NotBeNil)
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID: 123,
		})
		rerun, err := GetTestSingleRerun(ctx, 123)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rerun.ID, should.Equal(123))
	})
}

func TestGetTestFailure(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get test failure", t, func(t *ftt.Test) {
		_, err := GetTestFailure(ctx, 123)
		assert.Loosely(t, err, should.NotBeNil)
		testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID: 123,
		})
		rerun, err := GetTestFailure(ctx, 123)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rerun.ID, should.Equal(123))
	})
}

func TestGetTestNthSectionAnalysis(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get test nthsection analysis", t, func(t *ftt.Test) {
		_, err := GetTestNthSectionAnalysis(ctx, 123)
		assert.Loosely(t, err, should.NotBeNil)
		testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ID: 123,
		})
		rerun, err := GetTestNthSectionAnalysis(ctx, 123)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, rerun.ID, should.Equal(123))
	})
}

func TestGetTestNthSectionForAnalysis(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get test nthsection analysis", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{})
		nsa, err := GetTestNthSectionForAnalysis(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, nsa, should.BeNil)
		testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ID:                123,
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		nsa, err = GetTestNthSectionForAnalysis(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, nsa.ID, should.Equal(123))
	})
}

func TestGetInProgressReruns(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get in progress rerun", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{})
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:          100,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			Type:        model.RerunBuildType_CulpritVerification,
			Status:      bisectionpb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		})
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:          101,
			AnalysisKey: datastore.KeyForObj(ctx, tfa),
			Type:        model.RerunBuildType_NthSection,
			Status:      bisectionpb.RerunStatus_RERUN_STATUS_FAILED,
		})
		reruns, err := GetInProgressReruns(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(reruns), should.Equal(1))
		assert.Loosely(t, reruns[0].ID, should.Equal(100))
	})
}

func TestGetVerificationRerunsForTestCulprit(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get verification reruns for test culprit", t, func(t *ftt.Test) {
		suspectRerun := testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID: 100,
		})
		parentRerun := testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID: 101,
		})
		suspect := testutil.CreateSuspect(ctx, t, &testutil.SuspectCreationOption{
			SuspectRerunKey: datastore.KeyForObj(ctx, suspectRerun),
			ParentRerunKey:  datastore.KeyForObj(ctx, parentRerun),
		})
		s, p, err := GetVerificationRerunsForTestCulprit(ctx, suspect)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s.ID, should.Equal(100))
		assert.Loosely(t, p.ID, should.Equal(101))
	})
}

func TestGetTestNthSectionReruns(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)

	ftt.Run("Get test nthsection rerun", t, func(t *ftt.Test) {
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
			ID: 100,
		})
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:                    200,
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			CreateTime:            time.Unix(100, 0).UTC(),
		})
		testutil.CreateTestSingleRerun(ctx, t, &testutil.TestSingleRerunCreationOption{
			ID:                    201,
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			CreateTime:            time.Unix(50, 0).UTC(),
		})
		reruns, err := GetTestNthSectionReruns(ctx, nsa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(reruns), should.Equal(2))
		assert.Loosely(t, reruns[0].ID, should.Equal(201))
		assert.Loosely(t, reruns[1].ID, should.Equal(200))
	})
}
