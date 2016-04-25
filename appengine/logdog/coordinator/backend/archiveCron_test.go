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
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/proto/google"

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
					{Property: "Created", Descending: true},
				},
			},
		)

		// Install a base set of streams.
		//
		// If the stream was created >= 10min ago, it will be candidate for
		// archival.
		now := clock.Now(c)
		for _, v := range []struct {
			name  string
			d     time.Duration // Time before "now" for this streams Created.
			state coordinator.LogStreamState
		}{
			{"foo", 0, coordinator.LSStreaming},                    // Not candidate for archival (too soon).
			{"bar", 10 * time.Minute, coordinator.LSStreaming},     // Candidate for archival.
			{"baz", 10 * time.Minute, coordinator.LSArchiveTasked}, // Not candidate for archival (archive tasked).
			{"qux", 10 * time.Hour, coordinator.LSArchived},        // Not candidate for archival (archived).
			{"quux", 10 * time.Hour, coordinator.LSStreaming},      // Candidate for archival.
		} {
			ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, v.name))

			// The entry was created a week ago.
			ls.Created = now.Add(-v.d)
			ls.State = v.state
			ls.TerminatedTime = now // Doesn't matter, but required to Put the stream.
			ls.ArchivedTime = now   // Doesn't matter, but required to Put the stream.
			if err := ds.Get(c).Put(ls); err != nil {
				panic(err)
			}
		}
		ds.Get(c).Testable().CatchupIndexes()

		// This allows us to update our Context in test setup.
		tap := ct.ArchivalPublisher{}
		svcStub := ct.Services{
			AP: func() (coordinator.ArchivalPublisher, error) {
				return &tap, nil
			},
		}
		c = coordinator.WithServices(c, &svcStub)

		b := Backend{}

		tb := testBase{Context: c}
		r := httprouter.New()
		b.InstallHandlers(r, tb.base)

		s := httptest.NewServer(r)
		defer s.Close()

		endpoint := fmt.Sprintf("%s/archive/cron", s.URL)

		Convey(`With no configuration loaded, the endpoint will fail.`, func() {
			resp, err := http.Get(endpoint)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
		})

		Convey(`With no task topic configured, the endpoint will fail.`, func() {
			svcStub.InitConfig()

			resp, err := http.Get(endpoint)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
		})

		Convey(`With a configuration loaded`, func() {
			svcStub.InitConfig()
			svcStub.ServiceConfig.Coordinator.ArchiveTopic = "projects/test/topics/archive-test-topic"
			svcStub.ServiceConfig.Coordinator.ArchiveSettleDelay = google.NewDuration(10 * time.Second)
			svcStub.ServiceConfig.Coordinator.ArchiveDelayMax = google.NewDuration(10 * time.Minute)

			Convey(`A request to the endpoint will be successful.`, func() {
				resp, err := http.Get(endpoint)
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusOK)

				// Candidate tasks should be scheduled.
				So(tap.StreamNames(), ShouldResemble, []string{"bar", "quux"})

				// Hit the endpoint again, no additional tasks should be scheduled.
				Convey(`A subsequent endpoint hit will not schedule any additional tasks.`, func() {
					resp, err = http.Get(endpoint)
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)
					So(tap.StreamNames(), ShouldResemble, []string{"bar", "quux"})
				})
			})

			Convey(`When scheduling multiple tasks`, func() {
				b.multiTaskBatchSize = 5

				// Create a lot of archival candidate tasks to schedule.
				//
				// Note that since task queue names are returned sorted, we want to
				// name these streams greater than our stock stream names.
				names := []string{"bar", "quux"}
				for i := 0; i < 11; i++ {
					ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, fmt.Sprintf("stream-%02d", i)))

					ls.Created = now.Add(-(10 * time.Minute))
					ls.State = coordinator.LSStreaming
					ls.TerminalIndex = 1337
					ls.TerminatedTime = now
					if err := ds.Get(c).Put(ls); err != nil {
						panic(err)
					}

					names = append(names, ls.Name)
				}
				ds.Get(c).Testable().CatchupIndexes()

				Convey(`Will schedule all pages properly.`, func() {
					// Ensure that all of these tasks get added to the task queue.
					resp, err := http.Get(endpoint)
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusOK)
					So(tap.StreamNames(), ShouldResemble, names)

					// Use this opportunity to assert that none of the scheduled streams
					// have any settle or completion delay.
					for _, at := range tap.Tasks() {
						So(at.SettleDelay.Duration(), ShouldEqual, 0)
						So(at.CompletePeriod.Duration(), ShouldEqual, 0)
					}

					Convey(`Will not schedule additional tasks on the next run.`, func() {
						resp, err := http.Get(endpoint)
						So(err, ShouldBeNil)
						So(resp.StatusCode, ShouldEqual, http.StatusOK)
						So(tap.StreamNames(), ShouldResemble, names)
					})
				})

				Convey(`Will not schedule tasks if task scheduling fails.`, func() {
					tap.Err = errors.New("test error")

					// Ensure that all of these tasks get added to the task queue.
					resp, err := http.Get(endpoint)
					So(err, ShouldBeNil)
					So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
					So(tap.StreamNames(), ShouldResemble, []string{})

					Convey(`And will schedule streams next run.`, func() {
						tap.Err = nil

						resp, err := http.Get(endpoint)
						So(err, ShouldBeNil)
						So(resp.StatusCode, ShouldEqual, http.StatusOK)
						So(tap.StreamNames(), ShouldResemble, names)
					})
				})
			})
		})
	})
}
