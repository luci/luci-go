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
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestTerminateStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install()

		env.ModConfig(func(cfg *svcconfig.Coordinator) {
			cfg.ArchiveTopic = "projects/test/topics/archive"
			cfg.ArchiveSettleDelay = google.NewDuration(10 * time.Second)
			cfg.ArchiveDelayMax = google.NewDuration(24 * time.Hour)
		})

		svr := New()

		tls := ct.MakeStream(c, "proj-foo", "testing/+/foo/bar")

		req := logdog.TerminateStreamRequest{
			Project:       string(tls.Project),
			Id:            string(tls.Stream.ID),
			Secret:        tls.Prefix.Secret,
			TerminalIndex: 1337,
		}

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := svr.TerminateStream(c, &req)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			env.JoinGroup("services")

			Convey(`A non-terminal registered stream, "testing/+/foo/bar"`, func() {
				tls.WithProjectNamespace(c, func(c context.Context) {
					So(tls.Put(c), ShouldBeNil)

					// Create an archival request for Tumble so we can ensure that it is
					// canceled on termination.
					areq := mutations.CreateArchiveTask{
						ID:         tls.Stream.ID,
						Expiration: env.Clock.Now().Add(time.Hour),
					}
					arParent, arName := areq.TaskName(ds.Get(c))
					err := tumble.PutNamedMutations(c, arParent, map[string]tumble.Mutation{
						arName: &areq,
					})
					if err != nil {
						panic(err)
					}
				})
				ds.Get(c).Testable().CatchupIndexes()

				Convey(`Can be marked terminal and schedules an archival task.`, func() {
					_, err := svr.TerminateStream(c, &req)
					So(err, ShouldBeRPCOK)
					ds.Get(c).Testable().CatchupIndexes()

					// Reload the state and confirm.
					tls.WithProjectNamespace(c, func(c context.Context) {
						So(ds.Get(c).Get(tls.State), ShouldBeNil)
					})
					So(tls.State.TerminalIndex, ShouldEqual, 1337)
					So(tls.State.Terminated(), ShouldBeTrue)
					So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchiveTasked)
					So(env.ArchivalPublisher.Hashes(), ShouldResemble, []string{string(tls.Stream.ID)})

					// Assert that all archive tasks are scheduled ArchiveSettleDelay in
					// the future.
					for _, t := range env.ArchivalPublisher.Tasks() {
						So(t.SettleDelay.Duration(), ShouldEqual, 10*time.Second)
						So(t.CompletePeriod.Duration(), ShouldEqual, 24*time.Hour)
					}

					Convey(`Will cancel the expiration archive Tumble task.`, func() {
						// We will test this by reverting the stream to be not terminated
						// so that if the Tumble task gets fired, it will try and schedule
						// another archival task.
						env.ArchivalPublisher.Clear()

						tls.State.TerminalIndex = -1
						So(tls.Put(c), ShouldBeNil)

						env.Clock.Add(time.Hour)
						env.DrainTumbleAll(c)
						So(env.ArchivalPublisher.Hashes(), ShouldResemble, []string{})
					})

					Convey(`Can be marked terminal again (idempotent).`, func() {
						_, err := svr.TerminateStream(c, &req)
						So(err, ShouldBeRPCOK)

						// Reload state and confirm.
						So(tls.Get(c), ShouldBeNil)

						So(tls.State.Terminated(), ShouldBeTrue)
						So(tls.State.TerminalIndex, ShouldEqual, 1337)
						So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchiveTasked)
					})

					Convey(`Will reject attempts to change the terminal index.`, func() {
						req.TerminalIndex = 1338
						_, err := svr.TerminateStream(c, &req)
						So(err, ShouldBeRPCFailedPrecondition, "Log stream is incompatibly terminated.")

						// Reload state and confirm.
						So(tls.Get(c), ShouldBeNil)

						So(tls.State.TerminalIndex, ShouldEqual, 1337)
						So(tls.State.Terminated(), ShouldBeTrue)
						So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchiveTasked)
					})

					Convey(`Will reject attempts to clear the terminal index.`, func() {
						req.TerminalIndex = -1
						_, err := svr.TerminateStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Negative terminal index.")

						// Reload state and confirm.
						So(tls.Get(c), ShouldBeNil)

						So(tls.State.TerminalIndex, ShouldEqual, 1337)
						So(tls.State.Terminated(), ShouldBeTrue)
						So(tls.State.ArchivalState(), ShouldEqual, coordinator.ArchiveTasked)
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
				req.Id = "!!!invalid path!!!"
				_, err := svr.TerminateStream(c, &req)
				So(err, ShouldBeRPCInvalidArgument, "Invalid ID")
			})

			Convey(`Will fail if the stream is not registered.`, func() {
				_, err := svr.TerminateStream(c, &req)
				So(err, ShouldBeRPCNotFound, "is not registered")
			})
		})
	})
}
