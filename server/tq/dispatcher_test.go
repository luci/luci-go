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

package tq

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/tq/internal/metrics"
	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"
	"go.chromium.org/luci/server/tq/tqtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAddTask(t *testing.T) {
	t.Parallel()

	Convey("With dispatcher", t, func() {
		var now = time.Unix(1442540000, 0)

		ctx, _ := testclock.UseTime(context.Background(), now)
		submitter := &submitter{}
		ctx = UseSubmitter(ctx, submitter)

		d := Dispatcher{
			CloudProject:      "proj",
			CloudRegion:       "reg",
			DefaultTargetHost: "example.com",
			PushAs:            "push-as@example.com",
		}

		d.RegisterTaskClass(TaskClass{
			ID:        "test-dur",
			Prototype: &durationpb.Duration{}, // just some proto type
			Kind:      NonTransactional,
			Queue:     "queue-1",
		})

		task := &Task{
			Payload: durationpb.New(10 * time.Second),
			Title:   "hi",
			Delay:   123 * time.Second,
		}
		expectedPayload := []byte(`{
	"class": "test-dur",
	"type": "google.protobuf.Duration",
	"body": "10s"
}`)

		expectedScheduleTime := timestamppb.New(now.Add(123 * time.Second))
		expectedHeaders := defaultHeaders()
		expectedHeaders[ExpectedETAHeader] = fmt.Sprintf("%d.%06d", expectedScheduleTime.GetSeconds(), expectedScheduleTime.GetNanos()/1000)

		Convey("Nameless HTTP task", func() {
			So(d.AddTask(ctx, task), ShouldBeNil)

			So(submitter.reqs, ShouldHaveLength, 1)
			So(submitter.reqs[0].CreateTaskRequest, ShouldResembleProto, &taskspb.CreateTaskRequest{
				Parent: "projects/proj/locations/reg/queues/queue-1",
				Task: &taskspb.Task{
					ScheduleTime: expectedScheduleTime,
					MessageType: &taskspb.Task_HttpRequest{
						HttpRequest: &taskspb.HttpRequest{
							HttpMethod: taskspb.HttpMethod_POST,
							Url:        "https://example.com/internal/tasks/t/test-dur/hi",
							Headers:    expectedHeaders,
							Body:       expectedPayload,
							AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
								OidcToken: &taskspb.OidcToken{
									ServiceAccountEmail: "push-as@example.com",
								},
							},
						},
					},
				},
			})
		})

		Convey("HTTP task with no delay", func() {
			task.Delay = 0
			So(d.AddTask(ctx, task), ShouldBeNil)

			// See `var now = ...` above.
			expectedHeaders[ExpectedETAHeader] = "1442540000.000000"

			So(submitter.reqs, ShouldHaveLength, 1)
			So(submitter.reqs[0].CreateTaskRequest, ShouldResembleProto, &taskspb.CreateTaskRequest{
				Parent: "projects/proj/locations/reg/queues/queue-1",
				Task: &taskspb.Task{
					MessageType: &taskspb.Task_HttpRequest{
						HttpRequest: &taskspb.HttpRequest{
							HttpMethod: taskspb.HttpMethod_POST,
							Url:        "https://example.com/internal/tasks/t/test-dur/hi",
							Headers:    expectedHeaders,
							Body:       expectedPayload,
							AuthorizationHeader: &taskspb.HttpRequest_OidcToken{
								OidcToken: &taskspb.OidcToken{
									ServiceAccountEmail: "push-as@example.com",
								},
							},
						},
					},
				},
			})
		})

		Convey("Nameless GAE task", func() {
			d.GAE = true
			d.DefaultTargetHost = ""
			So(d.AddTask(ctx, task), ShouldBeNil)

			So(submitter.reqs, ShouldHaveLength, 1)
			expectedScheduleTime := timestamppb.New(now.Add(123 * time.Second))

			So(submitter.reqs[0].CreateTaskRequest, ShouldResembleProto, &taskspb.CreateTaskRequest{
				Parent: "projects/proj/locations/reg/queues/queue-1",
				Task: &taskspb.Task{
					ScheduleTime: expectedScheduleTime,
					MessageType: &taskspb.Task_AppEngineHttpRequest{
						AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
							HttpMethod:  taskspb.HttpMethod_POST,
							RelativeUri: "/internal/tasks/t/test-dur/hi",
							Headers:     expectedHeaders,
							Body:        expectedPayload,
						},
					},
				},
			})
		})

		Convey("Named task", func() {
			task.DeduplicationKey = "key"

			So(d.AddTask(ctx, task), ShouldBeNil)

			So(submitter.reqs, ShouldHaveLength, 1)
			So(submitter.reqs[0].CreateTaskRequest.Task.Name, ShouldEqual,
				"projects/proj/locations/reg/queues/queue-1/tasks/"+
					"ca0a124846df4b453ae63e3ad7c63073b0d25941c6e63e5708fd590c016edcef")
		})

		Convey("Titleless task", func() {
			task.Title = ""

			So(d.AddTask(ctx, task), ShouldBeNil)

			So(submitter.reqs, ShouldHaveLength, 1)
			So(
				submitter.reqs[0].CreateTaskRequest.Task.MessageType.(*taskspb.Task_HttpRequest).HttpRequest.Url,
				ShouldEqual,
				"https://example.com/internal/tasks/t/test-dur",
			)
		})

		Convey("Transient err", func() {
			submitter.err = func(title string) error {
				return status.Errorf(codes.Internal, "boo, go away")
			}
			err := d.AddTask(ctx, task)
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("Fatal err", func() {
			submitter.err = func(title string) error {
				return status.Errorf(codes.PermissionDenied, "boo, go away")
			}
			err := d.AddTask(ctx, task)
			So(err, ShouldNotBeNil)
			So(transient.Tag.In(err), ShouldBeFalse)
		})

		Convey("Unknown payload type", func() {
			err := d.AddTask(ctx, &Task{
				Payload: &timestamppb.Timestamp{},
			})
			So(err, ShouldErrLike, "no task class matching type")
			So(submitter.reqs, ShouldHaveLength, 0)
		})

		Convey("Bad task title: spaces", func() {
			task.Title = "No spaces please"
			err := d.AddTask(ctx, task)
			So(err, ShouldErrLike, "bad task title")
			So(err, ShouldErrLike, "must not contain spaces")
			So(submitter.reqs, ShouldHaveLength, 0)
		})

		Convey("Bad task title: too long", func() {
			task.Title = strings.Repeat("a", 2070)
			err := d.AddTask(ctx, task)
			So(err, ShouldErrLike, "bad task title")
			So(err, ShouldErrLike, `too long; must not exceed 2083 characters when combined with "/internal/tasks/t/test-dur"`)
			So(submitter.reqs, ShouldHaveLength, 0)
		})

		Convey("Custom task payload on GAE", func() {
			d.GAE = true
			d.DefaultTargetHost = ""
			d.RegisterTaskClass(TaskClass{
				ID:        "test-ts",
				Prototype: &timestamppb.Timestamp{}, // just some proto type
				Kind:      NonTransactional,
				Queue:     "queue-1",
				Custom: func(ctx context.Context, m proto.Message) (*CustomPayload, error) {
					ts := m.(*timestamppb.Timestamp)
					return &CustomPayload{
						Method:      "GET",
						Meta:        map[string]string{"k": "v"},
						RelativeURI: "/zzz",
						Body:        []byte(fmt.Sprintf("%d", ts.Seconds)),
					}, nil
				},
			})

			So(d.AddTask(ctx, &Task{
				Payload: &timestamppb.Timestamp{Seconds: 123},
				Delay:   444 * time.Second,
			}), ShouldBeNil)

			So(submitter.reqs, ShouldHaveLength, 1)
			st := timestamppb.New(now.Add(444 * time.Second))
			So(submitter.reqs[0].CreateTaskRequest, ShouldResembleProto, &taskspb.CreateTaskRequest{
				Parent: "projects/proj/locations/reg/queues/queue-1",
				Task: &taskspb.Task{
					ScheduleTime: st,
					MessageType: &taskspb.Task_AppEngineHttpRequest{
						AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
							HttpMethod:  taskspb.HttpMethod_GET,
							RelativeUri: "/zzz",
							Headers:     map[string]string{"k": "v", ExpectedETAHeader: fmt.Sprintf("%d.%06d", st.GetSeconds(), st.GetNanos()/1000)},
							Body:        []byte("123"),
						},
					},
				},
			})
		})
	})
}

