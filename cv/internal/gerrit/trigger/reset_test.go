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

package trigger

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/usertext"

	// Convey package also exports `Reset` method that conflicts with the function
	// in this package. This is a workaround since we can not dot import the convey
	// package.
	//
	// WARNING: importing the below packages as "." will make So() and
	// assertions, like ShouldErrLike, silently ignore the assertion results.
	// e.g., So(1, ShouldBeNil) will pass.
	c "github.com/smartystreets/goconvey/convey"
	la "go.chromium.org/luci/common/testing/assertions"
)

func TestReset(t *testing.T) {
	t.Parallel()

	c.Convey("Reset", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const ownerID int64 = 5
		const reviewerID int64 = 50
		const triggererID int64 = 100
		triggerer := gf.U(fmt.Sprintf("user-%d", triggererID))
		const gHost = "x-review.example.com"
		const lProject = "lProject"
		const changeNum = 10001
		triggerTime := ct.Clock.Now().Add(-2 * time.Minute)
		ci := gf.CI(
			10001, gf.PS(2),
			gf.Owner(fmt.Sprintf("user-%d", ownerID)),
			gf.CQ(2, triggerTime, triggerer),
			gf.Updated(clock.Now(ctx).Add(-1*time.Minute)),
			gf.Reviewer(gf.U(fmt.Sprintf("user-%d", reviewerID))),
		)
		triggers := Find(&FindInput{ChangeInfo: ci, ConfigGroup: &cfgpb.ConfigGroup{}})
		c.So(triggers.GetCqVoteTrigger(), la.ShouldResembleProto, &run.Trigger{
			Time:            timestamppb.New(triggerTime),
			Mode:            string(run.FullRun),
			Email:           fmt.Sprintf("user-%d@example.com", triggererID),
			GerritAccountId: triggererID,
		})
		c.So(triggers.GetCqVoteTrigger().GerritAccountId, c.ShouldEqual, 100)
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
			TriggerNewPatchsetRunAfterPS: 1,
		}
		c.So(datastore.Put(ctx, cl), c.ShouldBeNil)
		ct.GFake.CreateChange(&gf.Change{
			Host: gHost,
			Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
			ACLs: gf.ACLGrant(gf.OpRead, codes.PermissionDenied, lProject).Or(
				gf.ACLGrant(gf.OpReview, codes.PermissionDenied, lProject),
				gf.ACLGrant(gf.OpAlterVotesOfOthers, codes.PermissionDenied, lProject),
			),
		})

		input := ResetInput{
			CL: cl,
			ConfigGroups: []*prjcfg.ConfigGroup{{
				Content: &cfgpb.ConfigGroup{
					Verifiers: &cfgpb.Verifiers{Tryjob: &cfgpb.Verifiers_Tryjob{
						Builders: []*cfgpb.Verifiers_Tryjob_Builder{{
							Name:          "new patchset upload builder",
							ModeAllowlist: []string{string(run.NewPatchsetRun)},
						}},
					}},
				},
			}},
			LUCIProject:       lProject,
			Message:           "Full Run has passed",
			Requester:         "test",
			Notify:            gerrit.Whoms{gerrit.Whom_OWNER, gerrit.Whom_CQ_VOTERS},
			AddToAttentionSet: gerrit.Whoms{gerrit.Whom_REVIEWERS},
			AttentionReason:   usertext.StoppedRun,
			LeaseDuration:     30 * time.Second,
			CLMutator:         changelist.NewMutator(ct.TQDispatcher, nil, nil, nil),
			GFactory:          ct.GFactory(),
		}
		findTriggers := func(resultCI *gerritpb.ChangeInfo) *run.Triggers {
			for _, cg := range input.ConfigGroups {
				if ts := Find(&FindInput{ChangeInfo: resultCI, ConfigGroup: cg.Content}); ts != nil {
					return ts
				}
			}
			return nil
		}
		ts := findTriggers(ci)
		cqTrigger := ts.GetCqVoteTrigger()
		nprTrigger := ts.GetNewPatchsetRunTrigger()
		input.Triggers = &run.Triggers{}

		c.Convey("Fails PreCondition if CL is AccessDenied from code review site", func() {
			c.Convey("For CQ-Label trigger", func() {
				input.Triggers.CqVoteTrigger = cqTrigger
			})
			c.Convey("For NewPatchset trigger", func() {
				input.Triggers.NewPatchsetRunTrigger = nprTrigger
			})
			noAccessTime := ct.Clock.Now().UTC().Add(1 * time.Minute)
			cl.Access = &changelist.Access{
				ByProject: map[string]*changelist.Access_Project{
					lProject: {
						UpdateTime:   timestamppb.New(noAccessTime),
						NoAccessTime: timestamppb.New(noAccessTime),
					},
				},
			}
			err := Reset(ctx, input)
			c.So(err, la.ShouldErrLike, "failed to reset trigger because CV lost access to this CL")
			c.So(ErrResetPreconditionFailedTag.In(err), c.ShouldBeTrue)
		})
		isOutdated := func(cl *changelist.CL) bool {
			e := &changelist.CL{ID: cl.ID}
			c.So(datastore.Get(ctx, e), c.ShouldBeNil)
			return e.Snapshot.GetOutdated() != nil
		}

		c.Convey("Fails PreCondition if CL has newer PS in datastore", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
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
			c.So(datastore.Put(ctx, newCL), c.ShouldBeNil)
			err := Reset(ctx, input)
			c.So(err, la.ShouldErrLike, "failed to reset because ps 2 is not current for cl(99999)")
			c.So(ErrResetPreconditionFailedTag.In(err), c.ShouldBeTrue)
			c.So(isOutdated(cl), c.ShouldBeFalse)
		})

		c.Convey("Fails PreCondition if CL has newer PS in Gerrit", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.PS(3)(c.Info)
			})
			err := Reset(ctx, input)
			c.So(err, la.ShouldErrLike, "failed to reset because ps 2 is not current for x-review.example.com/10001")
			c.So(ErrResetPreconditionFailedTag.In(err), c.ShouldBeTrue)
			c.So(isOutdated(cl), c.ShouldBeFalse)
		})

		c.Convey("Cancelling CQ Vote fails if receive stale data from gerrit", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.Updated(clock.Now(ctx).Add(-3 * time.Minute))(c.Info)
			})
			err := Reset(ctx, input)
			c.So(err, la.ShouldErrLike, gerrit.ErrStaleData)
			c.So(transient.Tag.In(err), c.ShouldBeTrue)
			c.So(isOutdated(cl), c.ShouldBeFalse)
		})

		c.Convey("Cancelling NewPatchsetRun", func() {
			input.Triggers.NewPatchsetRunTrigger = nprTrigger
			input.Message = "reset new patchset run trigger"

			cl := &changelist.CL{ID: input.CL.ID}
			c.So(datastore.Get(ctx, cl), c.ShouldBeNil)
			originalValue := cl.TriggerNewPatchsetRunAfterPS

			c.So(Reset(ctx, input), c.ShouldBeNil)
			// cancelling a new patchset run doesn't mark the snapshot
			// as outdated.
			c.So(isOutdated(cl), c.ShouldBeFalse)

			cl = &changelist.CL{ID: input.CL.ID}
			c.So(datastore.Get(ctx, cl), c.ShouldBeNil)
			c.So(cl.TriggerNewPatchsetRunAfterPS, c.ShouldNotEqual, originalValue)
			c.So(cl.TriggerNewPatchsetRunAfterPS, c.ShouldEqual, input.CL.Snapshot.Patchset)
			change := ct.GFake.GetChange(input.CL.Snapshot.GetGerrit().GetHost(), int(input.CL.Snapshot.GetGerrit().GetInfo().GetNumber()))
			c.So(change.Info.GetMessages()[len(change.Info.GetMessages())-1].Message, c.ShouldEqual, input.Message)
		})

		splitSetReviewRequests := func() (onBehalf, asSelf []*gerritpb.SetReviewRequest) {
			for _, req := range ct.GFake.Requests() {
				switch r, ok := req.(*gerritpb.SetReviewRequest); {
				case !ok:
				case r.GetOnBehalfOf() != 0:
					// OnBehalfOf removes votes and must happen before any asSelf.
					c.So(asSelf, c.ShouldBeEmpty)
					onBehalf = append(onBehalf, r)
				default:
					asSelf = append(asSelf, r)
				}
			}
			return onBehalf, asSelf
		}
		c.Convey("cancel new patchset run and cq vote run at the same time", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			input.Triggers.NewPatchsetRunTrigger = nprTrigger
			cl := &changelist.CL{ID: input.CL.ID}
			c.So(datastore.Get(ctx, cl), c.ShouldBeNil)
			originalValue := cl.TriggerNewPatchsetRunAfterPS

			err := Reset(ctx, input)
			c.So(err, c.ShouldBeNil)
			c.So(isOutdated(cl), c.ShouldBeTrue) // snapshot is outdated
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			c.So(resultCI.Info.GetMessages(), c.ShouldHaveLength, 1)
			c.So(resultCI.Info.GetMessages()[0].GetMessage(), c.ShouldEqual, input.Message)
			c.So(gf.NonZeroVotes(resultCI.Info, CQLabelName), c.ShouldBeEmpty)

			onBehalfs, asSelf := splitSetReviewRequests()
			c.So(onBehalfs, c.ShouldHaveLength, 1)
			c.So(onBehalfs[0].GetOnBehalfOf(), c.ShouldEqual, triggererID)
			c.So(onBehalfs[0].GetNotifyDetails(), c.ShouldBeNil)
			c.So(asSelf, c.ShouldHaveLength, 1)
			c.So(asSelf[0].GetNotify(), c.ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
			c.So(asSelf[0].GetNotifyDetails(), la.ShouldResembleProto,
				&gerritpb.NotifyDetails{
					Recipients: []*gerritpb.NotifyDetails_Recipient{
						{
							RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
							Info: &gerritpb.NotifyDetails_Info{
								Accounts: []int64{ownerID, triggererID},
							},
						},
					},
				})
			c.So(asSelf[0].GetAddToAttentionSet(), la.ShouldResembleProto, []*gerritpb.AttentionSetInput{
				{User: strconv.FormatInt(reviewerID, 10), Reason: "ps#2: " + usertext.StoppedRun},
			})
			cl = &changelist.CL{ID: input.CL.ID}
			c.So(datastore.Get(ctx, cl), c.ShouldBeNil)
			c.So(cl.TriggerNewPatchsetRunAfterPS, c.ShouldNotEqual, originalValue)
			c.So(cl.TriggerNewPatchsetRunAfterPS, c.ShouldEqual, input.CL.Snapshot.Patchset)
		})
		c.Convey("Remove single vote", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			err := Reset(ctx, input)
			c.So(err, c.ShouldBeNil)
			c.So(isOutdated(cl), c.ShouldBeTrue) // snapshot is outdated
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			c.So(resultCI.Info.GetMessages(), c.ShouldHaveLength, 1)
			c.So(resultCI.Info.GetMessages()[0].GetMessage(), c.ShouldEqual, input.Message)
			c.So(gf.NonZeroVotes(resultCI.Info, CQLabelName), c.ShouldBeEmpty)

			onBehalfs, asSelf := splitSetReviewRequests()
			c.So(onBehalfs, c.ShouldHaveLength, 1)
			c.So(onBehalfs[0].GetOnBehalfOf(), c.ShouldEqual, triggererID)
			c.So(onBehalfs[0].GetNotifyDetails(), c.ShouldBeNil)
			c.So(asSelf, c.ShouldHaveLength, 1)
			c.So(asSelf[0].GetNotify(), c.ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
			c.So(asSelf[0].GetNotifyDetails(), la.ShouldResembleProto,
				&gerritpb.NotifyDetails{
					Recipients: []*gerritpb.NotifyDetails_Recipient{
						{
							RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
							Info: &gerritpb.NotifyDetails_Info{
								Accounts: []int64{ownerID, triggererID},
							},
						},
					},
				})
			c.So(asSelf[0].GetAddToAttentionSet(), la.ShouldResembleProto, []*gerritpb.AttentionSetInput{
				{User: strconv.FormatInt(reviewerID, 10), Reason: "ps#2: " + usertext.StoppedRun},
			})
			c.So(asSelf[0].GetTag(), c.ShouldResemble, fmt.Sprintf("autogenerated:cq:full-run:%d", triggerTime.Unix()))
		})

		c.Convey("Remove multiple votes", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.CQ(1, clock.Now(ctx).Add(-130*time.Second), gf.U("user-1"))(c.Info)
				gf.CQ(2, clock.Now(ctx).Add(-110*time.Second), gf.U("user-70"))(c.Info)
				gf.CQ(1, clock.Now(ctx).Add(-100*time.Second), gf.U("user-1000"))(c.Info)
			})

			c.Convey("Success", func() {
				err := Reset(ctx, input)
				c.So(err, c.ShouldBeNil)
				c.So(isOutdated(cl), c.ShouldBeTrue) // snapshot is outdated
				resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
				c.So(resultCI.Info.GetMessages(), c.ShouldHaveLength, 1)
				c.So(resultCI.Info.GetMessages()[0].GetMessage(), c.ShouldEqual, input.Message)
				c.So(gf.NonZeroVotes(resultCI.Info, CQLabelName), c.ShouldBeEmpty)

				onBehalfs, asSelf := splitSetReviewRequests()
				for _, r := range onBehalfs {
					c.So(r.GetNotify(), c.ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
					c.So(r.GetNotifyDetails(), c.ShouldBeNil)
				}
				// The triggering vote(s) must have been removed last, the order of
				// removals for the rest doesn't matter so long as it does the job.
				c.So(onBehalfs[len(onBehalfs)-1].GetOnBehalfOf(), c.ShouldEqual, 100)
				c.So(asSelf, c.ShouldHaveLength, 1)
				c.So(asSelf[0].GetNotify(), c.ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
				c.So(asSelf[0].GetNotifyDetails(), la.ShouldResembleProto,
					&gerritpb.NotifyDetails{
						Recipients: []*gerritpb.NotifyDetails_Recipient{
							{
								RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
								Info: &gerritpb.NotifyDetails_Info{
									Accounts: []int64{1, ownerID, 70, triggererID, 1000},
								},
							},
						},
					})
				c.So(asSelf[0].GetAddToAttentionSet(), la.ShouldResembleProto, []*gerritpb.AttentionSetInput{
					{User: strconv.FormatInt(reviewerID, 10), Reason: "ps#2: " + usertext.StoppedRun},
				})
			})

			c.Convey("Removing non-triggering votes fails", func() {
				ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
					c.ACLs = gf.ACLGrant(gf.OpRead, codes.PermissionDenied, lProject).Or(
						gf.ACLGrant(gf.OpReview, codes.PermissionDenied, lProject),
					) // no permission to vote on behalf of others
				})
				err := Reset(ctx, input)
				c.So(err, c.ShouldBeNil)
				c.So(isOutdated(cl), c.ShouldBeFalse)
				onBehalfs, _ := splitSetReviewRequests()
				c.So(onBehalfs, c.ShouldHaveLength, 3) // all non-triggering votes
				for _, r := range onBehalfs {
					switch r.GetOnBehalfOf() {
					case triggererID:
						// CV shouldn't remove triggering votes if removal of non-triggering
						// votes fails.
						c.So(r.GetOnBehalfOf(), c.ShouldNotEqual, triggererID)
					case 1, 70, 1000:
					default:
						panic(fmt.Errorf("unknown on_behalf_of %d", r.GetOnBehalfOf()))
					}
				}
			})
		})

		c.Convey("Removing votes from non-CQ labels used in additional modes", func() {
			const uLabel = "Ultra-Quick-Label"
			const qLabel = "Quick-Label"
			input.Triggers.CqVoteTrigger = cqTrigger
			input.ConfigGroups = []*prjcfg.ConfigGroup{
				{
					Content: &cfgpb.ConfigGroup{
						AdditionalModes: []*cfgpb.Mode{
							{
								Name:            "ULTRA_QUICK_RUN",
								CqLabelValue:    1,
								TriggeringLabel: uLabel,
								TriggeringValue: 1,
							},
							{
								Name:            "QUICK_RUN",
								CqLabelValue:    1,
								TriggeringLabel: qLabel,
								TriggeringValue: 1,
							},
						},
					},
				},
			}

			ultraQuick := func(value int, timeAndUser ...any) gf.CIModifier {
				return gf.Vote(uLabel, value, timeAndUser...)
			}
			quick := func(value int, timeAndUser ...any) gf.CIModifier {
				return gf.Vote(qLabel, value, timeAndUser...)
			}
			// Exact timestamps don't matter in this test, but in practice they affect
			// computation of the triggering vote.
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				// user-99 forgot to vote CQ+1.
				quick(1, clock.Now(ctx).Add(-300*time.Second), gf.U("user-99"))(c.Info)
				ultraQuick(1, clock.Now(ctx).Add(-200*time.Second), gf.U("user-99"))(c.Info)

				// user-100 actually triggered an ULTRA_QUICK_RUN.
				gf.CQ(1, clock.Now(ctx).Add(-150*time.Second), gf.U("user-100"))(c.Info)
				ultraQuick(1, clock.Now(ctx).Add(-150*time.Second), gf.U("user-100"))(c.Info)
				quick(1, clock.Now(ctx).Add(-150*time.Second), gf.U("user-100"))(c.Info)

				// user-101 CQ+1 was a noop.
				gf.CQ(1, clock.Now(ctx).Add(-120*time.Second), gf.U("user-101"))(c.Info)

				// user-102 votes for a QUICK_RUN is a noop, but should be removed as
				// as well.
				gf.CQ(1, clock.Now(ctx).Add(-110*time.Second), gf.U("user-101"))(c.Info)
				quick(1, clock.Now(ctx).Add(-110*time.Second), gf.U("user-102"))(c.Info)

				// user-103 votes is a noop, though weird, yet still must be removed.
				ultraQuick(3, clock.Now(ctx).Add(-100*time.Second), gf.U("user-104"))(c.Info)

				// user-104 votes is 0, and doesn't need a reset.
				ultraQuick(0, clock.Now(ctx).Add(-90*time.Second), gf.U("user-104"))(c.Info)
			})
			err := Reset(ctx, input)
			c.So(err, c.ShouldBeNil)
			c.So(isOutdated(cl), c.ShouldBeTrue) // snapshot is outdated

			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			c.So(gf.NonZeroVotes(resultCI.Info, CQLabelName), c.ShouldBeEmpty)
			c.So(gf.NonZeroVotes(resultCI.Info, qLabel), c.ShouldBeEmpty)
			c.So(gf.NonZeroVotes(resultCI.Info, uLabel), c.ShouldBeEmpty)

			onBehalfs, _ := splitSetReviewRequests()
			// The last request must be for account 100.
			c.So(onBehalfs[len(onBehalfs)-1].GetOnBehalfOf(), c.ShouldEqual, 100)
			c.So(onBehalfs[len(onBehalfs)-1].GetLabels(), c.ShouldResemble, map[string]int32{
				CQLabelName: 0,
				qLabel:      0,
				uLabel:      0,
			})
		})

		c.Convey("Skips zero votes", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.CQ(0, clock.Now(ctx).Add(-90*time.Second), gf.U("user-101"))(c.Info)
				gf.CQ(0, clock.Now(ctx).Add(-100*time.Second), gf.U("user-102"))(c.Info)
				gf.CQ(0, clock.Now(ctx).Add(-110*time.Second), gf.U("user-103"))(c.Info)
			})

			err := Reset(ctx, input)
			c.So(err, c.ShouldBeNil)
			c.So(isOutdated(cl), c.ShouldBeTrue) // snapshot is outdated
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			c.So(resultCI.Info.GetMessages(), c.ShouldHaveLength, 1)
			c.So(resultCI.Info.GetMessages()[0].GetMessage(), c.ShouldEqual, input.Message)
			c.So(gf.NonZeroVotes(resultCI.Info, CQLabelName), c.ShouldBeEmpty)
			onBehalfs, _ := splitSetReviewRequests()
			c.So(onBehalfs, c.ShouldHaveLength, 1)
			c.So(onBehalfs[0].GetOnBehalfOf(), c.ShouldEqual, triggererID)
		})

		c.Convey("Post Message even if triggering votes has been removed already", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.CQ(0, clock.Now(ctx), triggerer)(c.Info)
			})
			err := Reset(ctx, input)
			c.So(err, c.ShouldBeNil)
			c.So(isOutdated(cl), c.ShouldBeTrue) // snapshot is outdated
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber()))
			c.So(resultCI.Info.GetMessages(), c.ShouldHaveLength, 1)
			c.So(resultCI.Info.GetMessages()[0].GetMessage(), c.ShouldEqual, input.Message)
		})

		c.Convey("Post Message if CV has no permission to vote", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				c.ACLs = gf.ACLGrant(gf.OpRead, codes.PermissionDenied, lProject).Or(
					// Needed to post comments
					gf.ACLGrant(gf.OpReview, codes.PermissionDenied, lProject),
				)
			})
			c.So(Reset(ctx, input), c.ShouldBeNil)
			c.So(isOutdated(cl), c.ShouldBeFalse)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info
			// CQ+2 vote remains.
			c.So(gf.NonZeroVotes(resultCI, CQLabelName), la.ShouldResembleProto, []*gerritpb.ApprovalInfo{
				{
					User:  triggerer,
					Value: 2,
					Date:  timestamppb.New(triggerTime),
				},
			})
			// But CL is no longer triggered.
			c.So(findTriggers(resultCI).GetCqVoteTrigger(), c.ShouldBeNil)
			// Still, user should know what happened.
			expectedMsg := input.Message + `

CV failed to unset the Commit-Queue label on your behalf. Please unvote and revote on the Commit-Queue label to retry.

Bot data: {"action":"cancel","triggered_at":"2020-02-02T10:28:00Z","revision":"rev-010001-002"}`
			c.So(resultCI.GetMessages()[0].GetMessage(), c.ShouldEqual, expectedMsg)
		})

		c.Convey("Post Message if change is in bad state", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				gf.Status(gerritpb.ChangeStatus_ABANDONED)(c.Info)
				c.ACLs = func(op gf.Operation, _ string) *status.Status {
					if op == gf.OpAlterVotesOfOthers {
						return status.New(codes.FailedPrecondition, "change abandoned, no vote removals allowed")
					}
					return status.New(codes.OK, "")
				}
			})
			err := Reset(ctx, input)
			c.So(err, c.ShouldBeNil)
			c.So(isOutdated(cl), c.ShouldBeFalse)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info
			// CQ+2 vote remains.
			c.So(gf.NonZeroVotes(resultCI, CQLabelName), la.ShouldResembleProto, []*gerritpb.ApprovalInfo{
				{
					User:  triggerer,
					Value: 2,
					Date:  timestamppb.New(triggerTime),
				},
			})
			// But CL is no longer triggered.
			c.So(findTriggers(resultCI).GetCqVoteTrigger(), c.ShouldBeNil)
			// Still, user should know what happened.
			c.So(resultCI.GetMessages(), c.ShouldHaveLength, 1)
			c.So(resultCI.GetMessages()[0].GetMessage(), c.ShouldContainSubstring, "CV failed to unset the Commit-Queue label on your behalf")
		})

		c.Convey("Post Message also fails", func() {
			input.Triggers.CqVoteTrigger = cqTrigger
			ct.GFake.MutateChange(gHost, int(ci.GetNumber()), func(c *gf.Change) {
				c.ACLs = gf.ACLGrant(gf.OpRead, codes.PermissionDenied, lProject)
			})
			err := Reset(ctx, input)
			c.So(err, la.ShouldErrLike, "no permission to remove vote x-review.example.com/10001")
			c.So(isOutdated(cl), c.ShouldBeFalse)
			c.So(ErrResetPermanentTag.In(err), c.ShouldBeTrue)
			resultCI := ct.GFake.GetChange(gHost, int(ci.GetNumber())).Info
			c.So(gf.NonZeroVotes(resultCI, CQLabelName), la.ShouldResembleProto, []*gerritpb.ApprovalInfo{
				{
					User:  triggerer,
					Value: 2,
					Date:  timestamppb.New(triggerTime),
				},
			})
			c.So(resultCI.GetMessages(), c.ShouldBeEmpty)
		})
	})
}
