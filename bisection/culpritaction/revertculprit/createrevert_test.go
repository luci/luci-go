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

package revertculprit

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/model"
	bisectionpb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/testutil"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestGenerateRevertDescription(t *testing.T) {
	t.Parallel()

	Convey("generateRevertDescription", t, func() {
		ctx := memory.Use(context.Background())

		// Setup datastore
		failedBuild, _, analysis := testutil.CreateCompileFailureAnalysisAnalysisChain(
			ctx, 88128398584903, "chromium", 444)
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(ctx, analysis),
		}
		So(datastore.Put(ctx, heuristicAnalysis), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		suspect := &model.Suspect{
			Id:             1,
			Type:           model.SuspectType_Heuristic,
			Score:          10,
			ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "test.googlesource.com",
				Project: "chromium/src",
				Id:      "deadbeef",
			},
			ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
			VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			AnalysisType:       bisectionpb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
		}
		So(datastore.Put(ctx, suspect), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		analysisURL := util.ConstructCompileAnalysisURL("chromium", failedBuild.Id)
		buildURL := util.ConstructBuildURL(ctx, failedBuild.Id)
		bugURL := util.ConstructBuganizerURLForAnalysis(analysisURL,
			"https://test-review.googlesource.com/c/chromium/test/+/876543")

		culprit := &gerritpb.ChangeInfo{
			Number:          876543,
			Project:         "chromium/src",
			Status:          gerritpb.ChangeStatus_MERGED,
			Subject:         "[TestTag] Added new feature",
			CurrentRevision: "deadbeef",
			Revisions: map[string]*gerritpb.RevisionInfo{
				"deadbeef": {
					Commit: &gerritpb.CommitInfo{
						Author: &gerritpb.GitPersonInfo{
							Name:  "John Doe",
							Email: "jdoe@example.com",
						},
					},
				},
			},
		}
		Convey("culprit has no bug specified", func() {
			culprit.Revisions["deadbeef"].Commit.Message = `[TestTag] Added new feature

This is the body of the culprit CL.

Change-Id: I100deadbeef`
			description, err := generateRevertDescription(ctx, suspect, culprit)
			So(err, ShouldBeNil)
			So(description, ShouldEqual, fmt.Sprintf(`Revert "[TestTag] Added new feature"

This reverts commit deadbeef.

Reason for revert:
LUCI Bisection has identified this change as the culprit of a build failure. See the analysis: %s

Sample failed build: %s

If this is a false positive, please report it at %s

Original change's description:
> [TestTag] Added new feature
>
> This is the body of the culprit CL.
>
> Change-Id: I100deadbeef

No-Presubmit: true
No-Tree-Checks: true
No-Try: true`, analysisURL, buildURL, bugURL))
		})

		Convey("culprit has a bug specified with BUG =", func() {
			culprit.Revisions["deadbeef"].Commit.Message = `[TestTag] Added new feature

This is the body of the culprit CL.

BUG = 563412
Change-Id: I100deadbeef`
			description, err := generateRevertDescription(ctx, suspect, culprit)
			So(err, ShouldBeNil)
			So(description, ShouldEqual, fmt.Sprintf(
				`Revert "[TestTag] Added new feature"

This reverts commit deadbeef.

Reason for revert:
LUCI Bisection has identified this change as the culprit of a build failure. See the analysis: %s

Sample failed build: %s

If this is a false positive, please report it at %s

Original change's description:
> [TestTag] Added new feature
>
> This is the body of the culprit CL.
>
> BUG = 563412
> Change-Id: I100deadbeef

BUG = 563412
No-Presubmit: true
No-Tree-Checks: true
No-Try: true`, analysisURL, buildURL, bugURL))
		})

		Convey("culprit has bugs specified with Bug:", func() {
			culprit.Revisions["deadbeef"].Commit.Message = `[TestTag] Added new feature

This is the body of the culprit CL.

Bug: 123
Bug: 765
Change-Id: I100deadbeef`
			description, err := generateRevertDescription(ctx, suspect, culprit)
			So(err, ShouldBeNil)
			So(description, ShouldEqual, fmt.Sprintf(
				`Revert "[TestTag] Added new feature"

This reverts commit deadbeef.

Reason for revert:
LUCI Bisection has identified this change as the culprit of a build failure. See the analysis: %s

Sample failed build: %s

If this is a false positive, please report it at %s

Original change's description:
> [TestTag] Added new feature
>
> This is the body of the culprit CL.
>
> Bug: 123
> Bug: 765
> Change-Id: I100deadbeef

Bug: 123
Bug: 765
No-Presubmit: true
No-Tree-Checks: true
No-Try: true`, analysisURL, buildURL, bugURL))
		})

		Convey("culprit has bug delimiter in description", func() {
			culprit.Revisions["deadbeef"].Commit.Message = `[TestTag] Added new feature

This is the body of the culprit CL.
Bug link: https://bug-handler.test.com/b/id=1000123.

Bug: 123
Change-Id: I100deadbeef`
			description, err := generateRevertDescription(ctx, suspect, culprit)
			So(err, ShouldBeNil)
			So(description, ShouldEqual, fmt.Sprintf(
				`Revert "[TestTag] Added new feature"

This reverts commit deadbeef.

Reason for revert:
LUCI Bisection has identified this change as the culprit of a build failure. See the analysis: %s

Sample failed build: %s

If this is a false positive, please report it at %s

Original change's description:
> [TestTag] Added new feature
>
> This is the body of the culprit CL.
> Bug link: https://bug-handler.test.com/b/id=1000123.
>
> Bug: 123
> Change-Id: I100deadbeef

Bug: 123
No-Presubmit: true
No-Tree-Checks: true
No-Try: true`, analysisURL, buildURL, bugURL))
		})
	})
}
