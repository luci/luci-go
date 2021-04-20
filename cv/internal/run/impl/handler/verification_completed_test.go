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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
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
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/impl/submit"
	"go.chromium.org/luci/cv/internal/run/runtest"
	"go.chromium.org/luci/cv/internal/tree"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOnVerificationCompleted(t *testing.T) {
	t.Parallel()

	Convey("OnVerificationCompleted", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		runNotifier := run.NewNotifier(ct.TQDispatcher)

		rid := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
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
		rs := &state.RunState{
			Run:         r,
			RunNotifier: runNotifier,
		}
		h := &Impl{}

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
			ctx, cancel = clock.WithDeadline(ctx, now.Add(1*time.Minute))
			defer cancel()

			Convey("Works (Happy Path)", func() {
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.State.Run.Status, ShouldEqual, run.Status_SUBMITTING)
				So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
					Deadline:          timestamppb.New(now.Add(1 * time.Minute)), // deadline in ctx
					AttemptCount:      1,
					Cls:               []int64{2, 1}, // in submission order
					TreeOpen:          true,
					LastTreeCheckTime: timestamppb.New(now),
				})
				current, _, err := submit.LoadCurrentAndWaitlist(ctx, rid)
				So(err, ShouldBeNil)
				So(current, ShouldEqual, rid)
				runtest.AssertReceivedReadyForSubmission(ctx, rid, now.Add(10*time.Second))
			})

			Convey("Add Run to waitlist when submit queue is occupied", func() {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					// another run has taken the current slot
					anotherRunID := common.MakeRunID("infra", now, 1, []byte("cafecafe"))
					waitlisted, err := submit.TryAcquire(ctx, rs.RunNotifier, anotherRunID, cfg.GetSubmitOptions())
					So(waitlisted, ShouldBeFalse)
					So(err, ShouldBeNil)
					return nil
				}, nil), ShouldBeNil)
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.State.Run.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
				_, waitlist, err := submit.LoadCurrentAndWaitlist(ctx, rid)
				So(err, ShouldBeNil)
				So(waitlist.Index(rid), ShouldEqual, 0)
			})

			Convey("Revisit after 1 mintues if tree is closed", func() {
				ct.TreeFake.ModifyState(ctx, tree.Closed)
				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.State.Run.Status, ShouldEqual, run.Status_WAITING_FOR_SUBMISSION)
				So(res.State.Run.Submission, ShouldResembleProto, &run.Submission{
					Cls:               []int64{2, 1}, // in submission order
					TreeOpen:          false,
					LastTreeCheckTime: timestamppb.New(now),
				})
				So(res.SideEffectFn, ShouldBeNil)
				runtest.AssertInEventbox(ctx, rid, &eventpb.Event{
					Event: &eventpb.Event_Poke{
						Poke: &eventpb.Poke{},
					},
					ProcessAfter: timestamppb.New(now.Add(1 * time.Minute)),
				})
			})

			Convey("Treat Tree url not defined as open", func() {
				cfg := proto.Clone(cfg).(*cfgpb.Config)
				cfg.ConfigGroups[0].Verifiers = nil
				ct.Cfg.Update(ctx, rid.LUCIProject(), cfg)
				updateConfigGroupToLatest(rs)

				res, err := h.OnCQDVerificationCompleted(ctx, rs)
				So(err, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
				So(res.State.Run.Submission.TreeOpen, ShouldEqual, true)
				So(res.State.Run.Submission.LastTreeCheckTime, ShouldResembleProto, timestamppb.New(now))
			})
		})
	})
}
