// Copyright 2015 The LUCI Authors.
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

package memory

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	tq "go.chromium.org/luci/gae/service/taskqueue"
)

func TestTaskQueue(t *testing.T) {
	t.Parallel()

	ftt.Run("TaskQueue", t, func(t *ftt.Test) {
		now := time.Date(2000, time.January, 1, 1, 1, 1, 1, time.UTC)
		c, tc := testclock.UseTime(context.Background(), now)
		c = mathrand.Set(c, rand.New(rand.NewSource(clock.Now(c).UnixNano())))
		c = Use(c)

		tqt := tq.GetTestable(c)
		assert.Loosely(t, tqt, should.NotBeNil)

		assert.Loosely(t, tq.Raw(c), should.NotBeNil)

		t.Run("implements TQMultiReadWriter", func(t *ftt.Test) {
			t.Run("Add", func(t *ftt.Test) {
				task := &tq.Task{Path: "/hello/world"}

				t.Run("works", func(t *ftt.Test) {
					task.Delay = 4 * time.Second
					task.Header = http.Header{}
					task.Header.Add("Cat", "tabby")
					task.Payload = []byte("watwatwat")
					task.RetryOptions = &tq.RetryOptions{AgeLimit: 7 * time.Second}
					assert.Loosely(t, tq.Add(c, "", task), should.BeNil)

					var scheduled *tq.Task
					for _, task := range tqt.GetScheduledTasks()["default"] {
						scheduled = task
						break
					}
					assert.Loosely(t, scheduled, should.Match(&tq.Task{
						ETA:          now.Add(4 * time.Second),
						Header:       http.Header{"Cat": []string{"tabby"}},
						Method:       "POST",
						Name:         "16045561405319332057",
						Path:         "/hello/world",
						Payload:      []byte("watwatwat"),
						RetryOptions: &tq.RetryOptions{AgeLimit: 7 * time.Second},
					}))
				})

				t.Run("picks up namespace", func(t *ftt.Test) {
					c, err := info.Namespace(c, "coolNamespace")
					assert.Loosely(t, err, should.BeNil)

					task := &tq.Task{}
					assert.Loosely(t, tq.Add(c, "", task), should.BeNil)
					assert.Loosely(t, task.Header, should.Match(http.Header{
						"X-Appengine-Current-Namespace": {"coolNamespace"},
					}))

					t.Run("namespaced Testable only returns tasks for that namespace", func(t *ftt.Test) {
						assert.Loosely(t, tq.GetTestable(c).GetScheduledTasks()["default"], should.HaveLength(1))

						// Default namespace has no tasks in its queue.
						assert.Loosely(t, tq.GetTestable(info.MustNamespace(c, "")).GetScheduledTasks()["default"], should.HaveLength(0))
					})
				})

				t.Run("cannot add to bad queues", func(t *ftt.Test) {
					assert.Loosely(t, tq.Add(c, "waaat", &tq.Task{}).Error(), should.ContainSubstring("UNKNOWN_QUEUE"))

					t.Run("but you can add Queues when testing", func(t *ftt.Test) {
						tqt.CreateQueue("waaat")
						assert.Loosely(t, tq.Add(c, "waaat", task), should.BeNil)

						t.Run("you just can't add them twice", func(t *ftt.Test) {
							assert.Loosely(t, func() { tqt.CreateQueue("waaat") }, should.Panic)
						})
					})
				})

				t.Run("supplies a URL if it's missing", func(t *ftt.Test) {
					task.Path = ""
					assert.Loosely(t, tq.Add(c, "", task), should.BeNil)
					assert.Loosely(t, task.Path, should.Equal("/_ah/queue/default"))
				})

				t.Run("cannot add twice", func(t *ftt.Test) {
					task.Name = "bob"
					assert.Loosely(t, tq.Add(c, "", task), should.BeNil)

					// can't add the same one twice!
					assert.Loosely(t, tq.Add(c, "", task), should.Equal(tq.ErrTaskAlreadyAdded))
				})

				t.Run("cannot add deleted task", func(t *ftt.Test) {
					task.Name = "bob"
					assert.Loosely(t, tq.Add(c, "", task), should.BeNil)

					assert.Loosely(t, tq.Delete(c, "", task), should.BeNil)

					// can't add a deleted task!
					assert.Loosely(t, tq.Add(c, "", task), should.Equal(tq.ErrTaskAlreadyAdded))
				})

				t.Run("must use a reasonable method", func(t *ftt.Test) {
					task.Method = "Crystal"
					assert.Loosely(t, tq.Add(c, "", task).Error(), should.ContainSubstring("bad method"))
				})

				t.Run("payload gets dumped for non POST/PUT methods", func(t *ftt.Test) {
					task.Method = "HEAD"
					task.Payload = []byte("coool")
					assert.Loosely(t, tq.Add(c, "", task), should.BeNil)
					assert.Loosely(t, task.Payload, should.BeNil)
				})

				t.Run("invalid names are rejected", func(t *ftt.Test) {
					task.Name = "happy times"
					assert.Loosely(t, tq.Add(c, "", task).Error(), should.ContainSubstring("INVALID_TASK_NAME"))
				})

				t.Run("AddMulti also works", func(t *ftt.Test) {
					t2 := task.Duplicate()
					t2.Path = "/hi/city"

					expect := []*tq.Task{task, t2}

					assert.Loosely(t, tq.Add(c, "default", expect...), should.BeNil)
					assert.Loosely(t, len(expect), should.Equal(2))
					assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.Equal(2))

					names := []string{"16045561405319332057", "16045561405319332058"}

					for i := range expect {
						t.Run(fmt.Sprintf("task %d: %s", i, expect[i].Path), func(t *ftt.Test) {
							assert.Loosely(t, expect[i].Method, should.Equal("POST"))
							assert.Loosely(t, expect[i].ETA, should.HappenOnOrBefore(now))
							assert.Loosely(t, expect[i].Name, should.Equal(names[i]))
						})
					}

					t.Run("stats work too", func(t *ftt.Test) {
						delay := -time.Second * 400

						task := &tq.Task{Path: "/somewhere"}
						task.Delay = delay
						assert.Loosely(t, tq.Add(c, "", task), should.BeNil)

						stats, err := tq.Stats(c, "")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, stats[0].Tasks, should.Equal(3))
						assert.Loosely(t, stats[0].OldestETA, should.HappenOnOrBefore(clock.Now(c).Add(delay)))

						_, err = tq.Stats(c, "noexist")
						assert.Loosely(t, err.Error(), should.ContainSubstring("UNKNOWN_QUEUE"))
					})

					t.Run("can purge all tasks", func(t *ftt.Test) {
						assert.Loosely(t, tq.Add(c, "", &tq.Task{Path: "/wut/nerbs"}), should.BeNil)
						assert.Loosely(t, tq.Purge(c, ""), should.BeNil)

						assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.BeZero)
						assert.Loosely(t, len(tqt.GetTombstonedTasks()["default"]), should.BeZero)
						assert.Loosely(t, len(tqt.GetTransactionTasks()["default"]), should.BeZero)

						t.Run("purging a queue which DNE fails", func(t *ftt.Test) {
							assert.Loosely(t, tq.Purge(c, "noexist").Error(), should.ContainSubstring("UNKNOWN_QUEUE"))
						})
					})

				})
			})

			t.Run("Delete", func(t *ftt.Test) {
				task := &tq.Task{Path: "/hello/world"}
				assert.Loosely(t, tq.Add(c, "", task), should.BeNil)

				t.Run("works", func(t *ftt.Test) {
					err := tq.Delete(c, "", task)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.BeZero)
					assert.Loosely(t, len(tqt.GetTombstonedTasks()["default"]), should.Equal(1))
					assert.Loosely(t, tqt.GetTombstonedTasks()["default"][task.Name], should.Match(task))
				})

				t.Run("cannot delete a task twice", func(t *ftt.Test) {
					assert.Loosely(t, tq.Delete(c, "", task), should.BeNil)

					assert.Loosely(t, tq.Delete(c, "", task).Error(), should.ContainSubstring("TOMBSTONED_TASK"))

					t.Run("but you can if you do a reset", func(t *ftt.Test) {
						tqt.ResetTasks()

						assert.Loosely(t, tq.Add(c, "", task), should.BeNil)
						assert.Loosely(t, tq.Delete(c, "", task), should.BeNil)
					})
				})

				t.Run("cannot delete from bogus queues", func(t *ftt.Test) {
					err := tq.Delete(c, "wat", task)
					assert.Loosely(t, err.Error(), should.ContainSubstring("UNKNOWN_QUEUE"))
				})

				t.Run("cannot delete a missing task", func(t *ftt.Test) {
					task.Name = "tarntioarenstyw"
					err := tq.Delete(c, "", task)
					assert.Loosely(t, err.Error(), should.ContainSubstring("UNKNOWN_TASK"))
				})

				t.Run("DeleteMulti also works", func(t *ftt.Test) {
					t2 := task.Duplicate()
					t2.Name = ""
					t2.Path = "/hi/city"
					assert.Loosely(t, tq.Add(c, "", t2), should.BeNil)

					t.Run("usually works", func(t *ftt.Test) {
						assert.Loosely(t, tq.Delete(c, "", task, t2), should.BeNil)
						assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.BeZero)
						assert.Loosely(t, len(tqt.GetTombstonedTasks()["default"]), should.Equal(2))
					})
				})
			})
		})

		t.Run("works with transactions", func(t *ftt.Test) {
			task := &tq.Task{Path: "/hello/world"}
			assert.Loosely(t, tq.Add(c, "", task), should.BeNil)

			t2 := &tq.Task{Path: "/hi/city"}
			assert.Loosely(t, tq.Add(c, "", t2), should.BeNil)

			assert.Loosely(t, tq.Delete(c, "", t2), should.BeNil)

			t.Run("can view regular tasks", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					tqt := tq.Raw(c).GetTestable()

					assert.Loosely(t, tqt.GetScheduledTasks()["default"][task.Name], should.Match(task))
					assert.Loosely(t, tqt.GetTombstonedTasks()["default"][t2.Name], should.Match(t2))
					assert.Loosely(t, tqt.GetTransactionTasks()["default"], should.BeNil)
					return nil
				}, nil), should.BeNil)
			})

			t.Run("can add a new task", func(t *ftt.Test) {
				t3 := &tq.Task{Path: "/sandwitch/victory"}

				err := ds.RunInTransaction(c, func(c context.Context) error {
					tqt := tq.GetTestable(c)

					assert.Loosely(t, tq.Add(c, "", t3), should.BeNil)
					assert.Loosely(t, t3.Name, should.Equal("16045561405319332059"))

					assert.Loosely(t, tqt.GetScheduledTasks()["default"][task.Name], should.Match(task))
					assert.Loosely(t, tqt.GetTombstonedTasks()["default"][t2.Name], should.Match(t2))
					assert.Loosely(t, tqt.GetTransactionTasks()["default"][0], should.Match(t3))
					return nil
				}, nil)
				assert.Loosely(t, err, should.BeNil)

				for _, tsk := range tqt.GetScheduledTasks()["default"] {
					if tsk.Name == task.Name {
						assert.Loosely(t, tsk, should.Match(task))
					} else {
						assert.Loosely(t, tsk, should.Match(t3))
					}
				}

				assert.Loosely(t, tqt.GetTombstonedTasks()["default"][t2.Name], should.Match(t2))
				assert.Loosely(t, tqt.GetTransactionTasks()["default"], should.BeNil)
			})

			t.Run("can add a new task (but reset the state in a test)", func(t *ftt.Test) {
				t3 := &tq.Task{Path: "/sandwitch/victory"}

				var txnCtx context.Context
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					txnCtx = c
					tqt := tq.GetTestable(c)

					assert.Loosely(t, tq.Add(c, "", t3), should.BeNil)

					assert.Loosely(t, tqt.GetScheduledTasks()["default"][task.Name], should.Match(task))
					assert.Loosely(t, tqt.GetTombstonedTasks()["default"][t2.Name], should.Match(t2))
					assert.Loosely(t, tqt.GetTransactionTasks()["default"][0], should.Match(t3))

					tqt.ResetTasks()

					assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.Equal(0))
					assert.Loosely(t, len(tqt.GetTombstonedTasks()["default"]), should.Equal(0))
					assert.Loosely(t, len(tqt.GetTransactionTasks()["default"]), should.Equal(0))

					return nil
				}, nil), should.BeNil)

				assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.BeZero)
				assert.Loosely(t, len(tqt.GetTombstonedTasks()["default"]), should.BeZero)
				assert.Loosely(t, len(tqt.GetTransactionTasks()["default"]), should.BeZero)

				t.Run("and reusing a closed context is bad times", func(t *ftt.Test) {
					assert.Loosely(t, tq.Add(txnCtx, "", nil).Error(), should.ContainSubstring("expired"))
				})
			})

			t.Run("you can AddMulti as well", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					tqt := tq.GetTestable(c)

					task.Name = ""
					tasks := []*tq.Task{task.Duplicate(), task.Duplicate(), task.Duplicate()}
					assert.Loosely(t, tq.Add(c, "", tasks...), should.BeNil)
					assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.Equal(1))
					assert.Loosely(t, len(tqt.GetTransactionTasks()["default"]), should.Equal(3))
					return nil
				}, nil), should.BeNil)
				assert.Loosely(t, len(tqt.GetScheduledTasks()["default"]), should.Equal(4))
				assert.Loosely(t, len(tqt.GetTransactionTasks()["default"]), should.BeZero)
			})

			t.Run("unless you add too many things", func(t *ftt.Test) {
				task.Name = ""

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					for i := 0; i < 5; i++ {
						assert.Loosely(t, tq.Add(c, "", task.Duplicate()), should.BeNil)
					}
					assert.Loosely(t, tq.Add(c, "", task).Error(), should.ContainSubstring("BAD_REQUEST"))
					return nil
				}, nil), should.BeNil)
			})

			t.Run("unless you Add to a bad queue", func(t *ftt.Test) {
				task.Name = ""

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, tq.Add(c, "meat", task).Error(), should.ContainSubstring("UNKNOWN_QUEUE"))

					t.Run("unless you add it!", func(t *ftt.Test) {
						tq.Raw(c).GetTestable().CreateQueue("meat")
						assert.Loosely(t, tq.Add(c, "meat", task), should.BeNil)
					})

					return nil
				}, nil), should.BeNil)
			})

			t.Run("unless the task is named", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					err := tq.Add(c, "", task) // Note: "t" has a Name from initial Add.
					assert.Loosely(t, err, should.NotBeNil)
					assert.Loosely(t, err.Error(), should.ContainSubstring("INVALID_TASK_NAME"))

					return nil
				}, nil), should.BeNil)
			})

			t.Run("No other features are available, however", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, tq.Delete(c, "", task).Error(), should.ContainSubstring("cannot DeleteMulti from a transaction"))
					assert.Loosely(t, tq.Purge(c, "").Error(), should.ContainSubstring("cannot Purge from a transaction"))
					_, err := tq.Stats(c, "")
					assert.Loosely(t, err.Error(), should.ContainSubstring("cannot Stats from a transaction"))
					return nil
				}, nil), should.BeNil)
			})

			t.Run("can get the non-transactional taskqueue context though", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					noTxn := ds.WithoutTransaction(c)
					assert.Loosely(t, tq.Delete(noTxn, "", task), should.BeNil)
					assert.Loosely(t, tq.Purge(noTxn, ""), should.BeNil)
					_, err := tq.Stats(noTxn, "")
					assert.Loosely(t, err, should.BeNil)
					return nil
				}, nil), should.BeNil)
			})

			t.Run("adding a new task only happens if we don't errout", func(t *ftt.Test) {
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					t3 := &tq.Task{Path: "/sandwitch/victory"}
					assert.Loosely(t, tq.Add(c, "", t3), should.BeNil)
					return fmt.Errorf("nooooo")
				}, nil), should.ErrLike("nooooo"))

				assert.Loosely(t, tqt.GetScheduledTasks()["default"][task.Name], should.Match(task))
				assert.Loosely(t, tqt.GetTombstonedTasks()["default"][t2.Name], should.Match(t2))
				assert.Loosely(t, tqt.GetTransactionTasks()["default"], should.BeNil)
			})

			t.Run("likewise, a panic doesn't schedule anything", func(t *ftt.Test) {
				func() {
					defer func() { _ = recover() }()
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						assert.Loosely(t, tq.Add(c, "", &tq.Task{Path: "/sandwitch/victory"}), should.BeNil)

						panic(fmt.Errorf("nooooo"))
					}, nil), should.BeNil)
				}()

				assert.Loosely(t, tqt.GetScheduledTasks()["default"][task.Name], should.Match(task))
				assert.Loosely(t, tqt.GetTombstonedTasks()["default"][t2.Name], should.Match(t2))
				assert.Loosely(t, tqt.GetTransactionTasks()["default"], should.BeNil)
			})

		})

		t.Run("Pull queues", func(t *ftt.Test) {
			tqt.CreatePullQueue("pull")
			tqt.CreateQueue("push")

			t.Run("One task scenarios", func(t *ftt.Test) {
				t.Run("enqueue, lease, delete", func(t *ftt.Test) {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
						Tag:     "tag",
					})
					assert.Loosely(t, err, should.BeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))
					assert.Loosely(t, tasks[0].Payload, should.Match([]byte("zzz")))

					// "Disappears" from the queue while leased.
					tc.Add(30 * time.Second)
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks2), should.BeZero)

					// Remove after "processing".
					assert.Loosely(t, tq.Delete(c, "pull", tasks[0]), should.BeNil)

					// Still nothing there even after lease expires.
					tc.Add(50 * time.Second)
					tasks3, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks3), should.BeZero)
				})

				t.Run("enqueue, lease, loose", func(t *ftt.Test) {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					assert.Loosely(t, err, should.BeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))
					assert.Loosely(t, tasks[0].Payload, should.Match([]byte("zzz")))

					// Time passes, lease expires.
					tc.Add(61 * time.Second)

					// Available again, someone grabs it.
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks2), should.Equal(1))
					assert.Loosely(t, tasks2[0].Payload, should.Match([]byte("zzz")))

					// Previously leased task is no longer owned.
					err = tq.ModifyLease(c, tasks[0], "pull", time.Minute)
					assert.Loosely(t, err, should.ErrLike("TASK_LEASE_EXPIRED"))
				})

				t.Run("enqueue, lease, sleep", func(t *ftt.Test) {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					assert.Loosely(t, err, should.BeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))
					assert.Loosely(t, tasks[0].Payload, should.Match([]byte("zzz")))

					// Time passes, the lease expires.
					tc.Add(61 * time.Second)
					err = tq.ModifyLease(c, tasks[0], "pull", time.Minute)
					assert.Loosely(t, err, should.ErrLike("TASK_LEASE_EXPIRED"))
				})

				t.Run("enqueue, lease, extend", func(t *ftt.Test) {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					assert.Loosely(t, err, should.BeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))
					assert.Loosely(t, tasks[0].Payload, should.Match([]byte("zzz")))

					// Time passes, the lease is updated.
					tc.Add(59 * time.Second)
					err = tq.ModifyLease(c, tasks[0], "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)

					// Not available, still leased.
					tc.Add(30 * time.Second)
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks2), should.BeZero)
				})

				t.Run("enqueue, lease, return", func(t *ftt.Test) {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					assert.Loosely(t, err, should.BeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))
					assert.Loosely(t, tasks[0].Payload, should.Match([]byte("zzz")))

					// Put back by using 0 sec lease.
					err = tq.ModifyLease(c, tasks[0], "pull", 0)
					assert.Loosely(t, err, should.BeNil)

					// Available again.
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks2), should.Equal(1))
				})

				t.Run("lease by existing tag", func(t *ftt.Test) {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
						Tag:     "tag",
					})
					assert.Loosely(t, err, should.BeNil)

					// Try different tag first, should return nothing.
					tasks, err := tq.LeaseByTag(c, 1, "pull", time.Minute, "wrong_tag")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.BeZero)

					// Leased.
					tasks, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))

					// No ready tasks anymore.
					tasks2, err := tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks2), should.BeZero)
					tasks2, err = tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks2), should.BeZero)

					// Return back to the queue later.
					tc.Add(30 * time.Second)
					assert.Loosely(t, tq.ModifyLease(c, tasks[0], "pull", 0), should.BeNil)

					// Available again.
					tasks, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))
				})

				t.Run("transactions (success)", func(t *ftt.Test) {
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						return tq.Add(c, "pull", &tq.Task{
							Method:  "PULL",
							Payload: []byte("zzz"),
						})
					}, nil), should.BeNil)

					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.Equal(1))
				})

				t.Run("transactions (rollback)", func(t *ftt.Test) {
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						err := tq.Add(c, "pull", &tq.Task{
							Method:  "PULL",
							Payload: []byte("zzz"),
						})
						assert.Loosely(t, err, should.BeNil)
						return fmt.Errorf("meh")
					}, nil), should.ErrLike("meh"))

					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(tasks), should.BeZero)
				})

				t.Run("transactions (invalid ops)", func(t *ftt.Test) {
					assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
						_, err := tq.Lease(c, 1, "pull", time.Minute)
						assert.Loosely(t, err, should.ErrLike("cannot Lease"))

						_, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
						assert.Loosely(t, err, should.ErrLike("cannot LeaseByTag"))

						err = tq.ModifyLease(c, &tq.Task{}, "pull", time.Minute)
						assert.Loosely(t, err, should.ErrLike("cannot ModifyLease"))

						return nil
					}, nil), should.BeNil)
				})

				t.Run("wrong queue mode", func(t *ftt.Test) {
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "POST",
						Payload: []byte("zzz"),
					})
					assert.Loosely(t, err, should.ErrLike("INVALID_QUEUE_MODE"))

					err = tq.Add(c, "push", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					assert.Loosely(t, err, should.ErrLike("INVALID_QUEUE_MODE"))

					_, err = tq.Lease(c, 1, "push", time.Minute)
					assert.Loosely(t, err, should.ErrLike("INVALID_QUEUE_MODE"))

					err = tq.ModifyLease(c, &tq.Task{}, "push", time.Minute)
					assert.Loosely(t, err, should.ErrLike("INVALID_QUEUE_MODE"))
				})

				t.Run("bad requests", func(t *ftt.Test) {
					_, err := tq.Lease(c, 0, "pull", time.Minute)
					assert.Loosely(t, err, should.ErrLike("BAD_REQUEST"))

					_, err = tq.Lease(c, 1, "pull", -time.Minute)
					assert.Loosely(t, err, should.ErrLike("BAD_REQUEST"))

					err = tq.ModifyLease(c, &tq.Task{}, "pull", -time.Minute)
					assert.Loosely(t, err, should.ErrLike("BAD_REQUEST"))
				})

				t.Run("tombstoned task", func(t *ftt.Test) {
					task := &tq.Task{
						Method:  "PULL",
						Name:    "deleted",
						Payload: []byte("zzz"),
					}
					assert.Loosely(t, tq.Add(c, "pull", task), should.BeNil)
					assert.Loosely(t, tq.Delete(c, "pull", task), should.BeNil)

					err := tq.ModifyLease(c, task, "pull", time.Minute)
					assert.Loosely(t, err, should.ErrLike("TOMBSTONED_TASK"))
				})

				t.Run("missing task", func(t *ftt.Test) {
					err := tq.ModifyLease(c, &tq.Task{Name: "missing"}, "pull", time.Minute)
					assert.Loosely(t, err, should.ErrLike("UNKNOWN_TASK"))
				})
			})

			t.Run("Many-tasks scenarios (sorting)", func(t *ftt.Test) {
				t.Run("Lease sorts by ETA (no tags)", func(t *ftt.Test) {
					now := clock.Now(c)

					tasks := []*tq.Task{}
					for i := 0; i < 5; i++ {
						tasks = append(tasks, &tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-%d", i),
							ETA:    now.Add(time.Duration(i+1) * time.Second),
						})
					}

					// Add in some "random" order.
					err := tq.Add(c, "pull", tasks[4], tasks[2], tasks[0], tasks[1], tasks[3])
					assert.Loosely(t, err, should.BeNil)

					// Nothing to pull, no available tasks yet.
					leased, err := tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(leased), should.BeZero)

					tc.Add(time.Second)

					// First task appears.
					leased, err = tq.Lease(c, 1, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(leased), should.Equal(1))
					assert.Loosely(t, leased[0].Name, should.Equal("task-0"))

					tc.Add(4 * time.Second)

					// The rest of them appear, in sorted order.
					leased, err = tq.Lease(c, 100, "pull", time.Minute)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(leased), should.Equal(4))
					for i := 0; i < 4; i++ {
						assert.Loosely(t, leased[i].Name, should.Equal(fmt.Sprintf("task-%d", i+1)))
					}
				})
			})

			t.Run("Lease and forget (no tags)", func(t *ftt.Test) {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull", &tq.Task{
						Method: "PULL",
						Name:   fmt.Sprintf("task-%d", i),
						ETA:    now.Add(time.Duration(i+1) * time.Second),
					})
					assert.Loosely(t, err, should.BeNil)
				}

				tc.Add(time.Second)

				// Lease the first task for 3 sec.
				leased, err := tq.Lease(c, 1, "pull", 3*time.Second)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(1))
				assert.Loosely(t, leased[0].Name, should.Equal("task-0"))

				// "Forget" about the lease.
				tc.Add(10 * time.Second)

				// Lease all we have there.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(5))
				assert.Loosely(t, leased[0].Name, should.Equal("task-1"))
				assert.Loosely(t, leased[1].Name, should.Equal("task-2"))
				assert.Loosely(t, leased[2].Name, should.Equal("task-0"))
				assert.Loosely(t, leased[3].Name, should.Equal("task-3"))
				assert.Loosely(t, leased[4].Name, should.Equal("task-4"))
			})

			t.Run("Modify lease moves the task (no tags)", func(t *ftt.Test) {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull", &tq.Task{
						Method: "PULL",
						Name:   fmt.Sprintf("task-%d", i),
						ETA:    now.Add(time.Duration(i+1) * time.Second),
					})
					assert.Loosely(t, err, should.BeNil)
				}

				tc.Add(time.Second)

				// Lease the first task for 1 minute.
				leased, err := tq.Lease(c, 1, "pull", time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(1))
				assert.Loosely(t, leased[0].Name, should.Equal("task-0"))

				// 3 sec later release the lease.
				tc.Add(3 * time.Second)
				assert.Loosely(t, tq.ModifyLease(c, leased[0], "pull", 0), should.BeNil)

				tc.Add(10 * time.Second)

				// Lease all we have there.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(5))
				assert.Loosely(t, leased[0].Name, should.Equal("task-1"))
				assert.Loosely(t, leased[1].Name, should.Equal("task-2"))
				assert.Loosely(t, leased[2].Name, should.Equal("task-0"))
				assert.Loosely(t, leased[3].Name, should.Equal("task-3"))
				assert.Loosely(t, leased[4].Name, should.Equal("task-4"))
			})

			t.Run("Delete task deletes from the middle", func(t *ftt.Test) {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull", &tq.Task{
						Method: "PULL",
						Name:   fmt.Sprintf("task-%d", i),
						ETA:    now.Add(time.Duration(i+1) * time.Second),
					})
					assert.Loosely(t, err, should.BeNil)
				}

				tc.Add(time.Second)

				// Lease the first task for 3 sec.
				leased, err := tq.Lease(c, 1, "pull", 3*time.Second)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(1))
				assert.Loosely(t, leased[0].Name, should.Equal("task-0"))

				// Kill it.
				assert.Loosely(t, tq.Delete(c, "pull", leased[0]), should.BeNil)

				tc.Add(10 * time.Second)

				// Lease all we have there.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(4))
				assert.Loosely(t, leased[0].Name, should.Equal("task-1"))
				assert.Loosely(t, leased[1].Name, should.Equal("task-2"))
				assert.Loosely(t, leased[2].Name, should.Equal("task-3"))
				assert.Loosely(t, leased[3].Name, should.Equal("task-4"))
			})

			t.Run("Tags work", func(t *ftt.Test) {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull",
						&tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-%d", i),
							ETA:    now.Add(time.Duration(i+1) * time.Second),
						},
						&tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-a-%d", i),
							ETA:    now.Add(time.Duration(i+1) * time.Second),
							Tag:    "a",
						},
						&tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-b-%d", i),
							ETA:    now.Add(time.Duration(i+1) * time.Second),
							Tag:    "b",
						})
					assert.Loosely(t, err, should.BeNil)
				}

				tc.Add(time.Second)

				// Lease leases all regardless of tags.
				leased, err := tq.Lease(c, 100, "pull", time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(3))
				assert.Loosely(t, leased[0].Name, should.Equal("task-0"))
				assert.Loosely(t, leased[1].Name, should.Equal("task-a-0"))
				assert.Loosely(t, leased[2].Name, should.Equal("task-b-0"))

				// Nothing to least per tag for now.
				leased, err = tq.LeaseByTag(c, 100, "pull", time.Minute, "a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.BeZero)

				tc.Add(10 * time.Second)

				// Grab all "a" tasks.
				leased, err = tq.LeaseByTag(c, 100, "pull", time.Minute, "a")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(4))
				for i := 0; i < 4; i++ {
					assert.Loosely(t, leased[i].Name, should.Equal(fmt.Sprintf("task-a-%d", i+1)))
				}

				// Only "b" and untagged tasks left.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(8))
				for i := 0; i < 4; i++ {
					assert.Loosely(t, leased[i*2].Name, should.Equal(fmt.Sprintf("task-%d", i+1)))
					assert.Loosely(t, leased[i*2+1].Name, should.Equal(fmt.Sprintf("task-b-%d", i+1)))
				}
			})

			t.Run("LeaseByTag with empty tag, hitting tag first", func(t *ftt.Test) {
				now := clock.Now(c)

				// Nothing to pull, nothing there yet.
				leased, err := tq.LeaseByTag(c, 1, "pull", time.Minute, "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.BeZero)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull",
						&tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-a-%d", i),
							ETA:    now.Add(time.Duration(i*2+1) * time.Second),
							Tag:    "a",
						},
						&tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-%d", i),
							ETA:    now.Add(time.Duration(i*2+2) * time.Second),
						})
					assert.Loosely(t, err, should.BeNil)
				}

				// Nothing to pull, no available tasks yet.
				leased, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.BeZero)

				tc.Add(time.Minute)

				// Hits "a" first and fetches only "a" tasks.
				leased, err = tq.LeaseByTag(c, 100, "pull", time.Minute, "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(5))
				for i := 0; i < 5; i++ {
					assert.Loosely(t, leased[i].Name, should.Equal(fmt.Sprintf("task-a-%d", i)))
				}
			})

			t.Run("LeaseByTag with empty tag, hitting untagged first", func(t *ftt.Test) {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull",
						&tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-%d", i),
							ETA:    now.Add(time.Duration(i*2+1) * time.Second),
						},
						&tq.Task{
							Method: "PULL",
							Name:   fmt.Sprintf("task-a-%d", i),
							ETA:    now.Add(time.Duration(i*2+2) * time.Second),
							Tag:    "a",
						})
					assert.Loosely(t, err, should.BeNil)
				}

				tc.Add(time.Minute)

				// Hits "" first and fetches only "" tasks.
				leased, err := tq.LeaseByTag(c, 100, "pull", time.Minute, "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(leased), should.Equal(5))
				for i := 0; i < 5; i++ {
					assert.Loosely(t, leased[i].Name, should.Equal(fmt.Sprintf("task-%d", i)))
				}
			})
		})
	})
}
