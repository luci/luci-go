// Copyright 2026 The LUCI Authors.
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

package fixforward

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/llm"
)

func TestGenerateFixforwardCL(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// 1. Mock Gitiles
	mockGitilesData := map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/abc123def456^..abc123def456": `{
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
      "message": "Introduce a bug.\n\nChange-Id: I1234567890abcdef",
      "tree_diff": [
        {
          "type": "modify",
          "new_path": "src/test.cc"
        }
      ]
    }
  ]
}`,
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src/test.cc": `aW50IG1haW4oKSB7IHJldHVybiAwOyB9`,
	}
	ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

	// 2. Mock Gerrit
	mockedGerrit := gerrit.NewMockedClient(ctx, ctrl)
	fakeChangeInfo := &gerritpb.ChangeInfo{
		Number:  12345,
		Project: "chromium/src",
	}
	// Expect CreateChange
	mockedGerrit.Client.EXPECT().CreateChange(gomock.Any(), gomock.Any()).Return(fakeChangeInfo, nil)
	// 3. Mock LLM
	mockLLM := llm.NewMockClient(ctrl)
	mockResponseJSON := `{"files": [{"path": "src/test.cc", "edits": [{"old_text": "return 0;", "new_text": "return 1;"}]}], "message": "Fixed the bug."}`
	mockLLM.EXPECT().GenerateContentWithSchema(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockResponseJSON, nil)

	// Since we expect "return 0;" to become "return 1;" in the original content "int main() { return 0; }":
	expectedFileContent := []byte("int main() { return 1; }")
	// Update Gerrit Mock Expectation:
	mockedGerrit.Client.EXPECT().ChangeEditFileContent(gomock.Any(), &gerritpb.ChangeEditFileContentRequest{
		Number:   12345,
		Project:  "chromium/src",
		FilePath: "src/test.cc",
		Content:  expectedFileContent,
	}).Return(&emptypb.Empty{}, nil)
	// Expect ChangeEditPublish
	mockedGerrit.Client.EXPECT().ChangeEditPublish(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil)
	// Expect SetReview (SendForReview)
	mockedGerrit.Client.EXPECT().SetReview(gomock.Any(), gomock.Any(), gomock.Any()).Return(&gerritpb.ReviewResult{}, nil)

	gerritClient, err := gerrit.NewClient(mockedGerrit.Ctx, "chromium-review.googlesource.com")
	assert.Loosely(t, err, should.BeNil)

	// 4. Test GenerateFixforwardCL

	err = GenerateFixforwardCL(
		ctx,
		mockLLM,
		gerritClient,
		"abc123def456",
		"compile Error Log Here",
		"https://chromium.googlesource.com/chromium/src",
		"https://chromium-review.googlesource.com/c/chromium/src/+/123",
	)
	assert.Loosely(t, err, should.BeNil)
}

func TestGenerateFixforwardCL_LargeFile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	largeContent := make([]byte, 500001)
	for i := range largeContent {
		largeContent[i] = 'a'
	}
	largeBase64 := base64.StdEncoding.EncodeToString(largeContent)

	mockGitilesData := map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/abc123def456^..abc123def456": `{
  "log": [
    {
      "commit": "abc123def456",
      "tree_diff": [
        {
          "type": "modify",
          "new_path": "src/large.cc"
        }
      ]
    }
  ]
}`,
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src/large.cc": largeBase64,
	}
	ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

	mockedGerrit := gerrit.NewMockedClient(ctx, ctrl)
	gerritClient, err := gerrit.NewClient(mockedGerrit.Ctx, "chromium-review.googlesource.com")
	assert.Loosely(t, err, should.BeNil)

	mockLLM := llm.NewMockClient(ctrl)
	err = GenerateFixforwardCL(
		ctx,
		mockLLM,
		gerritClient,
		"abc123def456",
		"log",
		"https://chromium.googlesource.com/chromium/src",
		"",
	)
	assert.Loosely(t, err, should.NotBeNil)
	assert.Loosely(t, err.Error(), should.ContainSubstring("no modified files within size limit found"))
}

func TestGenerateFixforwardCL_NeighborFileNotEdited(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGitilesData := map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/abc123def456^..abc123def456": `{
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
      "message": "Introduce a bug.\n\nChange-Id: I1234567890abcdef",
      "tree_diff": [
        {
          "type": "modify",
          "new_path": "src/test.cc"
        }
      ]
    }
  ]
}`,
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src/test.cc": `aW50IG1haW4oKSB7IHJldHVybiAwOyB9`, // "int main() { return 0; }"
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src?format=JSON": `)]}'
{
  "entries": [
    {"type": "blob", "name": "neighbor.cc"}
  ]
}`,
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src/neighbor.cc": `Y29uc3QgY2hhciogc2VjcmV0ID0gInBhc3N3b3JkIjs=`, // "const char* secret = \"password\";"
	}
	ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

	mockedGerrit := gerrit.NewMockedClient(ctx, ctrl)
	fakeChangeInfo := &gerritpb.ChangeInfo{
		Number:  12345,
		Project: "chromium/src",
	}
	mockedGerrit.Client.EXPECT().CreateChange(gomock.Any(), gomock.Any()).Return(fakeChangeInfo, nil)

	// LLM attempts to modify both the culprit file and the neighbor file
	mockLLM := llm.NewMockClient(ctrl)
	mockResponseJSON := `{
		"files": [
			{"path": "src/test.cc", "edits": [{"old_text": "return 0;", "new_text": "return 1;"}]},
			{"path": "src/neighbor.cc", "edits": [{"old_text": "\"password\"", "new_text": "\"injected\""}]}
		],
		"message": "Fixed the bug with prompt injection attempt."
	}`
	mockLLM.EXPECT().GenerateContentWithSchema(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockResponseJSON, nil)

	// We expect ChangeEditFileContent ONLY for src/test.cc, NEVER for src/neighbor.cc
	expectedFileContent := []byte("int main() { return 1; }")
	mockedGerrit.Client.EXPECT().ChangeEditFileContent(gomock.Any(), &gerritpb.ChangeEditFileContentRequest{
		Number:   12345,
		Project:  "chromium/src",
		FilePath: "src/test.cc",
		Content:  expectedFileContent,
	}).Return(&emptypb.Empty{}, nil)

	mockedGerrit.Client.EXPECT().ChangeEditPublish(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil)
	mockedGerrit.Client.EXPECT().SetReview(gomock.Any(), gomock.Any(), gomock.Any()).Return(&gerritpb.ReviewResult{}, nil)

	gerritClient, err := gerrit.NewClient(mockedGerrit.Ctx, "chromium-review.googlesource.com")
	assert.Loosely(t, err, should.BeNil)

	err = GenerateFixforwardCL(
		ctx,
		mockLLM,
		gerritClient,
		"abc123def456",
		"compile Error Log Here",
		"https://chromium.googlesource.com/chromium/src",
		"https://chromium-review.googlesource.com/c/chromium/src/+/123",
	)
	assert.Loosely(t, err, should.BeNil)
}

func TestGenerateFixforwardCL_PromptInjectionNeighborOnlyFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockGitilesData := map[string]string{
		"https://chromium.googlesource.com/chromium/src/+log/abc123def456^..abc123def456": `{
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
      "message": "Introduce a bug.\n\nChange-Id: I1234567890abcdef",
      "tree_diff": [
        {
          "type": "modify",
          "new_path": "src/test.cc"
        }
      ]
    }
  ]
}`,
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src/test.cc": `aW50IG1haW4oKSB7IHJldHVybiAwOyB9`,
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src?format=JSON": `)]}'
{
  "entries": [
    {"type": "blob", "name": "neighbor.cc"}
  ]
}`,
		"https://chromium.googlesource.com/chromium/src/+/abc123def456/src/neighbor.cc": `Y29uc3QgY2hhciogc2VjcmV0ID0gInBhc3N3b3JkIjs=`,
	}
	ctx = gitiles.MockedGitilesClientContext(ctx, mockGitilesData)

	mockedGerrit := gerrit.NewMockedClient(ctx, ctrl)
	// CreateChange should NEVER be called when all edits target non-culprit files
	mockedGerrit.Client.EXPECT().CreateChange(gomock.Any(), gomock.Any()).Times(0)

	mockLLM := llm.NewMockClient(ctrl)
	mockResponseJSON := `{
		"files": [
			{"path": "src/neighbor.cc", "edits": [{"old_text": "\"password\"", "new_text": "\"injected\""}]}
		],
		"message": "Fixed the bug by hacking neighbor file."
	}`
	mockLLM.EXPECT().GenerateContentWithSchema(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockResponseJSON, nil)

	gerritClient, err := gerrit.NewClient(mockedGerrit.Ctx, "chromium-review.googlesource.com")
	assert.Loosely(t, err, should.BeNil)

	err = GenerateFixforwardCL(
		ctx,
		mockLLM,
		gerritClient,
		"abc123def456",
		"compile Error Log Here",
		"https://chromium.googlesource.com/chromium/src",
		"https://chromium-review.googlesource.com/c/chromium/src/+/123",
	)
	assert.Loosely(t, err, should.NotBeNil)
	assert.Loosely(t, err.Error(), should.ContainSubstring("no valid edits to culprit modified files generated by LLM"))
}
