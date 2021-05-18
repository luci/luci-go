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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/cvtesting"
	gf "go.chromium.org/luci/cv/internal/gerrit/gerritfake"
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
		rid := common.MakeRunID(lProject, ct.Clock.Now(), 1, []byte("deadbeef"))
		runCLs := common.CLIDs{1, 2}
		cgID := config.MakeConfigGroupID("cafecafe", "main")
		r := run.Run{
			ID:            rid,
			Status:        run.Status_RUNNING,
			CreateTime:    ct.Clock.Now().UTC().Add(-2 * time.Minute),
			StartTime:     ct.Clock.Now().UTC().Add(-1 * time.Minute),
			ConfigGroupID: cgID,
			CLs:           runCLs,
		}
		So(datastore.Put(ctx, &r,
			&run.RunCL{
				ID:  runCLs[0],
				Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "example.com",
							Info: gf.CI(1111),
						},
					},
					Deps: []*changelist.Dep{
						{Clid: 2, Kind: changelist.DepKind_HARD},
					},
				},
			},
			&run.RunCL{
				ID:  runCLs[1],
				Run: datastore.MakeKey(ctx, run.RunKind, string(rid)),
				Detail: &changelist.Snapshot{
					Kind: &changelist.Snapshot_Gerrit{
						Gerrit: &changelist.Gerrit{
							Host: "example.com",
							Info: gf.CI(2222),
						},
					},
				},
			},
		), ShouldBeNil)
		rs := &state.RunState{Run: r}
		h := &Impl{
			RM: run.NewNotifier(ct.TQDispatcher),
		}

		statuses := []run.Status{
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Run.Status = status
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}

		Convey("Submit", func() {
			vr := migration.VerifiedCQDRun{
				ID: rid,
				Payload: &migrationpb.ReportVerifiedRunRequest{
					Action: migrationpb.ReportVerifiedRunRequest_ACTION_SUBMIT,
				},
			}
			So(datastore.Put(ctx, &vr), ShouldBeNil)

			cfg := &cfgpb.Config{
				ConfigGroups: []*cfgpb.ConfigGroup{
					{
						Name: "main",
						Verifiers: &cfgpb.Verifiers{
							TreeStatus: &cfgpb.Verifiers_TreeStatus{
								Url: "tree.example.com",
							},
						},
					},
				},
			}
			ct.Cfg.Create(ctx, rid.LUCIProject(), cfg)
			updateConfigGroupToLatest := func(rs *state.RunState) {
				meta, err := config.GetLatestMeta(ctx, rs.Run.ID.LUCIProject())
				So(err, ShouldBeNil)
				So(meta.ConfigGroupIDs, ShouldHaveLength, 1)
				rs.Run.ConfigGroupID = meta.ConfigGroupIDs[0]
			}
			updateConfigGroupToLatest(rs)
			now := ct.Clock.Now().UTC()
			ctx = context.WithValue(ctx, &fakeTaskIDKey, "task-foo")

			Convey("Delegate to OnReadyForSubmission", func() {
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.PostProcessFn, ShouldNotBeNil)
				So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
				So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
					Deadline:          timestamppb.New(now.Add(submissionDuration)),
					Cls:               []int64{2, 1}, // in submission order
					TaskId:            "task-foo",
					TreeOpen:          true,
					LastTreeCheckTime: timestamppb.New(now),
				})
				So(submit.MustCurrentRun(ctx, lProject), ShouldEqual, rid)
				runtest.AssertReceivedReadyForSubmission(ctx, rid, now.Add(10*time.Second))
			})
		})
	})
}
