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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"
)

func TestHasAutoSubmit(t *testing.T) {
	t.Parallel()

	ftt.Run("HasAutoSubmit", t, func(t *ftt.Test) {
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
				AutoSubmitLabelName: {
					All: nil, // set in tests.
				},
			},
		}
		assert.Loosely(t, HasAutoSubmit(ci), should.BeFalse)
		ci.Labels[AutoSubmitLabelName].All = []*gerritpb.ApprovalInfo{{
			User:  gf.U("u-1"),
			Value: modeToVote[run.FullRun],
			Date:  timestamppb.New(now.Add(-15 * time.Minute)),
		}}
		assert.Loosely(t, HasAutoSubmit(ci), should.BeTrue)
	})
}

func TestFindCQTrigger(t *testing.T) {
	t.Parallel()

	user1 := gf.U("u-1")
	user2 := gf.U("u-2")
	user3 := gf.U("u-3")
	user4 := gf.U("u-4")
	const dryRunVote = 1
	const fullRunVote = 2
	cg := &cfgpb.ConfigGroup{}
	ftt.Run("findCQTrigger", t, func(t *ftt.Test) {
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

		t.Run("Abandoned CL", func(t *ftt.Test) {
			ci.Status = gerritpb.ChangeStatus_ABANDONED
			assert.Loosely(t, findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), should.BeNil)
		})
		t.Run("Merged CL", func(t *ftt.Test) {
			ci.Status = gerritpb.ChangeStatus_MERGED
			assert.Loosely(t, findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), should.BeNil)
		})
		t.Run("No votes", func(t *ftt.Test) {
			assert.Loosely(t, findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), should.BeNil)
		})
		t.Run("No Commit-Queue label info", func(t *ftt.Test) {
			ci.Labels = nil
			assert.Loosely(t, findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg}), should.BeNil)
		})
		t.Run("Single vote", func(t *ftt.Test) {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{{
				User:  user1,
				Value: dryRunVote,
				Date:  timestamppb.New(now.Add(-15 * time.Minute)),
			}}
			trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			assert.Loosely(t, trig, should.Match(&run.Trigger{
				Time:            timestamppb.New(now.Add(-15 * time.Minute)),
				Mode:            string(run.DryRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			}))
		})
		t.Run(">CQ+2 is clamped to CQ+2", func(t *ftt.Test) {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{{
				User:  user1,
				Value: 3,
				Date:  timestamppb.New(now.Add(-15 * time.Minute)),
			}}
			trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			assert.Loosely(t, trig, should.Match(&run.Trigger{
				Time:            timestamppb.New(now.Add(-15 * time.Minute)),
				Mode:            string(run.FullRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			}))
		})
		t.Run("Earliest votes wins", func(t *ftt.Test) {
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
			trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			assert.Loosely(t, trig, should.Match(&run.Trigger{
				Time:            timestamppb.New(now.Add(-15 * time.Minute)),
				Mode:            string(run.DryRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			}))
			t.Run("except when some later run was canceled via botdata message", func(t *ftt.Test) {
				cancelMsg, err := botdata.Append("", botdata.BotData{
					Action:      botdata.Cancel,
					TriggeredAt: now.Add(-15 * time.Minute),
					Revision:    ci.GetCurrentRevision(),
				})
				assert.NoErr(t, err)
				ci.Messages = append(ci.Messages, &gerritpb.ChangeMessageInfo{Message: cancelMsg})
				trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
				assert.Loosely(t, trig, should.Match(&run.Trigger{
					Time:            timestamppb.New(now.Add(-5 * time.Minute)),
					Mode:            string(run.DryRun),
					GerritAccountId: user2.GetAccountId(),
					Email:           user2.GetEmail(),
				}))
			})
		})
		t.Run("Earliest CQ+2 vote wins against even earlier CQ+1", func(t *ftt.Test) {
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
			trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			assert.Loosely(t, trig, should.Match(&run.Trigger{
				Time:            timestamppb.New(now.Add(-10 * time.Minute)),
				Mode:            string(run.FullRun),
				GerritAccountId: user2.GetAccountId(),
				Email:           user2.GetEmail(),
			}))
		})
		t.Run("Sticky Vote bumps trigger time to cur revision creation time", func(t *ftt.Test) {
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
			trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
			assert.Loosely(t, trig, should.Match(&run.Trigger{
				Time:            timestamppb.New(now.Add(-10 * time.Minute)),
				Mode:            string(run.FullRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			}))
		})
		t.Run("Additional modes", func(t *ftt.Test) {
			const customLabel = "Custom"
			const customRunMode = "CUSTOM_RUN"
			cg.AdditionalModes = []*cfgpb.Mode{
				{
					Name:            customRunMode,
					CqLabelValue:    +1,
					TriggeringLabel: customLabel,
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
			ci.Labels[customLabel] = &gerritpb.LabelInfo{All: []*gerritpb.ApprovalInfo{
				{
					User:  user1,
					Value: +1,
					Date:  timestamppb.New(now.Add(-15 * time.Minute)),
				},
			}}
			t.Run("Simplest possible custom run", func(t *ftt.Test) {
				trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
				assert.Loosely(t, trig, should.Match(&run.Trigger{
					Time:            timestamppb.New(now.Add(-15 * time.Minute)),
					Mode:            customRunMode,
					ModeDefinition:  cg.AdditionalModes[0],
					GerritAccountId: user1.GetAccountId(),
					Email:           user1.GetEmail(),
				}))
			})
			t.Run("Custom run despite other users' votes", func(t *ftt.Test) {
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
				ci.Labels[customLabel] = &gerritpb.LabelInfo{All: []*gerritpb.ApprovalInfo{
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
				trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
				assert.Loosely(t, trig.GetMode(), should.Equal(customRunMode))
			})
			t.Run("Not applicable cases", func(t *ftt.Test) {
				t.Run("Additional vote must have the same timestamp", func(t *ftt.Test) {
					t.Run("before", func(t *ftt.Test) {
						ci.Labels[customLabel].GetAll()[0].Date.Seconds++
						trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
						assert.Loosely(t, trig.GetMode(), should.Equal(string(run.DryRun)))
					})
					t.Run("after", func(t *ftt.Test) {
						ci.Labels[customLabel].GetAll()[0].Date.Seconds--
						trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
						assert.Loosely(t, trig.GetMode(), should.Equal(string(run.DryRun)))
					})
				})
				t.Run("Additional vote be from the same account", func(t *ftt.Test) {
					ci.Labels[customLabel].GetAll()[0].User = user2
					trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					assert.Loosely(t, trig.GetMode(), should.Equal(string(run.DryRun)))
				})
				t.Run("Additional vote must have expected value", func(t *ftt.Test) {
					ci.Labels[customLabel].GetAll()[0].Value = 100
					trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					assert.Loosely(t, trig.GetMode(), should.Equal(string(run.DryRun)))
				})
				t.Run("Additional vote must be for the correct label", func(t *ftt.Test) {
					ci.Labels[customLabel+"-Other"] = ci.Labels[customLabel]
					delete(ci.Labels, customLabel)
					trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					assert.Loosely(t, trig.GetMode(), should.Equal(string(run.DryRun)))
				})
				t.Run("CQ vote must have correct value", func(t *ftt.Test) {
					ci.Labels[CQLabelName].GetAll()[0].Value = fullRunVote
					trig := findCQTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg})
					assert.Loosely(t, trig.GetMode(), should.Equal(string(run.FullRun)))
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

	ftt.Run("findNewPatchsetRun", t, func(t *ftt.Test) {
		t.Run("Not configured to trigger new patchset runs", func(t *ftt.Test) {
			assert.Loosely(t,
				findNewPatchsetRunTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: cle.TriggerNewPatchsetRunAfterPS}),
				should.BeNil)
		})
		t.Run("Configured", func(t *ftt.Test) {
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
			t.Run("Current patchset not ended", func(t *ftt.Test) {
				assert.Loosely(t,
					findNewPatchsetRunTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: cle.TriggerNewPatchsetRunAfterPS}),
					should.Match(
						&run.Trigger{
							Mode:            string(run.NewPatchsetRun),
							Time:            timestamppb.New(ts2),
							Email:           owner.GetEmail(),
							GerritAccountId: owner.GetAccountId(),
						},
					))
			})
			t.Run("Current patchset already ended", func(t *ftt.Test) {
				// Set CLEntity NewPatchsetUploaded
				cle.TriggerNewPatchsetRunAfterPS = 2
				assert.Loosely(t,
					findNewPatchsetRunTrigger(&FindInput{ChangeInfo: ci, ConfigGroup: cg, TriggerNewPatchsetRunAfterPS: cle.TriggerNewPatchsetRunAfterPS}),
					should.BeNil)
			})
		})
	})
}
