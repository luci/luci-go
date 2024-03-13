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

package server

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/compilefailureanalysis/cancelanalysis"
	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"

	_ "go.chromium.org/luci/bisection/culpritaction/revertculprit"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestUpdateAnalysisProgress(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	Convey("UpdateAnalysisProgress Culprit Verification", t, func() {
		c, scheduler := tq.TestingContext(c, nil)
		cancelanalysis.RegisterTaskClass()
		culpritverification.RegisterTaskClass()

		// Setup the models
		// Set up suspects
		analysis := &model.CompileFailureAnalysis{
			Id: 1234,
		}
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
			Status:  pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
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
			Status:  pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
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
				RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
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
				RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED,
			},
			BotId: "abc",
		}

		server := &BotUpdatesServer{}
		_, err := server.UpdateAnalysisProgress(c, req1)
		So(err, ShouldBeNil)
		datastore.Get(c, singleRerun1)
		So(singleRerun1.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_FAILED)
		datastore.Get(c, suspect)
		So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_UnderVerification)

		_, err = server.UpdateAnalysisProgress(c, req2)
		So(err, ShouldBeNil)
		datastore.Get(c, singleRerun2)
		So(singleRerun2.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_PASSED)
		datastore.Get(c, suspect)
		So(suspect.VerificationStatus, ShouldEqual, model.SuspectVerificationStatus_ConfirmedCulprit)

		err = datastore.Get(c, analysis)
		So(err, ShouldBeNil)
		So(analysis.Status, ShouldEqual, pb.AnalysisStatus_FOUND)
		So(analysis.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		So(len(analysis.VerifiedCulprits), ShouldEqual, 1)
		So(analysis.VerifiedCulprits[0], ShouldResemble, datastore.KeyForObj(c, suspect))

		// Assert task
		task := &tpb.CancelAnalysisTask{
			AnalysisId: 1234,
		}
		expected := proto.Clone(task).(*tpb.CancelAnalysisTask)
		So(scheduler.Tasks().Payloads()[0], ShouldResembleProto, expected)
	})

	Convey("UpdateAnalysisProgress NthSection", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx

		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).AnyTimes()

		// Set up the config
		projectCfg := config.CreatePlaceholderProjectConfig()
		projectCfg.CompileAnalysisConfig.NthsectionEnabled = true
		projectCfg.CompileAnalysisConfig.CulpritVerificationEnabled = true
		cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
		So(config.SetTestProjectConfig(c, cfg), ShouldBeNil)

		Convey("Schedule run for next commit", func() {
			bbres := &bbpb.Build{
				Builder: &bbpb.BuilderID{
					Project: "chromium",
					Bucket:  "findit",
					Builder: "luci-bisection-single-revision",
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

			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			So(datastore.Put(c, fb), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:                 1234,
				CompileFailure:     datastore.KeyForObj(c, cf),
				FirstFailedBuildId: 1000,
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
				Status:             pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
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
					RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				BotId: "abc",
			}

			server := &BotUpdatesServer{}
			res, err := server.UpdateAnalysisProgress(c, req)
			datastore.GetTestable(c).CatchupIndexes()
			So(err, ShouldBeNil)
			So(datastore.Get(c, singleRerun), ShouldBeNil)
			So(singleRerun.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_FAILED)
			So(res, ShouldResemble, &pb.UpdateAnalysisProgressResponse{})
			// Check that another rerun is scheduled
			rr := &model.CompileRerunBuild{
				Id: 9999,
			}
			So(datastore.Get(c, rr), ShouldBeNil)
			So(datastore.Get(c, cfa), ShouldBeNil)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_RUNNING)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)
		})

		Convey("Culprit found", func() {
			c, scheduler := tq.TestingContext(c, nil)

			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			So(datastore.Put(c, fb), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:             3456,
				CompileFailure: datastore.KeyForObj(c, cf),
				InitialRegressionRange: &pb.RegressionRange{
					FirstFailed: &bbpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "ref",
					},
				},
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
				Status:             pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
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
				Status:             pb.RerunStatus_RERUN_STATUS_PASSED,
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
					RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
				},
				BotId: "abc",
			}

			// We do not expect any calls to ScheduleBuild
			mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(0)
			server := &BotUpdatesServer{}
			res, err := server.UpdateAnalysisProgress(c, req)
			So(err, ShouldBeNil)
			So(datastore.Get(c, singleRerun1), ShouldBeNil)
			So(singleRerun1.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_FAILED)
			So(res, ShouldResemble, &pb.UpdateAnalysisProgressResponse{})

			// Check that the nthsection analysis is updated with Suspect
			datastore.GetTestable(c).CatchupIndexes()
			So(datastore.Get(c, nsa), ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(nsa.Suspect, ShouldNotBeNil)
			nsaSuspect := &model.Suspect{
				Id:             nsa.Suspect.IntID(),
				ParentAnalysis: nsa.Suspect.Parent(),
			}
			So(datastore.Get(c, nsaSuspect), ShouldBeNil)
			So(nsaSuspect, ShouldResemble, &model.Suspect{
				Id:             nsaSuspect.Id,
				Type:           model.SuspectType_NthSection,
				ParentAnalysis: nsaSuspect.ParentAnalysis,
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "commit5",
					Ref:     "ref",
				},
				VerificationStatus: model.SuspectVerificationStatus_VerificationScheduled,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			})
			So(datastore.Get(c, cfa), ShouldBeNil)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)

			// Assert task
			task := &tpb.CulpritVerificationTask{
				AnalysisId: cfa.Id,
				SuspectId:  nsa.Suspect.IntID(),
				ParentKey:  datastore.KeyForObj(c, nsa).Encode(),
			}
			expected := proto.Clone(task).(*tpb.CulpritVerificationTask)
			So(scheduler.Tasks().Payloads()[0], ShouldResembleProto, expected)
		})

		Convey("Nthsection couldn't find suspect", func() {
			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			So(datastore.Put(c, fb), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:             3457,
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
				Id: 9768,
			}
			So(datastore.Put(c, rerunBuildModel), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Create 2 SingleRerun of 2 commits next to each other
			singleRerun1 := &model.SingleRerun{
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit5",
				},
				Status:             pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
				StartTime:          clock.Now(c).Add(time.Hour),
			}
			singleRerun2 := &model.SingleRerun{
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit6",
				},
				Status:             pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
				StartTime:          clock.Now(c),
			}
			singleRerun3 := &model.SingleRerun{
				RerunBuild: datastore.KeyForObj(c, rerunBuildModel),
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit8",
				},
				Status:             pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
				StartTime:          clock.Now(c),
			}

			So(datastore.Put(c, singleRerun1), ShouldBeNil)
			So(datastore.Put(c, singleRerun2), ShouldBeNil)
			So(datastore.Put(c, singleRerun3), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Update analysis
			req := &pb.UpdateAnalysisProgressRequest{
				AnalysisId: 3457,
				Bbid:       9768,
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit8",
				},
				RerunResult: &pb.RerunResult{
					RerunStatus: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
				},
				BotId: "abc",
			}

			// We do not expect any calls to ScheduleBuild
			mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(0)
			server := &BotUpdatesServer{}
			res, err := server.UpdateAnalysisProgress(c, req)
			So(err, ShouldBeNil)
			So(datastore.Get(c, singleRerun3), ShouldBeNil)
			So(singleRerun3.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_INFRA_FAILED)
			So(res, ShouldResemble, &pb.UpdateAnalysisProgressResponse{})

			// Check that the nthsection analysis is updated with Suspect
			datastore.GetTestable(c).CatchupIndexes()
			So(datastore.Get(c, nsa), ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(datastore.Get(c, cfa), ShouldBeNil)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
		})

		Convey("Nthsection regression range conflicts", func() {
			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			So(datastore.Put(c, fb), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:             2122,
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
				Id: 4343,
			}
			So(datastore.Put(c, rerunBuildModel), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			singleRerun1 := &model.SingleRerun{
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit9",
				},
				Status:             pb.RerunStatus_RERUN_STATUS_FAILED,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
				StartTime:          clock.Now(c),
			}
			singleRerun2 := &model.SingleRerun{
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit1",
				},
				Status:             pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:               model.RerunBuildType_NthSection,
				NthSectionAnalysis: datastore.KeyForObj(c, nsa),
				Analysis:           datastore.KeyForObj(c, cfa),
				StartTime:          clock.Now(c).Add(time.Hour),
				RerunBuild:         datastore.KeyForObj(c, rerunBuildModel),
			}

			So(datastore.Put(c, singleRerun1), ShouldBeNil)
			So(datastore.Put(c, singleRerun2), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Update analysis
			req := &pb.UpdateAnalysisProgressRequest{
				AnalysisId: 2122,
				Bbid:       4343,
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "ref",
					Id:      "commit1",
				},
				RerunResult: &pb.RerunResult{
					RerunStatus: pb.RerunStatus_RERUN_STATUS_PASSED, // This would result in a conflict, since commit 9 failed
				},
				BotId: "abc",
			}

			// We do not expect any calls to ScheduleBuild
			mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).Times(0)
			server := &BotUpdatesServer{}
			_, err := server.UpdateAnalysisProgress(c, req)
			So(err, ShouldBeNil)
			So(datastore.Get(c, singleRerun2), ShouldBeNil)
			So(singleRerun2.Status, ShouldEqual, pb.RerunStatus_RERUN_STATUS_PASSED)

			// Check analysis status
			datastore.GetTestable(c).CatchupIndexes()
			So(datastore.Get(c, nsa), ShouldBeNil)
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(datastore.Get(c, cfa), ShouldBeNil)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_NOTFOUND)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
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
			RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
		}
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldNotBeNil)
		req.BotId = "botid"
		So(verifyUpdateAnalysisProgressRequest(c, req), ShouldBeNil)
	})
}
