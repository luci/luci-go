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
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
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

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnCQDFinished(t *testing.T) {
	t.Parallel()

	Convey("OnCQDFinished", t, func() {
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
				CLs:        nil, // modified by createCL() calls below.
			},
		}
		cfg := &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "main"}}}
		prjcfgtest.Create(ctx, lProject, cfg)
		meta, err := prjcfg.GetLatestMeta(ctx, lProject)
		So(err, ShouldBeNil)
		So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
		rs.Run.ConfigGroupID = meta.ConfigGroupIDs[0]

		createdCLsCounter := 0
		createCL := func(ci *gerritpb.ChangeInfo) {
			createdCLsCounter++
			ct.GFake.CreateChange(&gf.Change{
				Host: gHost,
				Info: ci,
				ACLs: gf.ACLRestricted(lProject),
			})

			So(datastore.Put(ctx,
				&run.RunCL{
					ID:         common.CLID(createdCLsCounter),
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
					ID:         common.CLID(createdCLsCounter),
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
			rs.Run.CLs = append(rs.Run.CLs, common.CLID(createdCLsCounter))
		}

		h := &Impl{
			RM:        run.NewNotifier(ct.TQDispatcher),
			GFactory:  ct.GFake.Factory(),
			CLUpdater: &clUpdaterMock{},
		}

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Run.Status = status
				res, err := h.OnCQDFinished(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}

		writeFinishedRun := func(a *cvbqpb.Attempt) {
			fr := migration.FinishedCQDRun{
				AttemptKey: rid.AttemptKey(),
				RunID:      rid,
				Payload: &migrationpb.ReportedRun{
					Id:      string(rid),
					Attempt: a,
				},
			}
			So(datastore.Put(ctx, &fr), ShouldBeNil)
		}

		now := ct.Clock.Now().UTC()

		Convey("Dry run succeeded", func() {
			createCL(gf.CI(gChange,
				gf.Owner("user-1"),
				gf.CQ(+1, now.Add(-1*time.Minute), gf.U("user-2")),
				gf.Updated(now.Add(-1*time.Minute))))
			writeFinishedRun(&cvbqpb.Attempt{
				Key:     rid.AttemptKey(),
				Builds:  nil, // ignored by CV.
				EndTime: timestamppb.New(now.Add(-10 * time.Second)),
				Status:  cvbqpb.AttemptStatus_SUCCESS,
				GerritChanges: []*cvbqpb.GerritChange{
					{Host: gHost, Change: gChange, Mode: cvbqpb.Mode_DRY_RUN},
				},
			})
			res, err := h.OnCQDFinished(ctx, rs)
			So(err, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.PostProcessFn, ShouldBeNil)
			So(res.SideEffectFn, ShouldNotBeNil)
			So(res.State.Run.Status, ShouldEqual, run.Status_SUCCEEDED)
			So(res.State.Run.Submission, ShouldBeNil)
			So(res.State.Run.FinalizedByCQD, ShouldBeTrue)
			So(res.State.Run.EndTime, ShouldResemble, now.Add(-10*time.Second))
		})

		Convey("Full run", func() {
			a := &cvbqpb.Attempt{
				Key:           rid.AttemptKey(),
				Builds:        nil, // ignored by CV.
				EndTime:       timestamppb.New(now.Add(-10 * time.Second)),
				Status:        cvbqpb.AttemptStatus_SUCCESS,
				GerritChanges: make([]*cvbqpb.GerritChange, 0, 5),
			}
			rs.Run.CLs = nil
			for i := gChange; i < gChange+5; i++ {
				createCL(gf.CI(i,
					gf.Owner("user-1"),
					gf.CQ(+2, now.Add(-1*time.Minute), gf.U("user-2")),
					gf.Updated(now.Add(-1*time.Minute))))
				a.GerritChanges = append(a.GerritChanges, &cvbqpb.GerritChange{
					Host: gHost, Change: int64(i), Mode: cvbqpb.Mode_FULL_RUN,
					SubmitStatus: cvbqpb.GerritChange_SUCCESS,
				})
			}

			Convey("all CLs submitted", func() {
				writeFinishedRun(a)
				res, err := h.OnCQDFinished(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State.Run.Status, ShouldEqual, run.Status_SUCCEEDED)
				s := res.State.Run.Submission
				So(s.Cls, ShouldHaveLength, 5)
				So(s.SubmittedCls, ShouldHaveLength, 5)
				So(res.State.Run.FinalizedByCQD, ShouldBeTrue)
			})
			Convey("some CLs not submitted", func() {
				a.GerritChanges[2].SubmitStatus = cvbqpb.GerritChange_FAILURE
				a.GerritChanges[4].SubmitStatus = cvbqpb.GerritChange_PENDING
				writeFinishedRun(a)
				res, err := h.OnCQDFinished(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State.Run.Status, ShouldEqual, run.Status_FAILED)
				s := res.State.Run.Submission
				So(s.Cls, ShouldHaveLength, 5)
				So(s.SubmittedCls, ShouldHaveLength, 3)
				So(res.State.Run.FinalizedByCQD, ShouldBeTrue)
			})
		})

		for cqdStatus, expCVStatus := range map[cvbqpb.AttemptStatus]run.Status{
			cvbqpb.AttemptStatus_ABORTED:       run.Status_CANCELLED,
			cvbqpb.AttemptStatus_FAILURE:       run.Status_FAILED,
			cvbqpb.AttemptStatus_INFRA_FAILURE: run.Status_FAILED,
		} {
			Convey(cqdStatus.String(), func() {
				createCL(gf.CI(gChange,
					gf.Owner("user-1"),
					gf.CQ(+1, now.Add(-1*time.Minute), gf.U("user-2")),
					gf.Updated(now.Add(-1*time.Minute))))
				writeFinishedRun(&cvbqpb.Attempt{
					Key:     rid.AttemptKey(),
					Builds:  nil, // ignored by CV.
					EndTime: timestamppb.New(now.Add(-10 * time.Second)),
					Status:  cqdStatus,
					GerritChanges: []*cvbqpb.GerritChange{
						{Host: gHost, Change: gChange, Mode: cvbqpb.Mode_DRY_RUN},
					},
				})
				res, err := h.OnCQDFinished(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldBeNil)
				So(res.SideEffectFn, ShouldNotBeNil)
				So(res.State.Run.Status, ShouldEqual, expCVStatus)
				So(res.State.Run.Submission, ShouldBeNil)
				So(res.State.Run.FinalizedByCQD, ShouldBeTrue)
				So(res.State.Run.EndTime, ShouldResemble, now.Add(-10*time.Second))
			})
		}
	})
}
