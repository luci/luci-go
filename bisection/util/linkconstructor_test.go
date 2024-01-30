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
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/bisection/internal/gerrit"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
)

func TestConstructAnalysisURL(t *testing.T) {
	Convey("construct compile analysis URL", t, func() {
		So(ConstructCompileAnalysisURL("testproject", 123456789876543), ShouldEqual,
			"https://ci.chromium.org/ui/p/testproject/bisection/compile-analysis/b/123456789876543")
	})

	Convey("construct test analysis URL", t, func() {
		So(ConstructTestAnalysisURL("testproject", 123456789876543), ShouldEqual,
			"https://ci.chromium.org/ui/p/testproject/bisection/test-analysis/b/123456789876543")
	})
}

func TestConstructBuildURL(t *testing.T) {
	ctx := context.Background()

	Convey("construct build URL", t, func() {
		So(ConstructBuildURL(ctx, 123456789876543), ShouldEqual,
			"https://ci.chromium.org/b/123456789876543")
	})
}

func TestConstructGerritCodeReviewURL(t *testing.T) {
	ctx := context.Background()

	Convey("construct Gerrit code review URL", t, func() {
		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := gerrit.NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		gerritClient, err := gerrit.NewClient(ctx, "chromium-test.googlesource.com")
		So(err, ShouldBeNil)

		change := &gerritpb.ChangeInfo{
			Project: "chromium/test",
			Number:  123456,
		}

		So(ConstructGerritCodeReviewURL(ctx, gerritClient, change), ShouldEqual,
			"https://chromium-test.googlesource.com/c/chromium/test/+/123456")
	})
}

func TestConstructBuganizerURLForTestAnalysis(t *testing.T) {

	commitReviewURL := "https://chromium-test-review.googlesource.com/c/chromium/test/src/+/1234567"

	Convey("construct buganizer URL", t, func() {
		analysisURL := "https://ci.chromium.org/ui/p/chromium/bisection/compile-analysis/b/8766961581788295857"
		bugURL := ConstructBuganizerURLForAnalysis(commitReviewURL, analysisURL)
		expectedBugURL := "http://b.corp.google.com/createIssue?component=1199205" +
			"&description=Analysis%3A+https%3A%2F%2Fci.chromium.org%2Fui%2Fp%2Fchromium%2Fbi" +
			"section%2Fcompile-analysis%2Fb%2F8766961581788295857&format=PLAIN&priority=P3&title=Wrongly+" +
			"blamed+https%3A%2F%2Fchromium-test-review.googlesource.com%2Fc%2Fchromium%2F" +
			"test%2Fsrc%2F%2B%2F1234567&type=BUG"
		So(bugURL, ShouldEqual, expectedBugURL)
	})
}

func TestConstructTestHistoryURL(t *testing.T) {
	Convey("construct test history URL", t, func() {
		testURL := ConstructTestHistoryURL("chromium", "testID", "6363b77a587c3046")
		expectedTestURL := "https://ci.chromium.org/ui/test/chromium/testID?q=VHash%3A6363b77a587c3046"
		So(testURL, ShouldEqual, expectedTestURL)
	})
}
