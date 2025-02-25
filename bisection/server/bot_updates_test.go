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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/compilefailureanalysis/cancelanalysis"
	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"

	_ "go.chromium.org/luci/bisection/culpritaction/revertculprit"
)

func TestUpdateAnalysisProgress(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	cl := testclock.New(testclock.TestTimeUTC)
	c = clock.Set(c, cl)

	ftt.Run("UpdateAnalysisProgress Culprit Verification", t, func(t *ftt.Test) {
		c, scheduler := tq.TestingContext(c, nil)
		cancelanalysis.RegisterTaskClass()
		culpritverification.RegisterTaskClass()

		// Setup the models
		// Set up suspects
		analysis := &model.CompileFailureAnalysis{
			Id: 1234,
		}
		assert.Loosely(t, datastore.Put(c, analysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		assert.Loosely(t, datastore.Put(c, heuristicAnalysis), should.BeNil)
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
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Set up reruns
		rerunBuildModel := &model.CompileRerunBuild{
			Id: 8800,
		}
		assert.Loosely(t, datastore.Put(c, rerunBuildModel), should.BeNil)

		parentRerunBuildModel := &model.CompileRerunBuild{
			Id: 8801,
		}
		assert.Loosely(t, datastore.Put(c, parentRerunBuildModel), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		suspect.SuspectRerunBuild = datastore.KeyForObj(c, rerunBuildModel)
		suspect.ParentRerunBuild = datastore.KeyForObj(c, parentRerunBuildModel)
		assert.Loosely(t, datastore.Put(c, suspect), should.BeNil)
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
		assert.Loosely(t, datastore.Put(c, singleRerun1), should.BeNil)
		assert.Loosely(t, datastore.Put(c, singleRerun2), should.BeNil)
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
		assert.Loosely(t, err, should.BeNil)
		datastore.Get(c, singleRerun1)
		assert.Loosely(t, singleRerun1.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_FAILED))
		datastore.Get(c, suspect)
		assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_UnderVerification))

		_, err = server.UpdateAnalysisProgress(c, req2)
		assert.Loosely(t, err, should.BeNil)
		datastore.Get(c, singleRerun2)
		assert.Loosely(t, singleRerun2.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_PASSED))
		datastore.Get(c, suspect)
		assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_ConfirmedCulprit))

		err = datastore.Get(c, analysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, analysis.Status, should.Equal(pb.AnalysisStatus_FOUND))
		assert.Loosely(t, analysis.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		assert.Loosely(t, len(analysis.VerifiedCulprits), should.Equal(1))
		assert.Loosely(t, analysis.VerifiedCulprits[0], should.Match(datastore.KeyForObj(c, suspect)))

		// Assert task
		task := &tpb.CancelAnalysisTask{
			AnalysisId: 1234,
		}
		expected := proto.Clone(task).(*tpb.CancelAnalysisTask)
		assert.Loosely(t, scheduler.Tasks().Payloads()[0], should.Match(expected))
	})

	ftt.Run("UpdateAnalysisProgress NthSection", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx

		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).AnyTimes()

		// Define hosts bots should talk to.
		c = hosts.UseHosts(c, hosts.ModuleOptions{
			APIHost: "test-bisection-host",
		})

		// Set up the config
		projectCfg := config.CreatePlaceholderProjectConfig()
		projectCfg.CompileAnalysisConfig.NthsectionEnabled = true
		projectCfg.CompileAnalysisConfig.CulpritVerificationEnabled = true
		cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
		assert.Loosely(t, config.SetTestProjectConfig(c, cfg), should.BeNil)

		t.Run("Schedule run for next commit", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:                 1234,
				CompileFailure:     datastore.KeyForObj(c, cf),
				FirstFailedBuildId: 1000,
			}
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, cfa),
				BlameList:      testutil.CreateBlamelist(10),
			}
			assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)

			// Set up reruns
			rerunBuildModel := &model.CompileRerunBuild{
				Id: 8800,
			}
			assert.Loosely(t, datastore.Put(c, rerunBuildModel), should.BeNil)

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
			assert.Loosely(t, datastore.Put(c, singleRerun), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(c, singleRerun), should.BeNil)
			assert.Loosely(t, singleRerun.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_FAILED))
			assert.Loosely(t, res, should.Match(&pb.UpdateAnalysisProgressResponse{}))
			// Check that another rerun is scheduled
			rr := &model.CompileRerunBuild{
				Id: 9999,
			}
			assert.Loosely(t, datastore.Get(c, rr), should.BeNil)
			assert.Loosely(t, datastore.Get(c, cfa), should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_RUNNING))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))
		})

		t.Run("Culprit found", func(t *ftt.Test) {
			c, scheduler := tq.TestingContext(c, nil)

			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
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
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, cfa),
				BlameList:      testutil.CreateBlamelist(10),
			}
			assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)

			// Set up reruns
			rerunBuildModel := &model.CompileRerunBuild{
				Id: 9876,
			}
			assert.Loosely(t, datastore.Put(c, rerunBuildModel), should.BeNil)
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

			assert.Loosely(t, datastore.Put(c, singleRerun1), should.BeNil)
			assert.Loosely(t, datastore.Put(c, singleRerun2), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(c, singleRerun1), should.BeNil)
			assert.Loosely(t, singleRerun1.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_FAILED))
			assert.Loosely(t, res, should.Match(&pb.UpdateAnalysisProgressResponse{}))

			// Check that the nthsection analysis is updated with Suspect
			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, datastore.Get(c, nsa), should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, nsa.Suspect, should.NotBeNil)
			nsaSuspect := &model.Suspect{
				Id:             nsa.Suspect.IntID(),
				ParentAnalysis: nsa.Suspect.Parent(),
			}
			assert.Loosely(t, datastore.Get(c, nsaSuspect), should.BeNil)
			assert.Loosely(t, nsaSuspect, should.Match(&model.Suspect{
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
			}))
			assert.Loosely(t, datastore.Get(c, cfa), should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))

			// Assert task
			task := &tpb.CulpritVerificationTask{
				AnalysisId: cfa.Id,
				SuspectId:  nsa.Suspect.IntID(),
				ParentKey:  datastore.KeyForObj(c, nsa).Encode(),
			}
			expected := proto.Clone(task).(*tpb.CulpritVerificationTask)
			assert.Loosely(t, scheduler.Tasks().Payloads()[0], should.Match(expected))
		})

		t.Run("Nthsection couldn't find suspect", func(t *ftt.Test) {
			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:             3457,
				CompileFailure: datastore.KeyForObj(c, cf),
			}
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, cfa),
				BlameList:      testutil.CreateBlamelist(10),
			}
			assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)

			// Set up reruns
			rerunBuildModel := &model.CompileRerunBuild{
				Id: 9768,
			}
			assert.Loosely(t, datastore.Put(c, rerunBuildModel), should.BeNil)
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

			assert.Loosely(t, datastore.Put(c, singleRerun1), should.BeNil)
			assert.Loosely(t, datastore.Put(c, singleRerun2), should.BeNil)
			assert.Loosely(t, datastore.Put(c, singleRerun3), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(c, singleRerun3), should.BeNil)
			assert.Loosely(t, singleRerun3.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_INFRA_FAILED))
			assert.Loosely(t, res, should.Match(&pb.UpdateAnalysisProgressResponse{}))

			// Check that the nthsection analysis is updated with Suspect
			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, datastore.Get(c, nsa), should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, datastore.Get(c, cfa), should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})

		t.Run("Nthsection regression range conflicts", func(t *ftt.Test) {
			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build: datastore.KeyForObj(c, fb),
			}
			assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:             2122,
				CompileFailure: datastore.KeyForObj(c, cf),
			}
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa := &model.CompileNthSectionAnalysis{
				ParentAnalysis: datastore.KeyForObj(c, cfa),
				BlameList:      testutil.CreateBlamelist(10),
			}
			assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)

			// Set up reruns
			rerunBuildModel := &model.CompileRerunBuild{
				Id: 4343,
			}
			assert.Loosely(t, datastore.Put(c, rerunBuildModel), should.BeNil)
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

			assert.Loosely(t, datastore.Put(c, singleRerun1), should.BeNil)
			assert.Loosely(t, datastore.Put(c, singleRerun2), should.BeNil)
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
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, datastore.Get(c, singleRerun2), should.BeNil)
			assert.Loosely(t, singleRerun2.Status, should.Equal(pb.RerunStatus_RERUN_STATUS_PASSED))

			// Check analysis status
			datastore.GetTestable(c).CatchupIndexes()
			assert.Loosely(t, datastore.Get(c, nsa), should.BeNil)
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, datastore.Get(c, cfa), should.BeNil)
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_NOTFOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
		})
	})

	ftt.Run("verifyUpdateAnalysisProgressRequest", t, func(t *ftt.Test) {
		req := &pb.UpdateAnalysisProgressRequest{}
		assert.Loosely(t, verifyUpdateAnalysisProgressRequest(c, req), should.NotBeNil)
		req.AnalysisId = 123
		assert.Loosely(t, verifyUpdateAnalysisProgressRequest(c, req), should.NotBeNil)
		req.Bbid = 8888
		assert.Loosely(t, verifyUpdateAnalysisProgressRequest(c, req), should.NotBeNil)
		req.GitilesCommit = &bbpb.GitilesCommit{}
		assert.Loosely(t, verifyUpdateAnalysisProgressRequest(c, req), should.NotBeNil)
		req.RerunResult = &pb.RerunResult{
			RerunStatus: pb.RerunStatus_RERUN_STATUS_FAILED,
		}
		assert.Loosely(t, verifyUpdateAnalysisProgressRequest(c, req), should.NotBeNil)
		req.BotId = "botid"
		assert.Loosely(t, verifyUpdateAnalysisProgressRequest(c, req), should.BeNil)
	})
}
