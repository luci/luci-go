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
		// No GenAI and nthsection
		t.Run("No GenAI and nthsection", func(t *ftt.Test) {
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

		t.Run("GenAI running, no nthsection", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1002)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("NthSection running, no GenAI", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1003)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("GenAI suspect found, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1004)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("GenAI suspect found, run unfinished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1005)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, t, cfa)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("NthSection suspect found, run finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1008)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("NthSection suspect found, run unfinished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1009)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, t, cfa)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("GenAI and NthSection both error", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1010)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("GenAI suspect found, NthSection in progress", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1011)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("GenAI suspect found, NthSection not found", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1012)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("NthSection suspect found, verification running", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1013)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, t, cfa)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("NthSection suspect found, verification finished", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1014)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("NthSection and GenAI both running", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1015)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("NthSection not found, GenAI not found", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1016)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		t.Run("GenAI not found, NthSection running", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1017)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("GenAI running, NthSection not found", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1019)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("NthSection suspect found, run finished, verification scheduled", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1020)
			createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			nsa := createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			suspect := &model.Suspect{
				ParentAnalysis:     datastore.KeyForObj(c, nsa),
				VerificationStatus: model.SuspectVerificationStatus_VerificationScheduled,
			}
			assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()
			checkUpdateAnalysisStatus(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		t.Run("GenAI and NthSection both run, GenAI finds suspect first", func(t *ftt.Test) {
			cfa := createCompileFailureAnalysisModel(c, t, 1021)
			ga := createGenAIAnalysis(c, t, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, t, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			suspect := &model.Suspect{
				ParentAnalysis:     datastore.KeyForObj(c, ga),
				VerificationStatus: model.SuspectVerificationStatus_UnderVerification,
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

func createGenAIAnalysis(c context.Context, t testing.TB, cfa *model.CompileFailureAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.CompileGenAIAnalysis {
	t.Helper()
	ga := &model.CompileGenAIAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		Status:         status,
		RunStatus:      runStatus,
	}
	assert.Loosely(t, datastore.Put(c, ga), should.BeNil, truth.LineContext())
	datastore.GetTestable(c).CatchupIndexes()
	return ga
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
