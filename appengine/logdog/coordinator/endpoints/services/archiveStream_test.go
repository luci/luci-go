// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"errors"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestArchiveStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install()

		svr := New()

		now := ds.RoundTime(env.Clock.Now().UTC())

		// Register a testing log stream with an archive tasked.
		ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, "foo"))
		ls.State = coordinator.LSArchiveTasked
		ls.ArchivalKey = []byte("archival key")
		if err := ds.Get(c).Put(ls); err != nil {
			panic(err)
		}

		req := &logdog.ArchiveStreamRequest{
			Path:          string(ls.Path()),
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

				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.Terminated(), ShouldBeTrue)
				So(ls.Archived(), ShouldBeTrue)
				So(ls.ArchiveComplete(), ShouldBeTrue)

				So(ls.State, ShouldEqual, coordinator.LSArchived)
				So(ls.ArchivalKey, ShouldBeNil)
				So(ls.TerminatedTime, ShouldResemble, now)
				So(ls.ArchivedTime, ShouldResemble, now)
				So(ls.TerminalIndex, ShouldEqual, 13)
				So(ls.ArchiveLogEntryCount, ShouldEqual, 14)
				So(ls.ArchiveStreamURL, ShouldEqual, "gs://fake.stream")
				So(ls.ArchiveStreamSize, ShouldEqual, 10)
				So(ls.ArchiveIndexURL, ShouldEqual, "gs://fake.index")
				So(ls.ArchiveIndexSize, ShouldEqual, 20)
				So(ls.ArchiveDataURL, ShouldEqual, "gs://fake.data")
				So(ls.ArchiveDataSize, ShouldEqual, 30)
			})

			Convey(`Will mark the stream as partially archived if not complete.`, func() {
				req.LogEntryCount = 13

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeNil)

				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.Terminated(), ShouldBeTrue)
				So(ls.Archived(), ShouldBeTrue)
				So(ls.ArchiveComplete(), ShouldBeFalse)

				So(ls.State, ShouldEqual, coordinator.LSArchived)
				So(ls.ArchivalKey, ShouldBeNil)
				So(ls.TerminatedTime, ShouldResemble, now)
				So(ls.ArchivedTime, ShouldResemble, now)
				So(ls.TerminalIndex, ShouldEqual, 13)
				So(ls.ArchiveLogEntryCount, ShouldEqual, 13)
				So(ls.ArchiveStreamURL, ShouldEqual, "gs://fake.stream")
				So(ls.ArchiveStreamSize, ShouldEqual, 10)
				So(ls.ArchiveIndexURL, ShouldEqual, "gs://fake.index")
				So(ls.ArchiveIndexSize, ShouldEqual, 20)
				So(ls.ArchiveDataURL, ShouldEqual, "gs://fake.data")
				So(ls.ArchiveDataSize, ShouldEqual, 30)
			})

			Convey(`Will refuse to process an invalid stream path.`, func() {
				req.Path = "!!!invalid!!!"
				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeRPCInvalidArgument, "invalid log stream path")
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
				ls.State = coordinator.LSArchived
				ls.TerminalIndex = 1337
				ls.ArchiveLogEntryCount = 42
				ls.ArchivedTime = now
				ls.TerminatedTime = now
				So(ds.Get(c).Put(ls), ShouldBeNil)
				So(ls.Terminated(), ShouldBeTrue)
				So(ls.Archived(), ShouldBeTrue)

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeNil)

				ls.TerminalIndex = -1 // To make sure it reloaded.
				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.Terminated(), ShouldBeTrue)
				So(ls.Archived(), ShouldBeTrue)

				So(ls.State, ShouldEqual, coordinator.LSArchived)
				So(ls.TerminalIndex, ShouldEqual, 1337)
				So(ls.ArchiveLogEntryCount, ShouldEqual, 42)
			})

			Convey(`If the archive has failed, it is archived as an empty stream.`, func() {
				req.Error = "archive error"

				_, err := svr.ArchiveStream(c, req)
				So(err, ShouldBeNil)
				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.Archived(), ShouldBeTrue)

				So(ls.State, ShouldEqual, coordinator.LSArchived)
				So(ls.ArchivalKey, ShouldBeNil)
				So(ls.TerminalIndex, ShouldEqual, -1)
				So(ls.ArchiveLogEntryCount, ShouldEqual, 0)
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
