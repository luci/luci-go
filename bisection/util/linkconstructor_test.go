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

package util

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/bisection/internal/gerrit"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConstructAnalysisURL(t *testing.T) {
	ftt.Run("construct compile analysis URL", t, func(t *ftt.Test) {
		assert.Loosely(t, ConstructCompileAnalysisURL("testproject", 123456789876543), should.Equal(
			"https://ci.chromium.org/ui/p/testproject/bisection/compile-analysis/b/123456789876543"))
	})

	ftt.Run("construct test analysis URL", t, func(t *ftt.Test) {
		assert.Loosely(t, ConstructTestAnalysisURL("testproject", 123456789876543), should.Equal(
			"https://ci.chromium.org/ui/p/testproject/bisection/test-analysis/b/123456789876543"))
	})
}

func TestConstructBuildURL(t *testing.T) {
	ctx := context.Background()

	ftt.Run("construct build URL", t, func(t *ftt.Test) {
		assert.Loosely(t, ConstructBuildURL(ctx, 123456789876543), should.Equal(
			"https://ci.chromium.org/b/123456789876543"))
	})
}

func TestConstructGerritCodeReviewURL(t *testing.T) {
	ctx := context.Background()

	ftt.Run("construct Gerrit code review URL", t, func(t *ftt.Test) {
		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := gerrit.NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		gerritClient, err := gerrit.NewClient(ctx, "chromium-test.googlesource.com")
		assert.Loosely(t, err, should.BeNil)

		change := &gerritpb.ChangeInfo{
			Project: "chromium/test",
			Number:  123456,
		}

		assert.Loosely(t, ConstructGerritCodeReviewURL(ctx, gerritClient, change), should.Equal(
			"https://chromium-test.googlesource.com/c/chromium/test/+/123456"))
	})
}

func TestConstructBuganizerURLForTestAnalysis(t *testing.T) {

	commitReviewURL := "https://chromium-test-review.googlesource.com/c/chromium/test/src/+/1234567"

	ftt.Run("construct buganizer URL", t, func(t *ftt.Test) {
		analysisURL := "https://ci.chromium.org/ui/p/chromium/bisection/compile-analysis/b/8766961581788295857"
		bugURL := ConstructBuganizerURLForAnalysis(commitReviewURL, analysisURL)
		expectedBugURL := "http://b.corp.google.com/createIssue?component=1199205" +
			"&description=Analysis%3A+https%3A%2F%2Fci.chromium.org%2Fui%2Fp%2Fchromium%2Fbi" +
			"section%2Fcompile-analysis%2Fb%2F8766961581788295857&format=PLAIN&priority=P3&title=Wrongly+" +
			"blamed+https%3A%2F%2Fchromium-test-review.googlesource.com%2Fc%2Fchromium%2F" +
			"test%2Fsrc%2F%2B%2F1234567&type=BUG"
		assert.Loosely(t, bugURL, should.Equal(expectedBugURL))
	})
}

func TestConstructTestHistoryURL(t *testing.T) {
	ftt.Run("construct test history URL", t, func(t *ftt.Test) {
		testURL := ConstructTestHistoryURL("chromium", "testID", "6363b77a587c3046")
		expectedTestURL := "https://ci.chromium.org/ui/test/chromium/testID?q=VHash%3A6363b77a587c3046"
		assert.Loosely(t, testURL, should.Equal(expectedTestURL))
	})
}