func TestPushHandler(t *testing.T) {
	t.Parallel()

	Convey("With dispatcher", t, func() {
		var handlerErr error
		var handlerCb func(context.Context)

		d := Dispatcher{DisableAuth: true}
		ref := d.RegisterTaskClass(TaskClass{
			ID:        "test-1",
			Prototype: &emptypb.Empty{},
			Kind:      NonTransactional,
			Queue:     "queue",
			Handler: func(ctx context.Context, payload proto.Message) error {
				if handlerCb != nil {
					handlerCb(ctx)
				}
				return handlerErr
			},
		})

		var now = time.Unix(1442540100, 0)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx, _, _ = tsmon.WithFakes(ctx)
		tsmon.GetState(ctx).SetStore(store.NewInMemory(&target.Task{}))

		metric := func(m types.Metric, fieldVals ...any) any {
			return tsmon.GetState(ctx).Store().Get(ctx, m, time.Time{}, fieldVals)
		}

		metricDist := func(m types.Metric, fieldVals ...any) (count int64, sum float64) {
			val := metric(m, fieldVals...)
			if val != nil {
				So(val, ShouldHaveSameTypeAs, &distribution.Distribution{})
				count = val.(*distribution.Distribution).Count()
				sum = val.(*distribution.Distribution).Sum()
			}
			return
		}

		srv := router.New()
		d.InstallTasksRoutes(srv, "/pfx")

		call := func(body string, header http.Header) int {
			req := httptest.NewRequest("POST", "/pfx/ignored/part", strings.NewReader(body)).WithContext(ctx)
			req.Header = header
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			return rec.Result().StatusCode
		}

		Convey("Using class ID", func() {
			Convey("Success", func() {
				So(call(`{"class": "test-1", "body": {}}`, nil), ShouldEqual, 200)
			})
			Convey("Unknown", func() {
				So(call(`{"class": "unknown", "body": {}}`, nil), ShouldEqual, 404)
			})
		})

		Convey("Using type name", func() {
			Convey("Success", func() {
				So(call(`{"type": "google.protobuf.Empty", "body": {}}`, nil), ShouldEqual, 200)
			})
			Convey("Totally unknown", func() {
				So(call(`{"type": "unknown", "body": {}}`, nil), ShouldEqual, 404)
			})
			Convey("Not a registered task", func() {
				So(call(`{"type": "google.protobuf.Duration", "body": {}}`, nil), ShouldEqual, 404)
			})
		})

		Convey("Not a JSON body", func() {
			So(call(`blarg`, nil), ShouldEqual, 400)
		})

		Convey("Bad envelope", func() {
			So(call(`{}`, nil), ShouldEqual, 400)
		})

		Convey("Missing message body", func() {
			So(call(`{"class": "test-1"}`, nil), ShouldEqual, 400)
		})

		Convey("Bad message body", func() {
			So(call(`{"class": "test-1", "body": "huh"}`, nil), ShouldEqual, 400)
		})

		Convey("Handler fatal error", func() {
			handlerErr = errors.New("boo", Fatal)
			So(call(`{"class": "test-1", "body": {}}`, nil), ShouldEqual, 202)
		})

		Convey("Handler ignore error", func() {
			handlerErr = errors.New("boo", Ignore)
			So(call(`{"class": "test-1", "body": {}}`, nil), ShouldEqual, 204)
		})

		Convey("Handler transient error", func() {
			handlerErr = errors.New("boo", transient.Tag)
			So(call(`{"class": "test-1", "body": {}}`, nil), ShouldEqual, 500)
		})

		Convey("Handler non-fatal error", func() {
			handlerErr = errors.New("boo")
			So(call(`{"class": "test-1", "body": {}}`, nil), ShouldEqual, 429)
		})

		Convey("No handler", func() {
			ref.(*taskClassImpl).Handler = nil
			So(call(`{"class": "test-1", "body": {}}`, nil), ShouldEqual, 404)
		})

		Convey("Metrics work", func() {
			callWithHeaders := func(headers map[string]string) {
				hdr := make(http.Header)
				for k, v := range headers {
					hdr.Set(k, v)
				}
				So(call(`{"type": "google.protobuf.Empty", "body": {}}`, hdr), ShouldEqual, 200)
			}

			Convey("No ETA header", func() {
				const fakeDelayMS = 33

				handlerCb = func(ctx context.Context) {
					info := TaskExecutionInfo(ctx)
					So(info.ExecutionCount, ShouldEqual, 500)
					So(info.TaskID, ShouldEqual, "task-without-eta")
					So(info.expectedETA, ShouldBeZeroValue)
					So(info.submitterTraceContext, ShouldEqual, "zzz")
					clock.Get(ctx).(testclock.TestClock).Add(fakeDelayMS * time.Millisecond)
				}

				callWithHeaders(map[string]string{
					"X-CloudTasks-QueueName":          "some-q",
					"X-CloudTasks-TaskExecutionCount": "500",
					"X-CloudTasks-TaskName":           "task-without-eta",
					TraceContextHeader:                "zzz",
				})

				So(metric(metrics.ServerHandledCount, "test-1", "some-q", "OK", metrics.MaxRetryFieldValue), ShouldEqual, 1)

				durCount, durSum := metricDist(metrics.ServerDurationMS, "test-1", "some-q", "OK")
				So(durCount, ShouldEqual, 1)
				So(durSum, ShouldEqual, float64(fakeDelayMS))

				latCount, _ := metricDist(metrics.ServerTaskLatency, "test-1", "some-q", "OK", metrics.MaxRetryFieldValue)
				So(latCount, ShouldEqual, 0)
			})

			Convey("With ETA header", func() {
				var etaValue = time.Unix(1442540050, 1000)
				const fakeDelayMS = 33

				handlerCb = func(ctx context.Context) {
					info := TaskExecutionInfo(ctx)
					So(info.ExecutionCount, ShouldEqual, 5)
					So(info.TaskID, ShouldEqual, "task-with-eta")
					So(info.expectedETA.Equal(etaValue), ShouldBeTrue)
					clock.Get(ctx).(testclock.TestClock).Add(fakeDelayMS * time.Millisecond)
				}

				callWithHeaders(map[string]string{
					"X-CloudTasks-QueueName":          "some-q",
					"X-CloudTasks-TaskExecutionCount": "5",
					"X-CloudTasks-TaskName":           "task-with-eta",
					ExpectedETAHeader:                 "1442540050.000001",
				})

				latCount, latSum := metricDist(metrics.ServerTaskLatency, "test-1", "some-q", "OK", 5)
				So(latCount, ShouldEqual, 1)
				So(latSum, ShouldEqual, float64(now.Sub(etaValue).Milliseconds()+fakeDelayMS))
			})

			Convey("ServerRunning metric", func() {
				handlerCb = func(ctx context.Context) {
					d.ReportMetrics(ctx)
				}
				callWithHeaders(nil)

				// Was reported while the handler was running.
				So(metric(metrics.ServerRunning, "test-1"), ShouldEqual, 1)

				// Should report 0 now, since the handler is not running anymore.
				d.ReportMetrics(ctx)
				So(metric(metrics.ServerRunning, "test-1"), ShouldEqual, 0)
			})
		})
	})
}

