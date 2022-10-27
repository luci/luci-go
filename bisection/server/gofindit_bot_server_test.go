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

package server

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestUpdateAnalysisProgress(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	// For some reasons, AutoIndex does not work in this case
	// and it requires an explicit index
	// datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).AddIndexes(&datastore.IndexDefinition{
		Kind: "SingleRerun",
		SortBy: []datastore.IndexColumn{
			{
				Property: "rerun_build",
			},
			{
				Property: "start_time",
			},
		},
	})

	Convey("UpdateAnalysisProgress", t, func() {
		// Setup the models
		// Set up suspects
		analysis := &model.CompileFailureAnalysis{}
		So(datastore.Put(c, analysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		So(datastore.Put(c, heuristicAnalysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect := &model.Suspect{
			ParentAnalysis: datastore.KeyForObj(c, heuristicAnalysis),
			Score:          10,
			GitilesCommit: bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "3425",
			},
		}
		So(datastore.Put(c, suspect), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Set up reruns
		rerunBuildModel := &model.CompileRerunBuild{
			Id:      8800,
			Type:    model.RerunBuildType_CulpritVerification,
			Suspect: datastore.KeyForObj(c, suspect),
		}
		So(datastore.Put(c, rerunBuildModel), ShouldBeNil)

		parentRerunBuildModel := &model.CompileRerunBuild{
			Id:      8801,
			Type:    model.RerunBuildType_CulpritVerification,
			Suspect: datastore.KeyForObj(c, suspect),
		}
		So(datastore.Put(c, parentRerunBuildModel), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect.SuspectRerunBuild = datastore.KeyForObj(c, rerunBuildModel)
		suspect.ParentRerunBuild = datastore.KeyForObj(c, parentRerunBuildModel)
		So(datastore.Put(c, suspect), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Setup single rerun
		singleRerun1 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
			GitilesCommit: bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "3425",
			},
			Status: pb.RerunStatus_IN_PROGRESS,
		}

		singleRerun2 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, parentRerunBuildModel),
			GitilesCommit: bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "3426",
			},
			Status: pb.RerunStatus_IN_PROGRESS,
		}
		So(datastore.Put(c, singleRerun1), ShouldBeNil)
		So(datastore.Put(c, singleRerun2), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Update analysis
		req1 := &pb.UpdateAnalysisProgressRequest{
			AnalysisId: 1234,
			Bbid:       8800,
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "3425",
			},
			RerunResult: &pb.RerunResult{
				RerunStatus: pb.RerunStatus_FAILED,
			},
		}

		req2 := &pb.UpdateAnalysisProgressRequest{
			AnalysisId: 1234,
			Bbid:       8801,
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "3426",
			},
			RerunResult: &pb.RerunResult{
				RerunStatus: pb.RerunStatus_PASSED,
			},
		}

		server := &GoFinditBotServer{}
		_, err := server.UpdateAnalysisProgress(c, req1)
		So(err, ShouldBeNil)
		datastore.Get(c, singleRerun1)
		So(singleRerun1.Status, ShouldEqual, pb.RerunStatus_FAILED)
		datastore.Get(c, suspect)
		So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_UnderVerification)

		_, err = server.UpdateAnalysisProgress(c, req2)
		So(err, ShouldBeNil)
		datastore.Get(c, singleRerun2)
		So(singleRerun2.Status, ShouldEqual, pb.RerunStatus_PASSED)
		datastore.Get(c, suspect)
		So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_ConfirmedCulprit)

		err = datastore.Get(c, analysis)
		So(err, ShouldBeNil)
		So(analysis.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
		So(len(analysis.VerifiedCulprits), ShouldEqual, 1)
		So(analysis.VerifiedCulprits[0], ShouldResemble, datastore.KeyForObj(c, suspect))
	})

	Convey("verifyUpdateAnalysisProgressRequest", t, func() {
		req := &pb.UpdateAnalysisProgressRequest{}
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldNotBeNil)
		req.AnalysisId = 123
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldNotBeNil)
		req.Bbid = 8888
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldNotBeNil)
		req.GitilesCommit = &bbpb.GitilesCommit{}
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldNotBeNil)
		req.RerunResult = &pb.RerunResult{
			RerunStatus: pb.RerunStatus_FAILED,
		}
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldBeNil)
	})
}
