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

package gerrit

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/proto"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
)

const testGerritHost = "test-review.googlesource.com"
const testGerritProject = "chromium/test"

func TestHost(t *testing.T) {
	t.Parallel()

	Convey("Host", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)
		So(client.Host(ctx), ShouldEqual, testGerritHost)
	})
}

func TestGetChange(t *testing.T) {
	t.Parallel()

	Convey("GetChange", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("No change found", func() {
			// Set up mock response
			res := &gerritpb.ListChangesResponse{}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, testGerritProject, "abcdefgh")
			So(err, ShouldErrLike, "no change found")
			So(changeInfo, ShouldBeNil)
		})

		Convey("More than 1 change found", func() {
			// Set up mock response
			res := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{
					{
						Number:  123456,
						Project: testGerritProject,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
					{
						Number:  234567,
						Project: testGerritProject,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
				},
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, testGerritProject, "abcdefgh")
			So(err, ShouldErrLike, "multiple changes found")
			So(changeInfo, ShouldBeNil)
		})

		Convey("Exactly 1 change found", func() {
			// Set up mock response
			expectedChange := &gerritpb.ChangeInfo{
				Number:  123456,
				Project: testGerritProject,
				Status:  gerritpb.ChangeStatus_MERGED,
			}
			res := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{expectedChange},
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, testGerritProject, "abcdefgh")
			So(err, ShouldBeNil)
			So(changeInfo, ShouldResemble, expectedChange)
		})
	})
}

func TestRefetchChange(t *testing.T) {
	t.Parallel()

	Convey("RefetchChange", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("Latest change is returned", func() {
			change := &gerritpb.ChangeInfo{
				Number:  123456,
				Project: testGerritProject,
				Status:  gerritpb.ChangeStatus_NEW,
			}

			// Set up mock response
			res := &gerritpb.ChangeInfo{
				Number:  change.Number,
				Project: change.Project,
				Status:  gerritpb.ChangeStatus_MERGED,
			}
			mockClient.Client.EXPECT().GetChange(gomock.Any(), proto.MatcherEqual(
				&gerritpb.GetChangeRequest{
					Project: change.Project,
					Number:  change.Number,
					Options: queryOptions,
				},
			)).Return(res, nil).Times(1)

			latestChange, err := client.RefetchChange(ctx, change)
			So(err, ShouldBeNil)
			So(latestChange, ShouldResemble, res)
		})
	})
}

func TestGetReverts(t *testing.T) {
	t.Parallel()

	Convey("GetReverts", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("No revert found", func() {
			// Set up mock response
			res := &gerritpb.ListChangesResponse{}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(res, nil).Times(1)

			changeInfo := &gerritpb.ChangeInfo{
				Number:  123456,
				Project: testGerritProject,
			}
			reverts, err := client.GetReverts(ctx, changeInfo)
			So(err, ShouldBeNil)
			So(len(reverts), ShouldEqual, 0)
		})

		Convey("At least 1 revert found", func() {
			// Set up mock response
			res := &gerritpb.ListChangesResponse{
				Changes: []*gerritpb.ChangeInfo{
					{
						Number:  234567,
						Project: testGerritProject,
					},
					{
						Number:  345678,
						Project: testGerritProject,
					},
				},
			}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).
				Return(res, nil).Times(1)

			changeInfo := &gerritpb.ChangeInfo{
				Number:  123456,
				Project: testGerritProject,
			}
			reverts, err := client.GetReverts(ctx, changeInfo)
			So(err, ShouldBeNil)
			So(reverts, ShouldResemble, res.Changes)
		})
	})
}

