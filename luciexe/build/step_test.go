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

	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"

	"go.chromium.org/luci/luciexe"
)

func TestStepNoop(t *testing.T) {
	ftt.Run(`Step no-op mode`, t, func(t *ftt.Test) {
		ctx := memlogger.Use(context.Background())
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		t.Run(`Step creation`, func(t *ftt.Test) {
			t.Run(`ScheduleStep`, func(t *ftt.Test) {
				step, ctx := ScheduleStep(ctx, "some step")
				defer func() { step.End(nil) }()

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(
					logging.Info, "set status: SCHEDULED", logging.Fields{"build.step": "some step"}))

				assert.Loosely(t, step, should.NotBeNil)
				assert.Loosely(t, getCurrentStep(ctx).name, should.Match("some step"))

				assert.Loosely(t, step.Start, should.NotPanic)
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "set status: STARTED"))
				assert.Loosely(t, logs.Messages(), should.HaveLength(2))

				assert.Loosely(t, step.Start, should.NotPanic) // noop
				assert.Loosely(t, logs.Messages(), should.HaveLength(2))
			})

			t.Run(`StartStep`, func(t *ftt.Test) {
				step, ctx := StartStep(ctx, "some step")
				defer func() { step.End(nil) }()

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "set status: SCHEDULED"))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "set status: STARTED"))

				assert.Loosely(t, step, should.NotBeNil)
				assert.Loosely(t, getCurrentStep(ctx).name, should.Match("some step"))
				assert.Loosely(t, logs.Messages(), should.HaveLength(2))

				assert.Loosely(t, step.Start, should.NotPanic) // noop
				assert.Loosely(t, logs.Messages(), should.HaveLength(2))
			})

			t.Run(`Bad step name`, func(t *ftt.Test) {
				assert.Loosely(t, func() {
					StartStep(ctx, "bad | step")
				}, should.PanicLike("reserved character"))
			})
		})

		t.Run(`Step closure`, func(t *ftt.Test) {
			t.Run(`SUCCESS`, func(t *ftt.Test) {
				step, ctx := StartStep(ctx, "some step")
				step.End(nil)
				assert.Loosely(t, step.stepPb.Status, should.Resemble(bbpb.Status_SUCCESS))

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "set status: SUCCESS"))

				t.Run(`cannot double-close`, func(t *ftt.Test) {
					assert.Loosely(t, func() { step.End(nil) }, should.PanicLike("cannot mutate ended step"))
				})

				t.Run(`cancels context as well`, func(t *ftt.Test) {
					assert.Loosely(t, ctx.Err(), should.Resemble(context.Canceled))
				})
			})

			t.Run(`error`, func(t *ftt.Test) {
				step, _ := StartStep(ctx, "some step")
				step.End(errors.New("bad stuff"))
				assert.Loosely(t, step.stepPb.Status, should.Resemble(bbpb.Status_FAILURE))

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "set status: FAILURE: bad stuff"))
			})

			t.Run(`CANCELED`, func(t *ftt.Test) {
				step, _ := StartStep(ctx, "some step")
				step.End(context.Canceled)
				assert.Loosely(t, step.stepPb.Status, should.Resemble(bbpb.Status_CANCELED))

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Warning, "set status: CANCELED: context canceled"))
			})

			t.Run(`panic`, func(t *ftt.Test) {
				step, _ := StartStep(ctx, "some step")
				func() {
					defer func() {
						step.End(nil)
						recover() // so testing assertions can happen
					}()
					panic("doom!")
				}()
				assert.Loosely(t, step.stepPb.Status, should.Resemble(bbpb.Status_INFRA_FAILURE))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Error, "set status: INFRA_FAILURE: PANIC"))
			})

			t.Run(`with SummaryMarkdown`, func(t *ftt.Test) {
				step, _ := StartStep(ctx, "some step")
				step.SetSummaryMarkdown("cool story!")
				step.End(nil)
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "set status: SUCCESS\n  with SummaryMarkdown:\ncool story!"))
			})

			t.Run(`modify starts step`, func(t *ftt.Test) {
				step, _ := ScheduleStep(ctx, "some step")
				defer func() { step.End(nil) }()
				step.SetSummaryMarkdown("cool story!")
				assert.Loosely(t, step.stepPb.Status, should.Resemble(bbpb.Status_STARTED))
			})

			t.Run(`closure of un-started step`, func(t *ftt.Test) {
				step, ctx := ScheduleStep(ctx, "some step")
				assert.Loosely(t, func() { step.End(nil) }, should.NotPanic)
				assert.Loosely(t, step.stepPb.Status, should.Resemble(bbpb.Status_SUCCESS))
				assert.Loosely(t, step.stepPb.StartTime, should.NotBeNil)
				assert.Loosely(t, ctx.Err(), should.Resemble(context.Canceled))
			})
		})

		t.Run(`Recursive steps`, func(t *ftt.Test) {
			t.Run(`basic`, func(t *ftt.Test) {
				parent, ctx := StartStep(ctx, "parent")
				defer func() { parent.End(nil) }()

				child, ctx := StartStep(ctx, "child")
				defer func() { child.End(nil) }()

				assert.Loosely(t, parent.name, should.Match("parent"))
				assert.Loosely(t, child.name, should.Match("parent|child"))
			})

			t.Run(`creating child step with explicit parent`, func(t *ftt.Test) {
				parent, ctx := ScheduleStep(ctx, "parent")
				defer func() { parent.End(nil) }()

				child, _ := parent.ScheduleStep(ctx, "child")
				defer func() { child.End(nil) }()

				assert.Loosely(t, parent.name, should.Match("parent"))
				assert.Loosely(t, parent.stepPb.Status, should.Resemble(bbpb.Status_STARTED))
				assert.Loosely(t, child.name, should.Match("parent|child"))
				assert.Loosely(t, child.stepPb.Status, should.Resemble(bbpb.Status_SCHEDULED))
			})

			t.Run(`creating child step starts parent`, func(t *ftt.Test) {
				parent, ctx := ScheduleStep(ctx, "parent")
				defer func() { parent.End(nil) }()

				child, ctx := ScheduleStep(ctx, "child")
				defer func() { child.End(nil) }()

				assert.Loosely(t, parent.name, should.Match("parent"))
				assert.Loosely(t, parent.stepPb.Status, should.Resemble(bbpb.Status_STARTED))
				assert.Loosely(t, child.name, should.Match("parent|child"))
				assert.Loosely(t, child.stepPb.Status, should.Resemble(bbpb.Status_SCHEDULED))
			})
		})

		t.Run(`Step logs`, func(t *ftt.Test) {
			ctx := memlogger.Use(ctx)
			logs := logging.Get(ctx).(*memlogger.MemLogger)
			step, _ := StartStep(ctx, "some step")

			t.Run(`text`, func(t *ftt.Test) {
				log := step.Log("a log")
				_, err := log.Write([]byte("this is stuff"))
				assert.Loosely(t, err, should.BeNil)
				step.End(nil)
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "this is stuff"))
				assert.Loosely(t, log.UILink(), should.BeEmpty)
			})

			t.Run(`binary`, func(t *ftt.Test) {
				log := step.Log("a log", streamclient.Binary())
				_, err := log.Write([]byte("this is stuff"))
				assert.Loosely(t, err, should.BeNil)
				step.End(nil)
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Warning, "dropping BINARY log \"a log\""))
				assert.Loosely(t, log.UILink(), should.BeEmpty)
			})

			t.Run(`datagram`, func(t *ftt.Test) {
				log := step.LogDatagram("a log")
				assert.Loosely(t, log.WriteDatagram([]byte("this is stuff")), should.BeNil)
				step.End(nil)
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Warning, "dropping DATAGRAM log \"a log\""))
			})
		})
	})
}

