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
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry/transient"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAddTask(t *testing.T) {
	t.Parallel()

	Convey("With dispacher", t, func() {
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
			Handler: func(ctx context.Context, payload proto.Message) error {
				panic("unused in this test")
			},
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
					"cd953b04d276a05bdd0846091e9f0171d4e32465add60314d65aef9ef5fded0b")
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
			So(err, ShouldErrLike, "no TaskClass registered")
			So(submitter.reqs, ShouldHaveLength, 0)
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
