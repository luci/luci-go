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

package build

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestStepNoop(t *testing.T) {
	Convey("Step no-op mode", t, func() {
		ctx := memlogger.Use(context.Background())
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		Convey("Step creation", func() {
			Convey("ScheduleStep", func() {
				step, ctx := ScheduleStep(ctx, "some step")
				defer func() { step.End(nil) }()

				So(logs, memlogger.ShouldHaveLog,
					logging.Info, "set status: SCHEDULED", logging.Fields{"build.step": "some step"})

				So(step, ShouldNotBeNil)
				So(getState(ctx), ShouldResemble, ctxState{stepPrefix: "some step|"})

				So(step.Start, ShouldNotPanic)
				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: STARTED")

				So(step.Start, ShouldPanicLike, "cannot start step")
			})

			Convey("Step", func() {
				step, ctx := Step(ctx, "some step")
				defer func() { step.End(nil) }()

				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: SCHEDULED")
				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: STARTED")

				So(step, ShouldNotBeNil)
				So(getState(ctx), ShouldResemble, ctxState{stepPrefix: "some step|"})

				So(step.Start, ShouldPanicLike, "cannot start step")
			})

			Convey("Bad step name", func() {
				So(func() {
					Step(ctx, "bad | step")
				}, ShouldPanicLike, "reserved character")
			})
		})

		Convey("Step closure", func() {
			Convey("SUCCESS", func() {
				step, _ := Step(ctx, "some step")
				step.End(nil)
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_SUCCESS)

				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: SUCCESS")

				Convey("cannot double-close", func() {
					So(func() { step.End(nil) }, ShouldPanicLike, "cannot mutate ended step")
				})

			})

			Convey("error", func() {
				step, _ := Step(ctx, "some step")
				step.End(errors.New("bad stuff"))
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_FAILURE)

				So(logs, memlogger.ShouldHaveLog, logging.Error, "set status: FAILURE: bad stuff")
			})

			Convey("CANCELED", func() {
				step, _ := Step(ctx, "some step")
				step.End(context.Canceled)
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_CANCELED)

				So(logs, memlogger.ShouldHaveLog, logging.Warning, "set status: CANCELED: context canceled")
			})

			Convey("panic", func() {
				step, _ := Step(ctx, "some step")
				func() {
					defer func() {
						step.End(nil)
						recover() // so testing assertions can happen
					}()
					panic("doom!")
				}()
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_INFRA_FAILURE)
				So(logs, memlogger.ShouldHaveLog, logging.Error, "set status: INFRA_FAILURE: PANIC")
			})

			Convey("with SummaryMarkdown", func() {
				step, _ := Step(ctx, "some step")
				step.SetSummaryMarkdown("cool story!")
				step.End(nil)
				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: SUCCESS\n  with SummaryMarkdown:\ncool story!")
			})
		})
	})
}
