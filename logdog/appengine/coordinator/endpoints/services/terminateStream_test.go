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
	"errors"
	"testing"

	"go.chromium.org/luci/gae/filter/featureBreaker"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/taskqueue"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTerminateStream(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()

		svr := New(ServerSettings{NumQueues: 2})

		env.AddProject(c, "proj-foo")
		tls := ct.MakeStream(c, "proj-foo", "some-realm", "testing/+/foo/bar")

		req := logdog.TerminateStreamRequest{
			Project:       string(tls.Project),
			Id:            string(tls.Stream.ID),
			Secret:        tls.Prefix.Secret,
			TerminalIndex: 1337,
		}

		// The testable TQ object.
		ts := taskqueue.GetTestable(c)
		ts.CreatePullQueue(RawArchiveQueueName(0))
		ts.CreatePullQueue(RawArchiveQueueName(1))

		t.Run(`Returns Forbidden error if not a service.`, func(t *ftt.Test) {
			_, err := svr.TerminateStream(c, &req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run(`When logged in as a service`, func(t *ftt.Test) {
			env.ActAsService()

			t.Run(`A non-terminal registered stream, "testing/+/foo/bar"`, func(t *ftt.Test) {
				assert.Loosely(t, tls.Put(c), should.BeNil)
				ds.GetTestable(c).CatchupIndexes()

				t.Run(`Can be marked terminal and schedules an archival mutation.`, func(t *ftt.Test) {
					_, err := svr.TerminateStream(c, &req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())
					ds.GetTestable(c).CatchupIndexes()

					// Reload the state and confirm.
					tls.WithProjectNamespace(c, func(c context.Context) {
						assert.Loosely(t, ds.Get(c, tls.State), should.BeNil)
					})
					assert.Loosely(t, tls.State.TerminalIndex, should.Equal(1337))
					assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
					assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchiveTasked))

					t.Run(`Can be marked terminal again (idempotent).`, func(t *ftt.Test) {
						_, err := svr.TerminateStream(c, &req)
						assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())

						// Reload state and confirm.
						assert.Loosely(t, tls.Get(c), should.BeNil)

						assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
						assert.Loosely(t, tls.State.TerminalIndex, should.Equal(1337))
						assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchiveTasked))
					})

					t.Run(`Will reject attempts to change the terminal index.`, func(t *ftt.Test) {
						req.TerminalIndex = 1338
						_, err := svr.TerminateStream(c, &req)
						assert.Loosely(t, err, convey.Adapt(ShouldBeRPCFailedPrecondition)("Log stream is incompatibly terminated."))

						// Reload state and confirm.
						assert.Loosely(t, tls.Get(c), should.BeNil)

						assert.Loosely(t, tls.State.TerminalIndex, should.Equal(1337))
						assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
						assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchiveTasked))
					})

					t.Run(`Will reject attempts to clear the terminal index.`, func(t *ftt.Test) {
						req.TerminalIndex = -1
						_, err := svr.TerminateStream(c, &req)
						assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("Negative terminal index."))

						// Reload state and confirm.
						assert.Loosely(t, tls.Get(c), should.BeNil)

						assert.Loosely(t, tls.State.TerminalIndex, should.Equal(1337))
						assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
						assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchiveTasked))
					})
				})

				t.Run(`Will return an internal server error if Put() fails.`, func(t *ftt.Test) {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")
					_, err := svr.TerminateStream(c, &req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInternal)())
				})

				t.Run(`Will return an internal server error if Get() fails.`, func(t *ftt.Test) {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")
					_, err := svr.TerminateStream(c, &req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInternal)())
				})

				t.Run(`Will return a bad request error if the secret doesn't match.`, func(t *ftt.Test) {
					req.Secret[0] ^= 0xFF
					_, err := svr.TerminateStream(c, &req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("Request secret doesn't match the stream secret."))
				})
			})

			t.Run(`Will not try and terminate a stream with an invalid path.`, func(t *ftt.Test) {
				req.Id = "!!!invalid path!!!"
				_, err := svr.TerminateStream(c, &req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("Invalid ID"))
			})

			t.Run(`Will fail if the stream does not exist.`, func(t *ftt.Test) {
				_, err := svr.TerminateStream(c, &req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCNotFound)("log stream doesn't exist"))
			})
		})
	})
}
