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

package bisection

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRunBisector(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)
	ctx, skdr := tq.TestingContext(ctx, nil)
	luciAnalysisClient := &fakeLUCIAnalysisClient{}

	// Define hosts bots should talk to.
	ctx = hosts.UseHosts(ctx, hosts.ModuleOptions{
		APIHost: "test-bisection-host",
	})

	// Mock gitiles response.
	gitilesResponse := model.ChangeLogResponse{
		Log: []*model.ChangeLog{
			{
				Commit:  "3426",
				Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472131\nReviewed-by: blah blah\n",
				Committer: model.ChangeLogActor{
					Time: "Tue Oct 17 07:06:57 2023",
				},
			},
			{
				Commit:  "3425",
				Message: "Second Commit\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472130\nReviewed-by: blah blah\n",
				Committer: model.ChangeLogActor{
					Time: "Tue Oct 17 07:06:57 2023",
				},
			},
			{
				Commit:  "3424",
				Message: "Third Commit\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472129\nReviewed-by: blah blah\n",
				Committer: model.ChangeLogActor{
					Time: "Tue Oct 17 07:06:57 2023",
				},
			},
		},
	}

	// To test the case there is only 1 commit in blame list.
	gitilesResponseSingleCommit := model.ChangeLogResponse{
		Log: []*model.ChangeLog{
			{
				Commit:  "3426",
				Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472131\nReviewed-by: blah blah\n",
				Committer: model.ChangeLogActor{
					Time: "Tue Oct 17 07:06:57 2023",
				},
			},
			{
				Commit:  "3425",
				Message: "Second Commit\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472130\nReviewed-by: blah blah\n",
				Committer: model.ChangeLogActor{
					Time: "Tue Oct 17 07:06:57 2023",
				},
			},
		},
	}

	gitilesResponseStr, err := json.Marshal(gitilesResponse)
	if err != nil {
		panic(err.Error())
	}

	gitilesResponseSingleCommitStr, err := json.Marshal(gitilesResponseSingleCommit)
	if err != nil {
		panic(err.Error())
	}

	ctx = gitiles.MockedGitilesClientContext(ctx, map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/12345^1..23456": string(gitilesResponseStr),
		"https://chromium.googlesource.com/chromium/src/+log/12346^1..23457": string(gitilesResponseSingleCommitStr),
	})

	// Setup mock for buildbucket.
	ctl := gomock.NewController(t)
	defer ctl.Finish()
	mc := buildbucket.NewMockedClient(ctx, ctl)
	ctx = mc.Ctx

	bootstrapProperties := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"bs_key_1": structpb.NewStringValue("bs_val_1"),
		},
	}

	getBuildRes := &bbpb.Build{
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "ci",
			Builder: "linux-test",
		},
		Input: &bbpb.Build_Input{
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"builder_group":         structpb.NewStringValue("buildergroup1"),
					"$bootstrap/properties": structpb.NewStructValue(bootstrapProperties),
					"another_prop":          structpb.NewStringValue("another_val"),
				},
			},
		},
		Infra: &bbpb.BuildInfra{
			Swarming: &bbpb.BuildInfra_Swarming{
				TaskDimensions: []*bbpb.RequestedDimension{
					{
						Key:   "key",
						Value: "val",
					},
				},
			},
		},
	}

	scheduleBuildRes := &bbpb.Build{
		Id: 8765,
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "test-single-revision",
		},
		Number:     10,
		Status:     bbpb.Status_SCHEDULED,
		CreateTime: timestamppb.New(time.Unix(1000, 0)),
		Input: &bbpb.Build_Input{
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "refs/heads/main",
				Id:      "hash",
			},
		},
	}

	scheduleBuildRes1 := &bbpb.Build{
		Id: 8766,
		Builder: &bbpb.BuilderID{
			Project: "chromium",
			Bucket:  "findit",
			Builder: "test-single-revision",
		},
		Number:     11,
		Status:     bbpb.Status_SCHEDULED,
		CreateTime: timestamppb.New(time.Unix(1001, 0)),
		Input: &bbpb.Build_Input{
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Ref:     "refs/heads/main",
				Id:      "hash1",
			},
		},
	}

	mc.Client.EXPECT().GetBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(getBuildRes, nil).AnyTimes()
	mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(scheduleBuildRes, nil).Times(1)
	mc.Client.EXPECT().ScheduleBuild(gomock.Any(), gomock.Any(), gomock.Any()).Return(scheduleBuildRes1, nil).Times(1)

	Convey("No analysis", t, func() {
		err := Run(ctx, 123, luciAnalysisClient, 1)
		So(err, ShouldNotBeNil)
	})

	Convey("Bisection is not enabled", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, nil)
		enableBisection(ctx, false, tfa.Project)

		err := Run(ctx, 1000, luciAnalysisClient, 1)
		So(err, ShouldBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_DISABLED)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
	})

	Convey("No primary failure", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID: 1001,
		})
		enableBisection(ctx, true, tfa.Project)

		err := Run(ctx, 1001, luciAnalysisClient, 1)
		So(err, ShouldNotBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
	})

	Convey("Unsupported project", t, func() {
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:      1002,
			Project: "chromeos",
		})
		enableBisection(ctx, true, tfa.Project)

		err := Run(ctx, 1002, luciAnalysisClient, 1)
		So(err, ShouldBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_UNSUPPORTED)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
	})

	Convey("unmatched number of the commit in the blamelist", t, func() {
		tf := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:        101,
			IsPrimary: true,
			Variant: map[string]string{
				"test_suite": "test_suite",
			},
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "refs/heads/main",
					},
				},
			},
			StartPosition: 1,
			EndPosition:   4,
		})
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:              1002,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "12345",
			EndCommitHash:   "23456",
			Priority:        160,
		})
		enableBisection(ctx, true, tfa.Project)

		err := Run(ctx, 1002, luciAnalysisClient, 1)
		So(err, ShouldNotBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
	})

	Convey("Supported project", t, func() {
		tf := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:        101,
			IsPrimary: true,
			Variant: map[string]string{
				"test_suite": "test_suite",
			},
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "refs/heads/main",
					},
				},
			},
			StartPosition: 1,
			EndPosition:   3,
		})
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:              1002,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "12345",
			EndCommitHash:   "23456",
			Priority:        160,
		})
		enableBisection(ctx, true, tfa.Project)

		// Link the test failure with test failure analysis.
		tf.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		So(datastore.Put(ctx, tf), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		err := Run(ctx, 1002, luciAnalysisClient, 2)
		So(err, ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_RUNNING)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)
		err = datastore.Get(ctx, tf)
		So(err, ShouldBeNil)
		So(tf.TestSuiteName, ShouldEqual, "test_suite")
		So(tf.TestName, ShouldEqual, "test_name_0")

		// Check nthsection analysis.
		q := datastore.NewQuery("TestNthSectionAnalysis").Eq("parent_analysis_key", datastore.KeyForObj(ctx, tfa))
		nthSectionAnalyses := []*model.TestNthSectionAnalysis{}
		err = datastore.GetAll(ctx, q, &nthSectionAnalyses)
		So(err, ShouldBeNil)
		So(len(nthSectionAnalyses), ShouldEqual, 1)
		nsa := nthSectionAnalyses[0]

		So(nsa, ShouldResembleProto, &model.TestNthSectionAnalysis{
			ID:                nsa.ID,
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			StartTime:         nsa.StartTime,
			Status:            pb.AnalysisStatus_RUNNING,
			RunStatus:         pb.AnalysisRunStatus_STARTED,
			BlameList: &pb.BlameList{
				Commits: []*pb.BlameListSingleCommit{
					{
						Commit:      "3426",
						ReviewTitle: "Use TestActivationManager for all page activations",
						ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472131",
						Position:    3,
						CommitTime:  timestamppb.New(time.Date(2023, time.October, 17, 7, 6, 57, 0, time.UTC)),
					},
					{
						Commit:      "3425",
						ReviewTitle: "Second Commit",
						ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472130",
						Position:    2,
						CommitTime:  timestamppb.New(time.Date(2023, time.October, 17, 7, 6, 57, 0, time.UTC)),
					},
				},
				LastPassCommit: &pb.BlameListSingleCommit{
					Commit:      "3424",
					ReviewTitle: "Third Commit",
					ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472129",
					Position:    1,
					CommitTime:  timestamppb.New(time.Date(2023, time.October, 17, 7, 6, 57, 0, time.UTC)),
				},
			},
		})

		// Check that rerun model was created.
		q = datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
		reruns := []*model.TestSingleRerun{}
		err = datastore.GetAll(ctx, q, &reruns)
		So(err, ShouldBeNil)
		So(len(reruns), ShouldEqual, 2)

		So(reruns[0], ShouldResembleProto, &model.TestSingleRerun{
			ID:                    8765,
			Type:                  model.RerunBuildType_NthSection,
			AnalysisKey:           datastore.KeyForObj(ctx, tfa),
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			Priority:              160,
			Dimensions: &pb.Dimensions{
				Dimensions: []*pb.Dimension{
					{
						Key:   "key",
						Value: "val",
					},
				},
			},
			LUCIBuild: model.LUCIBuild{
				BuildID:     8765,
				Project:     "chromium",
				Bucket:      "findit",
				Builder:     "test-single-revision",
				BuildNumber: 10,
				CreateTime:  time.Unix(1000, 0),
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "refs/heads/main",
					Id:      "hash",
				},
				Status: bbpb.Status_SCHEDULED,
			},
			TestResults: model.RerunTestResults{
				IsFinalized: false,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey: datastore.KeyForObj(ctx, tf),
					},
				},
			},
		})

		So(reruns[1], ShouldResembleProto, &model.TestSingleRerun{
			ID:                    8766,
			Type:                  model.RerunBuildType_NthSection,
			AnalysisKey:           datastore.KeyForObj(ctx, tfa),
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			Priority:              160,
			Dimensions: &pb.Dimensions{
				Dimensions: []*pb.Dimension{
					{
						Key:   "key",
						Value: "val",
					},
				},
			},
			LUCIBuild: model.LUCIBuild{
				BuildID:     8766,
				Project:     "chromium",
				Bucket:      "findit",
				Builder:     "test-single-revision",
				BuildNumber: 11,
				CreateTime:  time.Unix(1001, 0),
				GitilesCommit: &bbpb.GitilesCommit{
					Host:    "chromium.googlesource.com",
					Project: "chromium/src",
					Ref:     "refs/heads/main",
					Id:      "hash1",
				},
				Status: bbpb.Status_SCHEDULED,
			},
			TestResults: model.RerunTestResults{
				IsFinalized: false,
				Results: []model.RerunSingleTestResult{
					{
						TestFailureKey: datastore.KeyForObj(ctx, tf),
					},
				},
			},
		})
	})

	Convey("Only 1 commit in blame list", t, func() {
		tf := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:        103,
			IsPrimary: true,
			Variant: map[string]string{
				"test_suite": "test_suite",
			},
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    "chromium.googlesource.com",
						Project: "chromium/src",
						Ref:     "refs/heads/main",
					},
				},
			},
			StartPosition: 1,
			EndPosition:   2,
		})
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:              1003,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "12346",
			EndCommitHash:   "23457",
			Priority:        160,
		})
		enableBisection(ctx, true, tfa.Project)

		// Link the test failure with test failure analysis.
		tf.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		So(datastore.Put(ctx, tf), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		err := Run(ctx, 1003, luciAnalysisClient, 2)
		So(err, ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_SUSPECTFOUND)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_STARTED)
		err = datastore.Get(ctx, tf)
		So(err, ShouldBeNil)
		So(tf.TestSuiteName, ShouldEqual, "test_suite")
		So(tf.TestName, ShouldEqual, "test_name_0")

		// Check suspect is created.
		q := datastore.NewQuery("Suspect")
		suspects := []*model.Suspect{}
		err = datastore.GetAll(ctx, q, &suspects)
		So(err, ShouldBeNil)
		So(len(suspects), ShouldEqual, 1)
		suspect := suspects[0]

		// Check nthsection analysis.
		q = datastore.NewQuery("TestNthSectionAnalysis").Eq("parent_analysis_key", datastore.KeyForObj(ctx, tfa))
		nthSectionAnalyses := []*model.TestNthSectionAnalysis{}
		err = datastore.GetAll(ctx, q, &nthSectionAnalyses)
		So(err, ShouldBeNil)
		So(len(nthSectionAnalyses), ShouldEqual, 1)
		nsa := nthSectionAnalyses[0]

		So(nsa, ShouldResembleProto, &model.TestNthSectionAnalysis{
			ID:                nsa.ID,
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
			StartTime:         nsa.StartTime,
			EndTime:           time.Unix(10000, 0).UTC(),
			Status:            pb.AnalysisStatus_SUSPECTFOUND,
			RunStatus:         pb.AnalysisRunStatus_ENDED,
			BlameList: &pb.BlameList{
				Commits: []*pb.BlameListSingleCommit{
					{
						Commit:      "3426",
						ReviewTitle: "Use TestActivationManager for all page activations",
						ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472131",
						Position:    2,
						CommitTime:  timestamppb.New(time.Date(2023, time.October, 17, 7, 6, 57, 0, time.UTC)),
					},
				},
				LastPassCommit: &pb.BlameListSingleCommit{
					Commit:      "3425",
					ReviewTitle: "Second Commit",
					ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472130",
					Position:    1,
					CommitTime:  timestamppb.New(time.Date(2023, time.October, 17, 7, 6, 57, 0, time.UTC)),
				},
			},
			CulpritKey: datastore.KeyForObj(ctx, suspect),
		})

		// Verify suspect.
		So(suspect, ShouldResemble, &model.Suspect{
			Id:             suspect.Id,
			Type:           model.SuspectType_NthSection,
			ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			ReviewUrl:      "https://chromium-review.googlesource.com/c/chromium/src/+/3472131",
			ReviewTitle:    "Use TestActivationManager for all page activations",
			GitilesCommit: bbpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "chromium/src",
				Id:      "3426",
				Ref:     "refs/heads/main",
			},
			AnalysisType:       pb.AnalysisType_TEST_FAILURE_ANALYSIS,
			VerificationStatus: model.SuspectVerificationStatus_Unverified,
			CommitTime:         time.Date(2023, time.October, 17, 7, 6, 57, 0, time.UTC),
		})

		// Check that no rerun models were created.
		q = datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
		reruns := []*model.TestSingleRerun{}
		err = datastore.GetAll(ctx, q, &reruns)
		So(err, ShouldBeNil)
		So(len(reruns), ShouldEqual, 0)

		// Check that a task was created.
		So(len(skdr.Tasks().Payloads()), ShouldEqual, 1)
		resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureCulpritVerificationTask)
		So(resultsTask, ShouldResembleProto, &tpb.TestFailureCulpritVerificationTask{
			AnalysisId: tfa.ID,
		})
	})
}

