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
	"fmt"
	"sort"
	"sync"
	"testing"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSubmit(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		ctx := context.Background()
		db := testutil.FakeDB{}
		sub := submitter{}
		ctx = db.Inject(ctx)

		makeRem := func(id string) *reminder.Reminder {
			r := &reminder.Reminder{ID: id}
			r.AttachPayload(&reminder.Payload{
				CreateTaskRequest: &taskspb.CreateTaskRequest{
					Parent: id + " body",
				},
			})
			So(db.SaveReminder(ctx, r), ShouldBeNil)
			return r
		}

		call := func(r *reminder.Reminder) error {
			return SubmitFromReminder(ctx, &sub, &db, r, TxnPathHappy)
		}

		r := makeRem("rem")
		So(db.AllReminders(), ShouldHaveLength, 1)

		Convey("Success, have payload deserialized", func() {
			So(call(r), ShouldBeNil)
			So(db.AllReminders(), ShouldHaveLength, 0)
			So(sub.req, ShouldHaveLength, 1)
			So(sub.req[0].CreateTaskRequest.Parent, ShouldEqual, "rem body")
		})

		Convey("Success, from raw reminder payload", func() {
			So(call(r.DropPayload()), ShouldBeNil)
			So(db.AllReminders(), ShouldHaveLength, 0)
			So(sub.req, ShouldHaveLength, 1)
			So(sub.req[0].CreateTaskRequest.Parent, ShouldEqual, "rem body")
		})

		Convey("Transient err", func() {
			sub.err = status.Errorf(codes.Internal, "boo")
			So(call(r), ShouldNotBeNil)
			So(db.AllReminders(), ShouldHaveLength, 1) // kept it
		})

		Convey("Fatal err", func() {
			sub.err = status.Errorf(codes.PermissionDenied, "boo")
			So(call(r), ShouldNotBeNil)
			So(db.AllReminders(), ShouldHaveLength, 0) // deleted it
		})

		Convey("Batch", func() {
			for i := 0; i < 5; i++ {
				makeRem(fmt.Sprintf("more-%d", i))
			}

			batch := db.AllReminders()
			for _, r := range batch {
				r.RawPayload = nil
			}
			So(batch, ShouldHaveLength, 6)

			// Reject `rem`, keep only `more-...`.
			sub.ban = stringset.NewFromSlice("rem body")

			n, err := SubmitBatch(ctx, &sub, &db, batch)
			So(err, ShouldNotBeNil) // had failing requests
			So(n, ShouldEqual, 5)   // still submitted 5 tasks

			// Verify they had correct payloads.
			var bodies []string
			for _, r := range sub.req {
				bodies = append(bodies, r.CreateTaskRequest.Parent)
			}
			sort.Strings(bodies)
			So(bodies, ShouldResemble, []string{
				"more-0 body", "more-1 body", "more-2 body", "more-3 body", "more-4 body",
			})

			// All reminders are deleted, even the one that matches the failed task.
			So(db.AllReminders(), ShouldHaveLength, 0)
		})
	})
}

type submitter struct {
	m   sync.Mutex
	ban stringset.Set
	err error
	req []*reminder.Payload
}

func (s *submitter) Submit(ctx context.Context, req *reminder.Payload) error {
	s.m.Lock()
	defer s.m.Unlock()
	if s.ban.Has(req.CreateTaskRequest.Parent) {
		return status.Errorf(codes.PermissionDenied, "boom")
	}
	s.req = append(s.req, req)
	return s.err
}
