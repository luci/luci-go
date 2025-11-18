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
	"testing"

	"github.com/golang/mock/gomock"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/llm"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestGenAiAnalysis(t *testing.T) {
	t.Parallel()
	// Set up logging to see output in console
	ctx := gologger.StdConfig.Use(context.Background())
	ctx = logging.SetLevel(ctx, logging.Info)
	// Set up UTC test clock
	cl := testclock.New(testclock.TestTimeUTC)
	ctx = clock.Set(ctx, cl)

	// Set up datastore emulator
	ctx = memory.Use(ctx)

	// Set up mocked Gitiles responses
	mockGitilesData := map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/def456ghi789..abc123def456": `{
  "log": [
    {
      "commit": "abc123def456",
      "tree": "tree123",
      "parents": ["parent123"],
      "author": {
        "name": "Test Author",
        "email": "test@chromium.org",
        "time": "Tue Jan 02 15:04:05 2024"
      },
      "committer": {
        "name": "Commit Bot",
        "email": "commit-bot@chromium.org",
        "time": "Tue Jan 02 15:04:05 2024"
      },
      "message": "Fix undefined function in test.cc\n\nThis change adds the missing function declaration that was causing\nthe compile failure in the test suite.\n\nBug: 123456\nChange-Id: I1234567890abcdef",
      "tree_diff": [
        {
          "type": "modify",
          "old_id": "old123",
          "old_mode": 33188,
          "old_path": "src/test.cc",
          "new_id": "new123",
          "new_mode": 33188,
          "new_path": "src/test.cc"
        }
      ]
    },
    {
      "commit": "commit234def567",
      "tree": "tree234",
      "parents": ["abc123def456"],
      "author": {
        "name": "Another Author",
        "email": "author2@chromium.org",
        "time": "Mon Jan 01 10:00:00 2024"
      },
      "committer": {
        "name": "Commit Bot",
        "email": "commit-bot@chromium.org",
        "time": "Mon Jan 01 10:00:00 2024"
      },
      "message": "Update header includes\n\nChange-Id: I2345678901bcdefg",
      "tree_diff": [
        {
          "type": "modify",
          "old_id": "header_old",
          "old_mode": 33188,
          "old_path": "src/header.h",
          "new_id": "header_new",
          "new_mode": 33188,
          "new_path": "src/header.h"
        }
      ]
    }
  ]
}`,
	}
	ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

	// Create fake data using testutil
	_, _, cfa := testutil.CreateCompileFailureAnalysisAnalysisChain(ctx, t, 123, "chromium", 456)

	// Create fake regression range
	regressionRange := &pb.RegressionRange{
		FirstFailed: &buildbucketpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "refs/heads/main",
			Id:      "abc123def456",
		},
		LastPassed: &buildbucketpb.GitilesCommit{
			Host:    "chromium.googlesource.com",
			Project: "chromium/src",
			Ref:     "refs/heads/main",
			Id:      "def456ghi789",
		},
	}

	// Create fake compile logs
	compileLogs := &model.CompileLogs{
		FailureSummaryLog: "error: 'undefined_function' was not declared in this scope\nsrc/test.cc:42:5: error: expected ';' before 'return'",
		StdOutLog:         "error: 'undefined_function' was not declared in this scope\nsrc/test.cc:42:5: error: expected ';' before 'return'",
		NinjaLog: &model.NinjaLog{
			Failures: []*model.NinjaLogFailure{
				{
					Rule:         "cxx",
					Output:       "compilation failed",
					OutputNodes:  []string{"obj/test.o"},
					Dependencies: []string{"src/test.cc", "src/header.h"},
				},
			},
		},
	}

	// Create mock client
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := llm.NewMockClient(ctrl)
	// Mock the GenAI response to return commit ID and justification
	mockResponse := "Commit ID: abc123def456\nJustification: Fix undefined function in test.cc"
	mockClient.EXPECT().GenerateContent(gomock.Any(), gomock.Any()).Return(mockResponse, nil)

	// Test the Analyze function with mock client
	result, err := Analyze(ctx, mockClient, cfa, regressionRange, compileLogs)
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, result, should.NotBeNil)

	t.Logf("Analysis completed with status: %v", result.Status)

	// Inspect the datastore to verify suspects were created
	t.Run("inspect_datastore", func(t *testing.T) {
		// Query for suspects that are children of the GenAI analysis
		var suspects []*model.Suspect
		q := datastore.NewQuery("Suspect").Ancestor(datastore.KeyForObj(ctx, result))
		err := datastore.GetAll(ctx, q, &suspects)
		assert.Loosely(t, err, should.BeNil)

		t.Logf("Found %d suspects in datastore", len(suspects))

		// Verify we have at least one suspect
		assert.Loosely(t, len(suspects), should.BeGreaterThan(0))

		// Verify the suspect has the expected properties
		if len(suspects) > 0 {
			suspect := suspects[0]
			assert.Loosely(t, suspect.Type, should.Equal(model.SuspectType_GenAI))
			assert.Loosely(t, suspect.VerificationStatus, should.Equal(model.SuspectVerificationStatus_Unverified))
			assert.Loosely(t, suspect.Justification, should.Equal("Fix undefined function in test.cc"))
			// Verify commit time is set (should match the time from mock Gitiles data)
			assert.Loosely(t, suspect.CommitTime.IsZero(), should.BeFalse)
			t.Logf("Suspect commit time: %v", suspect.CommitTime)
		}
	})
}

func TestPrepareStackTrace(t *testing.T) {
	t.Parallel()

	t.Run("valid_compile_logs", func(t *testing.T) {
		compileLogs := &model.CompileLogs{
			FailureSummaryLog: "error: 'undefined_function' was not declared in this scope\nsrc/test.cc:42:5: error: expected ';' before 'return'",
		}

		result, err := prepareStackTrace(compileLogs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.ContainSubstring("Failure Summary Log:"))
		assert.Loosely(t, result, should.ContainSubstring("undefined_function"))
	})

	t.Run("nil_compile_logs", func(t *testing.T) {
		_, err := prepareStackTrace(nil)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("compile logs are nil"))
	})

	t.Run("empty_compile_logs", func(t *testing.T) {
		compileLogs := &model.CompileLogs{}
		_, err := prepareStackTrace(compileLogs)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("no compile failure summary available"))
	})

	t.Run("only_failure_summary", func(t *testing.T) {
		compileLogs := &model.CompileLogs{
			FailureSummaryLog: "error: missing semicolon",
		}

		result, err := prepareStackTrace(compileLogs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.ContainSubstring("Failure Summary Log:"))
		assert.Loosely(t, result, should.ContainSubstring("missing semicolon"))
	})

	t.Run("without_failure_summary", func(t *testing.T) {
		compileLogs := &model.CompileLogs{
			StdOutLog: "error: linker error",
			NinjaLog: &model.NinjaLog{
				Failures: []*model.NinjaLogFailure{
					{
						Rule:   "link",
						Output: "linker error",
					},
				},
			},
		}

		_, err := prepareStackTrace(compileLogs)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("no compile failure summary available"))
	})
}

func TestPrepareBlamelist(t *testing.T) {
	t.Parallel()

	t.Run("valid_changelogs", func(t *testing.T) {
		changeLogs := []*model.ChangeLog{
			{
				Commit: "abc123def456",
				Author: model.ChangeLogActor{
					Time: "2024-01-02T15:04:05Z",
				},
				Message: "Fix undefined function in test.cc\n\nThis change adds the missing function declaration.",
				ChangeLogDiffs: []model.ChangeLogDiff{
					{
						Type:    model.ChangeType_MODIFY,
						OldPath: "src/test.cc",
						NewPath: "src/test.cc",
					},
					{
						Type:    model.ChangeType_ADD,
						NewPath: "src/new_file.h",
					},
				},
			},
			{
				Commit: "def456ghi789",
				Author: model.ChangeLogActor{
					Time: "2024-01-01T10:00:00Z",
				},
				Message: "Update header includes",
				ChangeLogDiffs: []model.ChangeLogDiff{
					{
						Type:    model.ChangeType_DELETE,
						OldPath: "src/old_header.h",
					},
					{
						Type:    model.ChangeType_RENAME,
						OldPath: "src/old_name.cc",
						NewPath: "src/new_name.cc",
					},
					{
						Type:    model.ChangeType_COPY,
						OldPath: "src/template.cc",
						NewPath: "src/copy.cc",
					},
				},
			},
		}

		result, err := prepareBlamelist(changeLogs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.ContainSubstring("Commits in regression range"))
		assert.Loosely(t, result, should.ContainSubstring("CL 1:"))
		assert.Loosely(t, result, should.ContainSubstring("Commit ID: abc123def456"))
		assert.Loosely(t, result, should.ContainSubstring("Fix undefined function"))
		assert.Loosely(t, result, should.ContainSubstring("MODIFY: src/test.cc"))
		assert.Loosely(t, result, should.ContainSubstring("ADD: src/new_file.h"))
		assert.Loosely(t, result, should.ContainSubstring("CL 2:"))
		assert.Loosely(t, result, should.ContainSubstring("Commit ID: def456ghi789"))
		assert.Loosely(t, result, should.ContainSubstring("DELETE: src/old_header.h"))
		assert.Loosely(t, result, should.ContainSubstring("RENAME: src/old_name.cc -> src/new_name.cc"))
		assert.Loosely(t, result, should.ContainSubstring("COPY: src/template.cc -> src/copy.cc"))
	})

	t.Run("empty_changelogs", func(t *testing.T) {
		var changeLogs []*model.ChangeLog
		_, err := prepareBlamelist(changeLogs)
		assert.Loosely(t, err, should.NotBeNil)
		assert.Loosely(t, err.Error(), should.ContainSubstring("no changelogs available"))
	})

	t.Run("changelog_without_diffs", func(t *testing.T) {
		changeLogs := []*model.ChangeLog{
			{
				Commit: "abc123def456",
				Author: model.ChangeLogActor{
					Time: "2024-01-02T15:04:05Z",
				},
				Message: "Simple commit without file changes",
			},
		}

		result, err := prepareBlamelist(changeLogs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, result, should.ContainSubstring("CL 1:"))
		assert.Loosely(t, result, should.ContainSubstring("Commit ID: abc123def456"))
		assert.Loosely(t, result, should.ContainSubstring("Simple commit without file changes"))
		assert.Loosely(t, result, should.NotContainSubstring("Changed files:"))
	})
}

func TestParseGenAIResponse(t *testing.T) {
	t.Parallel()

	t.Run("valid_response", func(t *testing.T) {
		response := "Commit ID: abc123def456\nJustification: Fix for bug #12345"
		commitID, justification, err := parseGenAIResponse(response)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, commitID, should.Equal("abc123def456"))
		assert.Loosely(t, justification, should.Equal("Fix for bug #12345"))
	})

	t.Run("missing_commit_id", func(t *testing.T) {
		response := "Justification: Missing commit ID"
		_, _, err := parseGenAIResponse(response)
		assert.Loosely(t, err, should.NotBeNil)
	})

	t.Run("missing_justification", func(t *testing.T) {
		response := "Commit ID: abc123def456"
		_, _, err := parseGenAIResponse(response)
		assert.Loosely(t, err, should.NotBeNil)
	})

	t.Run("empty_response", func(t *testing.T) {
		response := ""
		_, _, err := parseGenAIResponse(response)
		assert.Loosely(t, err, should.NotBeNil)
	})

	t.Run("extra_whitespace", func(t *testing.T) {
		response := "  Commit ID:  abc123def456  \n  Justification:  Leading and trailing spaces.  "
		commitID, justification, err := parseGenAIResponse(response)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, commitID, should.Equal("abc123def456"))
		assert.Loosely(t, justification, should.Equal("Leading and trailing spaces."))
	})

	t.Run("unordered_response", func(t *testing.T) {
		response := "Justification: Unordered response\nCommit ID: abc123def456"
		commitID, justification, err := parseGenAIResponse(response)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, commitID, should.Equal("abc123def456"))
		assert.Loosely(t, justification, should.Equal("Unordered response"))
	})
}
