// Copyright 2023 The LUCI Authors.
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
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
)

func TestOnCompletedPostGerritMessage(t *testing.T) {
	t.Parallel()

	Convey("onCompletedPostGerritMessage works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		const (
			lProject = "chromium"
			opID     = "1-1"
		)

		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{ConfigGroups: []*cfgpb.ConfigGroup{{Name: "single"}}})

		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-1-deadbeef",
				Status:        run.Status_PENDING,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						opID: {
							Work: &run.OngoingLongOps_Op_PostGerritMessage_{
								PostGerritMessage: &run.OngoingLongOps_Op_PostGerritMessage{
									Message: "foo",
								},
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

		Convey("on cancellation, cleans up Run's state", func() {
			result.Status = eventpb.LongOpCompleted_CANCELLED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_PENDING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
		})

		Convey("on success, cleans Run's state", func() {
			result.Status = eventpb.LongOpCompleted_SUCCEEDED
			postedAt := ct.Clock.Now().Add(-time.Second)
			result.Result = &eventpb.LongOpCompleted_PostGerritMessage_{
				PostGerritMessage: &eventpb.LongOpCompleted_PostGerritMessage{
					Time: timestamppb.New(postedAt),
				},
			}
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_PENDING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.LogEntries[0].GetTime().AsTime(), ShouldResemble, postedAt.UTC())
		})

		Convey("on failure, cleans Run's state and record reasons", func() {
			result.Status = eventpb.LongOpCompleted_FAILED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_PENDING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.LogEntries[0].GetInfo().GetMessage(), ShouldContainSubstring, "Failed to post gerrit message")
		})

		Convey("on expiration,cleans Run's state and record reasons", func() {
			result.Status = eventpb.LongOpCompleted_EXPIRED
			res, err := h.OnLongOpCompleted(ctx, rs, result)
			So(err, ShouldBeNil)
			So(res.State.Status, ShouldEqual, run.Status_PENDING)
			So(res.State.OngoingLongOps, ShouldBeNil)
			So(res.SideEffectFn, ShouldBeNil)
			So(res.PreserveEvents, ShouldBeFalse)
			So(res.State.LogEntries[0].GetInfo().GetMessage(), ShouldContainSubstring, "Failed to post the message to gerrit")
		})
	})
}
