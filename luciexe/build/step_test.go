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
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/assertions"
	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/luciexe"
)

func TestStepNoop(t *testing.T) {
	Convey(`Step no-op mode`, t, func() {
		ctx := memlogger.Use(context.Background())
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		Convey(`Step creation`, func() {
			Convey(`ScheduleStep`, func() {
				step, ctx := ScheduleStep(ctx, "some step")
				defer func() { step.End(nil) }()

				So(logs, memlogger.ShouldHaveLog,
					logging.Info, "set status: SCHEDULED", logging.Fields{"build.step": "some step"})

				So(step, ShouldNotBeNil)
				So(getCurrentStep(ctx).name, ShouldResemble, "some step")

				So(step.Start, ShouldNotPanic)
				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: STARTED")
				So(logs.Messages(), ShouldHaveLength, 2)

				So(step.Start, ShouldNotPanic) // noop
				So(logs.Messages(), ShouldHaveLength, 2)
			})

			Convey(`StartStep`, func() {
				step, ctx := StartStep(ctx, "some step")
				defer func() { step.End(nil) }()

				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: SCHEDULED")
				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: STARTED")

				So(step, ShouldNotBeNil)
				So(getCurrentStep(ctx).name, ShouldResemble, "some step")
				So(logs.Messages(), ShouldHaveLength, 2)

				So(step.Start, ShouldNotPanic) // noop
				So(logs.Messages(), ShouldHaveLength, 2)
			})

			Convey(`Bad step name`, func() {
				So(func() {
					StartStep(ctx, "bad | step")
				}, ShouldPanicLike, "reserved character")
			})
		})

		Convey(`Step closure`, func() {
			Convey(`SUCCESS`, func() {
				step, ctx := StartStep(ctx, "some step")
				step.End(nil)
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_SUCCESS)

				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: SUCCESS")

				Convey(`cannot double-close`, func() {
					So(func() { step.End(nil) }, ShouldPanicLike, "cannot mutate ended step")
				})

				Convey(`cancels context as well`, func() {
					So(ctx.Err(), ShouldResemble, context.Canceled)
				})
			})

			Convey(`error`, func() {
				step, _ := StartStep(ctx, "some step")
				step.End(errors.New("bad stuff"))
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_FAILURE)

				So(logs, memlogger.ShouldHaveLog, logging.Error, "set status: FAILURE: bad stuff")
			})

			Convey(`CANCELED`, func() {
				step, _ := StartStep(ctx, "some step")
				step.End(context.Canceled)
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_CANCELED)

				So(logs, memlogger.ShouldHaveLog, logging.Warning, "set status: CANCELED: context canceled")
			})

			Convey(`panic`, func() {
				step, _ := StartStep(ctx, "some step")
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

			Convey(`with SummaryMarkdown`, func() {
				step, _ := StartStep(ctx, "some step")
				step.SetSummaryMarkdown("cool story!")
				step.End(nil)
				So(logs, memlogger.ShouldHaveLog, logging.Info, "set status: SUCCESS\n  with SummaryMarkdown:\ncool story!")
			})

			Convey(`modify starts step`, func() {
				step, _ := ScheduleStep(ctx, "some step")
				defer func() { step.End(nil) }()
				step.SetSummaryMarkdown("cool story!")
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_STARTED)
			})

			Convey(`closure of un-started step`, func() {
				step, ctx := ScheduleStep(ctx, "some step")
				So(func() { step.End(nil) }, ShouldNotPanic)
				So(step.stepPb.Status, ShouldResemble, bbpb.Status_SUCCESS)
				So(step.stepPb.StartTime, ShouldNotBeNil)
				So(ctx.Err(), ShouldResemble, context.Canceled)
			})
		})

		Convey(`Recursive steps`, func() {
			Convey(`basic`, func() {
				parent, ctx := StartStep(ctx, "parent")
				defer func() { parent.End(nil) }()

				child, ctx := StartStep(ctx, "child")
				defer func() { child.End(nil) }()

				So(parent.name, ShouldResemble, "parent")
				So(child.name, ShouldResemble, "parent|child")
			})

			Convey(`creating child step with explicit parent`, func() {
				parent, ctx := ScheduleStep(ctx, "parent")
				defer func() { parent.End(nil) }()

				child, _ := parent.ScheduleStep(ctx, "child")
				defer func() { child.End(nil) }()

				So(parent.name, ShouldResemble, "parent")
				So(parent.stepPb.Status, ShouldResemble, bbpb.Status_STARTED)
				So(child.name, ShouldResemble, "parent|child")
				So(child.stepPb.Status, ShouldResemble, bbpb.Status_SCHEDULED)
			})

			Convey(`creating child step starts parent`, func() {
				parent, ctx := ScheduleStep(ctx, "parent")
				defer func() { parent.End(nil) }()

				child, ctx := ScheduleStep(ctx, "child")
				defer func() { child.End(nil) }()

				So(parent.name, ShouldResemble, "parent")
				So(parent.stepPb.Status, ShouldResemble, bbpb.Status_STARTED)
				So(child.name, ShouldResemble, "parent|child")
				So(child.stepPb.Status, ShouldResemble, bbpb.Status_SCHEDULED)
			})
		})

		Convey(`Step logs`, func() {
			ctx := memlogger.Use(ctx)
			logs := logging.Get(ctx).(*memlogger.MemLogger)
			step, _ := StartStep(ctx, "some step")

			Convey(`text`, func() {
				log := step.Log("a log")
				_, err := log.Write([]byte("this is stuff"))
				So(err, ShouldBeNil)
				step.End(nil)
				So(logs, memlogger.ShouldHaveLog, logging.Info, "this is stuff")
				So(log.UILink(), ShouldEqual, "")
			})

			Convey(`binary`, func() {
				log := step.Log("a log", streamclient.Binary())
				_, err := log.Write([]byte("this is stuff"))
				So(err, ShouldBeNil)
				step.End(nil)
				So(logs, memlogger.ShouldHaveLog, logging.Warning, "dropping BINARY log \"a log\"")
				So(log.UILink(), ShouldEqual, "")
			})

			Convey(`datagram`, func() {
				log := step.LogDatagram("a log")
				So(log.WriteDatagram([]byte("this is stuff")), ShouldBeNil)
				step.End(nil)
				So(logs, memlogger.ShouldHaveLog, logging.Warning, "dropping DATAGRAM log \"a log\"")
			})
		})
	})
}

