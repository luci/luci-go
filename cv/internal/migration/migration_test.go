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

package migration

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/run"
	runImpl "go.chromium.org/luci/cv/internal/run/impl"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchActiveRuns(t *testing.T) {
	Convey("FetchActiveRuns", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{allowGroup},
		})
		m := &MigrationServer{}

		Convey("Returns migration Runs", func() {
			const rid = "chromium/1111111111111-cafecafe"
			updateTime := timestamppb.New(clock.Now(ctx).Add(-2 * time.Minute))
			ci1 := &gerritpb.ChangeInfo{
				Project:         "infra/infra",
				Number:          11111,
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number:  7,
						Created: timestamppb.New(updateTime.AsTime().Add(-2 * time.Minute)),
					},
				},
				Labels: map[string]*gerritpb.LabelInfo{
					cqLabelName: {
						All: []*gerritpb.ApprovalInfo{
							{
								User: &gerritpb.AccountInfo{
									AccountId: 1,
									Email:     "user@example.com",
								},
								Value: 1,
								Date:  timestamppb.New(updateTime.AsTime().Add(-2 * time.Minute)),
							},
						},
					},
				},
			}
			ci2 := &gerritpb.ChangeInfo{
				Project:         "infra/infra",
				Number:          11110,
				CurrentRevision: "deadbeef",
				Revisions: map[string]*gerritpb.RevisionInfo{
					"deadbeef": {
						Number:  5,
						Created: timestamppb.New(updateTime.AsTime().Add(-3 * time.Minute)),
					},
				},
				Labels: map[string]*gerritpb.LabelInfo{
					cqLabelName: {
						All: []*gerritpb.ApprovalInfo{
							{
								User: &gerritpb.AccountInfo{
									AccountId: 1,
									Email:     "user@example.com",
								},
								Value: 1,
								Date:  timestamppb.New(updateTime.AsTime().Add(-4 * time.Minute)),
							},
						},
					},
				},
			}
			err := datastore.Put(ctx,
				&run.Run{
					ID:     rid,
					Status: run.Status_RUNNING,
					Mode:   run.DryRun,
					CLs:    []common.CLID{1, 2},
				},
				&runImpl.RunCL{
					ID:  1,
					Run: datastore.MakeKey(ctx, run.RunKind, rid),
					Detail: &changelist.Snapshot{
						ExternalUpdateTime: updateTime,
						Deps: []*changelist.Dep{
							{Clid: 2, Kind: changelist.DepKind_HARD},
						},
						Patchset:              7,
						MinEquivalentPatchset: 6,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host:  "https://chromium-review.googlesource.com/",
								Files: []string{"1.cpp"},
								Info:  ci1,
							},
						},
					},
				},
				&runImpl.RunCL{
					ID:  2,
					Run: datastore.MakeKey(ctx, run.RunKind, rid),
					Detail: &changelist.Snapshot{
						ExternalUpdateTime:    updateTime,
						Patchset:              5,
						MinEquivalentPatchset: 5,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host:  "https://chromium-review.googlesource.com/",
								Files: []string{"2.cpp"},
								Info:  ci2,
							},
						},
					},
				},
			)
			So(err, ShouldBeNil)
			resp, err := m.FetchActiveRuns(ctx, &migrationpb.FetchActiveRunsRequest{LuciProject: "chromium"})
			So(err, ShouldBeNil)
			So(resp.GetRuns(), ShouldHaveLength, 1)
			So(resp.GetRuns()[0], ShouldResembleProto, &migrationpb.Run{
				Attempt: &cvbqpb.Attempt{LuciProject: "chromium"},
				Id:      rid,
				Cls: []*migrationpb.RunCL{
					{
						Id: 1,
						Gc: &cvbqpb.GerritChange{
							Host:                       "https://chromium-review.googlesource.com/",
							Project:                    "infra/infra",
							Change:                     11111,
							Patchset:                   7,
							EarliestEquivalentPatchset: 6,
							Mode:                       cvbqpb.Mode_DRY_RUN,
						},
						Files: []string{"1.cpp"},
						Info:  ci1,
						Trigger: &migrationpb.RunCL_Trigger{
							Time:      timestamppb.New(updateTime.AsTime().Add(-2 * time.Minute)),
							AccountId: 1,
							Email:     "user@example.com",
						},
						Deps: []*migrationpb.RunCL_Dep{
							{Id: 2, Hard: true},
						},
					},
					{
						Id: 2,
						Gc: &cvbqpb.GerritChange{
							Host:                       "https://chromium-review.googlesource.com/",
							Project:                    "infra/infra",
							Change:                     11110,
							Patchset:                   5,
							EarliestEquivalentPatchset: 5,
							Mode:                       cvbqpb.Mode_DRY_RUN,
						},
						Files: []string{"2.cpp"},
						Info:  ci2,
						Trigger: &migrationpb.RunCL_Trigger{
							Time:      timestamppb.New(updateTime.AsTime().Add(-3 * time.Minute)),
							AccountId: 1,
							Email:     "user@example.com",
						},
					},
				},
			})
		})
		Convey("Excludes non-RUNNING runs", func() {
			err := datastore.Put(ctx,
				&run.Run{
					ID:     "chromium/1111111111111-deadbeef",
					Status: run.Status_SUCCEEDED,
				},
				&run.Run{
					ID:     "chromium/2222222222222-baaaaaad",
					Status: run.Status_FAILED,
				},
			)
			So(err, ShouldBeNil)
			resp, err := m.FetchActiveRuns(ctx, &migrationpb.FetchActiveRunsRequest{LuciProject: "chromium"})
			So(err, ShouldBeNil)
			So(resp.GetRuns(), ShouldHaveLength, 0)
		})
	})
}

