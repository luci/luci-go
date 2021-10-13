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

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestStart(t *testing.T) {
	t.Parallel()

	Convey("StartRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject           = "chromium"
			stabilizationDelay = time.Minute
			startLatency       = 2 * time.Minute
		)

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{
			Name: "combinable",
			CombineCls: &cfgpb.CombineCLs{
				StabilizationDelay: durationpb.New(stabilizationDelay),
			},
		}}})

		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-deadbeef",
				Status:        run.Status_PENDING,
				CreateTime:    clock.Now(ctx).UTC().Add(-startLatency),
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
			},
		}
		h, _ := makeTestHandler(&ct)

		Convey("Starts when Run is PENDING", func() {
			rs.Status = run.Status_PENDING
			res, err := h.Start(ctx, rs)
			So(err, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)

			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.StartTime, ShouldResemble, clock.Now(ctx).UTC())
			So(res.State.LogEntries, ShouldHaveLength, 1)
			So(res.State.LogEntries[0].GetStarted(), ShouldNotBeNil)

			So(res.State.NewLongOpIDs, ShouldHaveLength, 1)
			longOp := res.State.OngoingLongOps.GetOps()[res.State.NewLongOpIDs[0]]
			So(longOp.GetPostStartMessage(), ShouldBeTrue)
			So(longOp.GetDeadline().AsTime(), ShouldHappenOnOrAfter, clock.Now(ctx).Add(maxPostStartMessageDuration))

			So(ct.TSMonSentDistr(ctx, metricPickupLatencyS, lProject).Sum(),
				ShouldAlmostEqual, startLatency.Seconds())
			So(ct.TSMonSentDistr(ctx, metricPickupLatencyAdjustedS, lProject).Sum(),
				ShouldAlmostEqual, (startLatency - stabilizationDelay).Seconds())
		})

		statuses := []run.Status{
			run.Status_RUNNING,
			run.Status_WAITING_FOR_SUBMISSION,
			run.Status_SUBMITTING,
			run.Status_SUCCEEDED,
			run.Status_FAILED,
			run.Status_CANCELLED,
		}
		for _, status := range statuses {
			Convey(fmt.Sprintf("Noop when Run is %s", status), func() {
				rs.Status = status
				res, err := h.Start(ctx, rs)
				So(err, ShouldBeNil)
				So(res.State, ShouldEqual, rs)
				So(res.SideEffectFn, ShouldBeNil)
				So(res.PreserveEvents, ShouldBeFalse)
			})
		}
	})
}

func TestOnCompletedPostStartMessage(t *testing.T) {
	t.Parallel()

	Convey("onCompletedPostStartMessage works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

		const (
			lProject = "chromium"
			opID     = "1-1"
		)

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "single"}}})

		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-1-deadbeef",
				Status:        run.Status_RUNNING,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						opID: {
							Work: &run.OngoingLongOps_Op_PostStartMessage{
								PostStartMessage: true,
							},
						},
					},
				},
			},
		}
		result := &eventpb.LongOpCompleted{
			OperationId: opID,
		}
		h, _ := makeTestHandler(&ct)

		Convey("if Run isn't RUNNING, just cleans up the operation", func() {
			// NOTE: This should be rare. And since posting the starting message isn't
			// a critical operation, it's OK to ignore its failures if the Run is
			// already submitting the CL.
			rs.Run.Status = run.Status_SUBMITTING
			result.Status = eventpb.LongOpCompleted_FAILED
			// The result is set in practice but serves debugging purposes only,
			// and is ignored by the onCompletedPostStartMessage.
			result.Result = &eventpb.LongOpCompleted_PostStartMessage_{
				PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{
					PermanentErrors: map[int64]string{1: "Gerrit refused to post the start message"},
				},
			}

			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_SUBMITTING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		})

		Convey("on cancellation, cleans up Run's state", func() {
			// NOTE: as of this writing (Oct 2021), the only time posting start
			// message is cancelled is if the Run was already finalized. Therefore,
			// Run can't be in RUNNING state any more.
			// However, this test aims to cover possible future logic change in CV.
			result.Status = eventpb.LongOpCompleted_CANCELLED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		})

		Convey("on success, cleans Run's state", func() {
			result.Status = eventpb.LongOpCompleted_SUCCEEDED
			// The result is set in practice but serves debugging purposes only,
			// and is ignored by the onCompletedPostStartMessage.
			result.Result = &eventpb.LongOpCompleted_PostStartMessage_{
				PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{
					Posted: []int64{1},
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_RUNNING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		})

		Convey("on failure, fails the Runs", func() {
			result.Status = eventpb.LongOpCompleted_FAILED
			// The result is set in practice but serves debugging purposes only,
			// and is ignored by the onCompletedPostStartMessage.
			result.Result = &eventpb.LongOpCompleted_PostStartMessage_{
				PostStartMessage: &eventpb.LongOpCompleted_PostStartMessage{
					PermanentErrors: map[int64]string{1: "Gerrit refused to post the start message"},
					Posted:          []int64{2},
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_FAILED)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.SideEffectFn, ShouldNotBeNil) // side effects to finalize the Run.
		})

		Convey("on expiration, also fails the Runs", func() {
			result.Status = eventpb.LongOpCompleted_EXPIRED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_FAILED)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.SideEffectFn, ShouldNotBeNil) // side effects to finalize the Run.
		})
	})
}
