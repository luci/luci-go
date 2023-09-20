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

// Package revertculprit contains the logic to revert culprits
package revertculprit

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/testutil"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestProcessTestFailureCulpritTask(t *testing.T) {
	t.Parallel()

	Convey("processTestFailureCulpritTask", t, func() {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)

		// Set test clock
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)

		// Setup tsmon
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		// Set the service-level config for this test
		testCfg := &configpb.Config{
			TestAnalysisConfig: &configpb.TestAnalysisConfig{
				GerritConfig: &configpb.GerritConfig{
					ActionsEnabled: true,
					NthsectionSettings: &configpb.GerritConfig_NthSectionSettings{
						Enabled:                     true,
						ActionWhenVerificationError: false,
					},
				},
			},
		}
		So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

		// Setup datastore
		tfa := testutil.CreateTestFailureAnalysis(ctx, &testutil.TestFailureAnalysisCreationOption{
			Project:        "chromium",
			TestFailureKey: datastore.NewKey(ctx, "TestFailure", "", 1, nil),
			FailedBuildID:  123,
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, &testutil.TestNthSectionAnalysisCreationOption{
			ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		})
		suspect := &model.Suspect{
			Type:           model.SuspectType_NthSection,
			ParentAnalysis: datastore.KeyForObj(ctx, nsa),
			GitilesCommit: buildbucketpb.GitilesCommit{
				Host:    "test.googlesource.com",
				Project: "chromium/src",
				Id:      "12ab34cd56ef",
			},
			ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
			AnalysisType:       pb.AnalysisType_TEST_FAILURE_ANALYSIS,
			VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
		}
		So(datastore.Put(ctx, suspect), ShouldBeNil)
		tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, suspect)
		So(datastore.Put(ctx, tfa), ShouldBeNil)
		tf1 := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:          1,
			Project:     "chromium",
			IsPrimary:   true,
			Analysis:    tfa,
			TestID:      "testID1",
			VariantHash: "varianthash1",
		})
		tf2 := testutil.CreateTestFailure(ctx, &testutil.TestFailureCreationOption{
			ID:          2,
			Project:     "chromium",
			IsPrimary:   false,
			Analysis:    tfa,
			TestID:      "testID2",
			VariantHash: "varianthash2",
		})

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := gerrit.NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx
		culpritRes := &gerritpb.ListChangesResponse{
			Changes: []*gerritpb.ChangeInfo{{
				Number:          876543,
				Project:         "chromium/src",
				Status:          gerritpb.ChangeStatus_MERGED,
				Submitted:       timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Commit: &gerritpb.CommitInfo{
							Message: "Title.\n\nBody is here.\n\nChange-Id: I100deadbeef",
							Author: &gerritpb.GitPersonInfo{
								Name:  "John Doe",
								Email: "jdoe@example.com",
							},
						},
					},
				},
			}},
		}
		buildURL := util.ConstructBuildURL(ctx, tfa.FailedBuildID)
		bugURL := util.ConstructBuganizerURLForTestAnalysis("https://test-review.googlesource.com/c/chromium/test/+/876543", tfa.ID)
		testLinks := fmt.Sprintf("(%s)[%s]\n(%s)[%s]",
			tf1.TestID,
			util.ConstructTestHistoryURL(tf1.Project, tf1.TestID, tf1.VariantHash),
			tf2.TestID,
			util.ConstructTestHistoryURL(tf2.Project, tf2.TestID, tf2.VariantHash))

		Convey("gerrit action disabled", func() {
			testCfg := &configpb.Config{
				TestAnalysisConfig: &configpb.TestAnalysisConfig{
					GerritConfig: &configpb.GerritConfig{
						ActionsEnabled: false,
					},
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			err := processTestFailureCulpritTask(ctx, tfa.ID)
			So(err, ShouldBeNil)
			// Suspect action has been saved.
			So(datastore.Get(ctx, suspect), ShouldBeNil)
			So(suspect.InactionReason, ShouldEqual, pb.CulpritInactionReason_ACTIONS_DISABLED)
		})

		Convey("has existing revert", func() {
			Convey("has merged revert", func() {
				revertRes := &gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{
						{
							Number:  876549,
							Project: "chromium/src",
							Status:  gerritpb.ChangeStatus_MERGED,
						},
					},
				}
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(revertRes, nil).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID)
				So(err, ShouldBeNil)
				// Suspect action has been saved.
				So(datastore.Get(ctx, suspect), ShouldBeNil)
				So(suspect.InactionReason, ShouldEqual, pb.CulpritInactionReason_REVERTED_MANUALLY)
			})

			Convey("has new revert", func() {
				revertRes := &gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{
						{
							Number:  876549,
							Project: "chromium/src",
							Status:  gerritpb.ChangeStatus_NEW,
						},
					},
				}
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(revertRes, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    revertRes.Changes[0].Project,
						Number:     revertRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection recommends submitting this"+
							" revert because it has confirmed the target of this revert is"+
							" the cause of a test failure.\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n\n"+
							"If this is a false positive, please report it at %s", buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID)
				So(err, ShouldBeNil)
				// Suspect action has been saved.
				So(datastore.Get(ctx, suspect), ShouldBeNil)
				So(suspect.HasSupportRevertComment, ShouldBeTrue)
				So(suspect.SupportRevertCommentTime, ShouldEqual, time.Unix(10000, 0).UTC())
				// Check counter incremented.
				So(culpritActionCounter.Get(ctx, "chromium", "test", "comment_revert"), ShouldEqual, 1)
			})

			Convey("only abandoned revert", func() {
				revertRes := &gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{
						{
							Number:  876549,
							Project: "chromium/src",
							Status:  gerritpb.ChangeStatus_ABANDONED,
						},
					},
				}
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(revertRes, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    culpritRes.Changes[0].Project,
						Number:     culpritRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection has identified this"+
							" change as the cause of a test failure.\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n\n"+
							"If this is a false positive, please report it at %s", buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID)
				So(err, ShouldBeNil)
				// Suspect action has been saved.
				So(datastore.Get(ctx, suspect), ShouldBeNil)
				So(suspect.HasCulpritComment, ShouldBeTrue)
				So(suspect.CulpritCommentTime, ShouldEqual, time.Unix(10000, 0).UTC())
				// Check counter incremented.
				So(culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), ShouldEqual, 1)
			})
		})

		Convey("no existing revert", func() {
			Convey("has LUCI bisection comment", func() {
				lbEmail, err := gerrit.ServiceAccountEmail(ctx)
				So(err, ShouldBeNil)
				culpritRes.Changes[0].Messages = []*gerritpb.ChangeMessageInfo{
					{Author: &gerritpb.AccountInfo{Email: lbEmail}},
				}
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{Changes: []*gerritpb.ChangeInfo{}}, nil).Times(1)

				err = processTestFailureCulpritTask(ctx, tfa.ID)
				So(err, ShouldBeNil)
				// Suspect action has been saved.
				So(datastore.Get(ctx, suspect), ShouldBeNil)
				So(suspect.InactionReason, ShouldEqual, pb.CulpritInactionReason_CULPRIT_HAS_COMMENT)
			})

			Convey("comment culprit", func() {
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{Changes: []*gerritpb.ChangeInfo{}}, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    culpritRes.Changes[0].Project,
						Number:     culpritRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection has identified this"+
							" change as the cause of a test failure.\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n\n"+
							"If this is a false positive, please report it at %s", buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID)
				So(err, ShouldBeNil)
				// Suspect action has been saved.
				So(datastore.Get(ctx, suspect), ShouldBeNil)
				So(suspect.HasCulpritComment, ShouldBeTrue)
				So(suspect.CulpritCommentTime, ShouldEqual, time.Unix(10000, 0).UTC())
				// Check counter incremented.
				So(culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), ShouldEqual, 1)
			})
		})
	})
}
