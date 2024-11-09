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

package nthsection

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"
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

	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	ftt.Run("Create Snapshot", t, func(t *ftt.Test) {
		analysis := &model.CompileFailureAnalysis{}
		assert.Loosely(t, datastore.Put(c, analysis), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		blamelist := testutil.CreateBlamelist(4)
		nthSectionAnalysis := &model.CompileNthSectionAnalysis{
			BlameList:      blamelist,
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		assert.Loosely(t, datastore.Put(c, nthSectionAnalysis), should.BeNil)

		rerun1 := &model.SingleRerun{
			Type:   model.RerunBuildType_CulpritVerification,
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit1",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		assert.Loosely(t, datastore.Put(c, rerun1), should.BeNil)

		rerun2 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit3",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}
		assert.Loosely(t, datastore.Put(c, rerun2), should.BeNil)

		rerun3 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit0",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		assert.Loosely(t, datastore.Put(c, rerun3), should.BeNil)

		rerun4 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit2",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		assert.Loosely(t, datastore.Put(c, rerun4), should.BeNil)

		datastore.GetTestable(c).CatchupIndexes()

		snapshot, err := CreateSnapshot(c, nthSectionAnalysis)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, snapshot.BlameList, should.Resemble(blamelist))

		assert.Loosely(t, snapshot.NumInProgress, should.Equal(2))
		assert.Loosely(t, snapshot.NumInfraFailed, should.Equal(1))
		assert.Loosely(t, snapshot.Runs, should.Resemble([]*nthsectionsnapshot.Run{
			{
				Index:  0,
				Commit: "commit0",
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  1,
				Commit: "commit1",
				Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
				Type:   model.RerunBuildType_CulpritVerification,
			},
			{
				Index:  2,
				Commit: "commit2",
				Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
				Type:   model.RerunBuildType_NthSection,
			},
			{
				Index:  3,
				Commit: "commit3",
				Status: pb.RerunStatus_RERUN_STATUS_FAILED,
				Type:   model.RerunBuildType_NthSection,
			},
		}))
	})
}

func TestAnalyze(t *testing.T) {
	t.Parallel()
	ftt.Run("TestAnalyze", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		testutil.UpdateIndices(c)
		cl := testclock.New(testclock.TestTimeUTC)
		c = clock.Set(c, cl)

		c = hosts.UseHosts(c, hosts.ModuleOptions{
			APIHost: "test-bisection-host",
		})

		// Set up the config
		projectCfg := config.CreatePlaceholderProjectConfig()
		cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
		assert.Loosely(t, config.SetTestProjectConfig(c, cfg), should.BeNil)

		gitilesResponse := model.ChangeLogResponse{
			Log: []*model.ChangeLog{
				{
					Commit:  "3426",
					Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472131\nReviewed-by: blah blah\n",
				},
				{
					Commit:  "3425",
					Message: "Second Commit\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472130\nReviewed-by: blah blah\n",
				},
				{
					Commit:  "3424",
					Message: "Third Commit\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472129\nReviewed-by: blah blah\n",
				},
			},
		}
		gitilesResponseStr, err := json.Marshal(gitilesResponse)
		if err != nil {
			panic(err.Error())
		}

		// To test the case there is only 1 commit in blame list.
		gitilesResponseSingleCommit := model.ChangeLogResponse{
			Log: []*model.ChangeLog{
				{
					Commit:  "3426",
					Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472131\nReviewed-by: blah blah\n",
				},
				{
					Commit:  "3425",
					Message: "Second Commit\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472130\nReviewed-by: blah blah\n",
				},
			},
		}
		gitilesResponseSingleCommitStr, err := json.Marshal(gitilesResponseSingleCommit)
		if err != nil {
			panic(err.Error())
		}

		c = gitiles.MockedGitilesClientContext(c, map[string]string{
			"https://chromium.googlesource.com/chromium/src/+log/12345^1..23456": string(gitilesResponseStr),
			"https://chromium.googlesource.com/chromium/src/+log/12346^1..23457": string(gitilesResponseSingleCommitStr),
		})

		// Setup mock for buildbucket
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := buildbucket.NewMockedClient(c, ctl)
		c = mc.Ctx
		res := &bbpb.Build{
			Builder: &bbpb.BuilderID{
				Project: "chromium",
				Bucket:  "findit",
				Builder: "single-revision",
			},
			Input: &bbpb.Build_Input{
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "host",
					Project: "proj",
					Id:      "id1",
					Ref:     "ref",
				},
			},
			Id:         123,
			Status:     bbpb.Status_STARTED,
			CreateTime: &timestamppb.Timestamp{Seconds: 100},
			StartTime:  &timestamppb.Timestamp{Seconds: 101},
		}
		mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(res, nil).AnyTimes()
		mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(&bbpb.Build{}, nil).AnyTimes()

		t.Run("CheckBlameList", func(t *ftt.Test) {
			rr := &pb.RegressionRange{
				LastPassed: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "12345",
				},
				FirstFailed: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "23456",
				},
			}

			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build:         datastore.KeyForObj(c, fb),
				OutputTargets: []string{"abc.xyz"},
			}
			assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:                     123,
				CompileFailure:         datastore.KeyForObj(c, cf),
				InitialRegressionRange: rr,
				FirstFailedBuildId:     1000,
			}
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa, err := Analyze(c, cfa)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa, should.NotBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Fetch the nth section analysis
			q := datastore.NewQuery("CompileNthSectionAnalysis")
			nthsectionAnalyses := []*model.CompileNthSectionAnalysis{}
			err = datastore.GetAll(c, q, &nthsectionAnalyses)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(nthsectionAnalyses), should.Equal(1))
			nsa = nthsectionAnalyses[0]

			assert.Loosely(t, nsa.BlameList, should.Resemble(&pb.BlameList{
				Commits: []*pb.BlameListSingleCommit{
					{
						Commit:      "3426",
						ReviewTitle: "Use TestActivationManager for all page activations",
						ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472131",
					},
					{
						Commit:      "3425",
						ReviewTitle: "Second Commit",
						ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472130",
					},
				},
				LastPassCommit: &pb.BlameListSingleCommit{
					Commit:      "3424",
					ReviewTitle: "Third Commit",
					ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472129",
				},
			}))
		})

		t.Run("Only 1 commit in blamelist", func(t *ftt.Test) {
			c, skdr := tq.TestingContext(c, nil)
			rr := &pb.RegressionRange{
				LastPassed: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "refs/heads/main",
					Id:      "12346",
				},
				FirstFailed: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "refs/heads/main",
					Id:      "23457",
				},
			}

			fb := &model.LuciFailedBuild{
				LuciBuild: model.LuciBuild{Project: "chromium"},
			}
			assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build:         datastore.KeyForObj(c, fb),
				OutputTargets: []string{"abc.xyz"},
			}
			assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:                     124,
				CompileFailure:         datastore.KeyForObj(c, cf),
				InitialRegressionRange: rr,
				FirstFailedBuildId:     1000,
			}
			assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa, err := Analyze(c, cfa)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, nsa, should.NotBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Fetch the nth section analysis
			q := datastore.NewQuery("CompileNthSectionAnalysis").Ancestor(datastore.KeyForObj(c, cfa))
			nthsectionAnalyses := []*model.CompileNthSectionAnalysis{}
			err = datastore.GetAll(c, q, &nthsectionAnalyses)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(nthsectionAnalyses), should.Equal(1))
			nsa = nthsectionAnalyses[0]
			assert.Loosely(t, nsa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, nsa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
			assert.Loosely(t, cfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
			assert.Loosely(t, cfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))

			// Check that suspect was created.
			q = datastore.NewQuery("Suspect")
			suspects := []*model.Suspect{}
			err = datastore.GetAll(c, q, &suspects)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(suspects), should.Equal(1))
			suspect := suspects[0]
			assert.Loosely(t, suspect, should.Resemble(&model.Suspect{
				Id:             suspect.Id,
				Type:           model.SuspectType_NthSection,
				ParentAnalysis: datastore.KeyForObj(c, nsa),
				ReviewUrl:      "https://chromium-review.googlesource.com/c/chromium/src/+/3472131",
				ReviewTitle:    "Use TestActivationManager for all page activations",
				GitilesCommit: bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Id:      "3426",
					Ref:     "refs/heads/main",
				},
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
				VerificationStatus: model.SuspectVerificationStatus_VerificationScheduled,
			}))

			// Check that a task was created.
			assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1))
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.CulpritVerificationTask)
			assert.Loosely(t, resultsTask, should.Resemble(&tpb.CulpritVerificationTask{
				AnalysisId: cfa.Id,
				SuspectId:  suspect.Id,
				ParentKey:  nsa.Suspect.Parent().Encode(),
			}))
		})
	})
}

func TestGetPriority(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	ftt.Run("Get Priority", t, func(t *ftt.Test) {
		fb := &model.LuciFailedBuild{}
		assert.Loosely(t, datastore.Put(c, fb), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := &model.CompileFailure{
			Build: datastore.KeyForObj(c, fb),
		}
		assert.Loosely(t, datastore.Put(c, cf), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, cf),
		}
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		assert.Loosely(t, datastore.Put(c, nsa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		pri, err := getRerunPriority(c, nsa, nil, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(110))
		pri, err = getRerunPriority(c, nsa, nil, map[string]string{"id": "1"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(95))

		cfa.IsTreeCloser = true
		assert.Loosely(t, datastore.Put(c, cfa), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = getRerunPriority(c, nsa, nil, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, pri, should.Equal(40))
	})
}
