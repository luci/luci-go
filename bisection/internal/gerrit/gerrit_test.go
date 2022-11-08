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

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	. "go.chromium.org/luci/common/testing/assertions"
)

const testGerritHost = "test-review.googlesource.com"
const testGerritProject = "chromium/test"

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
		client, err := NewClient(ctx, testGerritHost, testGerritProject)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("No change found", func() {
			// Set up mock response
			res := &gerritpb.ListChangesResponse{}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, "abcdefgh")
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
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, "abcdefgh")
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
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

			changeInfo, err := client.GetChange(ctx, "abcdefgh")
			So(err, ShouldBeNil)
			So(changeInfo, ShouldResemble, expectedChange)
		})
	})
}

func TestGetRevertOf(t *testing.T) {
	t.Parallel()

	Convey("GetReverts", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost, testGerritProject)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		Convey("No revert found", func() {
			// Set up mock response
			res := &gerritpb.ListChangesResponse{}
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

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
			mockClient.Client.EXPECT().ListChanges(gomock.Any(), gomock.Any()).Return(res, nil).Times(1)

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
		client, err := NewClient(ctx, testGerritHost, testGerritProject)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Set up mock response
		expectedChange := &gerritpb.ChangeInfo{
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
		mockClient.Client.EXPECT().RevertChange(gomock.Any(), gomock.Any(), gomock.Any()).Return(expectedChange, nil).Times(1)

		changeInfo, err := client.CreateRevert(ctx, 123456, "LUCI Bisection created this revert automatically")
		So(err, ShouldBeNil)
		So(changeInfo, ShouldResemble, expectedChange)
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
		client, err := NewClient(ctx, testGerritHost, testGerritProject)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Set up mock response
		mockClient.Client.EXPECT().SetReview(gomock.Any(), gomock.Any()).Return(&gerritpb.ReviewResult{}, nil).Times(1)

		reviewResult, err := client.AddComment(ctx, 123456, "This change has been confirmed as the culprit.")
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
		client, err := NewClient(ctx, testGerritHost, testGerritProject)
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
		mockClient.Client.EXPECT().SetReview(gomock.Any(), gomock.Any()).Return(expectedResult, nil).Times(1)

		reviewerAccounts := []*gerritpb.AccountInfo{{AccountId: 10001}}
		ccAccounts := []*gerritpb.AccountInfo{{AccountId: 10003}}
		reviewResult, err := client.SendForReview(ctx, 123456,
			"This change has been identified as a possible culprit.", reviewerAccounts, ccAccounts)
		So(err, ShouldBeNil)
		So(reviewResult, ShouldResemble, expectedResult)
	})
}

func TestCommit(t *testing.T) {
	t.Parallel()

	Convey("Commit", t, func() {
		ctx := context.Background()

		// Set up mock Gerrit client
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := NewMockedClient(ctx, ctl)
		ctx = mockClient.Ctx

		// Set up Gerrit client
		client, err := NewClient(ctx, testGerritHost, testGerritProject)
		So(err, ShouldBeNil)
		So(client, ShouldNotBeNil)

		// Set up mock response
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
		mockClient.Client.EXPECT().SetReview(gomock.Any(), gomock.Any()).Return(expectedResult, nil).Times(1)

		ccAccounts := []*gerritpb.AccountInfo{{AccountId: 10001}, {AccountId: 10003}}
		reviewResult, err := client.Commit(ctx, 123456,
			"This change has been confirmed as the culprit and has been auto-reverted.", ccAccounts)
		So(err, ShouldBeNil)
		So(reviewResult, ShouldResemble, expectedResult)
	})
}
