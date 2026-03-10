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
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/emptypb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/llm"
	"go.chromium.org/luci/bisection/model"
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
	// Expect ChangeEditFileContent
	mockedGerrit.Client.EXPECT().ChangeEditFileContent(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil)
	// Expect ChangeEditPublish
	mockedGerrit.Client.EXPECT().ChangeEditPublish(gomock.Any(), gomock.Any()).Return(&emptypb.Empty{}, nil)
	// Expect SetReview (SendForReview)
	mockedGerrit.Client.EXPECT().SetReview(gomock.Any(), gomock.Any()).Return(&gerritpb.ReviewResult{}, nil)

	gerritClient, err := gerrit.NewClient(mockedGerrit.Ctx, "chromium-review.googlesource.com")
	assert.Loosely(t, err, should.BeNil)

	// 3. Mock LLM
	mockLLM := llm.NewMockClient(ctrl)
	mockResponseJSON := `{"files": [{"path": "src/test.cc", "content": "int main() { return 1; }"}], "message": "Fixed the bug."}`
	mockLLM.EXPECT().GenerateContentWithSchema(gomock.Any(), gomock.Any(), gomock.Any()).Return(mockResponseJSON, nil)

	// 4. Test GenerateFixforwardCL
	cfa := &model.CompileFailureAnalysis{
		Id:         999,
		CreateTime: time.Now(),
	}

	err = GenerateFixforwardCL(
		ctx,
		mockLLM,
		gerritClient,
		cfa,
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

	largeContent := make([]byte, 50001)
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
	// Gerrit or LLM methods should not be called because it exits early.
	gerritClient, err := gerrit.NewClient(mockedGerrit.Ctx, "chromium-review.googlesource.com")
	assert.Loosely(t, err, should.BeNil)

	mockLLM := llm.NewMockClient(ctrl)

	cfa := &model.CompileFailureAnalysis{Id: 999}
	err = GenerateFixforwardCL(
		ctx,
		mockLLM,
		gerritClient,
		cfa,
		"abc123def456",
		"log",
		"https://chromium.googlesource.com/chromium/src",
		"",
	)
	assert.Loosely(t, err, should.BeNil)
}