func TestClsOf(t *testing.T) {
	t.Parallel()

	Convey("clsOf works", t, func() {
		a := &cvbqpb.Attempt{}
		So(clsOf(a), ShouldEqual, "NO CLS")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
		}
		So(clsOf(a), ShouldEqual, "1 CLs: [abc 1/2]")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
			{Host: "abc", Change: 2, Patchset: 3},
		}
		So(clsOf(a), ShouldEqual, "2 CLs: [abc 1/2 2/3]")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
			{Host: "xyz", Change: 2, Patchset: 3},
			{Host: "xyz", Change: 3, Patchset: 4},
			{Host: "abc", Change: 4, Patchset: 5},
			{Host: "abc", Change: 5, Patchset: 6},
		}
		So(clsOf(a), ShouldEqual, "5 CLs: [abc 1/2] [xyz 2/3 3/4] [abc 4/5 5/6]")
	})
}

func TestFindTrigger(t *testing.T) {
	t.Parallel()
	user1 := &gerritpb.AccountInfo{
		AccountId: 1,
		Email:     "user.one@example.com",
	}
	user2 := &gerritpb.AccountInfo{
		AccountId: 2,
		Email:     "user.two@example.com",
	}
	user3 := &gerritpb.AccountInfo{
		AccountId: 3,
		Email:     "user.three@example.com",
	}
	Convey("findTrigger", t, func() {
		now := testclock.TestRecentTimeUTC
		ci := &gerritpb.ChangeInfo{
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
				cqLabelName: {
					All: []*gerritpb.ApprovalInfo{
						{
							User:  user1,
							Value: 1, // DryRun
							Date:  timestamppb.New(now.Add(-15 * time.Minute)),
						},
						{
							User:  user2,
							Value: 1, // DryRun
							Date:  timestamppb.New(now.Add(-20 * time.Minute)),
						},
						{
							User:  user3,
							Value: 2, // FullRun
							Date:  timestamppb.New(now.Add(-10 * time.Minute)),
						},
					},
				},
			},
		}
		Convey("Single Vote", func() {
			trigger, err := findTrigger(ci, run.FullRun)
			So(err, ShouldBeNil)
			So(trigger, ShouldResembleProto, &migrationpb.RunCL_Trigger{
				Time:      timestamppb.New(now.Add(-10 * time.Minute)),
				AccountId: user3.GetAccountId(),
				Email:     user3.GetEmail(),
			})
		})
		Convey("Multiple Votes", func() {
			trigger, err := findTrigger(ci, run.DryRun)
			So(err, ShouldBeNil)
			// expect the earlier one
			So(trigger, ShouldResembleProto, &migrationpb.RunCL_Trigger{
				Time:      timestamppb.New(now.Add(-20 * time.Minute)),
				AccountId: user2.GetAccountId(),
				Email:     user2.GetEmail(),
			})
		})
		Convey("Sticky Vote", func() {
			ci.CurrentRevision = "deadbeef"
			ci.Revisions["deadbeef"] = &gerritpb.RevisionInfo{
				Number:  3,
				Created: timestamppb.New(now.Add(-5 * time.Minute)),
			}
			trigger, err := findTrigger(ci, run.FullRun)
			So(err, ShouldBeNil)
			So(trigger, ShouldResembleProto, &migrationpb.RunCL_Trigger{
				Time:      ci.Revisions["deadbeef"].Created,
				AccountId: user3.GetAccountId(),
				Email:     user3.GetEmail(),
			})
		})
		Convey("Excludes vote from previous attempt", func() {
			cancelMsg, err := botdata.Append("", botdata.BotData{
				Action:      botdata.Cancel,
				TriggeredAt: now.Add(-18 * time.Minute),
				Revision:    "deadbeef~1",
			})
			So(err, ShouldBeNil)
			ci.Messages = append(ci.Messages, &gerritpb.ChangeMessageInfo{
				Message: cancelMsg,
			})
			trigger, err := findTrigger(ci, run.DryRun)
			So(err, ShouldBeNil)
			// Vote from user2 belongs to previously cancelled attempt of the same
			// revision.
			So(trigger, ShouldResembleProto, &migrationpb.RunCL_Trigger{
				Time:      timestamppb.New(now.Add(-15 * time.Minute)),
				AccountId: user1.GetAccountId(),
				Email:     user1.GetEmail(),
			})
		})
		Convey("Error when no Commit-Queue label info", func() {
			ci.Labels = nil
			_, err := findTrigger(ci, run.DryRun)
			So(err, ShouldErrLike, "missing LabelInfo for label \"Commit-Queue\"")
		})
		Convey("Error when no vote on Commit-Queue label for given mode", func() {
			ci.Labels = map[string]*gerritpb.LabelInfo{
				cqLabelName: {
					All: []*gerritpb.ApprovalInfo{
						{
							User:  user1,
							Value: 1,
							Date:  timestamppb.New(now.Add(-15 * time.Minute)),
						},
					},
				},
			}
			_, err := findTrigger(ci, run.FullRun)
			So(err, ShouldErrLike, "no vote found for \"Commit-Queue\" +2")
		})
	})
}
