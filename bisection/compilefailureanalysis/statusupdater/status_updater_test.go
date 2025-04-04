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

package statusupdater

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestUpdateAnalysisStatus(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("UpdateAnalysisStatus", t, func(t *ftt.Test) {
		// No heuristic and nthsection
		t.Run("No heuristic and nthsection", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1000)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("Have culprit", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1001)
			suspect := &model.Suspect{}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa.VerifiedCulprits = []*datastore.Key{datastore.KeyForObj(c, suspect)}
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("No nth section, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1002)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("nth section error, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1003)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("No nth section, heuristic suspect found, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1004)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("No nth section, heuristic suspect found, run unfinished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1005)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, t, cfa)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("No heuristic, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1006)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("Heuristic error, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1007)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("No heuristic, nthsection suspect found, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1008)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("No heuristic, nthsection suspect found, run unfinished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1009)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, t, cfa)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("Heuristic and nthsection both error", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1010)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("Heuristic suspect found, nth section in progress", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1011)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("Heuristic suspect found, nth section finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1012)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("Nthsection suspect found, heuristic not found, verification running", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1013)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, t, cfa)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("Nthsection suspect found, verification finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1014)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("Nthsection in progress, heuristic in progress", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1015)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("Nthsection not found, heuristic not found", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1016)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("Heuristic not found, nth section running", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1017)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("Heuristic running, nth section not found", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1019)
			createHeuristicAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("No heuristic, nthsection suspect found, run finished, verification schedule", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1020)
			nsa := createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			suspect := &model.Suspect{
				ParentAnalysis:     datastore.KeyForObj(c, nsa),
				VerificationStatus: model.SuspectVerificationStatus_VerificationScheduled,
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})
	})
}

func TestUpdateStatus(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("UpdateStatus", t, func(t *ftt.Test) {
		t.Run("Ended analysis will not update", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModelWithStatus(c, t, 1000, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			err := UpdateStatus(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})

		t.Run("Canceled analysis will not update", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModelWithStatus(c, t, 1001, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			err := UpdateStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_CANCELED))
		})

		t.Run("Update status", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModelWithStatus(c, t, 1001, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			err := UpdateStatus(c, cfa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})
	})
}

func TestUpdateNthSectionStatus(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("UpdateNthSectionStatus", t, func(t *ftt.Test) {
		t.Run("Ended analysis will not update", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModelWithStatus(c, t, 1000, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			nsa := createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			err := UpdateNthSectionStatus(c, nsa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})

		t.Run("Canceled analysis will not update", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModelWithStatus(c, t, 1001, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			nsa := createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			err := UpdateNthSectionStatus(c, nsa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_CANCELED))
		})

		t.Run("Update status", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModelWithStatus(c, t, 1002, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			nsa := createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			err := UpdateNthSectionStatus(c, nsa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_FOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})
	})
}

func createCompileFailureAnalysisModel(c context.Context, t testing.TB, id int64) *model.CompileFailureAnalysis {
	t.Helper()
	cfa := &model.CompileFailureAnalysis{
		Id: id,
	}
	assert.Loosely(t, datastore.Put(c, cfa), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

func createCompileFailureAnalysisModelWithStatus(c context.Context, t testing.TB, id int64, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.CompileFailureAnalysis {
	t.Helper()
	cfa := &model.CompileFailureAnalysis{
		Id:        id,
		Status:    status,
		RunStatus: runStatus,
	}
	assert.Loosely(t, datastore.Put(c, cfa), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

func createHeuristicAnalysis(c context.Context, t testing.TB, cfa *model.CompileFailureAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.CompileHeuristicAnalysis {
	t.Helper()
	ha := &model.CompileHeuristicAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		Status:         status,
		RunStatus:      runStatus,
	}
	assert.Loosely(t, datastore.Put(c, ha), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return ha
}

func createNthSectionAnalysis(c context.Context, t testing.TB, cfa *model.CompileFailureAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.CompileNthSectionAnalysis {
	t.Helper()
	nsa := &model.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		Status:         status,
		RunStatus:      runStatus,
	}
	assert.Loosely(t, datastore.Put(c, nsa), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return nsa
}

func createUnfinishedRerun(c context.Context, t testing.TB, cfa *model.CompileFailureAnalysis) {
	t.Helper()
	rerun := &model.SingleRerun{
		Analysis: datastore.KeyForObj(c, cfa),
		Status:   pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
	}
	assert.Loosely(t, datastore.Put(c, rerun), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
}

func checkUpdateAnalysisStatus(c context.Context, t testing.TB, cfa *model.CompileFailureAnalysis, expectedStatus pb.AnalysisStatus, expectedRunStatus pb.AnalysisRunStatus) {
	err := UpdateAnalysisStatus(c, cfa)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, cfa.Status, should.Equal(expectedStatus), truth.LineContext())
	assert.Loosely(t, cfa.RunStatus, should.Equal(expectedRunStatus), truth.LineContext())
}
