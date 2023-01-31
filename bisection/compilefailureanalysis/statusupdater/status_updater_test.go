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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/util/testutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestUpdateAnalysisStatus(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("UpdateAnalysisStatus", t, func() {
		// No heuristic and nthsection
		Convey("No heuristic and nthsection", func() {
			cfa := createCompileFailureAnalysisModel(c, 1000)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Have culprit", func() {
			cfa := createCompileFailureAnalysisModel(c, 1001)
			suspect := &model.Suspect{}
			So(datastore.Put(c, suspect), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa.VerifiedCulprits = []*datastore.Key{datastore.KeyForObj(c, suspect)}
			So(datastore.Put(c, cfa), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
		})

		Convey("No nth section, run finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1002)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		Convey("nth section error, run finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1003)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		Convey("No nth section, heuristic suspect found, run finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1004)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		Convey("No nth section, heuristic suspect found, run unfinished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1005)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, cfa)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		Convey("No heuristic, run finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1006)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		Convey("Heuristic error, run finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1007)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		Convey("No heuristic, nthsection suspect found, run finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1008)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		Convey("No heuristic, nthsection suspect found, run unfinished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1009)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, cfa)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		Convey("Heuristic and nthsection both error", func() {
			cfa := createCompileFailureAnalysisModel(c, 1010)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Heuristic suspect found, nth section in progress", func() {
			cfa := createCompileFailureAnalysisModel(c, 1011)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		Convey("Heuristic suspect found, nth section finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1012)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Nthsection suspect found, heuristic not found, verification running", func() {
			cfa := createCompileFailureAnalysisModel(c, 1013)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			createUnfinishedRerun(c, cfa)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})

		Convey("Nthsection suspect found, verification finished", func() {
			cfa := createCompileFailureAnalysisModel(c, 1014)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Nthsection in progress, heuristic in progress", func() {
			cfa := createCompileFailureAnalysisModel(c, 1015)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		Convey("Nthsection not found, heuristic not found", func() {
			cfa := createCompileFailureAnalysisModel(c, 1016)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Heuristic not found, nth section running", func() {
			cfa := createCompileFailureAnalysisModel(c, 1017)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		Convey("Heuristic running, nth section not found", func() {
			cfa := createCompileFailureAnalysisModel(c, 1019)
			createHeuristicAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
		})

		Convey("No heuristic, nthsection suspect found, run finished, verification schedule", func() {
			cfa := createCompileFailureAnalysisModel(c, 1020)
			nsa := createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			suspect := &model.Suspect{
				ParentAnalysis:     datastore.KeyForObj(c, nsa),
				VerificationStatus: model.SuspectVerificationStatus_VerificationScheduled,
			}
			So(datastore.Put(c, suspect), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()
			checkUpdateAnalysisStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
		})
	})
}

func TestUpdateStatus(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("UpdateStatus", t, func() {
		Convey("Ended analysis will not update", func() {
			cfa := createCompileFailureAnalysisModelWithStatus(c, 1000, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			err := UpdateStatus(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			So(err, ShouldBeNil)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Canceled analysis will not update", func() {
			cfa := createCompileFailureAnalysisModelWithStatus(c, 1001, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			err := UpdateStatus(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_CANCELED)
		})

		Convey("Update status", func() {
			cfa := createCompileFailureAnalysisModelWithStatus(c, 1001, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			err := UpdateStatus(c, cfa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			So(err, ShouldBeNil)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})
	})
}

func TestUpdateNthSectionStatus(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("UpdateNthSectionStatus", t, func() {
		Convey("Ended analysis will not update", func() {
			cfa := createCompileFailureAnalysisModelWithStatus(c, 1000, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			nsa := createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
			err := UpdateNthSectionStatus(c, nsa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			So(err, ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Canceled analysis will not update", func() {
			cfa := createCompileFailureAnalysisModelWithStatus(c, 1001, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			nsa := createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_CANCELED)
			err := UpdateNthSectionStatus(c, nsa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			So(err, ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_CANCELED)
		})

		Convey("Update status", func() {
			cfa := createCompileFailureAnalysisModelWithStatus(c, 1002, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			nsa := createNthSectionAnalysis(c, cfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
			err := UpdateNthSectionStatus(c, nsa, pb.AnalysisStatus_FOUND, pb.AnalysisRunStatus_ENDED)
			So(err, ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})
	})
}

func createCompileFailureAnalysisModel(c context.Context, id int64) *model.CompileFailureAnalysis {
	cfa := &model.CompileFailureAnalysis{
		Id: id,
	}
	So(datastore.Put(c, cfa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

func createCompileFailureAnalysisModelWithStatus(c context.Context, id int64, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.CompileFailureAnalysis {
	cfa := &model.CompileFailureAnalysis{
		Id:        id,
		Status:    status,
		RunStatus: runStatus,
	}
	So(datastore.Put(c, cfa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return cfa
}

func createHeuristicAnalysis(c context.Context, cfa *model.CompileFailureAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.CompileHeuristicAnalysis {
	ha := &model.CompileHeuristicAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		Status:         status,
		RunStatus:      runStatus,
	}
	So(datastore.Put(c, ha), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return ha
}

func createNthSectionAnalysis(c context.Context, cfa *model.CompileFailureAnalysis, status pb.AnalysisStatus, runStatus pb.AnalysisRunStatus) *model.CompileNthSectionAnalysis {
	nsa := &model.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		Status:         status,
		RunStatus:      runStatus,
	}
	So(datastore.Put(c, nsa), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
	return nsa
}

func createUnfinishedRerun(c context.Context, cfa *model.CompileFailureAnalysis) {
	rerun := &model.SingleRerun{
		Analysis: datastore.KeyForObj(c, cfa),
		Status:   pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
	}
	So(datastore.Put(c, rerun), ShouldBeNil)
	datastore.GetTestable(c).CatchupIndexes()
}

func checkUpdateAnalysisStatus(c context.Context, cfa *model.CompileFailureAnalysis, expectedStatus pb.AnalysisStatus, expectedRunStatus pb.AnalysisRunStatus) {
	err := UpdateAnalysisStatus(c, cfa)
	So(err, ShouldBeNil)
	So(cfa.Status, ShouldEqual, expectedStatus)
	So(cfa.RunStatus, ShouldEqual, expectedRunStatus)
}