func TestStepLog(t *testing.T) {
	ftt.Run(`Step logging`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		defer func() { buildState.End(nil) }()
		assert.Loosely(t, buildState, should.NotBeNil)

		t.Run(`logging redirects`, func(t *ftt.Test) {
			step, ctx := StartStep(ctx, "some step")
			logging.Infof(ctx, "hi there!")
			step.End(nil)

			assert.Loosely(t, step.stepPb, should.Resemble(&bbpb.Step{
				Name:      "some step",
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				EndTime:   timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_SUCCESS,
				Logs: []*bbpb.Log{
					{Name: "log", Url: "step/0/log/0"},
				},
			}))

			assert.Loosely(t, scFake.Data()["fakeNS/step/0/log/0"].GetStreamData(), should.ContainSubstring("hi there!"))
		})

		t.Run(`can open logs`, func(t *ftt.Test) {
			step, _ := StartStep(ctx, "some step")
			log := step.Log("some log")
			fmt.Fprintln(log, "here's some stuff")
			step.End(nil)

			assert.Loosely(t, step.stepPb, should.Resemble(&bbpb.Step{
				Name:      "some step",
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				EndTime:   timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_SUCCESS,
				Logs: []*bbpb.Log{
					// log link is removed because logging is not used for this step.
					// {Name: "log", Url: "step/0/log/0"},
					{Name: "some log", Url: "step/0/log/1"},
				},
			}))

			assert.Loosely(t, scFake.Data()["fakeNS/step/0/log/1"].GetStreamData(), should.ContainSubstring("here's some stuff"))

			// Check the link.
			wantLink := "https://logs.chromium.org/logs/example/builds/8888888888/+/fakeNS/step/0/log/1"
			assert.Loosely(t, log.UILink(), should.Equal(wantLink))
		})

		t.Run(`child log context is correct`, func(t *ftt.Test) {
			step, ctx := StartStep(ctx, "parent")
			defer step.End(nil)
			logging.Infof(ctx, "I am on the parent")

			// We use step.StartStep here specifically to make sure that logdog
			// namespace and logging propagate correctly to childCtx even when not
			// pulling current step from `ctx`.
			child, childCtx := step.StartStep(ctx, "child")
			defer child.End(nil)
			logging.Infof(childCtx, "I am on the child")

			assert.Loosely(t, environ.FromCtx(ctx).Get("LOGDOG_NAMESPACE"), should.Match("fakeNS/step/0/u"))
			assert.Loosely(t, environ.FromCtx(childCtx).Get("LOGDOG_NAMESPACE"), should.Match("fakeNS/step/1/u"))

			assert.Loosely(t, scFake.Data()["fakeNS/step/0/log/0"].GetStreamData(), should.ContainSubstring("I am on the parent"))
			assert.Loosely(t, scFake.Data()["fakeNS/step/1/log/0"].GetStreamData(), should.ContainSubstring("I am on the child"))
		})

		t.Run(`can open datagram logs`, func(t *ftt.Test) {
			step, _ := StartStep(ctx, "some step")
			log := step.LogDatagram("some log")
			log.WriteDatagram([]byte("here's some stuff"))
			step.End(nil)

			assert.Loosely(t, step.stepPb, should.Resemble(&bbpb.Step{
				Name:      "some step",
				StartTime: timestamppb.New(testclock.TestRecentTimeUTC),
				EndTime:   timestamppb.New(testclock.TestRecentTimeUTC),
				Status:    bbpb.Status_SUCCESS,
				Logs: []*bbpb.Log{
					// log link is removed because logging is not used for this step.
					// {Name: "log", Url: "step/0/log/0"},
					{Name: "some log", Url: "step/0/log/1"},
				},
			}))

			assert.Loosely(t, scFake.Data()["fakeNS/step/0/log/1"].GetDatagrams(), should.Contain("here's some stuff"))
		})

		t.Run(`sets LOGDOG_NAMESPACE`, func(t *ftt.Test) {
			_, subCtx := StartStep(ctx, "some step")
			assert.Loosely(t, environ.FromCtx(subCtx).Get(luciexe.LogdogNamespaceEnv), should.Equal("fakeNS/step/0/u"))

			_, subSubCtx := StartStep(ctx, "some sub step")
			assert.Loosely(t, environ.FromCtx(subSubCtx).Get(luciexe.LogdogNamespaceEnv), should.Equal("fakeNS/step/1/u"))
		})
	})
}
