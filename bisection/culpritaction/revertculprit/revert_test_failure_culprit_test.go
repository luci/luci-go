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
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/testutil"
)

func TestProcessTestFailureCulpritTask(t *testing.T) {
	t.Parallel()
	client := &fakeLUCIAnalysisClient{
		FailedConsistently: true,
	}

	ftt.Run("processTestFailureCulpritTask", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)

		// Set test clock
		cl := testclock.New(testclock.TestTimeUTC)
		cl.Set(time.Unix(10000, 0).UTC())
		ctx = clock.Set(ctx, cl)

		// Setup tsmon
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		// Set the project-level config for this test
		gerritConfig := &configpb.GerritConfig{
			ActionsEnabled: true,
			NthsectionSettings: &configpb.GerritConfig_NthSectionSettings{
				Enabled:                     true,
				ActionWhenVerificationError: false,
			},
		}
		projectCfg := config.CreatePlaceholderProjectConfig()
		projectCfg.TestAnalysisConfig.GerritConfig = gerritConfig
		cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
		assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil)

		// Setup datastore
		tfa := testutil.CreateTestFailureAnalysis(ctx, t, &testutil.TestFailureAnalysisCreationOption{
			Project:        "chromium",
			TestFailureKey: datastore.NewKey(ctx, "TestFailure", "", 1, nil),
			FailedBuildID:  123,
		})
		nsa := testutil.CreateTestNthSectionAnalysis(ctx, t, &testutil.TestNthSectionAnalysisCreationOption{
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
		assert.Loosely(t, datastore.Put(ctx, suspect), should.BeNil)
		tfa.VerifiedCulpritKey = datastore.KeyForObj(ctx, suspect)
		assert.Loosely(t, datastore.Put(ctx, tfa), should.BeNil)
		tf1 := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
			ID:          1,
			Project:     "chromium",
			IsPrimary:   true,
			Analysis:    tfa,
			TestID:      "testID1",
			VariantHash: "varianthash1",
		})
		tf2 := testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
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
		analysisURL := util.ConstructTestAnalysisURL("chromium", tfa.ID)
		buildURL := util.ConstructBuildURL(ctx, tfa.FailedBuildID)
		bugURL := util.ConstructBuganizerURLForAnalysis("https://test-review.googlesource.com/c/chromium/test/+/876543", analysisURL)
		testLinks := fmt.Sprintf("[%s](%s)\n[%s](%s)",
			tf1.TestID,
			util.ConstructTestHistoryURL(tf1.Project, tf1.TestID, tf1.VariantHash),
			tf2.TestID,
			util.ConstructTestHistoryURL(tf2.Project, tf2.TestID, tf2.VariantHash))

		t.Run("test no longer unexpected", func(t *ftt.Test) {
			err := processTestFailureCulpritTask(ctx, tfa.ID, &fakeLUCIAnalysisClient{
				FailedConsistently: false,
			})
			assert.Loosely(t, err, should.BeNil)
			// Suspect action has been saved.
			assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
			assert.Loosely(t, suspect.InactionReason, should.Equal(pb.CulpritInactionReason_TEST_NO_LONGER_UNEXPECTED))
			assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
		})

		t.Run("gerrit action disabled", func(t *ftt.Test) {
			projectCfg.TestAnalysisConfig.GerritConfig.ActionsEnabled = false
			cfg := map[string]*configpb.ProjectConfig{tfa.Project: projectCfg}
			assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil)

			err := processTestFailureCulpritTask(ctx, tfa.ID, client)
			assert.Loosely(t, err, should.BeNil)
			// Suspect action has been saved.
			assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
			assert.Loosely(t, suspect.InactionReason, should.Equal(pb.CulpritInactionReason_ACTIONS_DISABLED))
			assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
		})

		t.Run("has existing revert", func(t *ftt.Test) {
			t.Run("has merged revert", func(t *ftt.Test) {
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

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.InactionReason, should.Equal(pb.CulpritInactionReason_REVERTED_MANUALLY))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
			})

			t.Run("has new revert", func(t *ftt.Test) {
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
							" the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasSupportRevertComment, should.BeTrue)
				assert.That(t, suspect.SupportRevertCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_revert"), should.Equal(1))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
			})

			t.Run("only abandoned revert", func(t *ftt.Test) {
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
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n"+
							"A revert for this change was not created because an abandoned revert already exists.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
			})
		})

		t.Run("no existing revert", func(t *ftt.Test) {
			t.Run("has LUCI bisection comment", func(t *ftt.Test) {
				lbEmail, err := gerrit.ServiceAccountEmail(ctx)
				assert.Loosely(t, err, should.BeNil)
				culpritRes.Changes[0].Messages = []*gerritpb.ChangeMessageInfo{
					{Author: &gerritpb.AccountInfo{Email: lbEmail}},
				}
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{Changes: []*gerritpb.ChangeInfo{}}, nil).Times(1)

				err = processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.InactionReason, should.Equal(pb.CulpritInactionReason_CULPRIT_HAS_COMMENT))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
			})

			t.Run("comment culprit", func(t *ftt.Test) {
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
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n"+
							"A revert for this change was not created because the builder that this CL broke is not watched by gardeners, therefore less important. You can consider revert this CL, fix forward or let builder owners resolve it themselves.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
			})

			t.Run("comment culprit with more than 5 test failures", func(t *ftt.Test) {
				for i := 1; i < 8; i++ {
					testutil.CreateTestFailure(ctx, t, &testutil.TestFailureCreationOption{
						ID:          int64(i),
						Project:     "chromium",
						IsPrimary:   i == 1,
						Analysis:    tfa,
						TestID:      fmt.Sprintf("testID%d", i),
						VariantHash: fmt.Sprintf("varianthash%d", i),
					})
				}
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
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n"+
							"[testID1](https://ci.chromium.org/ui/test/chromium/testID1?q=VHash%%3Avarianthash1)\n"+
							"[testID2](https://ci.chromium.org/ui/test/chromium/testID2?q=VHash%%3Avarianthash2)\n"+
							"[testID3](https://ci.chromium.org/ui/test/chromium/testID3?q=VHash%%3Avarianthash3)\n"+
							"[testID4](https://ci.chromium.org/ui/test/chromium/testID4?q=VHash%%3Avarianthash4)\n"+
							"[testID5](https://ci.chromium.org/ui/test/chromium/testID5?q=VHash%%3Avarianthash5)\n"+
							"and 2 more ...\n"+
							"A revert for this change was not created because the builder that this CL broke is not watched by gardeners, therefore less important. You can consider revert this CL, fix forward or let builder owners resolve it themselves.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
			})
		})

		t.Run("revert creation", func(t *ftt.Test) {
			tfa.SheriffRotations = []string{"chromium"}
			assert.Loosely(t, datastore.Put(ctx, tfa), should.BeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			t.Run("revert has auto-revert off flag set", func(t *ftt.Test) {
				culpritRes.Changes[0].Revisions["deadbeef"].Commit.Message = "Title.\n\nBody is here.\n\nNOAUTOREVERT=true\n\nChange-Id: I100deadbeef"
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    culpritRes.Changes[0].Project,
						Number:     culpritRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection has identified this"+
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n"+
							"A revert for this change was not created because auto-revert has been disabled for this CL by its description.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)

				datastore.GetTestable(ctx).CatchupIndexes()
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
			})

			t.Run("revert was from an irrevertible author", func(t *ftt.Test) {
				culpritRes.Changes[0].Revisions["deadbeef"].Commit.Author = &gerritpb.GitPersonInfo{
					Name:  "ChromeOS Commit Bot",
					Email: "chromeos-commit-bot@chromium.org",
				}

				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    culpritRes.Changes[0].Project,
						Number:     culpritRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection has identified this"+
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n"+
							"A revert for this change was not created because LUCI Bisection cannot revert changes from this CL's author.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)

				datastore.GetTestable(ctx).CatchupIndexes()
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
			})

			t.Run("culprit has a downstream dependency", func(t *ftt.Test) {
				revertRes := &gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{},
				}
				relatedChanges := &gerritpb.GetRelatedChangesResponse{
					Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
						{
							Project: "chromium/src",
							Number:  876544,
							Status:  gerritpb.ChangeStatus_MERGED,
						},
						{
							Project: "chromium/src",
							Number:  876543,
							Status:  gerritpb.ChangeStatus_MERGED,
						},
					},
				}
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(revertRes, nil).Times(1)
				mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
					Return(relatedChanges, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    culpritRes.Changes[0].Project,
						Number:     culpritRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection has identified this"+
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n"+
							"A revert for this change was not created because there are merged changes depending on it.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)

				datastore.GetTestable(ctx).CatchupIndexes()
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
			})

			t.Run("revert creation is disabled", func(t *ftt.Test) {
				projectCfg.TestAnalysisConfig.GerritConfig.CreateRevertSettings = &configpb.GerritConfig_RevertActionSettings{
					Enabled: false,
				}
				cfg := map[string]*configpb.ProjectConfig{tfa.Project: projectCfg}
				assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    culpritRes.Changes[0].Project,
						Number:     culpritRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection has identified this"+
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n"+
							"A revert for this change was not created because LUCI Bisection's revert creation has been disabled.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)

				datastore.GetTestable(ctx).CatchupIndexes()
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
			})

			t.Run("exceed daily limit", func(t *ftt.Test) {
				// Set up config.
				projectCfg.TestAnalysisConfig.GerritConfig.CreateRevertSettings = &configpb.GerritConfig_RevertActionSettings{
					DailyLimit: 1,
					Enabled:    true,
				}
				cfg := map[string]*configpb.ProjectConfig{tfa.Project: projectCfg}
				assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil)

				// Add existing revert.
				testutil.CreateSuspect(ctx, t, &testutil.SuspectCreationOption{
					AnalysisType: pb.AnalysisType_TEST_FAILURE_ANALYSIS,
					ActionDetails: model.ActionDetails{
						IsRevertCreated:  true,
						RevertCreateTime: clock.Now(ctx).Add(-time.Hour),
					},
				})
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
					&gerritpb.SetReviewRequest{
						Project:    culpritRes.Changes[0].Project,
						Number:     culpritRes.Changes[0].Number,
						RevisionId: "current",
						Message: fmt.Sprintf("LUCI Bisection has identified this"+
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n"+
							"A revert for this change was not created because LUCI Bisection's daily limit for revert creation (1) has been reached; 1 reverts have already been created.\n\n"+
							"If this is a false positive, please report it at %s", analysisURL, buildURL, testLinks, bugURL),
					},
				)).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)

				datastore.GetTestable(ctx).CatchupIndexes()
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.HasCulpritComment, should.BeTrue)
				assert.That(t, suspect.CulpritCommentTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "comment_culprit"), should.Equal(1))
			})

			t.Run("revert created", func(t *ftt.Test) {
				// Set up config.
				projectCfg.TestAnalysisConfig.GerritConfig.CreateRevertSettings = &configpb.GerritConfig_RevertActionSettings{
					DailyLimit: 10,
					Enabled:    true,
				}
				cfg := map[string]*configpb.ProjectConfig{tfa.Project: projectCfg}
				assert.Loosely(t, config.SetTestProjectConfig(ctx, cfg), should.BeNil)

				revertRes := &gerritpb.ChangeInfo{
					Number:  876549,
					Project: "chromium/src",
					Status:  gerritpb.ChangeStatus_NEW,
				}

				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(culpritRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)
				mockClient.Client.EXPECT().RevertChange(gomock.Any(), proto.MatcherEqual(
					&gerritpb.RevertChangeRequest{
						Project: culpritRes.Changes[0].Project,
						Number:  culpritRes.Changes[0].Number,
						Message: fmt.Sprintf("Revert \"chromium/src~876543\"\n\n"+
							"This reverts commit 12ab34cd56ef.\n\n"+
							"Reason for revert:\n"+
							"LUCI Bisection has identified this"+
							" change as the cause of a test failure. See the analysis: %s\n\n"+
							"Sample build with failed test: %s\n"+
							"Affected test(s):\n%s\n\n"+
							"If this is a false positive, please report it at %s\n\n"+
							"Original change's description:\n"+
							"> Title.\n"+
							">\n"+
							"> Body is here.\n"+
							">\n"+
							"> Change-Id: I100deadbeef\n\n"+
							"No-Presubmit: true\n"+
							"No-Tree-Checks: true\n"+
							"No-Try: true", analysisURL, buildURL, testLinks, bugURL),
					},
				)).
					Return(revertRes, nil).Times(1)
				mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
					Return(&gerritpb.ListChangesResponse{
						Changes: []*gerritpb.ChangeInfo{
							{
								Number:  876549,
								Project: "chromium/src",
								Status:  gerritpb.ChangeStatus_MERGED,
							},
						},
					}, nil).Times(1)

				err := processTestFailureCulpritTask(ctx, tfa.ID, client)
				assert.Loosely(t, err, should.BeNil)

				datastore.GetTestable(ctx).CatchupIndexes()
				// Suspect action has been saved.
				assert.Loosely(t, datastore.Get(ctx, suspect), should.BeNil)
				assert.Loosely(t, suspect.IsRevertCreated, should.BeTrue)
				assert.That(t, suspect.RevertCreateTime, should.Match(time.Unix(10000, 0).UTC()))
				assert.Loosely(t, suspect.HasTakenActions, should.BeTrue)
				// Check counter incremented.
				assert.Loosely(t, culpritActionCounter.Get(ctx, "chromium", "test", "create_revert"), should.Equal(1))
			})
		})
	})
}

type fakeLUCIAnalysisClient struct {
	FailedConsistently bool
}

func (cl *fakeLUCIAnalysisClient) TestIsUnexpectedConsistently(ctx context.Context, project string, key lucianalysis.TestVerdictKey, sinceCommitPosition int64) (bool, error) {
	return cl.FailedConsistently, nil
}
