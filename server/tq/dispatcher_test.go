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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

func TestAddTask(t *testing.T) {
	t.Parallel()

	ftt.Run("With dispatcher", t, func(t *ftt.Test) {
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

		t.Run("Nameless HTTP task", func(t *ftt.Test) {
			assert.Loosely(t, d.AddTask(ctx, task), should.BeNil)

			assert.Loosely(t, submitter.reqs, should.HaveLength(1))
			assert.Loosely(t, submitter.reqs[0].CreateTaskRequest, should.Match(&taskspb.CreateTaskRequest{
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
			}))
		})

		t.Run("HTTP task with no delay", func(t *ftt.Test) {
			task.Delay = 0
			assert.Loosely(t, d.AddTask(ctx, task), should.BeNil)

			// See `var now = ...` above.
			expectedHeaders[ExpectedETAHeader] = "1442540000.000000"

			assert.Loosely(t, submitter.reqs, should.HaveLength(1))
			assert.Loosely(t, submitter.reqs[0].CreateTaskRequest, should.Match(&taskspb.CreateTaskRequest{
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
			}))
		})

		t.Run("Nameless GAE task", func(t *ftt.Test) {
			d.GAE = true
			d.DefaultTargetHost = ""
			assert.Loosely(t, d.AddTask(ctx, task), should.BeNil)

			assert.Loosely(t, submitter.reqs, should.HaveLength(1))
			expectedScheduleTime := timestamppb.New(now.Add(123 * time.Second))

			assert.Loosely(t, submitter.reqs[0].CreateTaskRequest, should.Match(&taskspb.CreateTaskRequest{
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
			}))
		})

		t.Run("Named task", func(t *ftt.Test) {
			task.DeduplicationKey = "key"

			assert.Loosely(t, d.AddTask(ctx, task), should.BeNil)

			assert.Loosely(t, submitter.reqs, should.HaveLength(1))
			assert.Loosely(t, submitter.reqs[0].CreateTaskRequest.Task.Name, should.Equal(
				"projects/proj/locations/reg/queues/queue-1/tasks/"+
					"ca0a124846df4b453ae63e3ad7c63073b0d25941c6e63e5708fd590c016edcef"))
		})

		t.Run("Titleless task", func(t *ftt.Test) {
			task.Title = ""

			assert.Loosely(t, d.AddTask(ctx, task), should.BeNil)

			assert.Loosely(t, submitter.reqs, should.HaveLength(1))
			assert.Loosely(t,
				submitter.reqs[0].CreateTaskRequest.Task.MessageType.(*taskspb.Task_HttpRequest).HttpRequest.Url,
				should.Equal(
					"https://example.com/internal/tasks/t/test-dur",
				))
		})

		t.Run("Transient err", func(t *ftt.Test) {
			submitter.err = func(title string) error {
				return status.Errorf(codes.Internal, "boo, go away")
			}
			err := d.AddTask(ctx, task)
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run("Fatal err", func(t *ftt.Test) {
			submitter.err = func(title string) error {
				return status.Errorf(codes.PermissionDenied, "boo, go away")
			}
			err := d.AddTask(ctx, task)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeFalse)
		})

		t.Run("Unknown payload type", func(t *ftt.Test) {
			err := d.AddTask(ctx, &Task{
				Payload: &timestamppb.Timestamp{},
			})
			assert.Loosely(t, err, should.ErrLike("no task class matching type"))
			assert.Loosely(t, submitter.reqs, should.HaveLength(0))
		})

		t.Run("Bad task title: spaces", func(t *ftt.Test) {
			task.Title = "No spaces please"
			err := d.AddTask(ctx, task)
			assert.Loosely(t, err, should.ErrLike("bad task title"))
			assert.Loosely(t, err, should.ErrLike("must not contain spaces"))
			assert.Loosely(t, submitter.reqs, should.HaveLength(0))
		})

		t.Run("Bad task title: too long", func(t *ftt.Test) {
			task.Title = strings.Repeat("a", 2070)
			err := d.AddTask(ctx, task)
			assert.Loosely(t, err, should.ErrLike("bad task title"))
			assert.Loosely(t, err, should.ErrLike(`too long; must not exceed 2083 characters when combined with "/internal/tasks/t/test-dur"`))
			assert.Loosely(t, submitter.reqs, should.HaveLength(0))
		})

		t.Run("QueuePicker", func(t *ftt.Test) {
			var queuePickerCB func(pb *timestamppb.Timestamp) (string, error)

			d.RegisterTaskClass(TaskClass{
				ID:        "test-ts",
				Prototype: &timestamppb.Timestamp{}, // just some proto type
				Kind:      NonTransactional,
				QueuePicker: func(ctx context.Context, task *Task) (string, error) {
					return queuePickerCB(task.Payload.(*timestamppb.Timestamp))
				},
			})

			t.Run("OK short", func(t *ftt.Test) {
				queuePickerCB = func(pb *timestamppb.Timestamp) (string, error) {
					assert.Loosely(t, pb.Seconds, should.Equal(1))
					return "short", nil
				}
				assert.Loosely(t, d.AddTask(ctx, &Task{Payload: &timestamppb.Timestamp{Seconds: 1}}), should.BeNil)
				assert.Loosely(t, submitter.reqs, should.HaveLength(1))
				assert.Loosely(t, submitter.reqs[0].CreateTaskRequest.Parent, should.Equal("projects/proj/locations/reg/queues/short"))
			})

			t.Run("OK long", func(t *ftt.Test) {
				queuePickerCB = func(*timestamppb.Timestamp) (string, error) {
					return "projects/zzz/locations/yyy/queues/long", nil
				}
				assert.Loosely(t, d.AddTask(ctx, &Task{Payload: &timestamppb.Timestamp{Seconds: 1}}), should.BeNil)
				assert.Loosely(t, submitter.reqs, should.HaveLength(1))
				assert.Loosely(t, submitter.reqs[0].CreateTaskRequest.Parent, should.Equal("projects/zzz/locations/yyy/queues/long"))
			})

			t.Run("Bad queue name", func(t *ftt.Test) {
				queuePickerCB = func(*timestamppb.Timestamp) (string, error) {
					return "", nil
				}
				err := d.AddTask(ctx, &Task{Payload: &timestamppb.Timestamp{Seconds: 1}})
				assert.Loosely(t, err, should.ErrLike("not a valid queue name"))
			})

			t.Run("Err", func(t *ftt.Test) {
				queuePickerCB = func(*timestamppb.Timestamp) (string, error) {
					return "zzz", errors.New("boom")
				}
				err := d.AddTask(ctx, &Task{Payload: &timestamppb.Timestamp{Seconds: 1}})
				assert.Loosely(t, err, should.ErrLike("boom"))
			})
		})

		t.Run("Custom task payload on GAE", func(t *ftt.Test) {
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

			assert.Loosely(t, d.AddTask(ctx, &Task{
				Payload: &timestamppb.Timestamp{Seconds: 123},
				Delay:   444 * time.Second,
			}), should.BeNil)

			assert.Loosely(t, submitter.reqs, should.HaveLength(1))
			st := timestamppb.New(now.Add(444 * time.Second))
			assert.Loosely(t, submitter.reqs[0].CreateTaskRequest, should.Match(&taskspb.CreateTaskRequest{
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
			}))
		})
	})
}

func TestPushHandler(t *testing.T) {
	t.Parallel()

	ftt.Run("With dispatcher", t, func(t *ftt.Test) {
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
			return tsmon.GetState(ctx).Store().Get(ctx, m, fieldVals)
		}

		metricDist := func(m types.Metric, fieldVals ...any) (count int64, sum float64) {
			val := metric(m, fieldVals...)
			if val != nil {
				assert.Loosely(t, val, should.HaveType[*distribution.Distribution])
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

		t.Run("Using class ID", func(t *ftt.Test) {
			t.Run("Success", func(t *ftt.Test) {
				assert.Loosely(t, call(`{"class": "test-1", "body": {}}`, nil), should.Equal(200))
			})
			t.Run("Unknown", func(t *ftt.Test) {
				assert.Loosely(t, call(`{"class": "unknown", "body": {}}`, nil), should.Equal(404))
			})
		})

		t.Run("Using type name", func(t *ftt.Test) {
			t.Run("Success", func(t *ftt.Test) {
				assert.Loosely(t, call(`{"type": "google.protobuf.Empty", "body": {}}`, nil), should.Equal(200))
			})
			t.Run("Totally unknown", func(t *ftt.Test) {
				assert.Loosely(t, call(`{"type": "unknown", "body": {}}`, nil), should.Equal(404))
			})
			t.Run("Not a registered task", func(t *ftt.Test) {
				assert.Loosely(t, call(`{"type": "google.protobuf.Duration", "body": {}}`, nil), should.Equal(404))
			})
		})

		t.Run("Not a JSON body", func(t *ftt.Test) {
			assert.Loosely(t, call(`blarg`, nil), should.Equal(400))
		})

		t.Run("Bad envelope", func(t *ftt.Test) {
			assert.Loosely(t, call(`{}`, nil), should.Equal(400))
		})

		t.Run("Missing message body", func(t *ftt.Test) {
			assert.Loosely(t, call(`{"class": "test-1"}`, nil), should.Equal(400))
		})

		t.Run("Bad message body", func(t *ftt.Test) {
			assert.Loosely(t, call(`{"class": "test-1", "body": "huh"}`, nil), should.Equal(400))
		})

		t.Run("Handler fatal error", func(t *ftt.Test) {
			handlerErr = errors.New("boo", Fatal)
			assert.Loosely(t, call(`{"class": "test-1", "body": {}}`, nil), should.Equal(202))
		})

		t.Run("Handler ignore error", func(t *ftt.Test) {
			handlerErr = errors.New("boo", Ignore)
			assert.Loosely(t, call(`{"class": "test-1", "body": {}}`, nil), should.Equal(204))
		})

		t.Run("Handler transient error", func(t *ftt.Test) {
			handlerErr = errors.New("boo", transient.Tag)
			assert.Loosely(t, call(`{"class": "test-1", "body": {}}`, nil), should.Equal(500))
		})

		t.Run("Handler non-fatal error", func(t *ftt.Test) {
			handlerErr = errors.New("boo")
			assert.Loosely(t, call(`{"class": "test-1", "body": {}}`, nil), should.Equal(429))
		})

		t.Run("No handler", func(t *ftt.Test) {
			ref.(*taskClassImpl).Handler = nil
			assert.Loosely(t, call(`{"class": "test-1", "body": {}}`, nil), should.Equal(404))
		})

		t.Run("Metrics work", func(t *ftt.Test) {
			callWithHeaders := func(headers map[string]string) {
				hdr := make(http.Header)
				for k, v := range headers {
					hdr.Set(k, v)
				}
				assert.Loosely(t, call(`{"type": "google.protobuf.Empty", "body": {}}`, hdr), should.Equal(200))
			}

			t.Run("No ETA header", func(t *ftt.Test) {
				const fakeDelayMS = 33

				handlerCb = func(ctx context.Context) {
					info := TaskExecutionInfo(ctx)
					assert.Loosely(t, info.ExecutionCount, should.Equal(500))
					assert.Loosely(t, info.TaskID, should.Equal("task-without-eta"))
					assert.Loosely(t, info.expectedETA, should.BeZero)
					assert.Loosely(t, info.submitterTraceContext, should.Equal("zzz"))
					clock.Get(ctx).(testclock.TestClock).Add(fakeDelayMS * time.Millisecond)
				}

				callWithHeaders(map[string]string{
					"X-CloudTasks-QueueName":          "some-q",
					"X-CloudTasks-TaskExecutionCount": "500",
					"X-CloudTasks-TaskName":           "task-without-eta",
					TraceContextHeader:                "zzz",
				})

				assert.Loosely(t, metric(metrics.ServerHandledCount, "test-1", "some-q", "OK", metrics.MaxRetryFieldValue), should.Equal(1))

				durCount, durSum := metricDist(metrics.ServerDurationMS, "test-1", "some-q", "OK")
				assert.Loosely(t, durCount, should.Equal(1))
				assert.Loosely(t, durSum, should.Equal(float64(fakeDelayMS)))

				latCount, _ := metricDist(metrics.ServerTaskLatency, "test-1", "some-q", "OK", metrics.MaxRetryFieldValue)
				assert.Loosely(t, latCount, should.BeZero)
			})

			t.Run("With ETA header", func(t *ftt.Test) {
				var etaValue = time.Unix(1442540050, 1000)
				const fakeDelayMS = 33

				handlerCb = func(ctx context.Context) {
					info := TaskExecutionInfo(ctx)
					assert.Loosely(t, info.ExecutionCount, should.Equal(5))
					assert.Loosely(t, info.TaskID, should.Equal("task-with-eta"))
					assert.Loosely(t, info.expectedETA.Equal(etaValue), should.BeTrue)
					clock.Get(ctx).(testclock.TestClock).Add(fakeDelayMS * time.Millisecond)
				}

				callWithHeaders(map[string]string{
					"X-CloudTasks-QueueName":          "some-q",
					"X-CloudTasks-TaskExecutionCount": "5",
					"X-CloudTasks-TaskName":           "task-with-eta",
					ExpectedETAHeader:                 "1442540050.000001",
				})

				latCount, latSum := metricDist(metrics.ServerTaskLatency, "test-1", "some-q", "OK", 5)
				assert.Loosely(t, latCount, should.Equal(1))
				assert.Loosely(t, latSum, should.Equal(float64(now.Sub(etaValue).Milliseconds()+fakeDelayMS)))
			})

			t.Run("ServerRunning metric", func(t *ftt.Test) {
				handlerCb = func(ctx context.Context) {
					d.ReportMetrics(ctx)
				}
				callWithHeaders(nil)

				// Was reported while the handler was running.
				assert.Loosely(t, metric(metrics.ServerRunning, "test-1"), should.Equal(1))

				// Should report 0 now, since the handler is not running anymore.
				d.ReportMetrics(ctx)
				assert.Loosely(t, metric(metrics.ServerRunning, "test-1"), should.BeZero)
			})
		})
	})
}

func TestTransactionalEnqueue(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
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

		t.Run("Happy path", func(t *ftt.Test) {
			task := &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			}
			err := d.AddTask(txn, task)
			assert.Loosely(t, err, should.BeNil)

			// Created the reminder.
			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))
			rem := db.AllReminders()[0]

			// But didn't submitted the task yet.
			assert.Loosely(t, submitter.reqs, should.BeEmpty)

			// The defer will submit the task and wipe the reminder.
			db.ExecDefers(ctx)
			assert.Loosely(t, db.AllReminders(), should.BeEmpty)
			assert.Loosely(t, submitter.reqs, should.HaveLength(1))
			req := submitter.reqs[0]

			// Make sure the reminder and the task look as expected.
			assert.Loosely(t, rem.ID, should.HaveLength(reminderKeySpaceBytes*2))
			assert.Loosely(t, rem.FreshUntil.Equal(now.Add(happyPathMaxDuration)), should.BeTrue)
			assert.Loosely(t, req.TaskClass, should.Equal("test-dur"))
			assert.Loosely(t, req.Created.Equal(now), should.BeTrue)
			assert.Loosely(t, req.Raw, should.Equal(task.Payload)) // the exact same pointer
			assert.Loosely(t, req.CreateTaskRequest.Task.Name, should.Equal("projects/proj/locations/reg/queues/queue-1/tasks/"+rem.ID))

			// The task request inside the reminder's raw payload is correct.
			remPayload, err := rem.DropPayload().Payload()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, req.CreateTaskRequest, should.Match(remPayload.CreateTaskRequest))
		})

		t.Run("Fatal Submit error", func(t *ftt.Test) {
			submitter.err = func(string) error { return status.Errorf(codes.PermissionDenied, "boom") }

			err := d.AddTask(txn, &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			})
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))
			db.ExecDefers(ctx)
			assert.Loosely(t, db.AllReminders(), should.BeEmpty)
		})

		t.Run("Transient Submit error", func(t *ftt.Test) {
			submitter.err = func(string) error { return status.Errorf(codes.Internal, "boom") }

			err := d.AddTask(txn, &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			})
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))
			db.ExecDefers(ctx)
			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))
		})

		t.Run("Slow", func(t *ftt.Test) {
			err := d.AddTask(txn, &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			})
			assert.Loosely(t, err, should.BeNil)

			tc.Add(happyPathMaxDuration + 1*time.Second)

			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))
			db.ExecDefers(ctx)
			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))
			assert.Loosely(t, submitter.reqs, should.BeEmpty)
		})
	})
}

