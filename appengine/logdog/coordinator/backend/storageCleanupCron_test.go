// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"errors"
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
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestHandleStorageCleanupCron(t *testing.T) {
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
			ls   coordinator.LogStreamState
		}{
			{"foo", 0, coordinator.LSPending},
			{"bar", 9 * time.Minute, coordinator.LSArchived},
			{"baz", 10 * time.Minute, coordinator.LSArchived},
			{"qux", 24 * time.Hour, coordinator.LSDone},
		} {
			ls, err := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, v.name))
			if err != nil {
				panic(err)
			}

			// The entry was created a week ago.
			ls.State = v.ls
			ls.Created = clock.Now(c).Add(-(7 * 24 * time.Hour))
			ls.Updated = clock.Now(c).Add(-v.d)
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

		Convey(`An endpoint hit will fail with no configuration loaded.`, func() {
			resp, err := http.Get(fmt.Sprintf("%s/archive/cron/storageCleanup", s.URL))
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
		})

		Convey(`With a configuration loaded`, func() {
			qName := "storage-cleanup-test-queue"
			c = ct.UseConfig(c, &svcconfig.Coordinator{
				StorageCleanupTaskQueue: qName,
				StorageCleanupDelay:     google.NewDuration(10 * time.Minute),
			})
			tb.Context = c

			Convey(`With no task queue configured, an endpoint hit will fail`, func() {
				resp, err := http.Get(fmt.Sprintf("%s/archive/cron/storageCleanup", s.URL))
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
			})

			Convey(`With a task queue configured`, func() {
				tq.Get(c).Testable().CreateQueue(qName)

				Convey(`An endpoint hit will be successful and idempotent.`, func() {
					resp, err := http.Get(fmt.Sprintf("%s/archive/cron/storageCleanup", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)

					// Candidate tasks should be scheduled.
					tasks := tq.Get(c).Testable().GetScheduledTasks()[qName]
					So(tasks, shouldHaveTasks, cleanupTaskPath("testing/+/baz"))

					// Hit the endpoint again, the same tasks should be scheduled.
					resp, err = http.Get(fmt.Sprintf("%s/archive/cron/storageCleanup", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)

					tasks2 := tq.Get(c).Testable().GetScheduledTasks()[qName]
					So(tasks2, ShouldResemble, tasks)
				})

				Convey(`Will return an error if task scheduling fails.`, func() {
					c, fb := featureBreaker.FilterTQ(c, nil)
					tb.Context = c
					fb.BreakFeatures(errors.New("test error"), "AddMulti")

					// Ensure that all of these tasks get added to the task queue.
					resp, err := http.Get(fmt.Sprintf("%s/archive/cron/storageCleanup", s.URL))
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
				})
			})
		})
	})
}
