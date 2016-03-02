// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"errors"
	"testing"
	"time"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTerminateStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		be := Server{}

		desc := ct.TestLogStreamDescriptor(c, "foo/bar")
		ls, err := ct.TestLogStream(c, desc)
		So(err, ShouldBeNil)

		req := logdog.TerminateStreamRequest{
			Path:          "testing/+/foo/bar",
			Secret:        ls.Secret,
			TerminalIndex: 1337,
		}

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := be.TerminateStream(c, &req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			Convey(`A non-terminal registered stream, "testing/+/foo/bar"`, func() {
				So(ls.Put(ds.Get(c)), ShouldBeNil)
				tc.Add(time.Second)

				Convey(`Can be marked terminal.`, func() {
					_, err := be.TerminateStream(c, &req)
					So(err, ShouldBeRPCOK)

					// Reload "ls" and confirm.
					So(ds.Get(c).Get(ls), ShouldBeNil)
					So(ls.TerminalIndex, ShouldEqual, 1337)
					So(ls.State, ShouldEqual, coordinator.LSTerminated)
					So(ls.Updated, ShouldResemble, ls.Created.Add(time.Second))

					Convey(`Can be marked terminal again (idempotent).`, func() {
						_, err := be.TerminateStream(c, &req)
						So(err, ShouldBeRPCOK)

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.TerminalIndex, ShouldEqual, 1337)
						So(ls.State, ShouldEqual, coordinator.LSTerminated)
					})

					Convey(`Will reject attempts to change the terminal index.`, func() {
						req.TerminalIndex = 1338
						_, err := be.TerminateStream(c, &req)
						So(err, ShouldBeRPCAlreadyExists, "Terminal index is already set")

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.State, ShouldEqual, coordinator.LSTerminated)
						So(ls.TerminalIndex, ShouldEqual, 1337)
					})

					Convey(`Will reject attempts to clear the terminal index.`, func() {
						req.TerminalIndex = -1
						_, err := be.TerminateStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Negative terminal index.")

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.State, ShouldEqual, coordinator.LSTerminated)
						So(ls.TerminalIndex, ShouldEqual, 1337)
					})
				})

				Convey(`Will return an internal server error if Put() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")
					_, err := be.TerminateStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Will return an internal server error if Get() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")
					_, err := be.TerminateStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Will return a bad request error if the secret doesn't match.`, func() {
					req.Secret[0] ^= 0xFF
					_, err := be.TerminateStream(c, &req)
					So(err, ShouldBeRPCInvalidArgument, "Request secret doesn't match the stream secret.")
				})
			})

			Convey(`Will not try and terminate a stream with an invalid path.`, func() {
				req.Path = "!!!invalid path!!!"
				_, err := be.TerminateStream(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "Invalid path")
			})

			Convey(`Will fail if the stream is not registered.`, func() {
				_, err := be.TerminateStream(c, &req)
				So(err, ShouldBeRPCNotFound, "is not registered")
			})
		})
	})
}
