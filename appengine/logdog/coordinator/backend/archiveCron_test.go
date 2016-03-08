// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/gae/filter/featureBreaker"
	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleArchiveCron(t *testing.T) {
	t.Parallel()

	Convey(`A testing environment`, t, func() {
		c := gaetesting.TestingContext()
		c, _ = testclock.UseTime(c, testclock.TestTimeUTC)

		// Add the archival index from "index.yaml".
		ds.Get(c).Testable().AddIndexes(
			&ds.IndexDefinition{
				Kind: "LogStream",
				SortBy: []ds.IndexColumn{
					{Property: "State"},
					{Property: "Updated"},
				},
			},
		)

		// Install a base set of streams.
		for _, v := range []struct {
			name string
			d    time.Duration
			term bool
		}{
			{"foo", 0, true},                 // Not candidate for archival (too soon).
			{"bar", 10 * time.Minute, true},  // Candidate for archival.
			{"baz", 10 * time.Minute, false}, // Not candidate for archival (not terminal).
			{"qux", 24 * time.Hour, false},   // Candidate for non-terminal archival.
		} {
			ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, v.name))

			// The entry was created a week ago.
			ls.Created = clock.Now(c).Add(-(7 * 24 * time.Hour))
			ls.Updated = clock.Now(c).Add(-v.d)
			if v.term {
				ls.State = coordinator.LSTerminated
			}
			if err := ls.Put(ds.Get(c)); err != nil {
				panic(err)
			}
		}
		ds.Get(c).Testable().CatchupIndexes()

		// This allows us to update our Context in test setup.
		b := Backend{
			multiTaskBatchSize: 5,
		}

		tb := testBase{Context: c}
		r := httprouter.New()
		b.InstallHandlers(r, tb.base)

		s := httptest.NewServer(r)
		defer s.Close()

		Convey(`With no configuration loaded`, func() {
			for _, ep := range []string{"terminal", "nonterminal"} {
				Convey(fmt.Sprintf(`A %q endpoint hit will fail.`, ep), func() {
					resp, err := http.Get(fmt.Sprintf("%s/archive/cron/%s", s.URL, ep))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
				})
			}
		})

		Convey(`With a configuration loaded`, func() {
			qName := "archive-test-queue"
			c = ct.UseConfig(c, &svcconfig.Coordinator{
				ArchiveDelay:     google.NewDuration(10 * time.Minute),
				ArchiveDelayMax:  google.NewDuration(24 * time.Hour),
				ArchiveTaskQueue: qName,
			})
			tb.Context = c

			Convey(`With no task queue configured`, func() {
				for _, ep := range []string{"terminal", "nonterminal", "purge"} {
					Convey(fmt.Sprintf(`A %q endpoint hit will fail.`, ep), func() {
						resp, err := http.Get(fmt.Sprintf("%s/archive/cron/%s", s.URL, ep))
						So(err, ShouldBeNil)
						So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
					})
				}
			})

			Convey(`With a task queue configured`, func() {
				tq.Get(c).Testable().CreateQueue(qName)

				Convey(`A terminal endpoint hit will be successful and idempotent.`, func() {
					resp, err := http.Get(fmt.Sprintf("%s/archive/cron/terminal", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)

					// Candidate tasks should be scheduled.
					tasks := tq.Get(c).Testable().GetScheduledTasks()[qName]
					So(tasks, shouldHaveTasks, archiveTaskPath("testing/+/bar"))

					// Hit the endpoint again, the same tasks should be scheduled.
					resp, err = http.Get(fmt.Sprintf("%s/archive/cron/terminal", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)

					tasks2 := tq.Get(c).Testable().GetScheduledTasks()[qName]
					So(tasks2, ShouldResemble, tasks)
				})

				Convey(`A non-terminal endpoint hit will be successful and idempotent.`, func() {
					resp, err := http.Get(fmt.Sprintf("%s/archive/cron/nonterminal", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)

					// Candidate tasks should be scheduled.
					tasks := tq.Get(c).Testable().GetScheduledTasks()[qName]
					So(tasks, shouldHaveTasks, archiveTaskPath("testing/+/qux"))

					// Hit the endpoint again, the same tasks should be scheduled.
					resp, err = http.Get(fmt.Sprintf("%s/archive/cron/nonterminal", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)

					tasks2 := tq.Get(c).Testable().GetScheduledTasks()[qName]
					So(tasks2, ShouldResemble, tasks)
				})

				Convey(`A terminal endpoint hit followed by a non-terminal endpoint hit will be successful.`, func() {
					resp, err := http.Get(fmt.Sprintf("%s/archive/cron/terminal", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)
					So(tq.Get(c).Testable().GetScheduledTasks()[qName], shouldHaveTasks, archiveTaskPath("testing/+/bar"))

					resp, err = http.Get(fmt.Sprintf("%s/archive/cron/nonterminal", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)
					So(tq.Get(c).Testable().GetScheduledTasks()[qName], shouldHaveTasks,
						archiveTaskPath("testing/+/bar"), archiveTaskPath("testing/+/qux"))
				})

				Convey(`When scheduling multiple tasks`, func() {
					// Create a lot of archival candidate tasks to schedule.
					var names []string
					for i := 0; i < 11; i++ {
						ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, fmt.Sprintf("stream-%d", i)))

						ls.Created = clock.Now(c).Add(-(10 * time.Minute))
						ls.Updated = ls.Created
						ls.State = coordinator.LSTerminated
						ls.TerminalIndex = 1337
						if err := ls.Put(ds.Get(c)); err != nil {
							panic(err)
						}

						names = append(names, ls.Name)
					}
					names = append(names, "bar")
					ds.Get(c).Testable().CatchupIndexes()

					Convey(`Will schedule all pages properly.`, func() {
						taskNames := make([]interface{}, len(names))
						for i, n := range names {
							taskNames[i] = archiveTaskPath(fmt.Sprintf("testing/+/%s", n))
						}

						// Ensure that all of these tasks get added to the task queue.
						resp, err := http.Get(fmt.Sprintf("%s/archive/cron/terminal", s.URL))
						So(err, ShouldBeNil)
						So(resp.StatusCode, ShouldEqual, http.StatusOK)
						So(tq.Get(c).Testable().GetScheduledTasks()[qName], shouldHaveTasks, taskNames...)

						Convey(`Will be successful when rescheduling the same tasks.`, func() {
							resp, err := http.Get(fmt.Sprintf("%s/archive/cron/terminal", s.URL))
							So(err, ShouldBeNil)
							So(resp.StatusCode, ShouldEqual, http.StatusOK)
							So(tq.Get(c).Testable().GetScheduledTasks()[qName], shouldHaveTasks, taskNames...)
						})
					})

					Convey(`Will return an error if task scheduling fails.`, func() {
						c, fb := featureBreaker.FilterTQ(c, nil)
						tb.Context = c
						fb.BreakFeatures(errors.New("test error"), "AddMulti")

						// Ensure that all of these tasks get added to the task queue.
						resp, err := http.Get(fmt.Sprintf("%s/archive/cron/terminal", s.URL))
						So(err, ShouldBeNil)
						So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
					})

					Convey(`Will return an error if a single task scheduling fails.`, func() {
						merr := make(errors.MultiError, len(names))
						merr[0] = errors.New("test error")

						c, fb := featureBreaker.FilterTQ(c, nil)
						tb.Context = c
						fb.BreakFeatures(merr, "AddMulti")

						// Ensure that all of these tasks get added to the task queue.
						resp, err := http.Get(fmt.Sprintf("%s/archive/cron/terminal", s.URL))
						So(err, ShouldBeNil)
						So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
					})

					Convey(`And a purge endpoint hit will purge the tasks.`, func() {
						resp, err := http.Get(fmt.Sprintf("%s/archive/cron/purge", s.URL))
						So(err, ShouldBeNil)
						So(resp.StatusCode, ShouldEqual, http.StatusOK)
						So(tq.Get(c).Testable().GetScheduledTasks()[qName], shouldHaveTasks)
					})
				})
			})
		})
	})
}