func TestTransactionalEnqueue(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		var now = time.Unix(1442540000, 0)

		submitter := &submitter{}
		db := testutil.FakeDB{}
		d := Dispatcher{
			CloudProject:      "proj",
			CloudRegion:       "reg",
			DefaultTargetHost: "example.com",
			PushAs:            "push-as@example.com",
		}
		d.RegisterTaskClass(TaskClass{
			ID:        "test-dur",
			Prototype: &durationpb.Duration{}, // just some proto type
			Kind:      Transactional,
			Queue:     "queue-1",
		})

		ctx, tc := testclock.UseTime(context.Background(), now)
		ctx = UseSubmitter(ctx, submitter)
		txn := db.Inject(ctx)

		Convey("Happy path", func() {
			task := &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			}
			err := d.AddTask(txn, task)
			So(err, ShouldBeNil)

			// Created the reminder.
			So(db.AllReminders(), ShouldHaveLength, 1)
			rem := db.AllReminders()[0]

			// But didn't submitted the task yet.
			So(submitter.reqs, ShouldBeEmpty)

			// The defer will submit the task and wipe the reminder.
			db.ExecDefers(ctx)
			So(db.AllReminders(), ShouldBeEmpty)
			So(submitter.reqs, ShouldHaveLength, 1)
			req := submitter.reqs[0]

			// Make sure the reminder and the task look as expected.
			So(rem.ID, ShouldHaveLength, reminderKeySpaceBytes*2)
			So(rem.FreshUntil.Equal(now.Add(happyPathMaxDuration)), ShouldBeTrue)
			So(req.TaskClass, ShouldEqual, "test-dur")
			So(req.Created.Equal(now), ShouldBeTrue)
			So(req.Raw, ShouldEqual, task.Payload) // the exact same pointer
			So(req.CreateTaskRequest.Task.Name, ShouldEqual, "projects/proj/locations/reg/queues/queue-1/tasks/"+rem.ID)

			// The task request inside the reminder's raw payload is correct.
			remPayload, err := rem.DropPayload().Payload()
			So(err, ShouldBeNil)
			So(req.CreateTaskRequest, ShouldResembleProto, remPayload.CreateTaskRequest)
		})

		Convey("Fatal Submit error", func() {
			submitter.err = func(string) error { return status.Errorf(codes.PermissionDenied, "boom") }

			err := d.AddTask(txn, &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			})
			So(err, ShouldBeNil)

			So(db.AllReminders(), ShouldHaveLength, 1)
			db.ExecDefers(ctx)
			So(db.AllReminders(), ShouldBeEmpty)
		})

		Convey("Transient Submit error", func() {
			submitter.err = func(string) error { return status.Errorf(codes.Internal, "boom") }

			err := d.AddTask(txn, &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			})
			So(err, ShouldBeNil)

			So(db.AllReminders(), ShouldHaveLength, 1)
			db.ExecDefers(ctx)
			So(db.AllReminders(), ShouldHaveLength, 1)
		})

		Convey("Slow", func() {
			err := d.AddTask(txn, &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			})
			So(err, ShouldBeNil)

			tc.Add(happyPathMaxDuration + 1*time.Second)

			So(db.AllReminders(), ShouldHaveLength, 1)
			db.ExecDefers(ctx)
			So(db.AllReminders(), ShouldHaveLength, 1)
			So(submitter.reqs, ShouldBeEmpty)
		})
	})
}

