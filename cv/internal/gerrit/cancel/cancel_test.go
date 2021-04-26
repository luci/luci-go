// Copyright 2021 The LUCI Authors.
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

package cancel

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCancel(t *testing.T) {
	t.Parallel()

	Convey("Cancel", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		user := gf.U("user-100")
		const gHost = "x-review.example.com"
		const lProject = "lProject"
		const changeNum = 10001
		triggerTime := ct.Clock.Now().Add(-2 * time.Minute)
		ci := gf.CI(
			10001, gf.PS(2),
			gf.CQ(2, triggerTime, user),
			gf.Updated(clock.Now(ctx).Add(-1*time.Minute)))
		So(trigger.Find(ci, &cfgpb.ConfigGroup{}).GerritAccountId, ShouldEqual, 100)
		cl := &changelist.CL{
			ID:         99999,
			ExternalID: changelist.MustGobID(gHost, int64(changeNum)),
			EVersion:   2,
			Snapshot: &changelist.Snapshot{
				ExternalUpdateTime:    timestamppb.New(clock.Now(ctx).Add(-3 * time.Minute)),
				LuciProject:           lProject,
				Patchset:              2,
				MinEquivalentPatchset: 1,
				Kind: &changelist.Snapshot_Gerrit{
					Gerrit: &changelist.Gerrit{
						Host: gHost,
						Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
					},
				},
			},
		}
		So(datastore.Put(ctx, cl), ShouldBeNil)
		ct.GFake.CreateChange(&gf.Change{
			Host: gHost,
			Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
			ACLs: gf.ACLGrant(gf.OpRead, codes.PermissionDenied, lProject).Or(
				gf.ACLGrant(gf.OpReview, codes.PermissionDenied, lProject),
				gf.ACLGrant(gf.OpAlterVotesOfOthers, codes.PermissionDenied, lProject),
			),
		})

		input := Input{
			CL:            cl,
			Trigger:       trigger.Find(ci, &cfgpb.ConfigGroup{}),
			LUCIProject:   lProject,
			Message:       "Full Run has passed",
			Requester:     "test",
			Notify:        OWNER | VOTERS,
			LeaseDuration: 30 * time.Second,
			RunCLExternalIDs: []changelist.ExternalID{
				changelist.MustGobID(gHost, int64(10002)),
				changelist.MustGobID(gHost, int64(10003)),
			},
		}

		Convey("Fails PreCondition if CL has newer PS in datastore", func() {
			newCI := proto.Clone(ci).(*gerritpb.ChangeInfo)
			gf.PS(3)(newCI)
			newCL := &changelist.CL{
				ID:         99999,
				ExternalID: changelist.MustGobID(gHost, int64(changeNum)),
				EVersion:   3,
				Snapshot: &changelist.Snapshot{
					ExternalUpdateTime:    timestamppb.New(clock.Now(ctx).Add(-1 * time.Minute)),
					LuciProject:           lProject,
					Patchset:              3,
					MinEquivalentPatchset: 3,
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: gHost,
							Info: newCI,
						},
					},
				},
			}
			So(datastore.Put(ctx, newCL), ShouldBeNil)
			err := Cancel(ctx, input)
			So(err, ShouldErrLike, "failed to cancel because ps 2 is not current for cl(99999)")
			So(ErrPreconditionFailedTag.In(err), ShouldBeTrue)
		})

		Convey("Fails PreCondition if CL has newer PS in Gerrit", func() {
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.PS(3)(c.Info)
			})
			err := Cancel(ctx, input)
			So(err, ShouldErrLike, "failed to cancel because ps 2 is not current for x-review.example.com/10001")
			So(ErrPreconditionFailedTag.In(err), ShouldBeTrue)
		})

		Convey("Fails if receive stale data from gerrit", func() {
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.Updated(clock.Now(ctx).Add(-3 * time.Minute))(c.Info)
			})
			err := Cancel(ctx, input)
			So(err, ShouldErrLike, "got stale change info from gerrit for x-review.example.com/10001")
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("Remove single votes", func() {
			err := Cancel(ctx, input)
			So(err, ShouldBeNil)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			So(resultCI.Info.GetMessages(), ShouldHaveLength, 1)
			So(resultCI.Info.GetMessages()[0].GetMessage(), ShouldEqual, input.Message)
			So(gf.NonZeroVotes(resultCI.Info, trigger.CQLabelName), ShouldBeEmpty)
			var setReviewReq *gerritpb.SetReviewRequest
			for _, req := range ct.GFake.Requests() {
				if r, ok := req.(*gerritpb.SetReviewRequest); ok {
					setReviewReq = r
				}
			}
			So(setReviewReq.GetOnBehalfOf(), ShouldEqual, user.GetAccountId())
			So(setReviewReq.GetNotifyDetails(), ShouldResembleProto,
				&gerritpb.NotifyDetails{
					Recipients: []*gerritpb.NotifyDetails_Recipient{
						{
							RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
							Info: &gerritpb.NotifyDetails_Info{
								Accounts: []int64{100},
							},
						},
					},
				})
		})

		Convey("Remove multiple votes", func() {
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.CQ(1, clock.Now(ctx).Add(-130*time.Second), gf.U("user-101"))(c.Info)
				gf.CQ(2, clock.Now(ctx).Add(-110*time.Second), gf.U("user-102"))(c.Info)
				gf.CQ(1, clock.Now(ctx).Add(-100*time.Second), gf.U("user-103"))(c.Info)
			})
			// Voting status:
			//  now-130s: +1 from user-101
			//  now-120s: +2 from user-100
			//  now-110s: +2 from user-102
			//  now-100s: +1 from user-103
			expectedRemovingOrder := []int64{103, 102, 101, 100}

			err := Cancel(ctx, input)
			So(err, ShouldBeNil)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			So(resultCI.Info.GetMessages(), ShouldHaveLength, 1)
			So(resultCI.Info.GetMessages()[0].GetMessage(), ShouldEqual, input.Message)
			So(gf.NonZeroVotes(resultCI.Info, trigger.CQLabelName), ShouldBeEmpty)
			actualRemovingOrder := []int64{}
			for _, req := range ct.GFake.Requests() {
				if r, ok := req.(*gerritpb.SetReviewRequest); ok {
					actualRemovingOrder = append(actualRemovingOrder, r.OnBehalfOf)
					if r.OnBehalfOf != user.GetAccountId() {
						So(r.GetNotify(), ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
						So(r.GetNotifyDetails(), ShouldBeNil)
					} else {
						So(r.GetNotify(), ShouldEqual, gerritpb.Notify_NOTIFY_OWNER)
						So(r.GetNotifyDetails(), ShouldResembleProto,
							&gerritpb.NotifyDetails{
								Recipients: []*gerritpb.NotifyDetails_Recipient{
									{
										RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
										Info: &gerritpb.NotifyDetails_Info{
											Accounts: []int64{100, 101, 102, 103},
										},
									},
								},
							})
					}
				}
			}
			So(actualRemovingOrder, ShouldResemble, expectedRemovingOrder)
		})

		Convey("Skips zero votes", func() {
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.CQ(0, clock.Now(ctx).Add(-90*time.Second), gf.U("user-101"))(c.Info)
				gf.CQ(0, clock.Now(ctx).Add(-100*time.Second), gf.U("user-102"))(c.Info)
				gf.CQ(0, clock.Now(ctx).Add(-110*time.Second), gf.U("user-103"))(c.Info)
			})

			err := Cancel(ctx, input)
			So(err, ShouldBeNil)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			So(resultCI.Info.GetMessages(), ShouldHaveLength, 1)
			So(resultCI.Info.GetMessages()[0].GetMessage(), ShouldEqual, input.Message)
			So(gf.NonZeroVotes(resultCI.Info, trigger.CQLabelName), ShouldBeEmpty)
			count := 0
			for _, req := range ct.GFake.Requests() {
				if r, ok := req.(*gerritpb.SetReviewRequest); ok {
					So(r.OnBehalfOf, ShouldEqual, user.GetAccountId())
					count++
				}
			}
			if count != 1 {
				So(fmt.Sprintf("expected exactly one request to remove vote on behalf of user-%d; got %d", user.GetAccountId(), count), ShouldBeEmpty)
			}
		})

		Convey("Post Message even if triggering votes has been removed already", func() {
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.CQ(0, clock.Now(ctx), user)(c.Info)
			})
			err := Cancel(ctx, input)
			So(err, ShouldBeNil)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			So(resultCI.Info.GetMessages(), ShouldHaveLength, 1)
			So(resultCI.Info.GetMessages()[0].GetMessage(), ShouldEqual, input.Message)
		})

		Convey("Post Message if CV has no permission to vote", func() {
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				c.ACLs = gf.ACLGrant(gf.OpRead, codes.PermissionDenied, lProject).Or(
					// Needed to post comments
					gf.ACLGrant(gf.OpReview, codes.PermissionDenied, lProject),
				)
			})
			err := Cancel(ctx, input)
			So(err, ShouldErrLike, "no permission to remove vote x-review.example.com/10001")
			So(ErrPermanentTag.In(err), ShouldBeTrue)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			So(gf.NonZeroVotes(resultCI.Info, trigger.CQLabelName), ShouldResembleProto, []*gerritpb.ApprovalInfo{
				{
					User:  user,
					Value: 2,
					Date:  timestamppb.New(triggerTime),
				},
			})
			So(resultCI.Info.GetMessages(), ShouldHaveLength, 1)
			expectedMsg := input.Message + `

CV failed to unset the Commit-Queue label on your behalf. Please unvote and revote on the Commit-Queue label to retry.

Bot data: {"action":"cancel","triggered_at":"2020-02-02T10:28:00Z","revision":"rev-010001-002","cls":["x-review.example.com:10002","x-review.example.com:10003"]}`
			So(resultCI.Info.GetMessages()[0].GetMessage(), ShouldEqual, expectedMsg)
		})

		Convey("Post Message also fails", func() {
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				c.ACLs = gf.ACLGrant(gf.OpRead, codes.PermissionDenied, lProject)
			})
			err := Cancel(ctx, input)
			So(err, ShouldErrLike, "no permission to remove vote x-review.example.com/10001")
			So(ErrPermanentTag.In(err), ShouldBeTrue)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			So(gf.NonZeroVotes(resultCI.Info, trigger.CQLabelName), ShouldResembleProto, []*gerritpb.ApprovalInfo{
				{
					User:  user,
					Value: 2,
					Date:  timestamppb.New(triggerTime),
				},
			})
			So(resultCI.Info.GetMessages(), ShouldBeEmpty)
		})
	})
}
