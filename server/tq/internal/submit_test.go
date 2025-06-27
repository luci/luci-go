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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"
)

func TestSubmit(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
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
			assert.Loosely(t, db.SaveReminder(ctx, r), should.BeNil)
			return r
		}

		call := func(r *reminder.Reminder) error {
			return SubmitFromReminder(ctx, &sub, &db, r, TxnPathHappy)
		}

		r := makeRem("rem")
		assert.Loosely(t, db.AllReminders(), should.HaveLength(1))

		t.Run("Success, have payload deserialized", func(t *ftt.Test) {
			assert.Loosely(t, call(r), should.BeNil)
			assert.Loosely(t, db.AllReminders(), should.HaveLength(0))
			assert.Loosely(t, sub.req, should.HaveLength(1))
			assert.Loosely(t, sub.req[0].CreateTaskRequest.Parent, should.Equal("rem body"))
		})

		t.Run("Success, from raw reminder payload", func(t *ftt.Test) {
			assert.Loosely(t, call(r.DropPayload()), should.BeNil)
			assert.Loosely(t, db.AllReminders(), should.HaveLength(0))
			assert.Loosely(t, sub.req, should.HaveLength(1))
			assert.Loosely(t, sub.req[0].CreateTaskRequest.Parent, should.Equal("rem body"))
		})

		t.Run("Transient err", func(t *ftt.Test) {
			sub.err = status.Errorf(codes.Internal, "boo")
			assert.Loosely(t, call(r), should.NotBeNil)
			assert.Loosely(t, db.AllReminders(), should.HaveLength(1)) // kept it
		})

		t.Run("Fatal err", func(t *ftt.Test) {
			sub.err = status.Errorf(codes.PermissionDenied, "boo")
			assert.Loosely(t, call(r), should.NotBeNil)
			assert.Loosely(t, db.AllReminders(), should.HaveLength(0)) // deleted it
		})

		t.Run("Batch", func(t *ftt.Test) {
			for i := range 5 {
				makeRem(fmt.Sprintf("more-%d", i))
			}

			batch := db.AllReminders()
			for _, r := range batch {
				r.RawPayload = nil
			}
			assert.Loosely(t, batch, should.HaveLength(6))

			// Reject `rem`, keep only `more-...`.
			sub.ban = stringset.NewFromSlice("rem body")

			n, err := SubmitBatch(ctx, &sub, &db, batch)
			assert.Loosely(t, err, should.NotBeNil) // had failing requests
			assert.Loosely(t, n, should.Equal(5))   // still submitted 5 tasks

			// Verify they had correct payloads.
			var bodies []string
			for _, r := range sub.req {
				bodies = append(bodies, r.CreateTaskRequest.Parent)
			}
			sort.Strings(bodies)
			assert.Loosely(t, bodies, should.Match([]string{
				"more-0 body", "more-1 body", "more-2 body", "more-3 body", "more-4 body",
			}))

			// All reminders are deleted, even the one that matches the failed task.
			assert.Loosely(t, db.AllReminders(), should.HaveLength(0))
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
