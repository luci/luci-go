// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"fmt"
	"math/rand"
	"net/http"
	"testing"
	"time"

	dsS "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	tqS "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestTaskQueue(t *testing.T) {
	t.Parallel()

	Convey("TaskQueue", t, func() {
		now := time.Date(2000, time.January, 1, 1, 1, 1, 1, time.UTC)
		c, tc := testclock.UseTime(context.Background(), now)
		c = mathrand.Set(c, rand.New(rand.NewSource(clock.Now(c).UnixNano())))
		c = Use(c)

		tq := tqS.Get(c)
		tqt := tq.Testable()
		So(tqt, ShouldNotBeNil)

		So(tq, ShouldNotBeNil)

		Convey("implements TQMultiReadWriter", func() {
			Convey("Add", func() {
				t := &tqS.Task{Path: "/hello/world"}

				Convey("works", func() {
					t.Delay = 4 * time.Second
					t.Header = http.Header{}
					t.Header.Add("Cat", "tabby")
					t.Payload = []byte("watwatwat")
					t.RetryOptions = &tqS.RetryOptions{AgeLimit: 7 * time.Second}
					So(tq.Add(t, ""), ShouldBeNil)

					name := "Z_UjshxM9ecyMQfGbZmUGOEcgxWU0_5CGLl_-RntudwAw2DqQ5-58bzJiWQN4OKzeuUb9O4JrPkUw2rOvk2Ax46THojnQ6avBQgZdrKcJmrwQ6o4qKfJdiyUbGXvy691yRfzLeQhs6cBhWrgf3wH-VPMcA4SC-zlbJ2U8An7I0zJQA5nBFnMNoMgT-2peGoay3rCSbj4z9VFFm9kS_i6JCaQH518ujLDSNCYdjTq6B6lcWrZAh0U_q3a1S2nXEwrKiw_t9MTNQFgAQZWyGBbvZQPmeRYtu8SPaWzTfd25v_YWgBuVL2rRSPSMvlDwE04nNdtvVzE8vNNiA1zRimmdzKeqATQF9_ReUvj4D7U8dcS703DZWfKMBLgBffY9jqCassOOOw77V72Oq5EVauUw3Qw0L6bBsfM9FtahTKUdabzRZjXUoze3EK4KXPt3-wdidau-8JrVf2XFocjjZbwHoxcGvbtT3b4nGLDlgwdC00bwaFBZWff"
					So(tqt.GetScheduledTasks()["default"][name], ShouldResemble, &tqS.Task{
						ETA:          now.Add(4 * time.Second),
						Header:       http.Header{"Cat": []string{"tabby"}},
						Method:       "POST",
						Name:         name,
						Path:         "/hello/world",
						Payload:      []byte("watwatwat"),
						RetryOptions: &tqS.RetryOptions{AgeLimit: 7 * time.Second},
					})
				})

				Convey("picks up namespace", func() {
					c, err := info.Get(c).Namespace("coolNamespace")
					So(err, ShouldBeNil)
					tq = tqS.Get(c)

					t := &tqS.Task{}
					So(tq.Add(t, ""), ShouldBeNil)
					So(t.Header, ShouldResemble, http.Header{
						"X-Appengine-Current-Namespace": {"coolNamespace"},
					})

				})

				Convey("cannot add to bad queues", func() {
					So(tq.Add(nil, "waaat").Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")

					Convey("but you can add Queues when testing", func() {
						tqt.CreateQueue("waaat")
						So(tq.Add(t, "waaat"), ShouldBeNil)

						Convey("you just can't add them twice", func() {
							So(func() { tqt.CreateQueue("waaat") }, ShouldPanic)
						})
					})
				})

				Convey("supplies a URL if it's missing", func() {
					t.Path = ""
					So(tq.Add(t, ""), ShouldBeNil)
					So(t.Path, ShouldEqual, "/_ah/queue/default")
				})

				Convey("cannot add twice", func() {
					t.Name = "bob"
					So(tq.Add(t, ""), ShouldBeNil)

					// can't add the same one twice!
					So(tq.Add(t, ""), ShouldEqual, tqS.ErrTaskAlreadyAdded)
				})

				Convey("cannot add deleted task", func() {
					t.Name = "bob"
					So(tq.Add(t, ""), ShouldBeNil)

					So(tq.Delete(t, ""), ShouldBeNil)

					// can't add a deleted task!
					So(tq.Add(t, ""), ShouldEqual, tqS.ErrTaskAlreadyAdded)
				})

				Convey("cannot set ETA+Delay", func() {
					t.ETA = clock.Now(c).Add(time.Hour)
					tc.Add(time.Second)
					t.Delay = time.Hour
					So(func() {
						So(tq.Add(t, ""), ShouldBeNil)
					}, ShouldPanic)
				})

				Convey("must use a reasonable method", func() {
					t.Method = "Crystal"
					So(tq.Add(t, "").Error(), ShouldContainSubstring, "bad method")
				})

				Convey("payload gets dumped for non POST/PUT methods", func() {
					t.Method = "HEAD"
					t.Payload = []byte("coool")
					So(tq.Add(t, ""), ShouldBeNil)
					So(t.Payload, ShouldBeNil)
				})

				Convey("invalid names are rejected", func() {
					t.Name = "happy times"
					So(tq.Add(t, "").Error(), ShouldContainSubstring, "INVALID_TASK_NAME")
				})

				Convey("AddMulti also works", func() {
					t2 := t.Duplicate()
					t2.Path = "/hi/city"

					expect := []*tqS.Task{t, t2}

					So(tq.AddMulti(expect, "default"), ShouldBeNil)
					So(len(expect), ShouldEqual, 2)
					So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 2)

					for i := range expect {
						Convey(fmt.Sprintf("task %d: %s", i, expect[i].Path), func() {
							So(expect[i].Method, ShouldEqual, "POST")
							So(expect[i].ETA, ShouldHappenOnOrBefore, now)
							So(len(expect[i].Name), ShouldEqual, 500)
						})
					}

					Convey("stats work too", func() {
						delay := -time.Second * 400

						t := &tqS.Task{Path: "/somewhere"}
						t.Delay = delay
						So(tq.Add(t, ""), ShouldBeNil)

						stats, err := tq.Stats("")
						So(err, ShouldBeNil)
						So(stats[0].Tasks, ShouldEqual, 3)
						So(stats[0].OldestETA, ShouldHappenOnOrBefore, clock.Now(c).Add(delay))

						_, err = tq.Stats("noexist")
						So(err.Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")
					})

					Convey("can purge all tasks", func() {
						So(tq.Add(&tqS.Task{Path: "/wut/nerbs"}, ""), ShouldBeNil)
						So(tq.Purge(""), ShouldBeNil)

						So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
						So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 0)
						So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 0)

						Convey("purging a queue which DNE fails", func() {
							So(tq.Purge("noexist").Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")
						})
					})

				})
			})

			Convey("Delete", func() {
				t := &tqS.Task{Path: "/hello/world"}
				So(tq.Add(t, ""), ShouldBeNil)

				Convey("works", func() {
					err := tq.Delete(t, "")
					So(err, ShouldBeNil)
					So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
					So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 1)
					So(tqt.GetTombstonedTasks()["default"][t.Name], ShouldResemble, t)
				})

				Convey("cannot delete a task twice", func() {
					So(tq.Delete(t, ""), ShouldBeNil)

					So(tq.Delete(t, "").Error(), ShouldContainSubstring, "TOMBSTONED_TASK")

					Convey("but you can if you do a reset", func() {
						tqt.ResetTasks()

						So(tq.Add(t, ""), ShouldBeNil)
						So(tq.Delete(t, ""), ShouldBeNil)
					})
				})

				Convey("cannot delete from bogus queues", func() {
					err := tq.Delete(t, "wat")
					So(err.Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")
				})

				Convey("cannot delete a missing task", func() {
					t.Name = "tarntioarenstyw"
					err := tq.Delete(t, "")
					So(err.Error(), ShouldContainSubstring, "UNKNOWN_TASK")
				})

				Convey("DeleteMulti also works", func() {
					t2 := t.Duplicate()
					t2.Name = ""
					t2.Path = "/hi/city"
					So(tq.Add(t2, ""), ShouldBeNil)

					Convey("usually works", func() {
						So(tq.DeleteMulti([]*tqS.Task{t, t2}, ""), ShouldBeNil)
						So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 0)
						So(len(tqt.GetTombstonedTasks()["default"]), ShouldEqual, 2)
					})
				})
			})
		})

		Convey("works with transactions", func() {
			t := &tqS.Task{Path: "/hello/world"}
			So(tq.Add(t, ""), ShouldBeNil)

			t2 := &tqS.Task{Path: "/hi/city"}
			So(tq.Add(t2, ""), ShouldBeNil)

			So(tq.Delete(t2, ""), ShouldBeNil)

			Convey("can view regular tasks", func() {
				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					tqt := tqS.GetRaw(c).Testable()

					So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
					So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
					So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("can add a new task", func() {
				t3 := &tqS.Task{Path: "/sandwitch/victory"}

				err := dsS.Get(c).RunInTransaction(func(c context.Context) error {
					tq := tqS.Get(c)
					tqt := tq.Testable()

					So(tq.Add(t3, ""), ShouldBeNil)
					So(t3.Name, ShouldEqual, "")

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
						tsk.Name = ""
						So(tsk, ShouldResemble, t3)
					}
				}

				So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
				So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
			})

			Convey("can add a new task (but reset the state in a test)", func() {
				t3 := &tqS.Task{Path: "/sandwitch/victory"}

				ttq := tqS.Interface(nil)

				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					ttq = tqS.Get(c)
					tqt := ttq.Testable()

					So(ttq.Add(t3, ""), ShouldBeNil)

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
					So(ttq.Add(nil, "").Error(), ShouldContainSubstring, "expired")
				})
			})

			Convey("you can AddMulti as well", func() {
				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					tq := tqS.Get(c)
					tqt := tq.Testable()

					t.Name = ""
					tasks := []*tqS.Task{t.Duplicate(), t.Duplicate(), t.Duplicate()}
					So(tq.AddMulti(tasks, ""), ShouldBeNil)
					So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 1)
					So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 3)
					return nil
				}, nil), ShouldBeNil)
				So(len(tqt.GetScheduledTasks()["default"]), ShouldEqual, 4)
				So(len(tqt.GetTransactionTasks()["default"]), ShouldEqual, 0)
			})

			Convey("unless you add too many things", func() {
				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					for i := 0; i < 5; i++ {
						So(tqS.Get(c).Add(t.Duplicate(), ""), ShouldBeNil)
					}
					So(tqS.Get(c).Add(t, "").Error(), ShouldContainSubstring, "BAD_REQUEST")
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("unless you Add to a bad queue", func() {
				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					So(tqS.Get(c).Add(t, "meat").Error(), ShouldContainSubstring, "UNKNOWN_QUEUE")

					Convey("unless you add it!", func() {
						tqS.GetRaw(c).Testable().CreateQueue("meat")
						So(tqS.Get(c).Add(t, "meat"), ShouldBeNil)
					})

					return nil
				}, nil), ShouldBeNil)
			})

			Convey("No other features are available, however", func() {
				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					So(tqS.Get(c).Delete(t, "").Error(), ShouldContainSubstring, "cannot DeleteMulti from a transaction")
					So(tqS.Get(c).Purge("").Error(), ShouldContainSubstring, "cannot Purge from a transaction")
					_, err := tqS.Get(c).Stats("")
					So(err.Error(), ShouldContainSubstring, "cannot Stats from a transaction")
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("can get the non-transactional taskqueue context though", func() {
				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					So(tqS.GetNoTxn(c).Delete(t, ""), ShouldBeNil)
					So(tqS.GetNoTxn(c).Purge(""), ShouldBeNil)
					_, err := tqS.GetNoTxn(c).Stats("")
					So(err, ShouldBeNil)
					return nil
				}, nil), ShouldBeNil)
			})

			Convey("adding a new task only happens if we don't errout", func() {
				So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
					t3 := &tqS.Task{Path: "/sandwitch/victory"}
					So(tqS.Get(c).Add(t3, ""), ShouldBeNil)
					return fmt.Errorf("nooooo")
				}, nil), ShouldErrLike, "nooooo")

				So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
				So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
				So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
			})

			Convey("likewise, a panic doesn't schedule anything", func() {
				func() {
					defer func() { _ = recover() }()
					So(dsS.Get(c).RunInTransaction(func(c context.Context) error {
						tq := tqS.Get(c)

						So(tq.Add(&tqS.Task{Path: "/sandwitch/victory"}, ""), ShouldBeNil)

						panic(fmt.Errorf("nooooo"))
					}, nil), ShouldBeNil)
				}()

				So(tqt.GetScheduledTasks()["default"][t.Name], ShouldResemble, t)
				So(tqt.GetTombstonedTasks()["default"][t2.Name], ShouldResemble, t2)
				So(tqt.GetTransactionTasks()["default"], ShouldBeNil)
			})

		})
	})
}
