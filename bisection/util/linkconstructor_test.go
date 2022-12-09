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
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/bisection/internal/gerrit"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/info"
)

func TestConstructAnalysisURL(t *testing.T) {
	ctx := memory.Use(context.Background())
	appID := info.AppID(ctx)

	Convey("construct analysis URL", t, func() {
		So(ConstructAnalysisURL(ctx, 123456789876543), ShouldEqual,
			fmt.Sprintf("https://%s.appspot.com/analysis/b/123456789876543", appID))
	})
}

func TestConstructBuildURL(t *testing.T) {
	ctx := context.Background()

	Convey("construct build URL", t, func() {
		So(ConstructBuildURL(ctx, 123456789876543), ShouldEqual,
			"https://ci.chromium.org/b/123456789876543")
	})
}

func TestConstructLUCIBisectionBugURL(t *testing.T) {
	ctx := context.Background()

	Convey("construct bug URL", t, func() {
		bugURL := ConstructLUCIBisectionBugURL(ctx,
			"https://luci-bisection.appspot.com/analysis/b/123456789876543",
			"https://chromium-test-review.googlesource.com/c/chromium/test/src/+/1234567")
		expectedBugURL := "https://bugs.chromium.org/p/chromium/issues/entry?" +
			"comment=Analysis%3A+https%3A%2F%2Fluci-bisection.appspot.com%2Fanalysis%2Fb%2F123456789876543&" +
			"components=Tools%3ETest%3EFindit&labels=LUCI-Bisection-Wrong%2CPri-3%2CType-Bug&" +
			"status=Available&summary=Wrongly+blamed+" +
			"https%3A%2F%2Fchromium-test-review.googlesource.com%2Fc%2Fchromium%2Ftest%2Fsrc%2F%2B%2F1234567"
		So(bugURL, ShouldEqual, expectedBugURL)
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
