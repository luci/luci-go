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

package sweep

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"

	taskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/server/tq/internal/reminder"
	"go.chromium.org/luci/server/tq/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBatching(t *testing.T) {
	t.Parallel()

	Convey("With BatchProcessor", t, func() {
		ctx := context.Background()
		db := &testutil.FakeDB{}
		sub := &submitter{}

		p := BatchProcessor{
			Context:           ctx,
			DB:                db,
			Submitter:         sub,
			BatchSize:         3,
			ConcurrentBatches: 20,
		}

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

		var r []*reminder.Reminder
		for i := 0; i < 100; i++ {
			r = append(r, makeRem(fmt.Sprintf("rem-%d", i)))
		}
		So(db.AllReminders(), ShouldHaveLength, len(r))

		Convey("Works", func() {
			So(p.Start(), ShouldBeNil)
			p.Enqueue(ctx, r)
			So(p.Stop(), ShouldEqual, len(r))
			So(sub.req, ShouldHaveLength, len(r))
			So(db.AllReminders(), ShouldHaveLength, 0)
		})

		Convey("Waits for processing to finish", func() {
			p.BatchSize = 1000 // to make sure Enqueue doesn't block

			var stopped int32

			ch := make(chan struct{})
			sub.cb = func(*reminder.Payload) error {
				if atomic.LoadInt32(&stopped) == 1 {
					panic("processing while stopped")
				}
				ch <- struct{}{}
				return nil
			}

			So(p.Start(), ShouldBeNil)
			p.Enqueue(ctx, r)

			result := make(chan int, 1)
			go func() {
				result <- p.Stop()
				atomic.StoreInt32(&stopped, 1)
			}()

			for i := 0; i < len(r); i++ {
				<-ch
			}
			So(<-result, ShouldEqual, len(r))
		})

		Convey("Context cancel", func() {
			ctx, cancel := context.WithCancel(ctx)
			p.Context = ctx

			var stopped int32
			var count int32
			sub.cb = func(*reminder.Payload) error {
				if atomic.LoadInt32(&stopped) == 1 {
					panic("processing while stopped")
				}
				if count := atomic.AddInt32(&count, 1); count >= 50 {
					if count > 50 {
						return status.Errorf(codes.Canceled, "boo")
					}
					cancel()
				}
				return nil
			}

			p.Start()
			p.Enqueue(ctx, r)
			p.Stop()
			atomic.StoreInt32(&stopped, 1)

			So(sub.req, ShouldHaveLength, 50)
		})
	})
}
