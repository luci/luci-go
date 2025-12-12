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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"
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

	ftt.Run("No analysis", t, func(t *ftt.Test) {
		err := Run(ctx, 123, luciAnalysisClient)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Bisection is not enabled", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, nil)
		enableBisection(ctx, t, false, tfa.Project)

		err := Run(ctx, 1000, luciAnalysisClient)
		assert.Loosely(t, err, should.BeNil)
		err = datastore.Get(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_DISABLED))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
	})

	ftt.Run("No primary failure", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID: 1001,
		})
		enableBisection(ctx, t, true, tfa.Project)

		err := Run(ctx, 1001, luciAnalysisClient)
		assert.Loosely(t, err, should.NotBeNil)
		err = datastore.Get(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_ERROR))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
	})

	ftt.Run("Unsupported project", t, func(t *ftt.Test) {
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:      1002,
			Project: "chromeos",
		})
		enableBisection(ctx, t, true, tfa.Project)

		err := Run(ctx, 1002, luciAnalysisClient)
		assert.Loosely(t, err, should.BeNil)
		err = datastore.Get(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_UNSUPPORTED))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
	})

	ftt.Run("unmatched number of the commit in the blamelist", t, func(t *ftt.Test) {
		tf := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
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
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:              1002,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "12345",
			EndCommitHash:   "23456",
			Priority:        160,
		})
		enableBisection(ctx, t, true, tfa.Project)

		err := Run(ctx, 1002, luciAnalysisClient)
		assert.Loosely(t, err, should.NotBeNil)
		err = datastore.Get(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_ERROR))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))
	})

	ftt.Run("Supported project", t, func(t *ftt.Test) {
		tf := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
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
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:              1002,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "12345",
			EndCommitHash:   "23456",
			Priority:        160,
		})
		enableBisection(ctx, t, true, tfa.Project)

		// Link the test failure with test failure analysis.
		tf.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		assert.Loosely(t, datastore.Put(ctx, tf), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		err := Run(ctx, 1002, luciAnalysisClient)
		assert.Loosely(t, err, should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err = datastore.Get(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_RUNNING))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))
		err = datastore.Get(ctx, tf)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tf.TestSuiteName, should.Equal("test_suite"))
		assert.Loosely(t, tf.TestName, should.Equal("test_name_0"))

		// Check nthsection analysis.
		q := datastore.NewQuery("TestNthSectionAnalysis").Eq("parent_analysis_key", datastore.KeyForObj(ctx, tfa))
		nthSectionAnalyses := []*model.TestNthSectionAnalysis{}
		err = datastore.GetAll(ctx, q, &nthSectionAnalyses)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(nthSectionAnalyses), should.Equal(1))
		nsa := nthSectionAnalyses[0]

		assert.Loosely(t, nsa, should.Match(&model.TestNthSectionAnalysis{
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
		}))

		// Check that rerun model was created.
		q = datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
		reruns := []*model.TestSingleRerun{}
		err = datastore.GetAll(ctx, q, &reruns)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(reruns), should.Equal(2))

		assert.Loosely(t, reruns[0], should.Match(&model.TestSingleRerun{
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
		}))

		assert.Loosely(t, reruns[1], should.Match(&model.TestSingleRerun{
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
		}))
	})

	ftt.Run("Only 1 commit in blame list", t, func(t *ftt.Test) {
		tf := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
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
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:              1003,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "12346",
			EndCommitHash:   "23457",
			Priority:        160,
		})
		enableBisection(ctx, t, true, tfa.Project)

		// Link the test failure with test failure analysis.
		tf.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		assert.Loosely(t, datastore.Put(ctx, tf), should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		err := Run(ctx, 1003, luciAnalysisClient)
		assert.Loosely(t, err, should.BeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		err = datastore.Get(ctx, tfa)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
		assert.Loosely(t, tfa.RunStatus, should.Equal(pb.AnalysisRunStatus_STARTED))
		err = datastore.Get(ctx, tf)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tf.TestSuiteName, should.Equal("test_suite"))
		assert.Loosely(t, tf.TestName, should.Equal("test_name_0"))

		// Check suspect is created.
		q := datastore.NewQuery("Suspect")
		suspects := []*model.Suspect{}
		err = datastore.GetAll(ctx, q, &suspects)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(1))
		suspect := suspects[0]

		// Check nthsection analysis.
		q = datastore.NewQuery("TestNthSectionAnalysis").Eq("parent_analysis_key", datastore.KeyForObj(ctx, tfa))
		nthSectionAnalyses := []*model.TestNthSectionAnalysis{}
		err = datastore.GetAll(ctx, q, &nthSectionAnalyses)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(nthSectionAnalyses), should.Equal(1))
		nsa := nthSectionAnalyses[0]

		assert.Loosely(t, nsa, should.Match(&model.TestNthSectionAnalysis{
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
		}))

		// Verify suspect.
		assert.Loosely(t, suspect, should.Match(&model.Suspect{
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
		}))

		// Check that no rerun models were created.
		q = datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa))
		reruns := []*model.TestSingleRerun{}
		err = datastore.GetAll(ctx, q, &reruns)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(reruns), should.BeZero)

		// Check that a task was created.
		assert.Loosely(t, len(skdr.Tasks().Payloads()), should.Equal(1))
		resultsTask := skdr.Tasks().Payloads()[0].(*tpb.TestFailureCulpritVerificationTask)
		assert.Loosely(t, resultsTask, should.Match(&tpb.TestFailureCulpritVerificationTask{
			AnalysisId: tfa.ID,
		}))
	})
}

func enableBisection(ctx context.Context, t testing.TB, enabled bool, project string) {
	t.Helper()
	projectCfg := config.CreatePlaceholderProjectConfig()
	projectCfg.TestAnalysisConfig.BisectorEnabled = enabled
	cfg := map[string]*configpb.ProjectConfig{project: projectCfg}
	assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil, truth.LineContext())
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