func TestStepLog(t *testing.T) {
	Convey(`Step logging`, t, func() {
		scFake, lc := streamclient.NewUnregisteredFake("fakeNS")
		ctx, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
		build := &bbpb.Build{
			Infra: &bbpb.BuildInfra{
				Logdog: &bbpb.BuildInfra_LogDog{
					Hostname: "logs.chromium.org",
					Project:  "example",
					Prefix:   "builds/8888888888",
				},
			},
		}
		buildState, ctx, err := Start(ctx, build, OptLogsink(lc))
		So(err, ShouldBeNil)
		defer func() { buildState.End(nil) }()
		So(buildState, ShouldNotBeNil)

		Convey(`logging redirects`, func() {
			step, ctx := StartStep(ctx, "some step")
			logging.Infof(ctx, "hi there!")
			step.End(nil)

			So(step.stepPb, assertions.ShouldResembleProto, &bbpb.Step{
				Name:      "some step",
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				EndTime:   timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_SUCCESS,
				Logs: []*bbpb.Log{
					{Name: "log", Url: "step/0/log/0"},
				},
			})

			So(scFake.Data()["fakeNS/step/0/log/0"].GetStreamData(), ShouldContainSubstring, "hi there!")
		})

		Convey(`can open logs`, func() {
			step, _ := StartStep(ctx, "some step")
			log := step.Log("some log")
			fmt.Fprintln(log, "here's some stuff")
			step.End(nil)

			So(step.stepPb, assertions.ShouldResembleProto, &bbpb.Step{
				Name:      "some step",
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				EndTime:   timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_SUCCESS,
				Logs: []*bbpb.Log{
					// log link is removed because logging is not used for this step.
					// {Name: "log", Url: "step/0/log/0"},
					{Name: "some log", Url: "step/0/log/1"},
				},
			})

			So(scFake.Data()["fakeNS/step/0/log/1"].GetStreamData(), ShouldContainSubstring, "here's some stuff")

			// Check the link.
			wantLink := "https://logs.chromium.org/logs/example/builds/8888888888/+/fakeNS/step/0/log/1"
			So(log.UILink(), ShouldEqual, wantLink)
		})

		Convey(`child log context is correct`, func() {
			step, ctx := StartStep(ctx, "parent")
			defer step.End(nil)
			logging.Infof(ctx, "I am on the parent")

			// We use step.StartStep here specifically to make sure that logdog
			// namespace and logging propagate correctly to childCtx even when not
			// pulling current step from `ctx`.
			child, childCtx := step.StartStep(ctx, "child")
			defer child.End(nil)
			logging.Infof(childCtx, "I am on the child")

			So(environ.FromCtx(ctx).Get("LOGDOG_NAMESPACE"), ShouldResemble, "fakeNS/step/0/u")
			So(environ.FromCtx(childCtx).Get("LOGDOG_NAMESPACE"), ShouldResemble, "fakeNS/step/1/u")

			So(scFake.Data()["fakeNS/step/0/log/0"].GetStreamData(), ShouldContainSubstring, "I am on the parent")
			So(scFake.Data()["fakeNS/step/1/log/0"].GetStreamData(), ShouldContainSubstring, "I am on the child")
		})

		Convey(`can open datagram logs`, func() {
			step, _ := StartStep(ctx, "some step")
			log := step.LogDatagram("some log")
			log.WriteDatagram([]byte("here's some stuff"))
			step.End(nil)

			So(step.stepPb, assertions.ShouldResembleProto, &bbpb.Step{
				Name:      "some step",
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				EndTime:   timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_SUCCESS,
				Logs: []*bbpb.Log{
					// log link is removed because logging is not used for this step.
					// {Name: "log", Url: "step/0/log/0"},
					{Name: "some log", Url: "step/0/log/1"},
				},
			})

			So(scFake.Data()["fakeNS/step/0/log/1"].GetDatagrams(), ShouldContain, "here's some stuff")
		})

		Convey(`sets LOGDOG_NAMESPACE`, func() {
			_, subCtx := StartStep(ctx, "some step")
			So(environ.FromCtx(subCtx).Get(luciexe.LogdogNamespaceEnv), ShouldEqual, "fakeNS/step/0/u")

			_, subSubCtx := StartStep(ctx, "some sub step")
			So(environ.FromCtx(subSubCtx).Get(luciexe.LogdogNamespaceEnv), ShouldEqual, "fakeNS/step/1/u")
		})
	})
}
