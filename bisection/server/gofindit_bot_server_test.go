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
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/util/testutil"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestUpdateAnalysisProgress(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	datastore.GetTestable(c).AddIndexes(&datastore.IndexDefinition{
		Kind: "SingleRerun",
		SortBy: []datastore.IndexColumn{
			{
				Property: "analysis",
			},
			{
				Property: "start_time",
			},
		},
	})

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

	Convey("UpdateAnalysisProgress Culprit Verification", t, func() {
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
			Id: 8800,
		}
		So(datastore.Put(c, rerunBuildModel), ShouldBeNil)

		parentRerunBuildModel := &model.CompileRerunBuild{
			Id: 8801,
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
			Status:  pb.RerunStatus_IN_PROGRESS,
			Type:    model.RerunBuildType_CulpritVerification,
			Suspect: datastore.KeyForObj(c, suspect),
		}

		singleRerun2 := &model.SingleRerun{
			RerunBuild: datastore.KeyForObj(c, parentRerunBuildModel),
			GitilesCommit: bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "3426",
			},
			Status:  pb.RerunStatus_IN_PROGRESS,
			Type:    model.RerunBuildType_CulpritVerification,
			Suspect: datastore.KeyForObj(c, suspect),
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
			BotId: "abc",
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
			BotId: "abc",
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

	Convey("UpdateAnalysisProgress NthSection", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx

		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).AnyTimes()

		Convey("Schedule run for next commit", func() {
			bbres := &bbpb.Build{
				Builder: &bbpb.BuilderID{
					Project: "chromium",
					Bucket:  "findit",
					Builder: "gofindit-single-revision",
				},
				Input: &bbpb.Build_Input{
					GitilesCommit: &bbpb.GitilesCommit{
						Host:    "host",
						Project: "proj",
						Id:      "id1",
						Ref:     "ref",
					},
				},
				Id:         9999,
				Status:     bbpb.Status_STARTED,
				CreateTime: &timestamppb.Timestamp{Seconds: 100},
				StartTime:  &timestamppb.Timestamp{Seconds: 101},
			}
			mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(bbres, nil).Times(1)

			cf := &model.CompileFailure{}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:             1234,
				CompileFailure: datastore.KeyForObj(c, cf),
			}
			So(datastore.Put(c, cfa), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, cfa),
				BlameList:      testutil.CreateBlamelist(10),
			}
			So(datastore.Put(c, nsa), ShouldBeNil)

			// Set up reruns
			rerunBuildModel := &model.CompileRerunBuild{
				Id: 8800,
			}
			So(datastore.Put(c, rerunBuildModel), ShouldBeNil)

			singleRerun := &model.SingleRerun{
				RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit5",
				},
				Status:             pb.RerunStatus_IN_PROGRESS,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
			}
			So(datastore.Put(c, singleRerun), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Update analysis
			req := &pb.UpdateAnalysisProgressRequest{
				AnalysisId: 1234,
				Bbid:       8800,
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit5",
				},
				RerunResult: &pb.RerunResult{
					RerunStatus: pb.RerunStatus_FAILED,
				},
				BotId: "abc",
			}

			server := &GoFinditBotServer{}
			res, err := server.UpdateAnalysisProgress(c, req)
			datastore.GetTestable(c).CatchupIndexes()
			So(err, ShouldBeNil)
			So(datastore.Get(c, singleRerun), ShouldBeNil)
			So(singleRerun.Status, ShouldEqual, pb.RerunStatus_FAILED)
			So(res, ShouldResemble, &pb.UpdateAnalysisProgressResponse{})
			// Check that another rerun is scheduled
			rr := &model.CompileRerunBuild{
				Id: 9999,
			}
			So(datastore.Get(c, rr), ShouldBeNil)
		})

		Convey("Culprit found", func() {
			cf := &model.CompileFailure{}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:             3456,
				CompileFailure: datastore.KeyForObj(c, cf),
			}
			So(datastore.Put(c, cfa), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, cfa),
				BlameList:      testutil.CreateBlamelist(10),
			}
			So(datastore.Put(c, nsa), ShouldBeNil)

			// Set up reruns
			rerunBuildModel := &model.CompileRerunBuild{
				Id: 9876,
			}
			So(datastore.Put(c, rerunBuildModel), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Create 2 SingleRerun of 2 commits next to each other
			singleRerun1 := &model.SingleRerun{
				RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit5",
				},
				Status:             pb.RerunStatus_IN_PROGRESS,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
				StartTime:          clock.Now(c).Add(time.Hour),
			}
			singleRerun2 := &model.SingleRerun{
				RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit6",
				},
				Status:             pb.RerunStatus_PASSED,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
				StartTime:          clock.Now(c),
			}

			So(datastore.Put(c, singleRerun1), ShouldBeNil)
			So(datastore.Put(c, singleRerun2), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Update analysis
			req := &pb.UpdateAnalysisProgressRequest{
				AnalysisId: 3456,
				Bbid:       9876,
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit5",
				},
				RerunResult: &pb.RerunResult{
					RerunStatus: pb.RerunStatus_FAILED,
				},
				BotId: "abc",
			}

			// We do not expect any calls to ScheduleBuild
			mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(0)
			server := &GoFinditBotServer{}
			res, err := server.UpdateAnalysisProgress(c, req)
			So(err, ShouldBeNil)
			So(datastore.Get(c, singleRerun1), ShouldBeNil)
			So(singleRerun1.Status, ShouldEqual, pb.RerunStatus_FAILED)
			So(res, ShouldResemble, &pb.UpdateAnalysisProgressResponse{})
		})
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
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldNotBeNil)
		req.BotId = "botid"
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldBeNil)
	})
}
