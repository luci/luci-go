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

func TestCleanupStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c := memory.Use(context.Background())
		be := Server{}

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		// Register a testing log stream (archived, not cleaned up).
		ls := ct.TestLogStream(c, ct.TestLogStreamDescriptor(c, "foo"))
		ls.State = coordinator.LSArchived
		if !ls.Archived() {
			panic("log stream not archived")
		}
		if err := ls.Put(ds.Get(c)); err != nil {
			panic(err)
		}

		req := &logdog.CleanupStreamRequest{
			Path: string(ls.Path()),
		}

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := be.CleanupStream(c, req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			Convey(`Will mark the stream as cleaned up.`, func() {
				_, err := be.CleanupStream(c, req)
				So(err, ShouldBeNil)

				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.State, ShouldEqual, coordinator.LSDone)
			})

			Convey(`Will refuse to process an invalid stream path.`, func() {
				req.Path = "!!!invalid!!!"
				_, err := be.CleanupStream(c, req)
				So(err, ShouldBeRPCInvalidArgument, "invalid log stream path")
			})

			Convey(`If stream is already cleaned up, will not update and return success.`, func() {
				ls.State = coordinator.LSDone
				So(ls.Put(ds.Get(c)), ShouldBeNil)

				_, err := be.CleanupStream(c, req)
				So(err, ShouldBeNil)

				ls.TerminalIndex = -1 // To make sure it reloaded.
				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.State, ShouldEqual, coordinator.LSDone)
			})

			Convey(`If log stream isn't archived yet, will refuse to cleanup.`, func() {
				ls.State = coordinator.LSPending
				So(ls.Put(ds.Get(c)), ShouldBeNil)

				_, err := be.CleanupStream(c, req)
				So(err, ShouldBeRPCInternal)

				ls.TerminalIndex = -1 // To make sure it reloaded.
				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.State, ShouldEqual, coordinator.LSPending)
			})

			Convey(`When datastore Get fails, returns internal error.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "GetMulti")

				_, err := be.CleanupStream(c, req)
				So(err, ShouldBeRPCInternal)
			})

			Convey(`When datastore Put fails, returns internal error.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "PutMulti")

				_, err := be.CleanupStream(c, req)
				So(err, ShouldBeRPCInternal)
			})
		})
	})
}
