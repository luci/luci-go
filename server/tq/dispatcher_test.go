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
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/server/tq/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAddTask(t *testing.T) {
	t.Parallel()

	Convey("With dispatcher", t, func() {
		var now = time.Unix(1442540000, 0)

		ctx, _ := testclock.UseTime(context.Background(), now)
		submitter := &submitter{}

		d := Dispatcher{
			Submitter:         submitter,
			CloudProject:      "proj",
			CloudRegion:       "reg",
			DefaultTargetHost: "example.com",
			PushAs:            "push-as@example.com",
		}

		d.RegisterTaskClass(TaskClass{
			ID:        "test-dur",
			Prototype: &durationpb.Duration{}, // just some proto type
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

		Convey("Nameless HTTP task", func() {
			So(d.AddTask(ctx, task), ShouldBeNil)

			So(submitter.reqs, ShouldHaveLength, 1)
			So(submitter.reqs[0], ShouldResembleProto, &taskspb.CreateTaskRequest{
				Parent: "projects/proj/locations/reg/queues/queue-1",
				Task: &taskspb.Task{
					ScheduleTime: timestamppb.New(now.Add(123 * time.Second)),
					MessageType: &taskspb.Task_HttpRequest{
						HttpRequest: &taskspb.HttpRequest{
							HttpMethod: taskspb.HttpMethod_POST,
							Url:        "https://example.com/internal/tasks/t/test-dur/hi",
							Headers:    defaultHeaders,
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
			So(submitter.reqs[0], ShouldResembleProto, &taskspb.CreateTaskRequest{
				Parent: "projects/proj/locations/reg/queues/queue-1",
				Task: &taskspb.Task{
					ScheduleTime: timestamppb.New(now.Add(123 * time.Second)),
					MessageType: &taskspb.Task_AppEngineHttpRequest{
						AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
							HttpMethod:  taskspb.HttpMethod_POST,
							RelativeUri: "/internal/tasks/t/test-dur/hi",
							Headers:     defaultHeaders,
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
			So(submitter.reqs[0].Task.Name, ShouldEqual,
				"projects/proj/locations/reg/queues/queue-1/tasks/"+
					"ca0a124846df4b453ae63e3ad7c63073b0d25941c6e63e5708fd590c016edcef")
		})

		Convey("Titleless task", func() {
			task.Title = ""

			So(d.AddTask(ctx, task), ShouldBeNil)

			So(submitter.reqs, ShouldHaveLength, 1)
			So(
				submitter.reqs[0].Task.MessageType.(*taskspb.Task_HttpRequest).HttpRequest.Url,
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

		Convey("Custom task payload on GAE", func() {
			d.GAE = true
			d.DefaultTargetHost = ""
			d.RegisterTaskClass(TaskClass{
				ID:        "test-ts",
				Prototype: &timestamppb.Timestamp{}, // just some proto type
				Queue:     "queue-1",
				Custom: func(ctx context.Context, m proto.Message) (*CustomPayload, error) {
					ts := m.(*timestamppb.Timestamp)
					return &CustomPayload{
						Method:      "GET",
						Headers:     map[string]string{"k": "v"},
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
			So(submitter.reqs[0], ShouldResembleProto, &taskspb.CreateTaskRequest{
				Parent: "projects/proj/locations/reg/queues/queue-1",
				Task: &taskspb.Task{
					ScheduleTime: timestamppb.New(now.Add(444 * time.Second)),
					MessageType: &taskspb.Task_AppEngineHttpRequest{
						AppEngineHttpRequest: &taskspb.AppEngineHttpRequest{
							HttpMethod:  taskspb.HttpMethod_GET,
							RelativeUri: "/zzz",
							Headers:     map[string]string{"k": "v"},
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

		d := Dispatcher{NoAuth: true}
		ref := d.RegisterTaskClass(TaskClass{
			ID:        "test-1",
			Prototype: &emptypb.Empty{},
			Queue:     "queue",
			Handler: func(ctx context.Context, payload proto.Message) error {
				return handlerErr
			},
		})

		srv := router.New()
		d.InstallTasksRoutes(srv, "/pfx")

		call := func(body string) int {
			req := httptest.NewRequest("POST", "/pfx/ignored/part", strings.NewReader(body))
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)
			return rec.Result().StatusCode
		}

		Convey("Using class ID", func() {
			Convey("Success", func() {
				So(call(`{"class": "test-1", "body": {}}`), ShouldEqual, 200)
			})
			Convey("Unknown", func() {
				So(call(`{"class": "unknown", "body": {}}`), ShouldEqual, 202)
			})
		})

		Convey("Using type name", func() {
			Convey("Success", func() {
				So(call(`{"type": "google.protobuf.Empty", "body": {}}`), ShouldEqual, 200)
			})
			Convey("Totally unknown", func() {
				So(call(`{"type": "unknown", "body": {}}`), ShouldEqual, 202)
			})
			Convey("Not a registered task", func() {
				So(call(`{"type": "google.protobuf.Duration", "body": {}}`), ShouldEqual, 202)
			})
		})

		Convey("Not a JSON body", func() {
			So(call(`blarg`), ShouldEqual, 202)
		})

		Convey("Bad envelope", func() {
			So(call(`{}`), ShouldEqual, 202)
		})

		Convey("Missing message body", func() {
			So(call(`{"class": "test-1"}`), ShouldEqual, 202)
		})

		Convey("Bad message body", func() {
			So(call(`{"class": "test-1", "body": "huh"}`), ShouldEqual, 202)
		})

		Convey("Handler asks for retry", func() {
			handlerErr = errors.New("boo", Retry)
			So(call(`{"class": "test-1", "body": {}}`), ShouldEqual, 429)
		})

		Convey("Handler transient error", func() {
			handlerErr = errors.New("boo", transient.Tag)
			So(call(`{"class": "test-1", "body": {}}`), ShouldEqual, 500)
		})

		Convey("Handler fatal error", func() {
			handlerErr = errors.New("boo")
			So(call(`{"class": "test-1", "body": {}}`), ShouldEqual, 202)
		})

		Convey("No handler", func() {
			ref.(*taskClassImpl).Handler = nil
			So(call(`{"class": "test-1", "body": {}}`), ShouldEqual, 202)
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
			Submitter:         submitter,
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
		txn := db.Inject(ctx)

		Convey("Happy path", func() {
			err := d.AddTask(txn, &Task{
				Payload: durationpb.New(5 * time.Second),
				Delay:   10 * time.Second,
			})
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
			remReq := &taskspb.CreateTaskRequest{}
			So(proto.Unmarshal(rem.Payload, remReq), ShouldBeNil)
			So(req, ShouldResembleProto, remReq)
			So(rem.ID, ShouldHaveLength, reminderKeySpaceBytes*2)
			So(req.Task.Name, ShouldEqual, "projects/proj/locations/reg/queues/queue-1/tasks/"+rem.ID)
			So(rem.FreshUntil.Equal(now.Add(happyPathMaxDuration)), ShouldBeTrue)
		})

		Convey("Fatal CreateTask error", func() {
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

		Convey("Transient CreateTask error", func() {
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
		sched := disp.SchedulerForTest()

		var success tqtesting.TaskList
		sched.TaskSucceeded = tqtesting.TasksCollector(&success)

		m := sync.Mutex{}
		etas := []time.Duration{}

		disp.RegisterTaskClass(TaskClass{
			ID:        "test-dur",
			Prototype: &durationpb.Duration{}, // just some proto type
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
		So(success.Payloads(), ShouldResembleProto, []*durationpb.Duration{
			{Seconds: 1},
			{Seconds: 2},
			{Seconds: 3},
			{Seconds: 4},
		})
	})
}

type submitter struct {
	err  func(title string) error
	m    sync.Mutex
	reqs []*taskspb.CreateTaskRequest
}

func (s *submitter) CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest) error {
	s.m.Lock()
	defer s.m.Unlock()
	s.reqs = append(s.reqs, req)
	if s.err == nil {
		return nil
	}
	return s.err(title(req))
}

func (s *submitter) titles() []string {
	var t []string
	for _, r := range s.reqs {
		t = append(t, title(r))
	}
	sort.Strings(t)
	return t
}

func title(req *taskspb.CreateTaskRequest) string {
	url := ""
	switch mt := req.Task.MessageType.(type) {
	case *taskspb.Task_HttpRequest:
		url = mt.HttpRequest.Url
	case *taskspb.Task_AppEngineHttpRequest:
		url = mt.AppEngineHttpRequest.RelativeUri
	}
	idx := strings.LastIndex(url, "/")
	return url[idx+1:]
}