func TestHasDependency(t *testing.T) {
	t.Parallel()

	Convey("HasMergedDependency", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("no related changes", func() {
			// Set up mock response
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(&gerritpb.GetRelatedChangesResponse{}, nil).Times(1)

			changeInfo := &gerritpb.ChangeInfo{
				Number:  123456,
				Project: testGerritProject,
			}
			hasDependency, err := client.HasDependency(ctx, changeInfo)
			So(err, ShouldBeNil)
			So(hasDependency, ShouldEqual, false)
		})

		Convey("change is newest merged commit", func() {
			// Set up mock response
			relatedChanges := &gerritpb.GetRelatedChangesResponse{
				Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
					{
						Project: testGerritProject,
						Number:  123456,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
					{
						Project: testGerritProject,
						Number:  123401,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
				},
			}
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(relatedChanges, nil).Times(1)

			changeInfo := &gerritpb.ChangeInfo{
				Number:  123456,
				Project: testGerritProject,
			}
			hasDependency, err := client.HasDependency(ctx, changeInfo)
			So(err, ShouldBeNil)
			So(hasDependency, ShouldEqual, false)
		})

		Convey("change has a merged dependency", func() {
			// Set up mock response
			relatedChanges := &gerritpb.GetRelatedChangesResponse{
				Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
					{
						Project: testGerritProject,
						Number:  123456,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
					{
						Project: testGerritProject,
						Number:  123401,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
				},
			}
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(relatedChanges, nil).Times(1)

			changeInfo := &gerritpb.ChangeInfo{
				Number:  123401,
				Project: testGerritProject,
			}
			hasDependency, err := client.HasDependency(ctx, changeInfo)
			So(err, ShouldBeNil)
			So(hasDependency, ShouldEqual, true)
		})

		Convey("change has an unmerged dependency", func() {
			// Set up mock response
			relatedChanges := &gerritpb.GetRelatedChangesResponse{
				Changes: []*gerritpb.GetRelatedChangesResponse_ChangeAndCommit{
					{
						Project: testGerritProject,
						Number:  123456,
						Status:  gerritpb.ChangeStatus_NEW,
					},
					{
						Project: testGerritProject,
						Number:  123401,
						Status:  gerritpb.ChangeStatus_MERGED,
					},
				},
			}
			mockClient.Client.EXPECT().GetRelatedChanges(gomock.Any(), gomock.Any()).
				Return(relatedChanges, nil).Times(1)

			changeInfo := &gerritpb.ChangeInfo{
				Number:  123401,
				Project: testGerritProject,
			}
			hasDependency, err := client.HasDependency(ctx, changeInfo)
			So(err, ShouldBeNil)
			So(hasDependency, ShouldEqual, false)
		})
	})
}

func TestCreateRevert(t *testing.T) {
	t.Parallel()

	Convey("CreateRevert", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Set up mock response
		expectedRevert := &gerritpb.ChangeInfo{
			Number:  234567,
			Project: testGerritProject,
			Status:  gerritpb.ChangeStatus_NEW,
			Created: &timestamppb.Timestamp{Seconds: 100},
			Updated: &timestamppb.Timestamp{Seconds: 100},
			Owner: &gerritpb.AccountInfo{
				Name:      "LUCI Bisection",
				Email:     "luci-bisection@test.com",
				AccountId: 10001,
			},
		}
		mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(expectedRevert, nil).Times(1)

		changeInfo := &gerritpb.ChangeInfo{
			Number:  123456,
			Project: testGerritProject,
		}
		revertInfo, err := client.CreateRevert(ctx, changeInfo, "LUCI Bisection created this revert automatically")
		So(err, ShouldBeNil)
		So(revertInfo, ShouldResemble, expectedRevert)
	})
}

func TestAddComment(t *testing.T) {
	t.Parallel()

	Convey("AddComment", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Set up mock response
		mockClient.Client.EXPECT().SetReview(gomock.Any(), gomock.Any()).
			Return(&gerritpb.ReviewResult{}, nil).Times(1)

		changeInfo := &gerritpb.ChangeInfo{
			Number:  123456,
			Project: testGerritProject,
		}
		reviewResult, err := client.AddComment(ctx, changeInfo, "This change has been confirmed as the culprit.")
		So(err, ShouldBeNil)
		So(reviewResult, ShouldNotBeNil)
	})
}