func TestTesting(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		var epoch = testclock.TestRecentTimeUTC

		ctx, tc := testclock.UseTime(context.Background(), epoch)
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			if testclock.HasTags(t, tqtesting.ClockTag) {
				tc.Add(d)
			}
		})

		disp := Dispatcher{}
		ctx, sched := TestingContext(ctx, &disp)

		var success tqtesting.TaskList
		sched.TaskSucceeded = tqtesting.TasksCollector(&success)

		m := sync.Mutex{}
		etas := []time.Duration{}

		disp.RegisterTaskClass(TaskClass{
			ID:        "test-dur",
			Prototype: &durationpb.Duration{}, // just some proto type
			Kind:      NonTransactional,
			Queue:     "queue-1",
			Handler: func(ctx context.Context, msg proto.Message) error {
				m.Lock()
				etas = append(etas, clock.Now(ctx).Sub(epoch))
				m.Unlock()
				if clock.Now(ctx).Sub(epoch) < 3*time.Second {
					disp.AddTask(ctx, &Task{
						Payload: &durationpb.Duration{
							Seconds: msg.(*durationpb.Duration).Seconds + 1,
						},
						Delay: time.Second,
					})
				}
				return nil
			},
		})

		So(disp.AddTask(ctx, &Task{Payload: &durationpb.Duration{Seconds: 1}}), ShouldBeNil)
		sched.Run(ctx, tqtesting.StopWhenDrained())
		So(etas, ShouldResemble, []time.Duration{
			0, 1 * time.Second, 2 * time.Second, 3 * time.Second,
		})

		So(success, ShouldHaveLength, 4)
		So(success.Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
			&durationpb.Duration{Seconds: 1},
			&durationpb.Duration{Seconds: 2},
			&durationpb.Duration{Seconds: 3},
			&durationpb.Duration{Seconds: 4},
		})
	})
}

