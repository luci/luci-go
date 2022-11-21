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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gerrit"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	configpb "go.chromium.org/luci/bisection/proto/config"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestRevertHeuristicCulprit(t *testing.T) {
	t.Parallel()

	Convey("RevertHeuristicCulprit", t, func() {
		ctx := memory.Use(context.Background())

		datastore.GetTestable(ctx).AddIndexes(
			&datastore.IndexDefinition{
				Kind: "Suspect",
				SortBy: []datastore.IndexColumn{
					{
						Property: "is_revert_created",
					},
					{
						Property: "revert_create_time",
					},
				},
			},
			&datastore.IndexDefinition{
				Kind: "Suspect",
				SortBy: []datastore.IndexColumn{
					{
						Property: "is_revert_committed",
					},
					{
						Property: "revert_commit_time",
					},
				},
			},
		)
		datastore.GetTestable(ctx).CatchupIndexes()

		// Set test clock
		cl := testclock.New(testclock.TestTimeUTC)
		ctx = clock.Set(ctx, cl)

		// Setup datastore
		failedBuild := &model.LuciFailedBuild{
			Id: 88128398584903,
			LuciBuild: model.LuciBuild{
				BuildId:     88128398584903,
				Project:     "chromium",
				Bucket:      "ci",
				Builder:     "android",
				BuildNumber: 123,
			},
			BuildFailureType: pb.BuildFailureType_COMPILE,
		}
		So(datastore.Put(ctx, failedBuild), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		compileFailure := &model.CompileFailure{
			Build: datastore.KeyForObj(ctx, failedBuild),
		}
		So(datastore.Put(ctx, compileFailure), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		analysis := &model.CompileFailureAnalysis{
			Id:             444,
			CompileFailure: datastore.KeyForObj(ctx, compileFailure),
		}
		So(datastore.Put(ctx, analysis), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()
		heuristicAnalysis := &model.CompileHeuristicAnalysis{
			ParentAnalysis: datastore.KeyForObj(ctx, analysis),
		}
		So(datastore.Put(ctx, heuristicAnalysis), ShouldBeNil)
		datastore.GetTestable(ctx).CatchupIndexes()

		analysisURL := util.ConstructAnalysisURL(ctx, failedBuild.Id)
		buildURL := util.ConstructBuildURL(ctx, failedBuild.Id)

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := gerrit.NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		Convey("must be confirmed culprit", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             1,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_UnderVerification,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
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
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			expectedErr := fmt.Sprintf("suspect (commit %s) has verification status"+
				" %s; must be %s to be reverted", heuristicSuspect.GitilesCommit.Id,
				heuristicSuspect.VerificationStatus,
				model.SuspectVerificationStatus_ConfirmedCulprit)
			So(err, ShouldErrLike, expectedErr)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, false)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, false)
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
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
					ActionsEnabled: false,
					CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
						Enabled:    true,
						DailyLimit: 10,
					},
					SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
						Enabled:    true,
						DailyLimit: 4,
					},
					MaxRevertibleCulpritAge: 21600, // 6 hours
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, false)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, false)
		})

		Convey("revert exists", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             3,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
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
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:    876543,
					Project:   "chromium/test",
					Status:    gerritpb.ChangeStatus_MERGED,
					Submitted: timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
				}},
			}
			revertRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:  876549,
					Project: "chromium/test",
					Status:  gerritpb.ChangeStatus_NEW,
				}},
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
						" revert because it has confirmed the target of this revert is the"+
						" culprit of a build failure. See the analysis: %s\n\n"+
						"Sample failed build: %s", analysisURL, buildURL),
				},
			)).Times(1)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, false)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, false)
		})

		Convey("culprit has a downstream dependency", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             4,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
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
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:    876543,
					Project:   "chromium/test",
					Status:    gerritpb.ChangeStatus_MERGED,
					Submitted: timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
				}},
			}
			revertRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{},
			}
			relatedChanges := &gerritpb.GetRelatedChangesResponse{
				Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
					{
						Project: "chromium/test",
						Number:  876544,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
					{
						Project: "chromium/test",
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
						" changes depending on it.\n\nSample failed build: %s",
						analysisURL, buildURL),
				},
			)).Times(1)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, false)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, false)
		})

		Convey("revert creation is disabled", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             5,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
					ActionsEnabled: true,
					CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
						Enabled:    false,
						DailyLimit: 10,
					},
					SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
						Enabled:    true,
						DailyLimit: 4,
					},
					MaxRevertibleCulpritAge: 21600, // 6 hours
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:    876543,
					Project:   "chromium/test",
					Status:    gerritpb.ChangeStatus_MERGED,
					Submitted: timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
				}},
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(culpritRes, nil).Times(1)
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    culpritRes.Changes[0].Project,
					Number:     culpritRes.Changes[0].Number,
					RevisionId: "current",
					Message: fmt.Sprintf("LUCI Bisection has identified this"+
						" change as the culprit of a build failure. See the analysis: %s\n\n"+
						"A revert for this change was not created because"+
						" LUCI Bisection's revert creation has been disabled.\n\n"+
						"Sample failed build: %s", analysisURL, buildURL),
				},
			)).Times(1)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, false)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, false)
		})

		Convey("culprit was committed too long ago", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             6,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
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
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:    876543,
					Project:   "chromium/test",
					Status:    gerritpb.ChangeStatus_MERGED,
					Submitted: timestamppb.New(clock.Now(ctx).Add(-time.Hour * 30)),
				}},
			}
			revertRes := &gerritpb.ChangeInfo{
				Number:  876549,
				Project: "chromium/test",
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
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    revertRes.Project,
					Number:     revertRes.Number,
					RevisionId: "current",
					Message: "LUCI Bisection could not automatically submit this revert" +
						" because the target of this revert was not committed recently.",
				},
			)).Times(1)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, true)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, false)
		})

		Convey("revert commit is disabled", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             7,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
					ActionsEnabled: true,
					CreateRevertSettings: &configpb.GerritConfig_RevertActionSettings{
						Enabled:    true,
						DailyLimit: 10,
					},
					SubmitRevertSettings: &configpb.GerritConfig_RevertActionSettings{
						Enabled:    false,
						DailyLimit: 4,
					},
					MaxRevertibleCulpritAge: 21600, // 6 hours
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:    876543,
					Project:   "chromium/test",
					Status:    gerritpb.ChangeStatus_MERGED,
					Submitted: timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
				}},
			}
			revertRes := &gerritpb.ChangeInfo{
				Number:  876549,
				Project: "chromium/test",
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
			mockClient.Client.EXPECT().SetReview(gomock.Any(), proto.MatcherEqual(
				&gerritpb.SetReviewRequest{
					Project:    revertRes.Project,
					Number:     revertRes.Number,
					RevisionId: "current",
					Message: "LUCI Bisection could not automatically submit this revert" +
						" because LUCI Bisection's revert submission has been disabled.",
				},
			)).Times(1)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, true)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, false)
		})

		Convey("revert for culprit is created and bot-committed", func() {
			// Setup suspect in datastore
			heuristicSuspect := &model.Suspect{
				Id:             8,
				Type:           model.SuspectType_Heuristic,
				Score:          10,
				ParentAnalysis: datastore.KeyForObj(ctx, heuristicAnalysis),
				GitilesCommit: buildbucketpb.GitilesCommit{
					Host:    "test.googlesource.com",
					Project: "chromium/test",
					Id:      "12ab34cd56ef",
				},
				ReviewUrl:          "https://test-review.googlesource.com/c/chromium/test/+/876543",
				VerificationStatus: model.SuspectVerificationStatus_ConfirmedCulprit,
			}
			So(datastore.Put(ctx, heuristicSuspect), ShouldBeNil)
			datastore.GetTestable(ctx).CatchupIndexes()

			// Set the service-level config for this test
			testCfg := &configpb.Config{
				GerritConfig: &configpb.GerritConfig{
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
				},
			}
			So(config.SetTestConfig(ctx, testCfg), ShouldBeNil)

			// Set up mock responses
			culpritRes := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{{
					Number:    876543,
					Project:   "chromium/test",
					Status:    gerritpb.ChangeStatus_MERGED,
					Submitted: timestamppb.New(clock.Now(ctx).Add(-time.Hour * 3)),
				}},
			}
			revertRes := &gerritpb.ChangeInfo{
				Number:  876549,
				Project: "chromium/test",
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
						"CQ":              2,
					},
				},
			)).Times(1)

			err := RevertHeuristicCulprit(ctx, heuristicSuspect)
			So(err, ShouldBeNil)

			suspect, err := datastoreutil.GetSuspect(ctx,
				heuristicSuspect.Id, heuristicSuspect.ParentAnalysis)
			So(err, ShouldBeNil)
			So(suspect, ShouldNotBeNil)
			So(suspect.RevertDetails.IsRevertCreated, ShouldEqual, true)
			So(suspect.RevertDetails.IsRevertCommitted, ShouldEqual, true)
		})
	})
}
