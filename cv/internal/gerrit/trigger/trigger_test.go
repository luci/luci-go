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

	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTrigger(t *testing.T) {
	t.Parallel()

	user1 := gf.U("u-1")
	user2 := gf.U("u-2")
	user3 := gf.U("u-3")
	user4 := gf.U("u-4")
	const dryRunVote = 1
	const fullRunVote = 2
	Convey("Find", t, func() {
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
			So(Find(ci), ShouldBeNil)
		})
		Convey("Merged CL", func() {
			ci.Status = gerritpb.ChangeStatus_MERGED
			So(Find(ci), ShouldBeNil)
		})
		Convey("No votes", func() {
			So(Find(ci), ShouldBeNil)
		})
		Convey("No Commit-Queue label info", func() {
			ci.Labels = nil
			So(Find(ci), ShouldBeNil)
		})
		Convey("Single vote", func() {
			ci.Labels[CQLabelName].All = []*gerritpb.ApprovalInfo{{
				User:  user1,
				Value: dryRunVote,
				Date:  timestamppb.New(now.Add(-15 * time.Minute)),
			}}
			trigger := Find(ci)
			So(trigger, ShouldResembleProto, &run.Trigger{
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
			trigger := Find(ci)
			So(trigger, ShouldResembleProto, &run.Trigger{
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
			trigger := Find(ci)
			So(trigger, ShouldResembleProto, &run.Trigger{
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
				trigger := Find(ci)
				So(trigger, ShouldResembleProto, &run.Trigger{
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
			trigger := Find(ci)
			So(trigger, ShouldResembleProto, &run.Trigger{
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
			trigger := Find(ci)
			So(trigger, ShouldResembleProto, &run.Trigger{
				Time:            timestamppb.New(now.Add(-10 * time.Minute)),
				Mode:            string(run.FullRun),
				GerritAccountId: user1.GetAccountId(),
				Email:           user1.GetEmail(),
			})
		})
	})
}
