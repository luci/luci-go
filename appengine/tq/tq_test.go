// Copyright 2017 The LUCI Authors.
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
	"bytes"
	"context"
	"fmt"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/server/router"
)

var epoch = time.Unix(1500000000, 0).UTC()

func TestDispatcher(t *testing.T) {
	t.Parallel()

	ftt.Run("With dispatcher", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = clock.Set(ctx, testclock.New(epoch))
		taskqueue.GetTestable(ctx).CreateQueue("another-q")

		d := Dispatcher{}
		r := router.New()

		installRoutes := func() {
			d.InstallRoutes(r, router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
				c.Request = c.Request.WithContext(ctx)
				next(c)
			}))
		}
		runTasks := func(ctx context.Context) []int {
			var codes []int
			for _, tasks := range taskqueue.GetTestable(ctx).GetScheduledTasks() {
				for _, task := range tasks {
					// Execute the task.
					req := httptest.NewRequest("POST", "http://example.com"+task.Path, bytes.NewReader(task.Payload))
					rw := httptest.NewRecorder()
					r.ServeHTTP(rw, req)
					codes = append(codes, rw.Code)
				}
			}
			return codes
		}

		t.Run("Single task", func(t *ftt.Test) {
			var calls []proto.Message
			handler := func(ctx context.Context, payload proto.Message) error {
				hdr, err := RequestHeaders(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, hdr, should.Match(&taskqueue.RequestHeaders{}))
				calls = append(calls, payload)
				return nil
			}

			// Abuse some well-known proto type to simplify the test. It's doesn't
			// matter what proto type we use here as long as it is registered in
			// protobuf type registry.
			d.RegisterTask(&durationpb.Duration{}, handler, "", nil)
			installRoutes()

			err := d.AddTask(ctx, &Task{
				Payload:          &durationpb.Duration{Seconds: 123},
				DeduplicationKey: "abc",
				NamePrefix:       "prefix",
				Title:            "abc-def",
				Delay:            30 * time.Second,
			})
			assert.Loosely(t, err, should.BeNil)

			// Added the task.
			expectedPath := "/internal/tasks/default/abc-def"
			expectedName := "prefix-afc6f8271b8598ee04e359916e6c584a9bc3c520a11dd5244e3399346ac0d3a7"
			expectedBody := []byte(`{"type":"google.protobuf.Duration","body":"123s"}`)
			tasks := taskqueue.GetTestable(ctx).GetScheduledTasks()
			assert.Loosely(t, tasks, should.Match(taskqueue.QueueData{
				"default": map[string]*taskqueue.Task{
					expectedName: {
						Path:    expectedPath,
						Payload: expectedBody,
						Name:    expectedName,
						Method:  "POST",
						ETA:     epoch.Add(30 * time.Second),
					},
				},
				"another-q": {},
			}))

			// Read a task with same dedup key. Should be silently ignored.
			err = d.AddTask(ctx, &Task{
				Payload:          &durationpb.Duration{Seconds: 123},
				DeduplicationKey: "abc",
				NamePrefix:       "prefix",
			})
			assert.Loosely(t, err, should.BeNil)

			// No new tasks.
			tasks = taskqueue.GetTestable(ctx).GetScheduledTasks()
			assert.Loosely(t, len(tasks["default"]), should.Match(1))

			t.Run("Executed", func(t *ftt.Test) {
				// Execute the task.
				assert.Loosely(t, runTasks(ctx), should.Match([]int{200}))
				assert.Loosely(t, calls, should.Match([]proto.Message{
					&durationpb.Duration{Seconds: 123},
				}))
			})

			t.Run("Deleted", func(t *ftt.Test) {
				assert.Loosely(t, d.DeleteTask(ctx, &Task{
					Payload:          &durationpb.Duration{Seconds: 123},
					DeduplicationKey: "abc",
					NamePrefix:       "prefix",
				}), should.BeNil)

				// Did not execute any tasks.
				assert.Loosely(t, runTasks(ctx), should.HaveLength(0))
				assert.Loosely(t, calls, should.HaveLength(0))
			})
		})

		t.Run("Deleting unknown task returns nil", func(t *ftt.Test) {
			handler := func(ctx context.Context, payload proto.Message) error { return nil }
			d.RegisterTask(&durationpb.Duration{}, handler, "default", nil)

			assert.Loosely(t, d.DeleteTask(ctx, &Task{
				Payload:          &durationpb.Duration{Seconds: 123},
				DeduplicationKey: "something",
			}), should.BeNil)
		})

		t.Run("Many tasks", func(t *ftt.Test) {
			handler := func(ctx context.Context, payload proto.Message) error { return nil }
			d.RegisterTask(&durationpb.Duration{}, handler, "default", nil)
			d.RegisterTask(&emptypb.Empty{}, handler, "another-q", nil)
			installRoutes()

			tasks := []*Task{}
			for i := range 200 {
				var task *Task

				if i%2 == 0 {
					task = &Task{
						DeduplicationKey: fmt.Sprintf("%d", i/2),
						Payload:          &durationpb.Duration{},
						Delay:            time.Duration(i) * time.Second,
					}
				} else {
					task = &Task{
						DeduplicationKey: fmt.Sprintf("%d", (i-1)/2),
						Payload:          &emptypb.Empty{},
						Delay:            time.Duration(i) * time.Second,
					}
				}

				tasks = append(tasks, task)

				// Mix in some duplicates.
				if i > 0 && i%100 == 0 {
					tasks = append(tasks, task)
				}
			}
			err := d.AddTask(ctx, tasks...)
			assert.Loosely(t, err, should.BeNil)

			// Added all the tasks.
			allTasks := taskqueue.GetTestable(ctx).GetScheduledTasks()
			delaysDefault := map[time.Duration]struct{}{}
			for _, task := range allTasks["default"] {
				delaysDefault[task.ETA.Sub(epoch)/time.Second] = struct{}{}
			}
			delaysAnotherQ := map[time.Duration]struct{}{}
			for _, task := range allTasks["another-q"] {
				delaysAnotherQ[task.ETA.Sub(epoch)/time.Second] = struct{}{}
			}
			assert.Loosely(t, len(delaysDefault), should.Equal(100))
			assert.Loosely(t, len(delaysAnotherQ), should.Equal(100))

			// Delete the tasks.
			assert.Loosely(t, d.DeleteTask(ctx, tasks...), should.BeNil)
			assert.Loosely(t, runTasks(ctx), should.HaveLength(0))
		})

		t.Run("Execution errors", func(t *ftt.Test) {
			var returnErr error
			panicNow := false
			handler := func(ctx context.Context, payload proto.Message) error {
				if panicNow {
					panic("must not be called")
				}
				return returnErr
			}

			d.RegisterTask(&durationpb.Duration{}, handler, "", nil)
			installRoutes()

			goodBody := `{"type":"google.protobuf.Duration","body":"123.000s"}`

			execute := func(body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(
					"POST",
					"http://example.com/internal/tasks/default/abc-def",
					bytes.NewReader([]byte(body)))
				rw := httptest.NewRecorder()
				r.ServeHTTP(rw, req)
				return rw
			}

			// Error conditions inside the task body.

			returnErr = nil
			rw := execute(goodBody)
			assert.Loosely(t, rw.Code, should.Equal(200))
			assert.Loosely(t, rw.Body.String(), should.Equal("OK\n"))

			returnErr = fmt.Errorf("fatal err")
			rw = execute(goodBody)
			assert.Loosely(t, rw.Code, should.Equal(202)) // no retry!
			assert.Loosely(t, rw.Body.String(), should.Equal("Fatal error: fatal err\n"))

			// 500 for retry on transient errors.
			returnErr = errors.New("transient err", transient.Tag)
			rw = execute(goodBody)
			assert.Loosely(t, rw.Code, should.Equal(500))
			assert.Loosely(t, rw.Body.String(), should.Equal("Transient error: transient err\n"))

			// 409 for retry on Retry-tagged errors. Retry tag trumps transient.Tag.
			returnErr = errors.New("retry me", transient.Tag, Retry)
			rw = execute(goodBody)
			assert.Loosely(t, rw.Code, should.Equal(409))
			assert.Loosely(t, rw.Body.String(), should.Equal("The handler asked for retry: retry me\n"))

			// Error conditions when routing the task.

			panicNow = true

			rw = execute("not a json")
			assert.Loosely(t, rw.Code, should.Equal(202)) // no retry!
			assert.Loosely(t, rw.Body.String(), should.HavePrefix("Bad payload, can't deserialize"))

			rw = execute(`{"type":"google.protobuf.Duration"}`)
			assert.Loosely(t, rw.Code, should.Equal(202)) // no retry!
			assert.Loosely(t, rw.Body.String(), should.HavePrefix("Bad payload, can't deserialize"))

			rw = execute(`{"type":"google.protobuf.Duration","body":"blah"}`)
			assert.Loosely(t, rw.Code, should.Equal(202)) // no retry!
			assert.Loosely(t, rw.Body.String(), should.HavePrefix("Bad payload, can't deserialize"))

			rw = execute(`{"type":"unknown.proto.type","body":"{}"}`)
			assert.Loosely(t, rw.Code, should.Equal(202)) // no retry!
			assert.Loosely(t, rw.Body.String(), should.HavePrefix("Bad payload, can't deserialize"))
		})

		t.Run("GetQueues", func(t *ftt.Test) {
			// Never called.
			handler := func(ctx context.Context, payload proto.Message) error {
				panic("handler was called in GetQueues")
			}

			t.Run("empty queue name", func(t *ftt.Test) {
				d.RegisterTask(&durationpb.Duration{}, handler, "", nil)
				assert.Loosely(t, d.GetQueues(), should.Match([]string{"default"}))
			})

			t.Run("multiple queue names", func(t *ftt.Test) {
				d.RegisterTask(&durationpb.Duration{}, handler, "default", nil)
				d.RegisterTask(&emptypb.Empty{}, handler, "another", nil)
				queues := d.GetQueues()
				sort.Strings(queues)
				assert.Loosely(t, queues, should.Match([]string{"another", "default"}))
			})

			t.Run("duplicated queue names", func(t *ftt.Test) {
				d.RegisterTask(&durationpb.Duration{}, handler, "default", nil)
				d.RegisterTask(&emptypb.Empty{}, handler, "default", nil)
				queues := d.GetQueues()
				sort.Strings(queues)
				assert.Loosely(t, queues, should.Match([]string{"default"}))
			})
		})
	})
}
