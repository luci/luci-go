// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"errors"
	"testing"
	"time"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/ephelper"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/logdog/services"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/appengine/ephelper/eptest"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTerminateStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		s := Service{
			ServiceBase: ephelper.ServiceBase{
				Middleware: ephelper.TestMode,
			},
		}

		desc := ct.TestLogStreamDescriptor(c, "foo/bar")
		ls, err := ct.TestLogStream(c, desc)
		So(err, ShouldBeNil)

		req := TerminateStreamRequest{
			Path:          "testing/+/foo/bar",
			Secret:        ls.Secret,
			TerminalIndex: 1337,
		}

		c = ct.UseConfig(c, &services.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		Convey(`Returns Forbidden error if not a service.`, func() {
			So(s.TerminateStream(c, &req), ShouldBeForbiddenError)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			Convey(`A non-terminal registered stream, "testing/+/foo/bar"`, func() {
				So(ls.Put(ds.Get(c)), ShouldBeNil)
				tc.Add(time.Second)

				Convey(`Can be marked terminal.`, func() {
					So(s.TerminateStream(c, &req), ShouldBeNil)

					// Reload "ls" and confirm.
					So(ds.Get(c).Get(ls), ShouldBeNil)
					So(ls.TerminalIndex, ShouldEqual, 1337)

					Convey(`The stream's Updated timestamp is updated.`, func() {
						So(ls.Updated, ShouldResembleV, ls.Created.Add(time.Second))
					})

					Convey(`Can be marked terminal again (idempotent).`, func() {
						So(s.TerminateStream(c, &req), ShouldBeNil)

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.TerminalIndex, ShouldEqual, 1337)
					})

					Convey(`Will reject attempts to change the terminal index.`, func() {
						req.TerminalIndex = 1338
						So(s.TerminateStream(c, &req), ShouldBeConflictError, "terminal index is already set")

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.TerminalIndex, ShouldEqual, 1337)
					})

					Convey(`Will reject attempts to clear the terminal index.`, func() {
						req.TerminalIndex = -1
						So(s.TerminateStream(c, &req), ShouldBeBadRequestError, "Negative terminal index.")

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.TerminalIndex, ShouldEqual, 1337)
					})
				})

				Convey(`Will return an internal server error if Put() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")
					So(s.TerminateStream(c, &req), ShouldBeInternalServerError)
				})

				Convey(`Will return an internal server error if Get() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")
					So(s.TerminateStream(c, &req), ShouldBeInternalServerError)
				})

				Convey(`Will return a bad request error if the secret doesn't match.`, func() {
					req.Secret[0] ^= 0xFF
					So(s.TerminateStream(c, &req), ShouldBeBadRequestError, "Request Secret doesn't match the stream secret.")
				})
			})

			Convey(`Will not try and terminate a stream with an invalid path.`, func() {
				req.Path = "!!!invalid path!!!"
				So(s.TerminateStream(c, &req), ShouldBeBadRequestError, "Invalid path")
			})

			Convey(`Will fail if the stream is not registered.`, func() {
				So(s.TerminateStream(c, &req), ShouldBeNotFoundError, "is not registered")
			})
		})
	})
}
