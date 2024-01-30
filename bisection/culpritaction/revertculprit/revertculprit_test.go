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
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/internal/rotationproxy"
	"go.chromium.org/luci/bisection/model"
	configpb "go.chromium.org/luci/bisection/proto/config"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/testutil"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestRevertCulprit(t *testing.T) {
	t.Parallel()

	Convey("RevertCulprit", t, func() {
		ctx := memory.Use(context.Background())
		testutil.UpdateIndices(ctx)

		// Set test clock
		cl := testclock.New(testclock.TestTimeUTC)
		ctx = clock.Set(ctx, cl)

		// Setup tsmon
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		// Setup datastore
		failedBuild, _, analysis := testutil.CreateCompileFailureAnalysisAnalysisChain(
			ctx, 88128398584903, "chromium", 444)
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(ctx, analysis),
		}
		So(datastore.Put(ctx, heuristicAnalysis), ShouldBeNil)

		nsa := &model.CompileNthSectionAnalysis{
			ParentAnalysis: datastore.KeyForObj(ctx, analysis),
		}
		So(datastore.Put(ctx, nsa), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		analysisURL := util.ConstructCompileAnalysisURL("chromium", failedBuild.Id)
		buildURL := util.ConstructBuildURL(ctx, failedBuild.Id)
		bugURL := util.ConstructBuganizerURLForAnalysis(analysisURL,
			"https://test-review.googlesource.com/c/chromium/test/+/876543")

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := gerrit.NewMockedClient(ctx, ctl)
		ctx = rotationproxy.MockedRotationProxyClientContext(mockClient.Ctx, map[string]string{
			"oncallator:chrome-build-sheriff": `{"emails":["jdoe@example.com", "esmith@example.com"],"updated_unix_timestamp":1669331526}`,
		})
		// Set the project-level config for this test
		gerritConfig := &configpb.GerritConfig{
			ActionsEnabled: true,
			CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 10,
			},
			SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
				Enabled:    true,
				DailyLimit: 4,
			},
			MaxRevertibleCulpritAge: 21600, // 6 hours
			NthsectionSettings: &configpb.GerritConfig_NthSectionSettings{
				Enabled:                     true,
				ActionWhenVerificationError: false,
			},
		}
		projectCfg := config.CreatePlaceholderProjectConfig()
		projectCfg.CompileAnalysisConfig.GerritConfig = gerritConfig
		cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
		So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

		Convey("must be confirmed culprit", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             1,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_UnderVerification,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := TakeCulpritAction(ctx, heuristicSuspect)
			expectedErr := fmt.Sprintf("suspect (commit %s) has verification status"+
				" %s and should not be reverted", heuristicSuspect.GitilesCommit.Id,
				heuristicSuspect.VerificationStatus)
			So(err, ShouldErrLike, expectedErr)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
		})

		Convey("nthsection actions must be enabled", func() {
			// Set up suspect in datastore
			nthsectionSuspect := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, nthsectionSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the project-level config for this test
			gerritConfig.NthsectionSettings.Enabled = false
			projectCfg := config.CreatePlaceholderProjectConfig()
			projectCfg.CompileAnalysisConfig.GerritConfig = gerritConfig
			cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
			So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

			err := TakeCulpritAction(ctx, nthsectionSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				nthsectionSuspect.Id, nthsectionSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
		})

		Convey("nthsection suspect must have correct status", func() {
			// Set up suspect in datastore
			nthsectionSuspect := &model.Suspect{
				Type:           model.SuspectType_NthSection,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				VerificationStatus: model.SuspectVerificationStatus_VerificationError,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, nthsectionSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			err := TakeCulpritAction(ctx, nthsectionSuspect)
			expectedErr := fmt.Sprintf("suspect (commit %s) has verification status"+
				" %s and should not be reverted", nthsectionSuspect.GitilesCommit.Id,
				nthsectionSuspect.VerificationStatus)
			So(err, ShouldErrLike, expectedErr)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				nthsectionSuspect.Id, nthsectionSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
		})

		Convey("all Gerrit actions disabled", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             2,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the project-level config for this test
			gerritConfig.ActionsEnabled = false
			projectCfg := config.CreatePlaceholderProjectConfig()
			projectCfg.CompileAnalysisConfig.GerritConfig = gerritConfig
			cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
			So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
				InactionReason:          pb.CulpritInactionReason_ACTIONS_DISABLED,
			})
		})

		Convey("already reverted", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             3,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
			revertRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{
					{
						Number:  876548,
						Project: "chromium/src",
						Status:  gerritpb.ChangeStatus_ABANDONED,
					},
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

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
				InactionReason:          pb.CulpritInactionReason_REVERTED_MANUALLY,
			})
		})

		Convey("only abandoned revert exists", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             4,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
			revertRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:  876549,
					Project: "chromium/src",
					Status:  gerritpb.ChangeStatus_ABANDONED,
				}},
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
						" change as the culprit of a build failure. See the analysis: %s\n\n"+
						"A revert for this change was not created because an abandoned"+
						" revert already exists.\n\nSample failed build: %s\n\nIf this is"+
						" a false positive, please report it at %s",
						analysisURL, buildURL, bugURL),
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       true,
				CulpritCommentTime:      testclock.TestTimeUTC.Round(time.Second),
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "comment_culprit"), ShouldEqual, 1)
		})

		Convey("active revert exists", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             5,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
			revertRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{
					{
						Number:  876548,
						Project: "chromium/src",
						Status:  gerritpb.ChangeStatus_ABANDONED,
					},
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
					Project:    revertRes.Changes[1].Project,
					Number:     revertRes.Changes[1].Number,
					RevisionId: "current",
					Message: fmt.Sprintf("LUCI Bisection recommends submitting this"+
						" revert because it has confirmed the target of this revert is the"+
						" culprit of a build failure. See the analysis: %s\n\n"+
						"Sample failed build: %s\n\nIf this is a false positive, please"+
						" report it at %s", analysisURL, buildURL, bugURL),
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:                "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:          false,
				IsRevertCommitted:        false,
				HasSupportRevertComment:  true,
				SupportRevertCommentTime: testclock.TestTimeUTC.Round(time.Second),
				HasCulpritComment:        false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "comment_revert"), ShouldEqual, 1)
		})

		Convey("non-sheriffable builder", func() {
			failedBuild.SheriffRotations = []string{}
			So(datastore.Put(ctx, failedBuild), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             6,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
								Message: "Title.\n\nBody is here.\n\nNOAUTOREVERT=true\n\nChange-Id: I100deadbeef",
								Author: &gerritpb.GitPersonInfo{
									Name:  "John Doe",
									Email: "jdoe@example.com",
								},
							},
						},
					},
				}},
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
						" change as the culprit of a build failure. See the analysis: %s\n\n"+
						"A revert for this change was not created because"+
						" the associated builder is not being watched by gardeners.\n\n"+
						"Sample failed build: %s\n\nIf this is a false positive, please report"+
						" it at %s", analysisURL, buildURL, bugURL),
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       true,
				CulpritCommentTime:      testclock.TestTimeUTC.Round(time.Second),
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "comment_culprit"), ShouldEqual, 1)
		})

		Convey("revert has auto-revert off flag set", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             6,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
								Message: "Title.\n\nBody is here.\n\nNOAUTOREVERT=true\n\nChange-Id: I100deadbeef",
								Author: &gerritpb.GitPersonInfo{
									Name:  "John Doe",
									Email: "jdoe@example.com",
								},
							},
						},
					},
				}},
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
						" change as the culprit of a build failure. See the analysis: %s\n\n"+
						"A revert for this change was not created because"+
						" auto-revert has been disabled for this CL by its description.\n\n"+
						"Sample failed build: %s\n\nIf this is a false positive, please report"+
						" it at %s", analysisURL, buildURL, bugURL),
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       true,
				CulpritCommentTime:      testclock.TestTimeUTC.Round(time.Second),
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "comment_culprit"), ShouldEqual, 1)
		})

		Convey("revert was from an irrevertible author", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             7,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
									Name:  "ChromeOS Commit Bot",
									Email: "chromeos-commit-bot@chromium.org",
								},
							},
						},
					},
				}},
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
						" change as the culprit of a build failure. See the analysis: %s\n\n"+
						"A revert for this change was not created because"+
						" LUCI Bisection cannot revert changes from this CL's author.\n\n"+
						"Sample failed build: %s\n\nIf this is a false positive, please report"+
						" it at %s", analysisURL, buildURL, bugURL),
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       true,
				CulpritCommentTime:      testclock.TestTimeUTC.Round(time.Second),
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "comment_culprit"), ShouldEqual, 1)
		})

		Convey("culprit has a downstream dependency", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             8,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
						" change as the culprit of a build failure. See the analysis: %s\n\n"+
						"A revert for this change was not created because there are merged"+
						" changes depending on it.\n\nSample failed build: %s\n\nIf this is"+
						" a false positive, please report it at %s",
						analysisURL, buildURL, bugURL),
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       true,
				CulpritCommentTime:      testclock.TestTimeUTC.Round(time.Second),
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "comment_culprit"), ShouldEqual, 1)
		})

		Convey("revert creation is disabled", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             9,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the project-level config for this test
			gerritConfig.CreateRevertSettings.Enabled = false
			projectCfg := config.CreatePlaceholderProjectConfig()
			projectCfg.CompileAnalysisConfig.GerritConfig = gerritConfig
			cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
			So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

			// Set up mock responses
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
						" change as the culprit of a build failure. See the analysis: %s\n\n"+
						"A revert for this change was not created because"+
						" LUCI Bisection's revert creation has been disabled.\n\n"+
						"Sample failed build: %s\n\nIf this is a false positive, please"+
						" report it at %s", analysisURL, buildURL, bugURL),
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       true,
				CulpritCommentTime:      testclock.TestTimeUTC.Round(time.Second),
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "comment_culprit"), ShouldEqual, 1)
		})

		Convey("culprit was committed too long ago", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             10,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:          876543,
					Project:         "chromium/src",
					Status:          gerritpb.ChangeStatus_MERGED,
					Submitted:       timestamppb.New(clock.Now(ctx).Add(-time.Hour * 30)),
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
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{revertRes},
				}, nil).Times(1)
			mockClient.Client.EXPECT().GetChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    revertRes.Project,
					Number:     revertRes.Number,
					RevisionId: "current",
					Message: "LUCI Bisection could not automatically submit this revert" +
						" because the target of this revert was not committed recently.",
					Reviewers: []*gerritpb.ReviewerInput{
						{
							Reviewer: "jdoe@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_REVIEWER,
						},
						{
							Reviewer: "esmith@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_REVIEWER,
						},
					},
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
		})

		Convey("revert commit is disabled", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             11,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the project-level config for this test
			gerritConfig.SubmitRevertSettings.Enabled = false
			projectCfg := config.CreatePlaceholderProjectConfig()
			projectCfg.CompileAnalysisConfig.GerritConfig = gerritConfig
			cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
			So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

			// Set up mock responses
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
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{revertRes},
				}, nil).Times(1)
			mockClient.Client.EXPECT().GetChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    revertRes.Project,
					Number:     revertRes.Number,
					RevisionId: "current",
					Message: "LUCI Bisection could not automatically submit this revert" +
						" because LUCI Bisection's revert submission has been disabled.",
					Reviewers: []*gerritpb.ReviewerInput{
						{
							Reviewer: "jdoe@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_REVIEWER,
						},
						{
							Reviewer: "esmith@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_REVIEWER,
						},
					},
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
		})

		Convey("revert for culprit is created and bot-committed", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             12,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
			revertRes := &gerritpb.ChangeInfo{
				Number:  876549,
				Project: "chromium/src",
				Status:  gerritpb.ChangeStatus_NEW,
			}
			pureRevertRes := &gerritpb.PureRevertInfo{
				IsPureRevert: true,
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(culpritRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{revertRes},
				}, nil).Times(1)
			mockClient.Client.EXPECT().GetChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().GetPureRevert(gomock.Any(), gomock.Any()).
				Return(pureRevertRes, nil).Times(1)
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    revertRes.Project,
					Number:     revertRes.Number,
					RevisionId: "current",
					Message:    "LUCI Bisection is automatically submitting this revert.",
					Labels: map[string]int32{
						"Owners-Override": 1,
						"Bot-Commit":      1,
						"Commit-Queue":    2,
					},
					Reviewers: []*gerritpb.ReviewerInput{
						{
							Reviewer: "jdoe@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
						},
						{
							Reviewer: "esmith@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
						},
					},
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       true,
				RevertCommitTime:        testclock.TestTimeUTC.Round(time.Second),
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "submit_revert"), ShouldEqual, 1)
		})

		Convey("revert for culprit is created then manually committed", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             13,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
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

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
		})

		Convey("revert for culprit is created but another revert was merged in the meantime", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             14,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
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
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{
						{
							Number:  876549,
							Project: "chromium/src",
							Status:  gerritpb.ChangeStatus_NEW,
						},
						{
							Number:  876551,
							Project: "chromium/src",
							Status:  gerritpb.ChangeStatus_MERGED,
						},
					},
				}, nil).Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
		})

		Convey("revert can be created and bot-committed even if creation request times out", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             15,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:          876543,
					Project:         "chromium/src",
					Status:          gerritpb.ChangeStatus_MERGED,
					Submitted:       timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
					Subject:         "Title.",
					CurrentRevision: "deadbeef",
					Revisions: map[string]*gerritpb.RevisionInfo{
						"deadbeef": {
							Commit: &gerritpb.CommitInfo{
								Message: `Title.

Body is here.

Change-Id: I100deadbeef`,
								Author: &gerritpb.GitPersonInfo{
									Name:  "John Doe",
									Email: "jdoe@example.com",
								},
							},
						},
					},
				}},
			}
			lbEmail, err := gerrit.ServiceAccountEmail(ctx)
			So(err, ShouldBeNil)
			revertRes := &gerritpb.ChangeInfo{
				Number:  876549,
				Project: "chromium/src",
				Status:  gerritpb.ChangeStatus_NEW,
				Owner: &gerritpb.AccountInfo{
					Email: lbEmail,
				},
				CurrentRevision: "deadbeff",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeff": {
						Commit: &gerritpb.CommitInfo{
							Message: fmt.Sprintf(
								`Revert "Title."

This reverts commit 12ab34cd56ef.

Reason for revert:
LUCI Bisection has identified this change as the culprit of a build failure. See the analysis: %s

Sample failed build: %s

If this is a false positive, please report it at %s

Original change's description:
> Title.
>
> Body is here.
>
> Change-Id: I100deadbeef

Change-Id: 987654321abcdef
No-Presubmit: true
No-Tree-Checks: true
No-Try: true`, analysisURL, buildURL, bugURL),
						},
					},
				},
			}
			pureRevertRes := &gerritpb.PureRevertInfo{
				IsPureRevert: true,
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(culpritRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
				Return(nil, status.Errorf(codes.DeadlineExceeded, "revert creation timed out")).
				Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{revertRes},
				}, nil).Times(2)
			mockClient.Client.EXPECT().GetChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().GetPureRevert(gomock.Any(), gomock.Any()).
				Return(pureRevertRes, nil).Times(1)
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    revertRes.Project,
					Number:     revertRes.Number,
					RevisionId: "current",
					Message:    "LUCI Bisection is automatically submitting this revert.",
					Labels: map[string]int32{
						"Owners-Override": 1,
						"Bot-Commit":      1,
						"Commit-Queue":    2,
					},
					Reviewers: []*gerritpb.ReviewerInput{
						{
							Reviewer: "jdoe@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
						},
						{
							Reviewer: "esmith@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
						},
					},
				},
			)).Times(1)

			err = TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       true,
				RevertCommitTime:        testclock.TestTimeUTC.Round(time.Second),
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "submit_revert"), ShouldEqual, 1)
		})

		Convey("revert is not bot-committed for non-timeout error when creating a revert", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             16,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:          876543,
					Project:         "chromium/src",
					Status:          gerritpb.ChangeStatus_MERGED,
					Submitted:       timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
					Subject:         "Title.",
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
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(culpritRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
				Return(nil, status.Errorf(codes.Internal, "revert creation failed internally")).
				Times(1)

			err := TakeCulpritAction(ctx, heuristicSuspect)
			So(err, ShouldNotBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "",
				IsRevertCreated:         false,
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
		})

		Convey("revert for nthsection suspect is created although verification error", func() {
			// Setup suspect in datastore
			suspect := &model.Suspect{
				Id:                 16,
				ParentAnalysis:     datastore.KeyForObj(ctx, nsa),
				VerificationStatus: model.SuspectVerificationStatus_VerificationError,
				Type:               model.SuspectType_NthSection,
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:    "https://test-review.googlesource.com/c/chromium/test/+/876543",
				AnalysisType: pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, suspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			gerritConfig.NthsectionSettings.ActionWhenVerificationError = true
			projectCfg := config.CreatePlaceholderProjectConfig()
			projectCfg.CompileAnalysisConfig.GerritConfig = gerritConfig
			cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
			So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

			// Set up mock responses
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
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
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

			err := TakeCulpritAction(ctx, suspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err = datastoreutil.GetSuspect(ctx,
				suspect.Id, suspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       false,
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
		})

		Convey("revert for culprit is created and bot-committed for nthsection", func() {
			// Setup suspect in datastore
			suspect := &model.Suspect{
				Id:             14,
				Type:           model.SuspectType_NthSection,
				ParentAnalysis: datastore.KeyForObj(ctx, nsa),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/src",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
				AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
			}
			So(datastore.Put(ctx, suspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the project-level config for this test
			gerritConfig.NthsectionSettings.ActionWhenVerificationError = true
			projectCfg := config.CreatePlaceholderProjectConfig()
			projectCfg.CompileAnalysisConfig.GerritConfig = gerritConfig
			cfg := map[string]*configpb.ProjectConfig{"chromium": projectCfg}
			So(config.SetTestProjectConfig(ctx, cfg), ShouldBeNil)

			// Set up mock responses
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
			revertRes := &gerritpb.ChangeInfo{
				Number:  876549,
				Project: "chromium/src",
				Status:  gerritpb.ChangeStatus_NEW,
			}
			pureRevertRes := &gerritpb.PureRevertInfo{
				IsPureRevert: true,
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(culpritRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)
			mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.ListChangesResponse{
					Changes: []*gerritpb.ChangeInfo{revertRes},
				}, nil).Times(1)
			mockClient.Client.EXPECT().GetChange(gomock.Any(), gomock.Any()).
				Return(revertRes, nil).Times(1)
			mockClient.Client.EXPECT().GetPureRevert(gomock.Any(), gomock.Any()).
				Return(pureRevertRes, nil).Times(1)
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    revertRes.Project,
					Number:     revertRes.Number,
					RevisionId: "current",
					Message:    "LUCI Bisection is automatically submitting this revert.",
					Labels: map[string]int32{
						"Owners-Override": 1,
						"Bot-Commit":      1,
						"Commit-Queue":    2,
					},
					Reviewers: []*gerritpb.ReviewerInput{
						{
							Reviewer: "jdoe@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
						},
						{
							Reviewer: "esmith@example.com",
							State:    gerritpb.ReviewerInput_REVIEWER_INPUT_STATE_CC,
						},
					},
				},
			)).Times(1)

			err := TakeCulpritAction(ctx, suspect)
			So(err, ShouldBeNil)

			datastore.GetTestable(ctx).CatchupIndexes()
			suspect, err = datastoreutil.GetSuspect(ctx,
				suspect.Id, suspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.ActionDetails, ShouldResemble, model.ActionDetails{
				RevertURL:               "https://test-review.googlesource.com/c/chromium/src/+/876549",
				IsRevertCreated:         true,
				RevertCreateTime:        testclock.TestTimeUTC.Round(time.Second),
				IsRevertCommitted:       true,
				RevertCommitTime:        testclock.TestTimeUTC.Round(time.Second),
				HasSupportRevertComment: false,
				HasCulpritComment:       false,
			})
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "create_revert"), ShouldEqual, 1)
			So(culpritActionCounter.Get(ctx, "chromium", "compile", "submit_revert"), ShouldEqual, 1)
		})

	})
}