func TestPubSubEnqueue(t *testing.T) {
	t.Parallel()

	Convey("With dispatcher", t, func() {
		var epoch = testclock.TestRecentTimeUTC

		ctx, tc := testclock.UseTime(context.Background(), epoch)
		db := testutil.FakeDB{}

		disp := Dispatcher{Sweeper: NewInProcSweeper(InProcSweeperOptions{})}
		ctx, sched := TestingContext(ctx, &disp)

		disp.RegisterTaskClass(TaskClass{
			ID:        "test-dur",
			Prototype: &durationpb.Duration{}, // just some proto type
			Kind:      Transactional,
			Topic:     "topic-1",
			Custom: func(_ context.Context, msg proto.Message) (*CustomPayload, error) {
				return &CustomPayload{
					Meta: map[string]string{"a": "b"},
					Body: []byte(fmt.Sprintf("%d", msg.(*durationpb.Duration).Seconds)),
				}, nil
			},
		})

		So(disp.AddTask(db.Inject(ctx), &Task{Payload: &durationpb.Duration{Seconds: 1}}), ShouldBeNil)

		Convey("Happy path", func() {
			db.ExecDefers(ctx) // actually enqueue

			So(sched.Tasks(), ShouldHaveLength, 1)

			task := sched.Tasks()[0]
			So(task.Payload, ShouldResembleProto, &durationpb.Duration{Seconds: 1})
			So(task.Message, ShouldResembleProto, &pubsubpb.PubsubMessage{
				Data: []byte("1"),
				Attributes: map[string]string{
					"a":                     "b",
					"X-Luci-Tq-Reminder-Id": task.Message.Attributes["X-Luci-Tq-Reminder-Id"],
				},
			})
		})

		Convey("Unhappy path", func() {
			// Not enqueued, but have a reminder.
			So(sched.Tasks(), ShouldHaveLength, 0)
			So(db.AllReminders(), ShouldHaveLength, 1)

			// Make reminder sufficiently stale to be eligible for sweeping.
			tc.Add(5 * time.Minute)

			// Run the sweeper to enqueue from the reminder.
			So(disp.Sweep(db.Inject(ctx)), ShouldBeNil)

			// Have the task now!
			So(sched.Tasks(), ShouldHaveLength, 1)

			task := sched.Tasks()[0]
			So(task.Payload, ShouldBeNil) // not available on non-happy path
			So(task.Message, ShouldResembleProto, &pubsubpb.PubsubMessage{
				Data: []byte("1"),
				Attributes: map[string]string{
					"a":                     "b",
					"X-Luci-Tq-Reminder-Id": task.Message.Attributes["X-Luci-Tq-Reminder-Id"],
				},
			})
		})
	})
}

type submitter struct {
	err  func(title string) error
	m    sync.Mutex
	reqs []*reminder.Payload
}

func (s *submitter) Submit(ctx context.Context, req *reminder.Payload) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.reqs = append(s.reqs, req)
	if s.err == nil {
		return nil
	}
	return s.err(title(req))
}

func title(req *reminder.Payload) string {
	url := ""
	switch mt := req.CreateTaskRequest.Task.MessageType.(type) {
	case *taskspb.Task_HttpRequest:
		url = mt.HttpRequest.Url
	case *taskspb.Task_AppEngineHttpRequest:
		url = mt.AppEngineHttpRequest.RelativeUri
	}
	idx := strings.LastIndex(url, "/")
	return url[idx+1:]
}
