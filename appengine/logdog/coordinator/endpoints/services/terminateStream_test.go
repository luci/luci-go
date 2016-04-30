// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"errors"
	"testing"
	"time"

	"github.com/luci/gae/filter/featureBreaker"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTerminateStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		var tt tumble.Testing
		c := tt.Context()
		tc := clock.Get(c).(testclock.TestClock)
		tt.EnableDelayedMutations(c)

		var tap ct.ArchivalPublisher
		svcStub := ct.Services{
			AP: func() (coordinator.ArchivalPublisher, error) {
				return &tap, nil
			},
		}
		svcStub.InitConfig()
		svcStub.ServiceConfig.Coordinator.ServiceAuthGroup = "test-services"
		svcStub.ServiceConfig.Coordinator.ArchiveTopic = "projects/test/topics/archive"
		svcStub.ServiceConfig.Coordinator.ArchiveSettleDelay = google.NewDuration(10 * time.Second)
		svcStub.ServiceConfig.Coordinator.ArchiveDelayMax = google.NewDuration(24 * time.Hour)
		c = coordinator.WithServices(c, &svcStub)

		svr := New()

		desc := ct.TestLogStreamDescriptor(c, "foo/bar")
		ls := ct.TestLogStream(c, desc)

		req := logdog.TerminateStreamRequest{
			Path:          "testing/+/foo/bar",
			Secret:        ls.Secret,
			TerminalIndex: 1337,
		}

		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := svr.TerminateStream(c, &req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			Convey(`A non-terminal registered stream, "testing/+/foo/bar"`, func() {
				So(ds.Get(c).Put(ls), ShouldBeNil)

				// Create an archival request for Tumble so we can ensure that it is
				// canceled on termination.
				areq := mutations.CreateArchiveTask{
					Path:       ls.Path(),
					Expiration: tc.Now().Add(time.Hour),
				}
				arParent, arName := areq.TaskName(ds.Get(c))
				err := tumble.PutNamedMutations(c, arParent, map[string]tumble.Mutation{
					arName: &areq,
				})
				if err != nil {
					panic(err)
				}
				ds.Get(c).Testable().CatchupIndexes()

				Convey(`Can be marked terminal and schedules an archival task.`, func() {
					_, err := svr.TerminateStream(c, &req)
					So(err, ShouldBeRPCOK)
					ds.Get(c).Testable().CatchupIndexes()

					// Reload "ls" and confirm.
					So(ds.Get(c).Get(ls), ShouldBeNil)
					So(ls.TerminalIndex, ShouldEqual, 1337)
					So(ls.State, ShouldEqual, coordinator.LSArchiveTasked)
					So(ls.Terminated(), ShouldBeTrue)
					So(tap.StreamNames(), ShouldResemble, []string{ls.Name})

					// Assert that all archive tasks are scheduled ArchiveSettleDelay in
					// the future.
					for _, t := range tap.Tasks() {
						So(t.SettleDelay.Duration(), ShouldEqual, svcStub.ServiceConfig.Coordinator.ArchiveSettleDelay.Duration())
						So(t.CompletePeriod.Duration(), ShouldEqual, svcStub.ServiceConfig.Coordinator.ArchiveDelayMax.Duration())
					}

					Convey(`Will cancel the expiration archive Tumble task.`, func() {
						// We will test this by reverting the stream to a LSStreaming state
						// so that if the Tumble task gets fired, it will try and schedule
						// another archival task.
						tap.Clear()

						ls.State = coordinator.LSStreaming
						So(ds.Get(c).Put(ls), ShouldBeNil)

						tc.Add(time.Hour)
						tt.Drain(c)
						So(tap.StreamNames(), ShouldResemble, []string{})
					})

					Convey(`Can be marked terminal again (idempotent).`, func() {
						_, err := svr.TerminateStream(c, &req)
						So(err, ShouldBeRPCOK)

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)

						So(ls.Terminated(), ShouldBeTrue)
						So(ls.TerminalIndex, ShouldEqual, 1337)
						So(ls.State, ShouldEqual, coordinator.LSArchiveTasked)
					})

					Convey(`Will reject attempts to change the terminal index.`, func() {
						req.TerminalIndex = 1338
						_, err := svr.TerminateStream(c, &req)
						So(err, ShouldBeRPCFailedPrecondition, "Log stream is not in streaming state.")

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)

						So(ls.Terminated(), ShouldBeTrue)
						So(ls.State, ShouldEqual, coordinator.LSArchiveTasked)
						So(ls.TerminalIndex, ShouldEqual, 1337)
					})

					Convey(`Will reject attempts to clear the terminal index.`, func() {
						req.TerminalIndex = -1
						_, err := svr.TerminateStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Negative terminal index.")

						// Reload "ls" and confirm.
						So(ds.Get(c).Get(ls), ShouldBeNil)

						So(ls.Terminated(), ShouldBeTrue)
						So(ls.State, ShouldEqual, coordinator.LSArchiveTasked)
						So(ls.TerminalIndex, ShouldEqual, 1337)
					})
				})

				Convey(`Will return an internal server error if Put() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")
					_, err := svr.TerminateStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Will return an internal server error if Get() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")
					_, err := svr.TerminateStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Will return a bad request error if the secret doesn't match.`, func() {
					req.Secret[0] ^= 0xFF
					_, err := svr.TerminateStream(c, &req)
					So(err, ShouldBeRPCInvalidArgument, "Request secret doesn't match the stream secret.")
				})
			})

			Convey(`Will not try and terminate a stream with an invalid path.`, func() {
				req.Path = "!!!invalid path!!!"
				_, err := svr.TerminateStream(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "Invalid path")
			})

			Convey(`Will fail if the stream is not registered.`, func() {
				_, err := svr.TerminateStream(c, &req)
				So(err, ShouldBeRPCNotFound, "is not registered")
			})
		})
	})
}
