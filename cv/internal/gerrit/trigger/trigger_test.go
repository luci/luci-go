// Copyright 2020 The LUCI Authors.
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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smarty/assertions"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFindCQTrigger(t *testing.T) {
	t.Parallel()

	user1 := gf.U("u-1")
	user2 := gf.U("u-2")
	user3 := gf.U("u-3")
	user4 := gf.U("u-4")
	const dryRunVote = 1
	const fullRunVote = 2
	cg := &cfgpb.ConfigGroup{}
	Convey("findCQTrigger", t, func() {
		now := testclock.TestRecentTimeUTC
		ci := &gerritpb.ChangeInfo{
			Status:          gerritpb.ChangeStatus_NEW,
			CurrentRevision: "deadbeef~1",
			Revisions: map[string]*gerritpb.RevisionInfo{
				"deadbeef~1": {
					Number:  2,
					Created: timestamppb.New(now.Add(-30 * time.Minute)),
				},
				"deadbeef~2": {
					Number:  1,
					Created: timestamppb.New(now.Add(-1 * time.Hour)),
				},
			},
			Labels: map[string]*gerritpb.LabelInfo{
				CQLabelName: {
					All: nil, // set in tests.
				},
			},
		}

		Convey("Abandoned CL", func() {
			ci.Status = gerritpb.ChangeStatus_ABANDONED
			So(findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), ShouldBeNil)
		})
		Convey("Merged CL", func() {
			ci.Status = gerritpb.ChangeStatus_MERGED
			So(findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), ShouldBeNil)
		})
		Convey("No votes", func() {
			So(findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), ShouldBeNil)
		})
		Convey("No Commit-Queue label info", func() {
			ci.Labels = nil
			So(findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), ShouldBeNil)
		})
		Convey("Single vote", func() {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{{
				User:  user1,
				Value: dryRunVote,
				Date:  timestamppb.New(now.Add(-15 * time.Minute)),
			}}
			t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			So(t, ShouldResembleProto, &run.Trigger{
				Time:            timestamppb.New(now.Add(-15 * time.Minute)),
				Mode:            string(run.DryRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			})
		})
		Convey(">CQ+2 is clamped to CQ+2", func() {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{{
				User:  user1,
				Value: 3,
				Date:  timestamppb.New(now.Add(-15 * time.Minute)),
			}}
			t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			So(t, ShouldResembleProto, &run.Trigger{
				Time:            timestamppb.New(now.Add(-15 * time.Minute)),
				Mode:            string(run.FullRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			})
		})
		Convey("Earliest votes wins", func() {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{
				{
					User:  user1,
					Value: dryRunVote,
					Date:  timestamppb.New(now.Add(-15 * time.Minute)),
				},
				{
					User:  user2,
					Value: dryRunVote,
					Date:  timestamppb.New(now.Add(-5 * time.Minute)),
				},
			}
			t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			So(t, ShouldResembleProto, &run.Trigger{
				Time:            timestamppb.New(now.Add(-15 * time.Minute)),
				Mode:            string(run.DryRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			})
			Convey("except when some later run was canceled via botdata message", func() {
				cancelMsg, err := botdata.Append("", botdata.BotData{
					Action:      botdata.Cancel,
					TriggeredAt: now.Add(-15 * time.Minute),
					Revision:    ci.GetCurrentRevision(),
				})
				So(err, ShouldBeNil)
				ci.Messages = append(ci.Messages, &gerritpb.ChangeMessageInfo{Message: cancelMsg})
				t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
				So(t, ShouldResembleProto, &run.Trigger{
					Time:            timestamppb.New(now.Add(-5 * time.Minute)),
					Mode:            string(run.DryRun),
					GerritAccountId: user2.GetAccountId(),
					Email:           user2.GetEmail(),
				})
			})
		})
		Convey("Earliest CQ+2 vote wins against even earlier CQ+1", func() {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{
				{
					User:  user1,
					Value: dryRunVote,
					Date:  timestamppb.New(now.Add(-15 * time.Minute)),
				},
				{
					User:  user2,
					Value: fullRunVote,
					Date:  timestamppb.New(now.Add(-10 * time.Minute)),
				},
				{
					User:  user3,
					Value: dryRunVote,
					Date:  timestamppb.New(now.Add(-5 * time.Minute)),
				},
				{
					User:  user4,
					Value: fullRunVote,
					Date:  timestamppb.New(now.Add(-1 * time.Minute)),
				},
			}
			t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			So(t, ShouldResembleProto, &run.Trigger{
				Time:            timestamppb.New(now.Add(-10 * time.Minute)),
				Mode:            string(run.FullRun),
				GerritAccountId: user2.GetAccountId(),
				Email:           user2.GetEmail(),
			})
		})
		Convey("Sticky Vote bumps trigger time to cur revision creation time", func() {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{{
				User:  user1,
				Value: fullRunVote,
				Date:  timestamppb.New(now.Add(-15 * time.Minute)),
			}}
			ci.CurrentRevision = "deadbeef"
			ci.Revisions["deadbeef"] = &gerritpb.RevisionInfo{
				Number:  3,
				Created: timestamppb.New(now.Add(-10 * time.Minute)),
			}
			t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			So(t, ShouldResembleProto, &run.Trigger{
				Time:            timestamppb.New(now.Add(-10 * time.Minute)),
				Mode:            string(run.FullRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			})
		})
		Convey("Additional modes", func() {
			const quickLabel = "Quick"
			cg.AdditionalModes = []*cfgpb.Mode{
				{
					Name:            string(run.QuickDryRun),
					CqLabelValue:    +1,
					TriggeringLabel: quickLabel,
					TriggeringValue: +1,
				},
			}
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{
				{
					User:  user1,
					Value: dryRunVote,
					Date:  timestamppb.New(now.Add(-15 * time.Minute)),
				},
			}
			ci.Labels[quickLabel] = &gerritpb.LabelInfo{All: []*gerritpb.ApprovalInfo{
				{
					User:  user1,
					Value: +1,
					Date:  timestamppb.New(now.Add(-15 * time.Minute)),
				},
			}}
			Convey("Simplest possible QuickDryRun", func() {
				t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
				So(t, ShouldResembleProto, &run.Trigger{
					Time:            timestamppb.New(now.Add(-15 * time.Minute)),
					Mode:            string(run.QuickDryRun),
					GerritAccountId: user1.GetAccountId(),
					Email:           user1.GetEmail(),
					AdditionalLabel: quickLabel,
				})
			})
			Convey("QuickDryRun despite other users' votes", func() {
				ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{
					{
						User:  user1,
						Value: dryRunVote,
						Date:  timestamppb.New(now.Add(-15 * time.Minute)),
					},
					{
						User:  user2,
						Value: dryRunVote,
						Date:  timestamppb.New(now.Add(-10 * time.Minute)),
					},
				}
				ci.Labels[quickLabel] = &gerritpb.LabelInfo{All: []*gerritpb.ApprovalInfo{
					{
						User:  user2,
						Value: +2,
						Date:  timestamppb.New(now.Add(-20 * time.Minute)),
					},
					{
						User:  user1,
						Value: +1,
						Date:  timestamppb.New(now.Add(-15 * time.Minute)),
					},
				}}
				t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
				So(t.GetMode(), ShouldEqual, run.QuickDryRun)
			})
			Convey("Custom mode", func() {
				cg.AdditionalModes[0].Name = "CUSTOM_RUN"
				t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
				So(t, ShouldResembleProto, &run.Trigger{
					Time:            timestamppb.New(now.Add(-15 * time.Minute)),
					Mode:            "CUSTOM_RUN",
					GerritAccountId: user1.GetAccountId(),
					Email:           user1.GetEmail(),
					AdditionalLabel: quickLabel,
				})
			})
			Convey("Not applicable cases", func() {
				Convey("Additional vote must have the same timestamp", func() {
					Convey("before", func() {
						ci.Labels[quickLabel].GetAll()[0].Date.Seconds++
						t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
						So(t.GetMode(), ShouldEqual, run.DryRun)
					})
					Convey("after", func() {
						ci.Labels[quickLabel].GetAll()[0].Date.Seconds--
						t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
						So(t.GetMode(), ShouldEqual, run.DryRun)
					})
				})
				Convey("Additional vote be from the same account", func() {
					ci.Labels[quickLabel].GetAll()[0].User = user2
					t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					So(t.GetMode(), ShouldEqual, run.DryRun)
				})
				Convey("Additional vote must have expected value", func() {
					ci.Labels[quickLabel].GetAll()[0].Value = 100
					t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					So(t.GetMode(), ShouldEqual, run.DryRun)
				})
				Convey("Additional vote must be for the correct label", func() {
					ci.Labels[quickLabel+"-Other"] = ci.Labels[quickLabel]
					delete(ci.Labels, quickLabel)
					t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					So(t.GetMode(), ShouldEqual, run.DryRun)
				})
				Convey("CQ vote must have correct value", func() {
					ci.Labels[CQLabelName].GetAll()[0].Value = fullRunVote
					t := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					So(t.GetMode(), ShouldEqual, run.FullRun)
				})
			})
		})
	})
}

func TestFindNewPatchsetRunTrigger(t *testing.T) {
	t.Parallel()
	owner := gf.U("owner")
	uploader1 := gf.U("ul-1")
	uploader2 := gf.U("ul-2")
	ts1 := testclock.TestRecentTimeUTC
	ts2 := ts1.Add(30 * time.Minute)
	// Create change info.
	ci := &gerritpb.ChangeInfo{
		Owner:           owner,
		Status:          gerritpb.ChangeStatus_NEW,
		CurrentRevision: "deadbeef~2",
		Revisions: map[string]*gerritpb.RevisionInfo{
			"deadbeef~1": {
				Number:   1,
				Uploader: uploader1,
				Created:  timestamppb.New(ts1),
			},
			"deadbeef~2": {
				Number:   2,
				Uploader: uploader2,
				Created:  timestamppb.New(ts2),
			},
		},
	}

	// Create empty config.
	cg := &cfgpb.ConfigGroup{}

	// Create CLEntity.
	cle := &changelist.CL{
		TriggerNewPatchsetRunAfterPS: 1,
	}

	Convey("findNewPatchsetRun", t, func() {
		Convey("Not configured to trigger new patchset runs", func() {
			So(
				findNewPatchsetRunTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: cle.TriggerNewPatchsetRunAfterPS}),
				ShouldBeNil,
			)
		})
		Convey("Configured", func() {
			// Add npr builder to config.
			cg.Verifiers = &cfgpb.Verifiers{
				Tryjob: &cfgpb.Verifiers_Tryjob{
					Builders: []*cfgpb.Verifiers_Tryjob_Builder{
						{
							Name:          "builder_name",
							ModeAllowlist: []string{string(run.NewPatchsetRun)},
						},
					},
				},
			}
			Convey("Current patchset not ended", func() {
				So(
					findNewPatchsetRunTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: cle.TriggerNewPatchsetRunAfterPS}),
					ShouldResembleProto,
					&run.Trigger{
						Mode:  string(run.NewPatchsetRun),
						Time:  timestamppb.New(ts2),
						Email: owner.Email,
					},
				)
			})
			Convey("Current patchset already ended", func() {
				// Set CLEntity NewPatchsetUploaded
				cle.TriggerNewPatchsetRunAfterPS = 2
				So(
					findNewPatchsetRunTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: cle.TriggerNewPatchsetRunAfterPS}),
					ShouldBeNil,
				)
			})
		})
	})
}
