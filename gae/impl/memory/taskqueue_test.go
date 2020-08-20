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
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	tq "go.chromium.org/gae/service/taskqueue"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTaskQueue(t *testing.T) {
	t.Parallel()

	Convey("TaskQueue", t, func() {
		now := time.Date(2000, time.January, 1, 1, 1, 1, 1, time.UTC)
		c, tc := testclock.UseTime(context.Background(), now)
		c = mathrand.Set(c, rand.New(rand.NewSource(clock.Now(c).UnixNano())))
		c = Use(c)

		tqt := tq.GetTestable(c)
		So(tqt, ShouldNotBeNil)

		So(tq.Raw(c), ShouldNotBeNil)

		Convey("implements TQMultiReadWriter", func() {
			Convey("Add", func() {
				t := &tq.Task{Path: "/hello/world"}

				Convey("works", func() {
					t.Delay = 4 * time.Second
					t.Header = http.Header{}
					t.Header.Add("Cat", "tabby")
					t.Payload = []byte("watwatwat")
					t.RetryOptions = &tq.RetryOptions{AgeLimit: 7 * time.Second}
					So(tq.Add(c, "", t), ShouldBeNil)

					var scheduled *tq.Task
					for _, t := range tqt.GetScheduledTasks()["default"] {
						scheduled = t
						break
					}
					So(scheduled, ShouldResemble, &tq.Task{
						ETA:          now.Add(4 * time.Second),
						Header:       http.Header{"Cat": []string{"tabby"}},
						Method:       "POST",
						Name:         "16045561405319332057",
						Path:         "/hello/world",
						Payload:      []byte("watwatwat"),
						RetryOptions: &tq.RetryOptions{AgeLimit: 7 * time.Second},
					})
				})

				Convey("picks up namespace", func() {
					c, err := info.Namespace(c, "coolNamespace")
					So(err, ShouldBeNil)

					t := &tq.Task{}
					So(tq.Add(c, "", t), ShouldBeNil)
					So(t.Header, ShouldResemble, http.Header{
						"X-Appengine-Current-Namespace": {"coolNamespace"},
					})

					Convey("namespaced Testable only returns tasks for that namespace", func() {
						So(tq.GetTestable(c).GetScheduledTasks()["default"], ShouldHaveLength, 1)

						// Default namespace has no tasks in its queue.
						So(tq.GetTestable(info.MustNamespace(c, "")).GetScheduledTasks()["default"], ShouldHaveLength, 0)
					})
				})

				Convey("cannot add to bad queues", func() {
					So(tq.Add(c, "waaat", &tq.Task{}).Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")

					Convey("but you can add Queues when testing", func() {
						tqt.CreateQueue("waaat")
						So(tq.Add(c, "waaat", t), ShouldBeNil)

						Convey("you just can't add them twice", func() {
							So(func() { tqt.CreateQueue("waaat") }, ShouldPanic)
						})
					})
				})

				Convey("supplies a URL if it's missing", func() {
					t.Path = ""
					So(tq.Add(c, "", t), ShouldBeNil)
					So(t.Path, ShouldEqual, "/_ah/queue/default")
				})

				Convey("cannot add twice", func() {
					t.Name = "bob"
					So(tq.Add(c, "", t), ShouldBeNil)

					// can't add the same one twice!
					So(tq.Add(c, "", t), ShouldEqual, tq.ErrTaskAlreadyAdded)
				})

				Convey("cannot add deleted task", func() {
					t.Name = "bob"
					So(tq.Add(c, "", t), ShouldBeNil)

					So(tq.Delete(c, "", t), ShouldBeNil)

					// can't add a deleted task!
					So(tq.Add(c, "", t), ShouldEqual, tq.ErrTaskAlreadyAdded)
				})

				Convey("must use a reasonable method", func() {
					t.Method = "Crystal"
					So(tq.Add(c, "", t).Error(), ShouldContainSubstring, "bad method")
				})

				Convey("payload gets dumped for non POST/PUT methods", func() {
					t.Method = "HEAD"
					t.Payload = []byte("coool")
					So(tq.Add(c, "", t), ShouldBeNil)
					So(t.Payload, ShouldBeNil)
				})

				Convey("invalid names are rejected", func() {
					t.Name = "happy times"
					So(tq.Add(c, "", t).Error(), ShouldContainSubstring, "INVALID_TASK_NAME")
				})

				Convey("AddMulti also works", func() {
					t2 := t.Duplicate()
					t2.Path = "/hi/city"

					expect := []*tq.Task{t, t2}

					So(tq.Add(c, "default", expect...), ShouldBeNil)
					So(len(expect), ShouldEqual, 2)
					So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 2)

					names := []string{"16045561405319332057", "16045561405319332058"}

					for i := range expect {
						Convey(fmt.Sprintf("task %d: %s", i, expect[i].Path), func() {
							So(expect[i].Method, ShouldEqual, "POST")
							So(expect[i].ETA, ShouldHappenOnOrBefore, now)
							So(expect[i].Name, ShouldEqual, names[i])
						})
					}

					Convey("stats work too", func() {
						delay := -time.Second * 400

						t := &tq.Task{Path: "/somewhere"}
						t.Delay = delay
						So(tq.Add(c, "", t), ShouldBeNil)

						stats, err := tq.Stats(c, "")
						So(err, ShouldBeNil)
						So(stats[0].Tasks, ShouldEqual, 3)
						So(stats[0].OldestETA, ShouldHappenOnOrBefore, clock.Now(c).Add(delay))

						_, err = tq.Stats(c, "noexist")
						So(err.Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")
					})

					Convey("can purge all tasks", func() {
						So(tq.Add(c, "", &tq.Task{Path: "/wut/nerbs"}), ShouldBeNil)
						So(tq.Purge(c, ""), ShouldBeNil)

						So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
						So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 0)
						So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 0)

						Convey("purging a queue which DNE fails", func() {
							So(tq.Purge(c, "noexist").Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")
						})
					})

				})
			})

			Convey("Delete", func() {
				t := &tq.Task{Path: "/hello/world"}
				So(tq.Add(c, "", t), ShouldBeNil)

				Convey("works", func() {
					err := tq.Delete(c, "", t)
					So(err, ShouldBeNil)
					So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
					So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 1)
					So(tqt.GetTombstonedTasks()["default"][t.Name], ShouldResemble, t)
				})

				Convey("cannot delete a task twice", func() {
					So(tq.Delete(c, "", t), ShouldBeNil)

					So(tq.Delete(c, "", t).Error(), ShouldContainSubstring, "TOMBSTONED_TASK")

					Convey("but you can if you do a reset", func() {
						tqt.ResetTasks()

						So(tq.Add(c, "", t), ShouldBeNil)
						So(tq.Delete(c, "", t), ShouldBeNil)
					})
				})

				Convey("cannot delete from bogus queues", func() {
					err := tq.Delete(c, "wat", t)
					So(err.Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")
				})

				Convey("cannot delete a missing task", func() {
					t.Name = "tarntioarenstyw"
					err := tq.Delete(c, "", t)
					So(err.Error(), ShouldContainSubstring, "UNKNOWN_TASK")
				})

				Convey("DeleteMulti also works", func() {
					t2 := t.Duplicate()
					t2.Name = ""
					t2.Path = "/hi/city"
					So(tq.Add(c, "", t2), ShouldBeNil)

					Convey("usually works", func() {
						So(tq.Delete(c, "", t, t2), ShouldBeNil)
						So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
						So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 2)
					})
				})
			})
		})

		Convey("works with transactions", func() {
			t := &tq.Task{Path: "/hello/world"}
			So(tq.Add(c, "", t), ShouldBeNil)

			t2 := &tq.Task{Path: "/hi/city"}
			So(tq.Add(c, "", t2), ShouldBeNil)

			So(tq.Delete(c, "", t2), ShouldBeNil)

			Convey("can view regular tasks", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					tqt := tq.Raw(c).GetTestable()

					So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
					So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
					So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("can add a new task", func() {
				t3 := &tq.Task{Path: "/sandwitch/victory"}

				err := ds.RunInTransaction(c, func(c context.Context) error {
					tqt := tq.GetTestable(c)

					So(tq.Add(c, "", t3), ShouldBeNil)
					So(t3.Name, ShouldEqual, "16045561405319332059")

					So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
					So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
					So(tqt.GetTransactionTasks()["default"][0], ShouldResemble, t3)
					return nil
				}, nil)
				So(err, ShouldBeNil)

				for _, tsk := range tqt.GetScheduledTasks()["default"] {
					if tsk.Name == t.Name {
						So(tsk, ShouldResemble, t)
					} else {
						So(tsk, ShouldResemble, t3)
					}
				}

				So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
				So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
			})

			Convey("can add a new task (but reset the state in a test)", func() {
				t3 := &tq.Task{Path: "/sandwitch/victory"}

				var txnCtx context.Context
				So(ds.RunInTransaction(c, func(c context.Context) error {
					txnCtx = c
					tqt := tq.GetTestable(c)

					So(tq.Add(c, "", t3), ShouldBeNil)

					So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
					So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
					So(tqt.GetTransactionTasks()["default"][0], ShouldResemble, t3)

					tqt.ResetTasks()

					So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
					So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 0)
					So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 0)

					return nil
				}, nil), ShouldBeNil)

				So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
				So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 0)
				So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 0)

				Convey("and reusing a closed context is bad times", func() {
					So(tq.Add(txnCtx, "", nil).Error(), ShouldContainSubstring, "expired")
				})
			})

			Convey("you can AddMulti as well", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					tqt := tq.GetTestable(c)

					t.Name = ""
					tasks := []*tq.Task{t.Duplicate(), t.Duplicate(), t.Duplicate()}
					So(tq.Add(c, "", tasks...), ShouldBeNil)
					So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 1)
					So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 3)
					return nil
				}, nil), ShouldBeNil)
				So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 4)
				So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 0)
			})

			Convey("unless you add too many things", func() {
				t.Name = ""

				So(ds.RunInTransaction(c, func(c context.Context) error {
					for i := 0; i < 5; i++ {
						So(tq.Add(c, "", t.Duplicate()), ShouldBeNil)
					}
					So(tq.Add(c, "", t).Error(), ShouldContainSubstring, "BAD_REQUEST")
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("unless you Add to a bad queue", func() {
				t.Name = ""

				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(tq.Add(c, "meat", t).Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")

					Convey("unless you add it!", func() {
						tq.Raw(c).GetTestable().CreateQueue("meat")
						So(tq.Add(c, "meat", t), ShouldBeNil)
					})

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("unless the task is named", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					err := tq.Add(c, "", t) // Note: "t" has a Name from initial Add.
					So(err, ShouldNotBeNil)
					So(err.Error(), ShouldContainSubstring, "INVALID_TASK_NAME")

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("No other features are available, however", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					So(tq.Delete(c, "", t).Error(), ShouldContainSubstring, "cannot DeleteMulti from a transaction")
					So(tq.Purge(c, "").Error(), ShouldContainSubstring, "cannot Purge from a transaction")
					_, err := tq.Stats(c, "")
					So(err.Error(), ShouldContainSubstring, "cannot Stats from a transaction")
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("can get the non-transactional taskqueue context though", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					noTxn := ds.WithoutTransaction(c)
					So(tq.Delete(noTxn, "", t), ShouldBeNil)
					So(tq.Purge(noTxn, ""), ShouldBeNil)
					_, err := tq.Stats(noTxn, "")
					So(err, ShouldBeNil)
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("adding a new task only happens if we don't errout", func() {
				So(ds.RunInTransaction(c, func(c context.Context) error {
					t3 := &tq.Task{Path: "/sandwitch/victory"}
					So(tq.Add(c, "", t3), ShouldBeNil)
					return fmt.Errorf("nooooo")
				}, nil), ShouldErrLike, "nooooo")

				So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
				So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
				So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
			})

			Convey("likewise, a panic doesn't schedule anything", func() {
				func() {
					defer func() { _ = recover() }()
					So(ds.RunInTransaction(c, func(c context.Context) error {
						So(tq.Add(c, "", &tq.Task{Path: "/sandwitch/victory"}), ShouldBeNil)

						panic(fmt.Errorf("nooooo"))
					}, nil), ShouldBeNil)
				}()

				So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
				So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
				So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
			})

		})

		Convey("Pull queues", func() {
			tqt.CreatePullQueue("pull")
			tqt.CreateQueue("push")

			Convey("One task scenarios", func() {
				Convey("enqueue, lease, delete", func() {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
						Tag:     "tag",
					})
					So(err, ShouldBeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)
					So(tasks[0].Payload, ShouldResemble, []byte("zzz"))

					// "Disappears" from the queue while leased.
					tc.Add(30 * time.Second)
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks2), ShouldEqual, 0)

					// Remove after "processing".
					So(tq.Delete(c, "pull", tasks[0]), ShouldBeNil)

					// Still nothing there even after lease expires.
					tc.Add(50 * time.Second)
					tasks3, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks3), ShouldEqual, 0)
				})

				Convey("enqueue, lease, loose", func() {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					So(err, ShouldBeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)
					So(tasks[0].Payload, ShouldResemble, []byte("zzz"))

					// Time passes, lease expires.
					tc.Add(61 * time.Second)

					// Available again, someone grabs it.
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks2), ShouldEqual, 1)
					So(tasks2[0].Payload, ShouldResemble, []byte("zzz"))

					// Previously leased task is no longer owned.
					err = tq.ModifyLease(c, tasks[0], "pull", time.Minute)
					So(err, ShouldErrLike, "TASK_LEASE_EXPIRED")
				})

				Convey("enqueue, lease, sleep", func() {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					So(err, ShouldBeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)
					So(tasks[0].Payload, ShouldResemble, []byte("zzz"))

					// Time passes, the lease expires.
					tc.Add(61 * time.Second)
					err = tq.ModifyLease(c, tasks[0], "pull", time.Minute)
					So(err, ShouldErrLike, "TASK_LEASE_EXPIRED")
				})

				Convey("enqueue, lease, extend", func() {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					So(err, ShouldBeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)
					So(tasks[0].Payload, ShouldResemble, []byte("zzz"))

					// Time passes, the lease is updated.
					tc.Add(59 * time.Second)
					err = tq.ModifyLease(c, tasks[0], "pull", time.Minute)
					So(err, ShouldBeNil)

					// Not available, still leased.
					tc.Add(30 * time.Second)
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks2), ShouldEqual, 0)
				})

				Convey("enqueue, lease, return", func() {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					So(err, ShouldBeNil)

					// Lease.
					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)
					So(tasks[0].Payload, ShouldResemble, []byte("zzz"))

					// Put back by using 0 sec lease.
					err = tq.ModifyLease(c, tasks[0], "pull", 0)
					So(err, ShouldBeNil)

					// Available again.
					tasks2, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks2), ShouldEqual, 1)
				})

				Convey("lease by existing tag", func() {
					// Enqueue.
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
						Tag:     "tag",
					})
					So(err, ShouldBeNil)

					// Try different tag first, should return nothing.
					tasks, err := tq.LeaseByTag(c, 1, "pull", time.Minute, "wrong_tag")
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 0)

					// Leased.
					tasks, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)

					// No ready tasks anymore.
					tasks2, err := tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
					So(err, ShouldBeNil)
					So(len(tasks2), ShouldEqual, 0)
					tasks2, err = tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks2), ShouldEqual, 0)

					// Return back to the queue later.
					tc.Add(30 * time.Second)
					So(tq.ModifyLease(c, tasks[0], "pull", 0), ShouldBeNil)

					// Available again.
					tasks, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)
				})

				Convey("transactions (success)", func() {
					So(ds.RunInTransaction(c, func(c context.Context) error {
						return tq.Add(c, "pull", &tq.Task{
							Method:  "PULL",
							Payload: []byte("zzz"),
						})
					}, nil), ShouldBeNil)

					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 1)
				})

				Convey("transactions (rollback)", func() {
					So(ds.RunInTransaction(c, func(c context.Context) error {
						err := tq.Add(c, "pull", &tq.Task{
							Method:  "PULL",
							Payload: []byte("zzz"),
						})
						So(err, ShouldBeNil)
						return fmt.Errorf("meh")
					}, nil), ShouldErrLike, "meh")

					tasks, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(tasks), ShouldEqual, 0)
				})

				Convey("transactions (invalid ops)", func() {
					So(ds.RunInTransaction(c, func(c context.Context) error {
						_, err := tq.Lease(c, 1, "pull", time.Minute)
						So(err, ShouldErrLike, "cannot Lease")

						_, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "tag")
						So(err, ShouldErrLike, "cannot LeaseByTag")

						err = tq.ModifyLease(c, &tq.Task{}, "pull", time.Minute)
						So(err, ShouldErrLike, "cannot ModifyLease")

						return nil
					}, nil), ShouldBeNil)
				})

				Convey("wrong queue mode", func() {
					err := tq.Add(c, "pull", &tq.Task{
						Method:  "POST",
						Payload: []byte("zzz"),
					})
					So(err, ShouldErrLike, "INVALID_QUEUE_MODE")

					err = tq.Add(c, "push", &tq.Task{
						Method:  "PULL",
						Payload: []byte("zzz"),
					})
					So(err, ShouldErrLike, "INVALID_QUEUE_MODE")

					_, err = tq.Lease(c, 1, "push", time.Minute)
					So(err, ShouldErrLike, "INVALID_QUEUE_MODE")

					err = tq.ModifyLease(c, &tq.Task{}, "push", time.Minute)
					So(err, ShouldErrLike, "INVALID_QUEUE_MODE")
				})

				Convey("bad requests", func() {
					_, err := tq.Lease(c, 0, "pull", time.Minute)
					So(err, ShouldErrLike, "BAD_REQUEST")

					_, err = tq.Lease(c, 1, "pull", -time.Minute)
					So(err, ShouldErrLike, "BAD_REQUEST")

					err = tq.ModifyLease(c, &tq.Task{}, "pull", -time.Minute)
					So(err, ShouldErrLike, "BAD_REQUEST")
				})

				Convey("tombstoned task", func() {
					task := &tq.Task{
						Method:  "PULL",
						Name:    "deleted",
						Payload: []byte("zzz"),
					}
					So(tq.Add(c, "pull", task), ShouldBeNil)
					So(tq.Delete(c, "pull", task), ShouldBeNil)

					err := tq.ModifyLease(c, task, "pull", time.Minute)
					So(err, ShouldErrLike, "TOMBSTONED_TASK")
				})

				Convey("missing task", func() {
					err := tq.ModifyLease(c, &tq.Task{Name: "missing"}, "pull", time.Minute)
					So(err, ShouldErrLike, "UNKNOWN_TASK")
				})
			})

			Convey("Many-tasks scenarios (sorting)", func() {
				Convey("Lease sorts by ETA (no tags)", func() {
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
					So(err, ShouldBeNil)

					// Nothing to pull, no available tasks yet.
					leased, err := tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(leased), ShouldEqual, 0)

					tc.Add(time.Second)

					// First task appears.
					leased, err = tq.Lease(c, 1, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(leased), ShouldEqual, 1)
					So(leased[0].Name, ShouldEqual, "task-0")

					tc.Add(4 * time.Second)

					// The rest of them appear, in sorted order.
					leased, err = tq.Lease(c, 100, "pull", time.Minute)
					So(err, ShouldBeNil)
					So(len(leased), ShouldEqual, 4)
					for i := 0; i < 4; i++ {
						So(leased[i].Name, ShouldEqual, fmt.Sprintf("task-%d", i+1))
					}
				})
			})

			Convey("Lease and forget (no tags)", func() {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull", &tq.Task{
						Method: "PULL",
						Name:   fmt.Sprintf("task-%d", i),
						ETA:    now.Add(time.Duration(i+1) * time.Second),
					})
					So(err, ShouldBeNil)
				}

				tc.Add(time.Second)

				// Lease the first task for 3 sec.
				leased, err := tq.Lease(c, 1, "pull", 3*time.Second)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 1)
				So(leased[0].Name, ShouldEqual, "task-0")

				// "Forget" about the lease.
				tc.Add(10 * time.Second)

				// Lease all we have there.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 5)
				So(leased[0].Name, ShouldEqual, "task-1")
				So(leased[1].Name, ShouldEqual, "task-2")
				So(leased[2].Name, ShouldEqual, "task-0")
				So(leased[3].Name, ShouldEqual, "task-3")
				So(leased[4].Name, ShouldEqual, "task-4")
			})

			Convey("Modify lease moves the task (no tags)", func() {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull", &tq.Task{
						Method: "PULL",
						Name:   fmt.Sprintf("task-%d", i),
						ETA:    now.Add(time.Duration(i+1) * time.Second),
					})
					So(err, ShouldBeNil)
				}

				tc.Add(time.Second)

				// Lease the first task for 1 minute.
				leased, err := tq.Lease(c, 1, "pull", time.Minute)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 1)
				So(leased[0].Name, ShouldEqual, "task-0")

				// 3 sec later release the lease.
				tc.Add(3 * time.Second)
				So(tq.ModifyLease(c, leased[0], "pull", 0), ShouldEqual, nil)

				tc.Add(10 * time.Second)

				// Lease all we have there.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 5)
				So(leased[0].Name, ShouldEqual, "task-1")
				So(leased[1].Name, ShouldEqual, "task-2")
				So(leased[2].Name, ShouldEqual, "task-0")
				So(leased[3].Name, ShouldEqual, "task-3")
				So(leased[4].Name, ShouldEqual, "task-4")
			})

			Convey("Delete task deletes from the middle", func() {
				now := clock.Now(c)

				for i := 0; i < 5; i++ {
					err := tq.Add(c, "pull", &tq.Task{
						Method: "PULL",
						Name:   fmt.Sprintf("task-%d", i),
						ETA:    now.Add(time.Duration(i+1) * time.Second),
					})
					So(err, ShouldBeNil)
				}

				tc.Add(time.Second)

				// Lease the first task for 3 sec.
				leased, err := tq.Lease(c, 1, "pull", 3*time.Second)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 1)
				So(leased[0].Name, ShouldEqual, "task-0")

				// Kill it.
				So(tq.Delete(c, "pull", leased[0]), ShouldBeNil)

				tc.Add(10 * time.Second)

				// Lease all we have there.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 4)
				So(leased[0].Name, ShouldEqual, "task-1")
				So(leased[1].Name, ShouldEqual, "task-2")
				So(leased[2].Name, ShouldEqual, "task-3")
				So(leased[3].Name, ShouldEqual, "task-4")
			})

			Convey("Tags work", func() {
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
					So(err, ShouldBeNil)
				}

				tc.Add(time.Second)

				// Lease leases all regardless of tags.
				leased, err := tq.Lease(c, 100, "pull", time.Minute)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 3)
				So(leased[0].Name, ShouldEqual, "task-0")
				So(leased[1].Name, ShouldEqual, "task-a-0")
				So(leased[2].Name, ShouldEqual, "task-b-0")

				// Nothing to least per tag for now.
				leased, err = tq.LeaseByTag(c, 100, "pull", time.Minute, "a")
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 0)

				tc.Add(10 * time.Second)

				// Grab all "a" tasks.
				leased, err = tq.LeaseByTag(c, 100, "pull", time.Minute, "a")
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 4)
				for i := 0; i < 4; i++ {
					So(leased[i].Name, ShouldEqual, fmt.Sprintf("task-a-%d", i+1))
				}

				// Only "b" and untagged tasks left.
				leased, err = tq.Lease(c, 100, "pull", time.Minute)
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 8)
				for i := 0; i < 4; i++ {
					So(leased[i*2].Name, ShouldEqual, fmt.Sprintf("task-%d", i+1))
					So(leased[i*2+1].Name, ShouldEqual, fmt.Sprintf("task-b-%d", i+1))
				}
			})

			Convey("LeaseByTag with empty tag, hitting tag first", func() {
				now := clock.Now(c)

				// Nothing to pull, nothing there yet.
				leased, err := tq.LeaseByTag(c, 1, "pull", time.Minute, "")
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 0)

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
					So(err, ShouldBeNil)
				}

				// Nothing to pull, no available tasks yet.
				leased, err = tq.LeaseByTag(c, 1, "pull", time.Minute, "")
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 0)

				tc.Add(time.Minute)

				// Hits "a" first and fetches only "a" tasks.
				leased, err = tq.LeaseByTag(c, 100, "pull", time.Minute, "")
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 5)
				for i := 0; i < 5; i++ {
					So(leased[i].Name, ShouldEqual, fmt.Sprintf("task-a-%d", i))
				}
			})

			Convey("LeaseByTag with empty tag, hitting untagged first", func() {
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
					So(err, ShouldBeNil)
				}

				tc.Add(time.Minute)

				// Hits "" first and fetches only "" tasks.
				leased, err := tq.LeaseByTag(c, 100, "pull", time.Minute, "")
				So(err, ShouldBeNil)
				So(len(leased), ShouldEqual, 5)
				for i := 0; i < 5; i++ {
					So(leased[i].Name, ShouldEqual, fmt.Sprintf("task-%d", i))
				}
			})
		})
	})
}
