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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestRunBisector(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testutil.UpdateIndices(ctx)
	cl := testclock.New(testclock.TestTimeUTC)
	cl.Set(time.Unix(10000, 0).UTC())
	ctx = clock.Set(ctx, cl)
	luciAnalysisClient := &fakeLUCIAnalysisClient{}

	// Mock gitiles response.
	gitilesResponse := model.ChangeLogResponse{
		Log: []*model.ChangeLog{
			{
				Commit:  "3424",
				Message: "Use TestActivationManager for all page activations\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472129\nReviewed-by: blah blah\n",
			},
			{
				Commit:  "3425",
				Message: "Second Commit\n\nblah blah\n\nChange-Id: blah\nBug: blah\nReviewed-on: https://chromium-review.googlesource.com/c/chromium/src/+/3472130\nReviewed-by: blah blah\n",
			},
		},
	}
	gitilesResponseStr, err := json.Marshal(gitilesResponse)
	if err != nil {
		panic(err.Error())
	}

	ctx = gitiles.MockedGitilesClientContext(ctx, map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/12345..23456": string(gitilesResponseStr),
	})

	Convey("No analysis", t, func() {
		err := Run(ctx, 123, luciAnalysisClient)
		So(err, ShouldNotBeNil)
	})

	Convey("Bisection is not enabled", t, func() {
		enableBisection(ctx, false)
		tfa := testutil.CreateTestFailureAnalysis(ctx, nil)

		err := Run(ctx, 1000, luciAnalysisClient)
		So(err, ShouldBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_DISABLED)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
	})

	Convey("No primary failure", t, func() {
		enableBisection(ctx, true)
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID: 1001,
		})

		err := Run(ctx, 1001, luciAnalysisClient)
		So(err, ShouldNotBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_ERROR)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
	})

	Convey("Unsupported project", t, func() {
		enableBisection(ctx, true)
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:      1002,
			Project: "chromeos",
		})

		err := Run(ctx, 1002, luciAnalysisClient)
		So(err, ShouldBeNil)
		err = datastore.Get(ctx, tfa)
		So(err, ShouldBeNil)
		So(tfa.Status, ShouldEqual, pb.AnalysisStatus_UNSUPPORTED)
		So(tfa.RunStatus, ShouldEqual, pb.AnalysisRunStatus_ENDED)
	})

	Convey("Supported project", t, func() {
		enableBisection(ctx, true)
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
		})
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			ID:              1002,
			TestFailure:     tf,
			StartCommitHash: "12345",
			EndCommitHash:   "23456",
		})

		// Link the test failure with test failure analysis.
		tf.AnalysisKey = datastore.KeyForObj(ctx, tfa)
		So(datastore.Put(ctx, tf), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		err := Run(ctx, 1002, luciAnalysisClient)
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
		q := datastore.NewQuery("TestNthSectionAnalysis").Ancestor(datastore.KeyForObj(ctx, tfa))
		nthSectionAnalyses := []*model.TestNthSectionAnalysis{}
		err = datastore.GetAll(ctx, q, &nthSectionAnalyses)
		So(err, ShouldBeNil)
		So(len(nthSectionAnalyses), ShouldEqual, 1)
		nsa := nthSectionAnalyses[0]

		So(nsa, ShouldResembleProto, &model.TestNthSectionAnalysis{
			ID:             nsa.ID,
			ParentAnalysis: datastore.KeyForObj(ctx, tfa),
			StartTime:      nsa.StartTime,
			Status:         pb.AnalysisStatus_RUNNING,
			RunStatus:      pb.AnalysisRunStatus_STARTED,
			BlameList: &pb.BlameList{
				Commits: []*pb.BlameListSingleCommit{
					{
						Commit:      "3424",
						ReviewTitle: "Use TestActivationManager for all page activations",
						ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472129",
					},
					{
						Commit:      "3425",
						ReviewTitle: "Second Commit",
						ReviewUrl:   "https://chromium-review.googlesource.com/c/chromium/src/+/3472130",
					},
				},
			},
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
			BlameList:      blamelist,
			ParentAnalysis: datastore.KeyForObj(c, tfa),
		}
		So(datastore.Put(c, nsa), ShouldBeNil)

		rerun1 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			LuciBuild: model.LuciBuild{
				GitilesCommit: bbpb.GitilesCommit{
					Id: "commit1",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}

		So(datastore.Put(c, rerun1), ShouldBeNil)

		rerun2 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_FAILED,
			LuciBuild: model.LuciBuild{
				GitilesCommit: bbpb.GitilesCommit{
					Id: "commit3",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		So(datastore.Put(c, rerun2), ShouldBeNil)

		rerun3 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
			LuciBuild: model.LuciBuild{
				GitilesCommit: bbpb.GitilesCommit{
					Id: "commit0",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}

		So(datastore.Put(c, rerun3), ShouldBeNil)

		rerun4 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_INFRA_FAILED,
			LuciBuild: model.LuciBuild{
				GitilesCommit: bbpb.GitilesCommit{
					Id: "commit2",
				},
			},
			NthSectionAnalysisKey: datastore.KeyForObj(c, nsa),
		}
		So(datastore.Put(c, rerun4), ShouldBeNil)

		rerun5 := &model.TestSingleRerun{
			Status: pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED,
			LuciBuild: model.LuciBuild{
				GitilesCommit: bbpb.GitilesCommit{
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

func enableBisection(ctx context.Context, enabled bool) {
	testCfg := &configpb.Config{
		TestAnalysisConfig: &configpb.TestAnalysisConfig{
			BisectorEnabled: enabled,
		},
	}
	So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)
}

type fakeLUCIAnalysisClient struct {
}

func (cl *fakeLUCIAnalysisClient) ReadTestNames(ctx context.Context, project string, keys []lucianalysis.TestVerdictKey) (map[lucianalysis.TestVerdictKey]string, error) {
	results := map[lucianalysis.TestVerdictKey]string{}
	for i, key := range keys {
		results[key] = fmt.Sprintf("test_name_%d", i)
	}
	return results, nil
}