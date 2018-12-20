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
	"context"
	"testing"
	"time"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRescheduleArchiveTask(t *testing.T) {
	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install(true)

		// Make it so that any 2s sleep timers progress.
		env.Clock.SetTimerCallback(func(d time.Duration, tmr clock.Timer) {
			env.Clock.Add(3 * time.Second)
		})

		// By default, the testing user is a service.
		env.JoinGroup("services")

		svr := New()

		Convey(`Returns Forbidden error if not a service.`, func() {
			env.LeaveAllGroups()

			_, err := svr.RescheduleArchiveTask(c, &logdog.ArchiveDispatchTask{})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`Denies empty request`, func() {
			_, err := svr.RescheduleArchiveTask(c, &logdog.ArchiveDispatchTask{})
			So(err, ShouldBeRPCInvalidArgument)
		})

		Convey(`Denies non-existant log stream`, func() {
			_, err := svr.RescheduleArchiveTask(c, &logdog.ArchiveDispatchTask{
				Id: "111111notexist",
			})
			So(err, ShouldBeRPCNotFound)
		})

		// These tests are flaky because env.IterateTumbleAll() appears to be flaky.
		tls := ct.MakeStream(c, "proj-foo", "testing/+/foo/bar")
		tls.WithProjectNamespace(c, func(c context.Context) {
			Convey(`Dispatch an archive task`, func() {
				So(tls.Put(c), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()
				_, err := svr.RescheduleArchiveTask(c, &logdog.ArchiveDispatchTask{
					Id: string(tls.Stream.ID),
				})
				So(err, ShouldBeNil)
				Convey(`And it makes it to the Archival Publisher`, func() {
					env.Clock.Set(clock.Now(c).Add(time.Hour))
					env.IterateTumbleAll(c)
					// It's possible this might be flaky, but it seems to work okay for now.
					SkipSo(env.ArchivalPublisher.Hashes(), ShouldResemble, []string{string(tls.Stream.ID)})
				})
			})

			Convey(`Dispatch second archive`, func() {
				// Having a non-zero archival key is the hint that this is a retry.
				tls.State.ArchivalKey = []byte("fake1234key")
				So(tls.Put(c), ShouldBeNil)
				datastore.GetTestable(c).CatchupIndexes()
				_, err := svr.RescheduleArchiveTask(c, &logdog.ArchiveDispatchTask{
					Id: string(tls.Stream.ID),
				})
				So(err, ShouldBeNil)
				env.Clock.Set(clock.Now(c).Add(time.Hour))
				env.IterateTumbleAll(c)
				SkipSo(tls.Get(c), ShouldBeNil)
				SkipSo(tls.State.ArchiveRetryCount, ShouldEqual, 1)
				SkipSo(env.ArchivalPublisher.Hashes(), ShouldResemble, []string{string(tls.Stream.ID)})
			})
		})
	})
}
