// Copyright 2025 The LUCI Authors.
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

package genai

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/internal/resultdb"
	"go.chromium.org/luci/bisection/llm"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util/testutil"
)

// mockGitilesData provides test data for gitiles mocking with 3 commits
var mockGitilesData = map[string]string{
	"https://chromium.googlesource.com/chromium/src/+log/start123..end456": `{
  "log": [
    {
      "commit": "commit3",
      "tree": "tree3",
      "parents": ["commit2"],
      "author": {
        "name": "Author 3",
        "email": "author3@chromium.org",
        "time": "Wed Oct 17 10:00:00 2023"
      },
      "committer": {
        "name": "Commit Bot",
        "email": "commit-bot@chromium.org",
        "time": "Wed Oct 17 10:00:00 2023"
      },
      "message": "Fix\n\nChange-Id: 1\nReviewed-on: https://review.example.com/1\n",
      "tree_diff": [
        {
          "type": "modify",
          "old_path": "src/test.cc",
          "new_path": "src/test.cc"
        }
      ]
    },
    {
      "commit": "commit2",
      "tree": "tree2",
      "parents": ["commit1"],
      "author": {
        "name": "Author 2",
        "email": "author2@chromium.org",
        "time": "Wed Oct 17 09:00:00 2023"
      },
      "committer": {
        "name": "Commit Bot",
        "email": "commit-bot@chromium.org",
        "time": "Wed Oct 17 09:00:00 2023"
      },
      "message": "Update\n\nChange-Id: 2\nReviewed-on: https://review.example.com/2\n",
      "tree_diff": [
        {
          "type": "modify",
          "old_path": "src/framework.cc",
          "new_path": "src/framework.cc"
        }
      ]
    },
    {
      "commit": "commit1",
      "tree": "tree1",
      "parents": ["start123"],
      "author": {
        "name": "Author 1",
        "email": "author1@chromium.org",
        "time": "Wed Oct 17 08:00:00 2023"
      },
      "committer": {
        "name": "Commit Bot",
        "email": "commit-bot@chromium.org",
        "time": "Wed Oct 17 08:00:00 2023"
      },
      "message": "Add\n\nChange-Id: 3\nReviewed-on: https://review.example.com/3\n",
      "tree_diff": [
        {
          "type": "add",
          "new_path": "src/new_file.cc"
        }
      ]
    }
  ]
}`,
}

func TestConstructPrompt(t *testing.T) {
	t.Parallel()

	ftt.Run("Constructs prompt correctly", t, func(t *ftt.Test) {
		testID := "test.example.TestName"
		failureSummary := "Failure Kind: CRASH\n\nPrimary Error Message:\nSegmentation fault"
		blamelist := "Commits in regression range (newest to oldest):\n\nCL 1:\n  Commit ID: abc123\n  Message: Fix bug"

		prompt, err := constructPrompt(testID, failureSummary, blamelist)
		assert.Loosely(t, err, should.BeNil)

		// Verify the prompt contains all the input components
		assert.Loosely(t, prompt, should.ContainSubstring(testID))
		assert.Loosely(t, prompt, should.ContainSubstring(failureSummary))
		assert.Loosely(t, prompt, should.ContainSubstring(blamelist))

		// Verify the prompt contains key instructions
		assert.Loosely(t, prompt, should.ContainSubstring("You are an experienced software engineer"))
		assert.Loosely(t, prompt, should.ContainSubstring("Test failure information:"))
		assert.Loosely(t, prompt, should.ContainSubstring("Blamelist:"))
		assert.Loosely(t, prompt, should.ContainSubstring("TOP 3 most likely culprit commits"))
		assert.Loosely(t, prompt, should.ContainSubstring("confidence score"))
	})
}