func TestSendForReview(t *testing.T) {
	t.Parallel()

	Convey("SendForReview", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Set up mock response
		expectedResult := &gerritpb.ReviewResult{
			Reviewers: map[string]*gerritpb.AddReviewerResult{
				"jdoe@example.com": {
					Input: "jdoe@example.com",
					Reviewers: []*gerritpb.ReviewerInfo{
						{
							Account: &gerritpb.AccountInfo{
								Name:      "John Doe",
								Email:     "jdoe@example.com",
								AccountId: 10001,
							},
							Approvals: map[string]int32{
								"Verified":    0,
								"Code-Review": 0,
							},
						},
					},
				},
				"10003": {
					Input: "10003",
					Ccs: []*gerritpb.ReviewerInfo{
						{
							Account: &gerritpb.AccountInfo{
								Name:      "Eve Smith",
								Email:     "esmith@example.com",
								AccountId: 10003,
							},
							Approvals: map[string]int32{
								"Verified":    0,
								"Code-Review": 0,
							},
						},
					},
				},
			},
		}
		mockClient.Client.EXPECT().SetReview(gomock.Any(), gomock.Any()).
			Return(expectedResult, nil).Times(1)

		changeInfo := &gerritpb.ChangeInfo{
			Number:  123456,
			Project: testGerritProject,
		}
		reviewerEmails := []string{"jdoe@example.com"}
		ccEmails := []string{"esmith@example.com"}
		reviewResult, err := client.SendForReview(ctx, changeInfo,
			"This change has been identified as a possible culprit.", reviewerEmails, ccEmails)
		So(err, ShouldBeNil)
		So(reviewResult, ShouldResemble, expectedResult)
	})
}

func TestCommitRevert(t *testing.T) {
	t.Parallel()

	Convey("CommitRevert", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("change which isn't a pure revert cannot be committed", func() {
			// Set up mock response
			mockClient.Client.EXPECT().GetPureRevert(gomock.Any(), gomock.Any()).
				Return(&gerritpb.PureRevertInfo{
					IsPureRevert: false,
				}, nil).Times(1)

			revertInfo := &gerritpb.ChangeInfo{
				Number:  234567,
				Project: testGerritProject,
			}
			ccEmails := []string{"jdoe@example.com", "esmith@example.com"}
			reviewResult, err := client.CommitRevert(ctx, revertInfo,
				"This revert has been submitted automatically.", ccEmails)
			So(err, ShouldErrLike, "not a pure revert")
			So(reviewResult, ShouldBeNil)
		})

		Convey("change which is a pure revert can be committed", func() {
			// Set up mock responses
			mockClient.Client.EXPECT().GetPureRevert(gomock.Any(), gomock.Any()).
				Return(&gerritpb.PureRevertInfo{
					IsPureRevert: true,
				}, nil).Times(1)
			expectedResult := &gerritpb.ReviewResult{
				Labels: map[string]int32{
					"Owners-Override": 1,
					"Bot-Commit":      1,
					"CQ":              2,
				},
				Reviewers: map[string]*gerritpb.AddReviewerResult{
					"90000": {
						Input: "90000",
						Reviewers: []*gerritpb.ReviewerInfo{
							{
								Account: &gerritpb.AccountInfo{
									Name:      "LUCI Bisection",
									Email:     "luci-bisection@example.com",
									AccountId: 90000,
								},
								Approvals: map[string]int32{
									"Verified":    0,
									"Code-Review": 0,
								},
							},
						},
					},
					"jdoe@example.com": {
						Input: "jdoe@example.com",
						Ccs: []*gerritpb.ReviewerInfo{
							{
								Account: &gerritpb.AccountInfo{
									Name:      "John Doe",
									Email:     "jdoe@example.com",
									AccountId: 10001,
								},
								Approvals: map[string]int32{
									"Verified":    0,
									"Code-Review": 0,
								},
							},
						},
					},
					"esmith@example.com": {
						Input: "esmith@example.com",
						Ccs: []*gerritpb.ReviewerInfo{
							{
								Account: &gerritpb.AccountInfo{
									Name:      "Eve Smith",
									Email:     "esmith@example.com",
									AccountId: 10003,
								},
								Approvals: map[string]int32{
									"Verified":    0,
									"Code-Review": 0,
								},
							},
						},
					},
				},
			}
			mockClient.Client.EXPECT().SetReview(gomock.Any(), gomock.Any()).
				Return(expectedResult, nil).Times(1)

			revertInfo := &gerritpb.ChangeInfo{
				Number:  234567,
				Project: testGerritProject,
			}
			ccEmails := []string{"jdoe@example.com", "esmith@example.com"}
			reviewResult, err := client.CommitRevert(ctx, revertInfo,
				"This change has been confirmed as the culprit and has been auto-reverted.",
				ccEmails)
			So(err, ShouldBeNil)
			So(reviewResult, ShouldResemble, expectedResult)
		})

	})
}
