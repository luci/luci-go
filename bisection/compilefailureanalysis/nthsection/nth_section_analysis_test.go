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
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"

	. "github.com/smartystreets/goconvey/convey"
	tpb "go.chromium.org/luci/bisection/task/proto"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	Convey("Create Snapshot", t, func() {
		analysis := &model.CompileFailureAnalysis{}
		So(datastore.Put(c, analysis), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		blamelist := testutil.CreateBlamelist(4)
		nthSectionAnalysis := &model.CompileNthSectionAnalysis{
			BlameList:      blamelist,
			ParentAnalysis: datastore.KeyForObj(c, analysis),
		}
		So(datastore.Put(c, nthSectionAnalysis), ShouldBeNil)

		rerun1 := &model.SingleRerun{
			Type:   model.RerunBuildType_CulpritVerification,
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit1",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		So(datastore.Put(c, rerun1), ShouldBeNil)

		rerun2 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit3",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}
		So(datastore.Put(c, rerun2), ShouldBeNil)

		rerun3 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit0",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		So(datastore.Put(c, rerun3), ShouldBeNil)

		rerun4 := &model.SingleRerun{
			Type:   model.RerunBuildType_NthSection,
			Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
			GitilesCommit: bbpb.GitilesCommit{
				Id: "commit2",
			},
			Analysis: datastore.KeyForObj(c, analysis),
		}

		So(datastore.Put(c, rerun4), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		snapshot, err := CreateSnapshot(c, nthSectionAnalysis)
		So(err, ShouldBeNil)
		So(snapshot.BlameList, ShouldResembleProto, blamelist)

		So(snapshot.NumInProgress, ShouldEqual, 2)
		So(snapshot.NumInfraFailed, ShouldEqual, 1)
		So(snapshot.Runs, ShouldResemble, []*nthsectionsnapshot.Run{
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
		})
	})
}

func TestAnalyze(t *testing.T) {
	t.Parallel()
	Convey("TestAnalyze", t, func() {
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
		So(config.SetTestProjectConfig(c, cfg), ShouldBeNil)

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

		Convey("CheckBlameList", func() {
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
			So(datastore.Put(c, fb), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build:         datastore.KeyForObj(c, fb),
				OutputTargets: []string{"abc.xyz"},
			}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:                     123,
				CompileFailure:         datastore.KeyForObj(c, cf),
				InitialRegressionRange: rr,
				FirstFailedBuildId:     1000,
			}
			So(datastore.Put(c, cfa), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa, err := Analyze(c, cfa)
			So(err, ShouldBeNil)
			So(nsa, ShouldNotBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Fetch the nth section analysis
			q := datastore.NewQuery("CompileNthSectionAnalysis")
			nthsectionAnalyses := []*model.CompileNthSectionAnalysis{}
			err = datastore.GetAll(c, q, &nthsectionAnalyses)
			So(err, ShouldBeNil)
			So(len(nthsectionAnalyses), ShouldEqual, 1)
			nsa = nthsectionAnalyses[0]

			So(nsa.BlameList, ShouldResembleProto, &pb.BlameList{
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
			})
		})

		Convey("Only 1 commit in blamelist", func() {
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
			So(datastore.Put(c, fb), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cf := &model.CompileFailure{
				Build:         datastore.KeyForObj(c, fb),
				OutputTargets: []string{"abc.xyz"},
			}
			So(datastore.Put(c, cf), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			cfa := &model.CompileFailureAnalysis{
				Id:                     124,
				CompileFailure:         datastore.KeyForObj(c, cf),
				InitialRegressionRange: rr,
				FirstFailedBuildId:     1000,
			}
			So(datastore.Put(c, cfa), ShouldBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			nsa, err := Analyze(c, cfa)
			So(err, ShouldBeNil)
			So(nsa, ShouldNotBeNil)
			datastore.GetTestable(c).CatchupIndexes()

			// Fetch the nth section analysis
			q := datastore.NewQuery("CompileNthSectionAnalysis").Ancestor(datastore.KeyForObj(c, cfa))
			nthsectionAnalyses := []*model.CompileNthSectionAnalysis{}
			err = datastore.GetAll(c, q, &nthsectionAnalyses)
			So(err, ShouldBeNil)
			So(len(nthsectionAnalyses), ShouldEqual, 1)
			nsa = nthsectionAnalyses[0]
			So(nsa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(nsa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
			So(cfa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
			So(cfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)

			// Check that suspect was created.
			q = datastore.NewQuery("Suspect")
			suspects := []*model.Suspect{}
			err = datastore.GetAll(c, q, &suspects)
			So(err, ShouldBeNil)
			So(len(suspects), ShouldEqual, 1)
			suspect := suspects[0]
			So(suspect, ShouldResemble, &model.Suspect{
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
			})

			// Check that a task was created.
			So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
			resultsTask := skdr.Tasks().Payloads()[0].(*tpb.CulpritVerificationTask)
			So(resultsTask, ShouldResembleProto, &tpb.CulpritVerificationTask{
				AnalysisId: cfa.Id,
				SuspectId:  suspect.Id,
				ParentKey:  nsa.Suspect.Parent().Encode(),
			})
		})
	})
}

func TestGetPriority(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	Convey("Get Priority", t, func() {
		fb := &model.LuciFailedBuild{}
		So(datastore.Put(c, fb), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cf := &model.CompileFailure{
			Build: datastore.KeyForObj(c, fb),
		}
		So(datastore.Put(c, cf), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		cfa := &model.CompileFailureAnalysis{
			CompileFailure: datastore.KeyForObj(c, cf),
		}
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(c, cfa),
		}
		So(datastore.Put(c, nsa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()

		pri, err := getRerunPriority(c, nsa, nil, nil)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 110)
		pri, err = getRerunPriority(c, nsa, nil, map[string]string{"id": "1"})
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 95)

		cfa.IsTreeCloser = true
		So(datastore.Put(c, cfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		pri, err = getRerunPriority(c, nsa, nil, nil)
		So(err, ShouldBeNil)
		So(pri, ShouldEqual, 40)
	})
}