func TestPrepareBlamelist(t *testing.T) {
	t.Parallel()

	ftt.Run("Valid changelogs", t, func(t *ftt.Test) {
		changelogs := []*model.ChangeLog{
			{
				Commit:  "commit1",
				Message: "Fix bug\n\nSome details\n\nChange-Id: abc\nReviewed-on: https://review.example.com/1\n",
				Author: model.ChangeLogActor{
					Time: "2023-10-17 10:00:00",
				},
				ChangeLogDiffs: []model.ChangeLogDiff{
					{
						Type:    model.ChangeType_MODIFY,
						NewPath: "src/file.cc",
					},
				},
			},
		}

		blamelist, err := prepareBlamelist(changelogs)
		assert.Loosely(t, err, should.BeNil)

		expectedBlamelist := `Commits in regression range (newest to oldest):

CL 1:
  Commit ID: commit1
  Time: 2023-10-17 10:00:00
  Message: Fix bug
  Changed files:
    MODIFY: src/file.cc

`
		assert.Loosely(t, blamelist, should.Equal(expectedBlamelist))
	})

	ftt.Run("Empty changelogs", t, func(t *ftt.Test) {
		changelogs := []*model.ChangeLog{}

		blamelist, err := prepareBlamelist(changelogs)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("no changelogs available"))
		assert.Loosely(t, blamelist, should.BeEmpty)
	})
}

