// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"errors"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestArchiveStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c := memory.Use(context.Background())
		be := Server{}

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		// Register a testing log stream (not archived).
		ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, "foo"))
		if err := ls.Put(ds.Get(c)); err != nil {
			panic(err)
		}

		req := &logdog.ArchiveStreamRequest{
			Path:          string(ls.Path()),
			Complete:      true,
			TerminalIndex: 13,
			StreamUrl:     "gs://fake.stream",
			StreamSize:    10,
			IndexUrl:      "gs://fake.index",
			IndexSize:     20,
			DataUrl:       "gs://fake.data",
			DataSize:      30,
		}

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := be.ArchiveStream(c, req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			Convey(`Will mark the stream as archived.`, func() {
				_, err := be.ArchiveStream(c, req)
				So(err, ShouldBeNil)

				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.Archived(), ShouldBeTrue)
				So(ls.ArchiveWhole, ShouldBeTrue)
				So(ls.TerminalIndex, ShouldEqual, 13)
				So(ls.ArchiveStreamURL, ShouldEqual, "gs://fake.stream")
				So(ls.ArchiveStreamSize, ShouldEqual, 10)
				So(ls.ArchiveIndexURL, ShouldEqual, "gs://fake.index")
				So(ls.ArchiveIndexSize, ShouldEqual, 20)
				So(ls.ArchiveDataURL, ShouldEqual, "gs://fake.data")
				So(ls.ArchiveDataSize, ShouldEqual, 30)
			})

			Convey(`Will refuse to process an invalid stream path.`, func() {
				req.Path = "!!!invalid!!!"
				_, err := be.ArchiveStream(c, req)
				So(err, ShouldBeRPCInvalidArgument, "invalid log stream path")
			})

			Convey(`If index URL is missing, will refuse to mark the stream archived.`, func() {
				req.IndexUrl = ""

				_, err := be.ArchiveStream(c, req)
				So(err, ShouldBeRPCInvalidArgument)
			})

			Convey(`If stream URL is missing, will refuse to mark the stream archived.`, func() {
				req.StreamUrl = ""

				_, err := be.ArchiveStream(c, req)
				So(err, ShouldBeRPCInvalidArgument)
			})

			Convey(`If stream is already archived, will not update and return success.`, func() {
				ls.State = coordinator.LSArchived
				ls.TerminalIndex = 1337
				So(ls.Archived(), ShouldBeTrue)
				So(ls.Put(ds.Get(c)), ShouldBeNil)

				_, err := be.ArchiveStream(c, req)
				So(err, ShouldBeNil)

				ls.TerminalIndex = -1 // To make sure it reloaded.
				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.Archived(), ShouldBeTrue)
				So(ls.TerminalIndex, ShouldEqual, 1337)
			})

			Convey(`When datastore Get fails, returns internal error.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "GetMulti")

				_, err := be.ArchiveStream(c, req)
				So(err, ShouldBeRPCInternal)
			})

			Convey(`When datastore Put fails, returns internal error.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "PutMulti")

				_, err := be.ArchiveStream(c, req)
				So(err, ShouldBeRPCInternal)
			})
		})
	})
}
