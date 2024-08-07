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

package longops

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestExecutePostActionOp(t *testing.T) {
	t.Parallel()

	Convey("report", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		postActionCfg := &cfgpb.ConfigGroup_PostAction{
			Name: "vote verification labels",
			Action: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels_{
				VoteGerritLabels: &cfgpb.ConfigGroup_PostAction_VoteGerritLabels{},
			},
		}
		op := &ExecutePostActionOp{
			Base: &Base{
				Op: &run.OngoingLongOps_Op{
					Deadline:        timestamppb.New(ct.Clock.Now().Add(time.Minute)),
					CancelRequested: false,
					Work: &run.OngoingLongOps_Op_ExecutePostAction{
						ExecutePostAction: &run.OngoingLongOps_Op_ExecutePostActionPayload{
							Name: postActionCfg.GetName(),
							Kind: &run.OngoingLongOps_Op_ExecutePostActionPayload_ConfigAction{
								ConfigAction: postActionCfg,
							},
						},
					},
				},
				IsCancelRequested: func() bool { return false },
			},
			GFactory: ct.GFactory(),
		}

		Convey("returns status", func() {
			exeErr := errors.New("this is a successful error")

			Convey("CANCELLED", func() {
				op.IsCancelRequested = func() bool { return true }
				res := op.report(ctx, exeErr, "votes cancelled")
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_CANCELLED)
				So(res.GetExecutePostAction().GetSummary(), ShouldEqual, "votes cancelled")
			})
			Convey("EXPIRED", func() {
				nctx, cancel := context.WithCancel(ctx)
				cancel()
				res := op.report(nctx, exeErr, "votes expired")
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_EXPIRED)
				So(res.GetExecutePostAction().GetSummary(), ShouldEqual, "votes expired")
			})
			Convey("FAILED", func() {
				res := op.report(ctx, exeErr, "votes failed")
				So(res.GetStatus(), ShouldEqual, eventpb.LongOpCompleted_FAILED)
				So(res.GetExecutePostAction().GetSummary(), ShouldEqual, "votes failed")
			})
		})
	})
}
