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

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOnCompletedPostAction(t *testing.T) {
	t.Parallel()

	Convey("onCompletedPostAction", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		const (
			lProject = "chromium"
			opID     = "1-1"
		)
		prjcfgtest.Create(ctx, lProject, &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{{Name: "single"}}})
		opPayload := &run.OngoingLongOps_Op_ExecutePostActionPayload{
			Name: "label-vote",
			Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction{
				ConfigAction: &cfgpb.ConfigGroup_PostAction{
					Name: "label-vote",
					Action: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels_{
						VoteGerritLabels: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels{},
					},
				},
			},
		}
		opResult := &eventpb.LongOpCompleted{
			OperationId: opID,
			Result: &eventpb.LongOpCompleted_ExecutePostAction{
				ExecutePostAction: &eventpb.LongOpCompleted_ExecutePostActionResult{},
			},
		}
		rs := &state.RunState{
			Run: run.Run{
				ID:            lProject + "/1111111111111-1-deadbeef",
				Status:        run.Status_SUCCEEDED,
				Mode:          run.DryRun,
				ConfigGroupID: prjcfgtest.MustExist(ctx, lProject).ConfigGroupIDs[0],
				OngoingLongOps: &run.OngoingLongOps{
					Ops: map[string]*run.OngoingLongOps_Op{
						opID: {
							Work: &run.OngoingLongOps_Op_ExecutePostAction{
								ExecutePostAction: opPayload,
							},
						},
					},
				},
			},
		}
		h, _ := makeTestHandler(&ct)
		var err error
		var res *Result
		Convey("execution summary is set", func() {
			opResult.Status = eventpb.LongOpCompleted_CANCELLED
			opResult.GetExecutePostAction().Summary = "this is a summary"
			res, err = h.OnLongOpCompleted(ctx, rs, opResult)
			So(err, ShouldBeNil)
			So(res.State.LogEntries[0].GetInfo(), ShouldResembleProto, &run.LogEntry_Info{
				Label:   "PostAction[label-vote]",
				Message: "this is a summary",
			})
		})
		Convey("execution summary is not set", func() {
			var expected string
			Convey("the op succeeded", func() {
				opResult.Status = eventpb.LongOpCompleted_SUCCEEDED
				expected = "the execution succeeded"
			})
			Convey("the op expired", func() {
				opResult.Status = eventpb.LongOpCompleted_EXPIRED
				expected = "the execution deadline was exceeded"
			})
			Convey("the op cancelled", func() {
				opResult.Status = eventpb.LongOpCompleted_CANCELLED
				expected = "the execution was cancelled"
			})
			Convey("the op failed", func() {
				opResult.Status = eventpb.LongOpCompleted_FAILED
				expected = "the execution failed"
			})
			res, err = h.OnLongOpCompleted(ctx, rs, opResult)
			So(err, ShouldBeNil)
			So(res.State.LogEntries[0].GetInfo(), ShouldResembleProto, &run.LogEntry_Info{
				Label:   "PostAction[label-vote]",
				Message: expected,
			})
		})
		So(res.State.OngoingLongOps, ShouldBeNil)
		So(res.SideEffectFn, ShouldBeNil)
		So(res.PreserveEvents, ShouldBeFalse)
	})
}