func TestAnalyze(t *testing.T) {
	t.Parallel()

	ftt.Run("Analyze succeeds with 3 suspects", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		ctx, skdr := tq.TestingContext(ctx, nil)

		// Setup gitiles mocking
		ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockLLMClient := llm.NewMockClient(ctl)
		mockResultDBClient := resultdb.NewMockClient(ctl)

		// Create primary test failure
		tf := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:        2000,
			IsPrimary: true,
			TestID:    "test_id",
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

		// Create test failure analysis
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:              1000,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "start123",
			EndCommitHash:   "end456",
		})

		// Setup mock LLM response with structured JSON
		genaiResponse := `{
  "suspects": [
    {
      "commit_id": "commit3",
      "confidence_score": 9,
      "justification": "Direct fix to the failing component."
    },
    {
      "commit_id": "commit2",
      "confidence_score": 5,
      "justification": "Framework update could cause issues."
    },
    {
      "commit_id": "commit1",
      "confidence_score": 2,
      "justification": "Minor change with possible side effects."
    }
  ]
}`

		mockLLMClient.EXPECT().GenerateContentWithSchema(gomock.Any(), gomock.Any(), gomock.Any()).Return(genaiResponse, nil).Times(1)

		// Run analysis
		err := Analyze(ctx, tfa, mockLLMClient, mockResultDBClient)
		assert.Loosely(t, err, should.BeNil)

		// Retrieve the genaiAnalysis from datastore
		datastore.GetTestable(ctx).CatchupIndexes()
		var genaiAnalyses []*model.TestGenAIAnalysis
		q := datastore.NewQuery("TestGenAIAnalysis")
		err = datastore.GetAll(ctx, q, &genaiAnalyses)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(genaiAnalyses), should.Equal(1))
		genaiAnalysis := genaiAnalyses[0]

		// Verify GenAI analysis status
		assert.Loosely(t, genaiAnalysis.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
		assert.Loosely(t, genaiAnalysis.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))

		// Verify parent analysis status is also updated to SUSPECTFOUND
		assert.Loosely(t, datastore.Get(ctx, tfa), should.BeNil)
		assert.Loosely(t, tfa.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))

		// Verify 3 suspects were created
		suspects := []*model.Suspect{}
		suspectQuery := datastore.NewQuery("Suspect").Ancestor(datastore.KeyForObj(ctx, genaiAnalysis))
		err = datastore.GetAll(ctx, suspectQuery, &suspects)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(3))

		// Verify all suspect details
		// Suspect 1: commit3 with score 9
		assert.Loosely(t, suspects[0].GitilesCommit.Id, should.Equal("commit3"))
		assert.Loosely(t, suspects[0].Score, should.Equal(9))
		assert.Loosely(t, suspects[0].Justification, should.Equal("Direct fix to the failing component."))
		assert.Loosely(t, suspects[0].Type, should.Equal(model.SuspectType_GenAI))
		assert.Loosely(t, suspects[0].VerificationStatus, should.Equal(model.SuspectVerificationStatus_Unverified))
		assert.Loosely(t, suspects[0].ReviewUrl, should.Equal("https://review.example.com/1"))
		assert.Loosely(t, suspects[0].ReviewTitle, should.Equal("Fix"))
		assert.Loosely(t, suspects[0].GitilesCommit.Host, should.Equal("chromium.googlesource.com"))
		assert.Loosely(t, suspects[0].GitilesCommit.Project, should.Equal("chromium/src"))
		assert.Loosely(t, suspects[0].GitilesCommit.Ref, should.Equal("refs/heads/main"))

		// Suspect 2: commit2 with score 5
		assert.Loosely(t, suspects[1].GitilesCommit.Id, should.Equal("commit2"))
		assert.Loosely(t, suspects[1].Score, should.Equal(5))
		assert.Loosely(t, suspects[1].Justification, should.Equal("Framework update could cause issues."))
		assert.Loosely(t, suspects[1].Type, should.Equal(model.SuspectType_GenAI))
		assert.Loosely(t, suspects[1].VerificationStatus, should.Equal(model.SuspectVerificationStatus_Unverified))
		assert.Loosely(t, suspects[1].ReviewUrl, should.Equal("https://review.example.com/2"))
		assert.Loosely(t, suspects[1].ReviewTitle, should.Equal("Update"))

		// Suspect 3: commit1 with score 2
		assert.Loosely(t, suspects[2].GitilesCommit.Id, should.Equal("commit1"))
		assert.Loosely(t, suspects[2].Score, should.Equal(2))
		assert.Loosely(t, suspects[2].Justification, should.Equal("Minor change with possible side effects."))
		assert.Loosely(t, suspects[2].Type, should.Equal(model.SuspectType_GenAI))
		assert.Loosely(t, suspects[2].VerificationStatus, should.Equal(model.SuspectVerificationStatus_Unverified))
		assert.Loosely(t, suspects[2].ReviewUrl, should.Equal("https://review.example.com/3"))
		assert.Loosely(t, suspects[2].ReviewTitle, should.Equal("Add"))

		// Verify verification tasks were scheduled
		tasks := skdr.Tasks().Payloads()
		taskCount := 0
		for _, task := range tasks {
			if _, ok := task.(*tpb.TestFailureCulpritVerificationTask); ok {
				taskCount++
			}
		}
		assert.Loosely(t, taskCount, should.Equal(3))
	})

	ftt.Run("Analyze truncates suspects to 3 when LLM returns more", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)
		ctx, skdr := tq.TestingContext(ctx, nil)

		// Setup gitiles mocking (reusing shared mock data)
		ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockLLMClient := llm.NewMockClient(ctl)
		mockResultDBClient := resultdb.NewMockClient(ctl)

		// Create primary test failure
		tf := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:        2002,
			IsPrimary: true,
			TestID:    "test_id",
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

		// Create test failure analysis
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:              1002,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "start123",
			EndCommitHash:   "end456",
		})

		// Setup mock LLM response with 4 suspects (more than max of 3)
		genaiResponse := `{
  "suspects": [
    {
      "commit_id": "commit3",
      "confidence_score": 10,
      "justification": "Most recent change, highly suspicious."
    },
    {
      "commit_id": "commit2",
      "confidence_score": 8,
      "justification": "Related to failing component."
    },
    {
      "commit_id": "commit1",
      "confidence_score": 6,
      "justification": "Modified test file directly."
    },
    {
      "commit_id": "commit3",
      "confidence_score": 4,
      "justification": "Duplicate entry that should be truncated."
    }
  ]
}`

		mockLLMClient.EXPECT().GenerateContentWithSchema(gomock.Any(), gomock.Any(), gomock.Any()).Return(genaiResponse, nil).Times(1)

		// Run analysis
		err := Analyze(ctx, tfa, mockLLMClient, mockResultDBClient)
		assert.Loosely(t, err, should.BeNil)

		// Retrieve the genaiAnalysis from datastore
		datastore.GetTestable(ctx).CatchupIndexes()
		var genaiAnalyses []*model.TestGenAIAnalysis
		q := datastore.NewQuery("TestGenAIAnalysis")
		err = datastore.GetAll(ctx, q, &genaiAnalyses)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(genaiAnalyses), should.Equal(1))
		genaiAnalysis := genaiAnalyses[0]

		// Verify analysis status
		assert.Loosely(t, genaiAnalysis.Status, should.Equal(pb.AnalysisStatus_SUSPECTFOUND))
		assert.Loosely(t, genaiAnalysis.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))

		// Verify ONLY 3 suspects were created (truncated from 4)
		suspects := []*model.Suspect{}
		suspectQuery := datastore.NewQuery("Suspect").Ancestor(datastore.KeyForObj(ctx, genaiAnalysis))
		err = datastore.GetAll(ctx, suspectQuery, &suspects)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.Equal(3))

		// Verify the top 3 suspects are saved (commit3, commit2, commit1)
		// The 4th suspect (duplicate commit3) should be truncated
		assert.Loosely(t, suspects[0].GitilesCommit.Id, should.Equal("commit3"))
		assert.Loosely(t, suspects[0].Score, should.Equal(10))
		assert.Loosely(t, suspects[1].GitilesCommit.Id, should.Equal("commit2"))
		assert.Loosely(t, suspects[1].Score, should.Equal(8))
		assert.Loosely(t, suspects[2].GitilesCommit.Id, should.Equal("commit1"))
		assert.Loosely(t, suspects[2].Score, should.Equal(6))

		// Verify ONLY 3 verification tasks were scheduled (not 4)
		tasks := skdr.Tasks().Payloads()
		taskCount := 0
		for _, task := range tasks {
			if _, ok := task.(*tpb.TestFailureCulpritVerificationTask); ok {
				taskCount++
			}
		}
		assert.Loosely(t, taskCount, should.Equal(3))
	})

	ftt.Run("Analyze fails with LLM error", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)

		// Setup gitiles mocking (reusing shared mock data)
		ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockLLMClient := llm.NewMockClient(ctl)
		mockResultDBClient := resultdb.NewMockClient(ctl)

		tf := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:        2001,
			IsPrimary: true,
			TestID:    "test_id",
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

		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			ID:              1001,
			TestFailureKey:  datastore.KeyForObj(ctx, tf),
			StartCommitHash: "start123",
			EndCommitHash:   "end456",
		})

		// Setup mock LLM to return error
		mockLLMClient.EXPECT().GenerateContentWithSchema(gomock.Any(), gomock.Any(), gomock.Any()).Return("", fmt.Errorf("LLM API error")).Times(1)

		// Run analysis
		err := Analyze(ctx, tfa, mockLLMClient, mockResultDBClient)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("failed to call GenAI"))

		datastore.GetTestable(ctx).CatchupIndexes()

		// Verify the genaiAnalysis entity in datastore was updated to ERROR status
		var genaiAnalyses []*model.TestGenAIAnalysis
		q := datastore.NewQuery("TestGenAIAnalysis")
		err = datastore.GetAll(ctx, q, &genaiAnalyses)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(genaiAnalyses), should.Equal(1))
		savedAnalysis := genaiAnalyses[0]
		assert.Loosely(t, savedAnalysis.Status, should.Equal(pb.AnalysisStatus_ERROR))
		assert.Loosely(t, savedAnalysis.RunStatus, should.Equal(pb.AnalysisRunStatus_ENDED))

		// Verify no suspects were created
		suspects := []*model.Suspect{}
		suspectQuery := datastore.NewQuery("Suspect").Ancestor(datastore.KeyForObj(ctx, savedAnalysis))
		err = datastore.GetAll(ctx, suspectQuery, &suspects)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(suspects), should.BeZero)
	})
}