func TestTesting(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
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

		assert.Loosely(t, disp.AddTask(ctx, &Task{Payload: &durationpb.Duration{Seconds: 1}}), should.BeNil)
		sched.Run(ctx, tqtesting.StopWhenDrained())
		assert.Loosely(t, etas, should.Match([]time.Duration{
			0, 1 * time.Second, 2 * time.Second, 3 * time.Second,
		}))

		assert.Loosely(t, success, should.HaveLength(4))
		assert.Loosely(t, success.Payloads(), should.Match([]protoreflect.ProtoMessage{
			&durationpb.Duration{Seconds: 1},
			&durationpb.Duration{Seconds: 2},
			&durationpb.Duration{Seconds: 3},
			&durationpb.Duration{Seconds: 4},
		}))
	})
}

func TestPubSubEnqueue(t *testing.T) {
	t.Parallel()

	ftt.Run("With dispatcher", t, func(t *ftt.Test) {
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

		assert.Loosely(t, disp.AddTask(db.Inject(ctx), &Task{Payload: &durationpb.Duration{Seconds: 1}}), should.BeNil)

		t.Run("Happy path", func(t *ftt.Test) {
			db.ExecDefers(ctx) // actually enqueue

			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))

			task := sched.Tasks()[0]
			assert.Loosely(t, task.Payload, should.Match(&durationpb.Duration{Seconds: 1}))
			assert.Loosely(t, task.Message, should.Match(&pubsubpb.PubsubMessage{
				Data: []byte("1"),
				Attributes: map[string]string{
					"a":                     "b",
					"X-Luci-Tq-Reminder-Id": task.Message.Attributes["X-Luci-Tq-Reminder-Id"],
				},
			}))
		})

		t.Run("Unhappy path", func(t *ftt.Test) {
			// Not enqueued, but have a reminder.
			assert.Loosely(t, sched.Tasks(), should.HaveLength(0))
			assert.Loosely(t, db.AllReminders(), should.HaveLength(1))

			// Make reminder sufficiently stale to be eligible for sweeping.
			tc.Add(5 * time.Minute)

			// Run the sweeper to enqueue from the reminder.
			assert.Loosely(t, disp.Sweep(db.Inject(ctx)), should.BeNil)

			// Have the task now!
			assert.Loosely(t, sched.Tasks(), should.HaveLength(1))

			task := sched.Tasks()[0]
			assert.Loosely(t, task.Payload, should.BeNil) // not available on non-happy path
			assert.Loosely(t, task.Message, should.Match(&pubsubpb.PubsubMessage{
				Data: []byte("1"),
				Attributes: map[string]string{
					"a":                     "b",
					"X-Luci-Tq-Reminder-Id": task.Message.Attributes["X-Luci-Tq-Reminder-Id"],
				},
			}))
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
