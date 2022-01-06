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

package handler

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/run/runtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOnVerificationCompleted(t *testing.T) {
	t.Parallel()

	Convey("OnVerificationCompleted", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const lProject = "infra"
		const gHost = "x-review.example.com"
		const gChange = 123
		const quickLabel = "quick"
		rid := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		rs := &state.RunState{
			Run: run.Run{
				ID:         rid,
				Status:     run.Status_RUNNING,
				CreateTime: ct.Clock.Now().UTC().Add(-2 * time.Minute),
				StartTime:  ct.Clock.Now().UTC().Add(-1 * time.Minute),
				CLs:        common.CLIDs{1},
			},
		}
		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{
				{
					Name: "main",
					Verifiers: &cfgpb.Verifiers{
						TreeStatus: &cfgpb.Verifiers_TreeStatus{
							Url: "tree.example.com",
						},
					},
					AdditionalModes: []*cfgpb.Mode{
						{
							CqLabelValue:    1,
							Name:            string(run.QuickDryRun),
							TriggeringLabel: quickLabel,
							TriggeringValue: 1,
						},
					},
				},
			},
		}
		prjcfgtest.Create(ctx, lProject, cfg)
		meta, err := prjcfg.GetLatestMeta(ctx, lProject)
		So(err, ShouldBeNil)
		So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
		rs.ConfigGroupID = meta.ConfigGroupIDs[0]

		createCL := func(ci *gerritpb.ChangeInfo) {
			ct.GFake.CreateChange(&gf.Change{
				Host: gHost,
				Info: ci,
				ACLs: gf.ACLRestricted(lProject),
			})

			So(datastore.Put(ctx,
				&run.RunCL{
					ID:         1,
					Run:        datastore.MakeKey(ctx, run.RunKind, string(rid)),
					ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
					Detail: &changelist.Snapshot{
						LuciProject: lProject,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
							},
						},
					},
					Trigger: trigger.Find(ci, cfg.ConfigGroups[0]),
				},
				&changelist.CL{
					ID:         1,
					ExternalID: changelist.MustGobID(gHost, ci.GetNumber()),
					Snapshot: &changelist.Snapshot{
						LuciProject: lProject,
						Kind: &changelist.Snapshot_Gerrit{
							Gerrit: &changelist.Gerrit{
								Host: gHost,
								Info: proto.Clone(ci).(*gerritpb.ChangeInfo),
							},
						},
					},
				},
			), ShouldBeNil)
		}

		h, _ := makeTestHandler(&ct)

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Status = status
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				switch {
				case run.IsEnded(status):
					So(res.PreserveEvents, ShouldBeFalse)
				default:
					So(res.PreserveEvents, ShouldBeTrue)
				}
			})
		}

		now := ct.Clock.Now().UTC()
		Convey("Submit", func() {
			rs.Mode = run.FullRun
			createCL(gf.CI(gChange,
				gf.Owner("user-1"),
				gf.CQ(+2, now.Add(-1*time.Minute), gf.U("user-2")),
				gf.Updated(now.Add(-1*time.Minute))))
			vr := migration.VerifiedCQDRun{
				ID: rid,
				Payload: &migrationpb.ReportVerifiedRunRequest{
					Action: migrationpb.ReportVerifiedRunRequest_ACTION_SUBMIT,
				},
			}
			So(datastore.Put(ctx, &vr), ShouldBeNil)

			ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")

			Convey("Delegate to OnReadyForSubmission", func() {
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldNotBeNil)
				So(res.State.Status, ShouldEqual, run.Status_SUBMITTING)
				So(res.State.Submission, ShouldResembleProto, &run.Submission{
					Deadline:          timestamppb.New(now.Add(submissionDuration)),
					Cls:               []int64{1},
					TaskId:            "task-foo",
					TreeOpen:          true,
					LastTreeCheckTime: timestamppb.New(now),
				})
				So(submit.MustCurrentRun(ctx, lProject), ShouldEqual, rid)
				runtest.AssertReceivedReadyForSubmission(ctx, rid, now.Add(10*time.Second))
			})
		})

		Convey("Dry run passes verification", func() {
			vr := migration.VerifiedCQDRun{
				ID: rid,
				Payload: &migrationpb.ReportVerifiedRunRequest{
					Action: migrationpb.ReportVerifiedRunRequest_ACTION_DRY_RUN_OK,
				},
			}
			So(datastore.Put(ctx, &vr), ShouldBeNil)
			Convey("Dry run", func() {
				rs.Mode = run.DryRun
				createCL(gf.CI(gChange,
					gf.Owner("user-1"),
					gf.CQ(+1, now.Add(-1*time.Minute), gf.U("user-2")),
					gf.Updated(now.Add(-1*time.Minute))))
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.SideEffectFn, ShouldNotBeNil)
				So(res.State.Status, ShouldEqual, run.Status_SUCCEEDED)
				ci := ct.GFake.GetChange(gHost, gChange).Info
				So(gf.NonZeroVotes(ci, trigger.CQLabelName), ShouldBeEmpty)
				So(gf.LastMessage(ci).GetMessage(), ShouldContainSubstring, "Dry run: This CL passed the CQ dry run.")
			})

			Convey("Quick dry run", func() {
				rs.Mode = run.QuickDryRun
				createCL(gf.CI(gChange,
					gf.Owner("user-1"),
					gf.CQ(+1, now.Add(-1*time.Minute), gf.U("user-2")),
					gf.Vote(quickLabel, +1, now.Add(-1*time.Minute), gf.U("user-2")),
					gf.Updated(now.Add(-1*time.Minute))))
				_, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(gf.LastMessage(ct.GFake.GetChange(gHost, gChange).Info).GetMessage(), ShouldContainSubstring, "CL passed")
			})
		})

		Convey("Run fails verification", func() {
			rs.Mode = run.FullRun
			createCL(gf.CI(gChange,
				gf.Owner("user-1"),
				gf.CQ(+2, now.Add(-1*time.Minute), gf.U("user-2")),
				gf.Updated(now.Add(-1*time.Minute))))
			vr := migration.VerifiedCQDRun{
				ID: rid,
				Payload: &migrationpb.ReportVerifiedRunRequest{
					Action:       migrationpb.ReportVerifiedRunRequest_ACTION_FAIL,
					FinalMessage: "builder abc failed",
				},
			}
			So(datastore.Put(ctx, &vr), ShouldBeNil)
			Convey("Cancel triggers and post message", func() {
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.SideEffectFn, ShouldNotBeNil)
				So(res.State.Status, ShouldEqual, run.Status_FAILED)
				ci := ct.GFake.GetChange(gHost, gChange).Info
				So(gf.NonZeroVotes(ci, trigger.CQLabelName), ShouldBeEmpty)
				So(gf.LastMessage(ci).GetMessage(), ShouldContainSubstring, "builder abc failed")

				Convey("Notify the owner and voters with attention", func() {
					reqs := []*gerritpb.SetReviewRequest{}
					for _, req := range ct.GFake.Requests() {
						switch r, ok := req.(*gerritpb.SetReviewRequest); {
						case !ok:
						case r.GetOnBehalfOf() != 0:
						default:
							reqs = append(reqs, r)
						}
					}
					sort.SliceStable(reqs, func(i, j int) bool {
						return reqs[i].Number < reqs[j].Number
					})
					So(reqs, ShouldHaveLength, 1)
					So(reqs[0].GetNumber(), ShouldEqual, ci.GetNumber())
					So(reqs[0].GetNotify(), ShouldEqual, gerritpb.Notify_NOTIFY_NONE)
					So(reqs[0].GetNotifyDetails(), ShouldResembleProto, &gerritpb.NotifyDetails{
						Recipients: []*gerritpb.NotifyDetails_Recipient{
							{
								RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
								Info: &gerritpb.NotifyDetails_Info{
									Accounts: []int64{1, 2},
								},
							},
						},
					})
					So(reqs[0].GetAddToAttentionSet(), ShouldResembleProto, []*gerritpb.AttentionSetInput{
						// The attention set includes the owner and voter(s).
						{User: "1", Reason: "ps#1: CQ full run failed."},
						{User: "2", Reason: "ps#1: CQ full run failed."},
					})
				})
			})
		})
	})
}