func TestCreateSnapshot(t *testing.T) {
	t.Parallel()
	c := memory.Use(context.Background())
	testutil.UpdateIndices(c)

	Convey("Create Snapshot", t, func() {
		tfa := &model.TestFailureAnalysis{}
		So(datastore.Put(c, tfa), ShouldBeNil)
		datastore.GetTestable(c).CatchupIndexes()
		blamelist := testutil.CreateBlamelist(5)
		nsa := &model.TestNthSectionAnalysis{
			BlameList:         blamelist,
			ParentAnalysisKey: datastore.KeyForObj(c, tfa),
		}
		So(datastore.Put(c, nsa), ShouldBeNil)

		rerun1 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit1",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}

		So(datastore.Put(c, rerun1), ShouldBeNil)

		rerun2 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit3",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		So(datastore.Put(c, rerun2), ShouldBeNil)

		rerun3 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit0",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}

		So(datastore.Put(c, rerun3), ShouldBeNil)

		rerun4 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit2",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		So(datastore.Put(c, rerun4), ShouldBeNil)

		rerun5 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED,
			LUCIBuild: model.LUCIBuild{
				GitilesCommit: &bbpb.GitilesCommit{
					Id: "commit4",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		So(datastore.Put(c, rerun5), ShouldBeNil)

		datastore.GetTestable(c).CatchupIndexes()

		snapshot, err := CreateSnapshot(c, nsa)
		So(err, ShouldBeNil)
		So(snapshot.BlameList, ShouldResembleProto, blamelist)
		So(snapshot.NumInProgress, ShouldEqual, 2)
		So(snapshot.NumInfraFailed, ShouldEqual, 1)
		So(snapshot.NumTestSkipped, ShouldEqual, 1)
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
				Type:   model.RerunBuildType_NthSection,
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
			{
				Index:  4,
				Commit: "commit4",
				Status: pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED,
				Type:   model.RerunBuildType_NthSection,
			},
		})
	})
}

func enableBisection(ctx context.Context, enabled bool, project string) {
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.TestAnalysisConfig.BisectorEnabled = enabled
	cfg := map[string]*configpb.ProjectConfig{project: projectCfg}
	So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)
}

type fakeLUCIAnalysisClient struct {
}

func (cl *fakeLUCIAnalysisClient) ReadLatestVerdict(ctx context.Context, project string, keys []lucianalysis.TestVerdictKey) (map[lucianalysis.TestVerdictKey]lucianalysis.TestVerdictResult, error) {
	results := map[lucianalysis.TestVerdictKey]lucianalysis.TestVerdictResult{}
	for i, key := range keys {
		results[key] = lucianalysis.TestVerdictResult{
			TestName: fmt.Sprintf("test_name_%d", i),
			Status:   pb.TestVerdictStatus_EXPECTED,
		}
	}
	return results, nil
}
