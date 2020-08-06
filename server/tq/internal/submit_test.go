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

package internal

import (
	"context"
	"testing"

	taskspb "google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubmitFromReminder(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		ctx := context.Background()
		db := testutil.FakeDB{}
		sub := submitter{}
		ctx = db.Inject(ctx)

		r := &reminder.Reminder{ID: "rem"}
		r.Payload, _ = proto.Marshal(&taskspb.CreateTaskRequest{
			Parent: "from rem",
		})
		So(db.SaveReminder(ctx, r), ShouldBeNil)
		So(db.AllReminders(), ShouldHaveLength, 1)

		call := func(r *reminder.Reminder, req *taskspb.CreateTaskRequest) error {
			return SubmitFromReminder(ctx, &sub, &db, r, req)
		}

		Convey("Success, existing request", func() {
			So(call(r, &taskspb.CreateTaskRequest{Parent: "zzz"}), ShouldBeNil)
			So(db.AllReminders(), ShouldHaveLength, 0)
			So(sub.req, ShouldHaveLength, 1)
			So(sub.req[0].Parent, ShouldEqual, "zzz")
		})

		Convey("Success, from reminder", func() {
			So(call(r, nil), ShouldBeNil)
			So(db.AllReminders(), ShouldHaveLength, 0)
			So(sub.req, ShouldHaveLength, 1)
			So(sub.req[0].Parent, ShouldEqual, "from rem")
		})

		Convey("Transient err", func() {
			sub.err = status.Errorf(codes.Internal, "boo")
			So(call(r, nil), ShouldNotBeNil)
			So(db.AllReminders(), ShouldHaveLength, 1) // kept it
		})

		Convey("Fatal err", func() {
			sub.err = status.Errorf(codes.PermissionDenied, "boo")
			So(call(r, nil), ShouldNotBeNil)
			So(db.AllReminders(), ShouldHaveLength, 0) // deleted it
		})
	})
}

type submitter struct {
	err error
	req []*taskspb.CreateTaskRequest
}

func (s *submitter) CreateTask(ctx context.Context, req *taskspb.CreateTaskRequest) error {
	s.req = append(s.req, req)
	return s.err
}
