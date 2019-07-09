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

package services

import (
	"errors"
	"testing"
	"time"

	"go.chromium.org/gae/filter/featureBreaker"
	ds "go.chromium.org/gae/service/datastore"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestArchiveStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install(true)

		svr := New()

		// Register a testing log stream with an archive tasked.
		tls := ct.MakeStream(c, "proj-foo", "testing/+/foo")
		tls.State.ArchivalKey = []byte("archival key")
		So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchiveTasked)
		if err := tls.Put(c); err != nil {
			panic(err)
		}

		// Advance the clock to differentiate updates from new stream.
		env.Clock.Add(time.Hour)
		now := ds.RoundTime(env.Clock.Now().UTC())

		req := &logdog.ArchiveStreamRequest{
			Project:       string(tls.Project),
			Id:            string(tls.Stream.ID),
			TerminalIndex: 13,
			LogEntryCount: 14,
			StreamUrl:     "gs://fake.stream",
			StreamSize:    10,
			IndexUrl:      "gs://fake.index",
			IndexSize:     20,
			DataUrl:       "gs://fake.data",
			DataSize:      30,
		}

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := svr.ArchiveStream(c, req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			env.JoinGroup("services")

			Convey(`Will mark the stream as archived.`, func() {
				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeNil)

				So(tls.Get(c), ShouldBeNil)
				So(tls.State.Terminated(), ShouldBeTrue)
				So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchivedComplete)

				So(tls.State.Updated, ShouldResemble, now)
				So(tls.State.ArchivalKey, ShouldBeNil)
				So(tls.State.TerminatedTime, ShouldResemble, now)
				So(tls.State.ArchivedTime, ShouldResemble, now)
				So(tls.State.TerminalIndex, ShouldEqual, 13)
				So(tls.State.ArchiveLogEntryCount, ShouldEqual, 14)
				So(tls.State.ArchiveStreamURL, ShouldEqual, "gs://fake.stream")
				So(tls.State.ArchiveStreamSize, ShouldEqual, 10)
				So(tls.State.ArchiveIndexURL, ShouldEqual, "gs://fake.index")
				So(tls.State.ArchiveIndexSize, ShouldEqual, 20)
				So(tls.State.ArchiveDataURL, ShouldEqual, "gs://fake.data")
				So(tls.State.ArchiveDataSize, ShouldEqual, 30)
			})

			Convey(`Will mark the stream as partially archived if not complete.`, func() {
				req.LogEntryCount = 13

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeNil)

				So(tls.Get(c), ShouldBeNil)
				So(tls.State.Terminated(), ShouldBeTrue)
				So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchivedPartial)

				So(tls.State.Updated, ShouldResemble, now)
				So(tls.State.ArchivalKey, ShouldBeNil)
				So(tls.State.TerminatedTime, ShouldResemble, now)
				So(tls.State.ArchivedTime, ShouldResemble, now)
				So(tls.State.TerminalIndex, ShouldEqual, 13)
				So(tls.State.ArchiveLogEntryCount, ShouldEqual, 13)
				So(tls.State.ArchiveStreamURL, ShouldEqual, "gs://fake.stream")
				So(tls.State.ArchiveStreamSize, ShouldEqual, 10)
				So(tls.State.ArchiveIndexURL, ShouldEqual, "gs://fake.index")
				So(tls.State.ArchiveIndexSize, ShouldEqual, 20)
				So(tls.State.ArchiveDataURL, ShouldEqual, "gs://fake.data")
				So(tls.State.ArchiveDataSize, ShouldEqual, 30)
			})

			Convey(`Will refuse to process an invalid stream hash.`, func() {
				req.Id = "!!!invalid!!!"
				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeRPCInvalidArgument, "Invalid ID")
			})

			Convey(`If index URL is missing, will refuse to mark the stream archived.`, func() {
				req.IndexUrl = ""

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeRPCInvalidArgument)
			})

			Convey(`If stream URL is missing, will refuse to mark the stream archived.`, func() {
				req.StreamUrl = ""

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeRPCInvalidArgument)
			})

			Convey(`If stream is already archived, will not update and return success.`, func() {
				tls.State.TerminalIndex = 1337
				tls.State.ArchiveLogEntryCount = 42
				tls.State.ArchivedTime = now
				tls.State.TerminatedTime = now

				So(tls.State.Terminated(), ShouldBeTrue)
				So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchivedPartial)
				So(tls.Put(c), ShouldBeNil)

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeNil)

				tls.State.TerminalIndex = -1 // To make sure it reloaded.
				So(tls.Get(c), ShouldBeNil)
				So(tls.State.Terminated(), ShouldBeTrue)
				So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchivedPartial)

				So(tls.State.TerminalIndex, ShouldEqual, 1337)
				So(tls.State.ArchiveLogEntryCount, ShouldEqual, 42)
			})

			Convey(`If the archive has failed, it is archived as an empty stream.`, func() {
				req.Error = "archive error"

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeNil)
				So(tls.Get(c), ShouldBeNil)
				So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchivedComplete)

				So(tls.State.ArchivalKey, ShouldBeNil)
				So(tls.State.TerminalIndex, ShouldEqual, -1)
				So(tls.State.ArchiveLogEntryCount, ShouldEqual, 0)
			})

			Convey(`When datastore Get fails, returns internal error.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "GetMulti")

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeRPCInternal)
			})

			Convey(`When datastore Put fails, returns internal error.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "PutMulti")

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeRPCInternal)
			})
		})
	})
}
