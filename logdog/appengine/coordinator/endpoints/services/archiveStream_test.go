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

	"go.chromium.org/luci/gae/filter/featureBreaker"
	ds "go.chromium.org/luci/gae/service/datastore"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestArchiveStream(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()

		svr := New(ServerSettings{NumQueues: 2})

		// Register a testing log stream with an archive tasked.
		env.AddProject(c, "proj-foo")
		tls := ct.MakeStream(c, "proj-foo", "some-realm", "testing/+/foo")
		tls.State.ArchivalKey = []byte("archival key")
		assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchiveTasked))
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
		}

		t.Run(`Returns Forbidden error if not a service.`, func(t *ftt.Test) {
			_, err := svr.ArchiveStream(c, req)
			assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
		})

		t.Run(`When logged in as a service`, func(t *ftt.Test) {
			env.ActAsService()

			t.Run(`Will mark the stream as archived.`, func(t *ftt.Test) {
				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, tls.Get(c), should.BeNil)
				assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
				assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchivedComplete))

				assert.Loosely(t, tls.State.Updated, should.Resemble(now))
				assert.Loosely(t, tls.State.ArchivalKey, should.BeNil)
				assert.Loosely(t, tls.State.TerminatedTime, should.Resemble(now))
				assert.Loosely(t, tls.State.ArchivedTime, should.Resemble(now))
				assert.Loosely(t, tls.State.TerminalIndex, should.Equal(13))
				assert.Loosely(t, tls.State.ArchiveLogEntryCount, should.Equal(14))
				assert.Loosely(t, tls.State.ArchiveStreamURL, should.Equal("gs://fake.stream"))
				assert.Loosely(t, tls.State.ArchiveStreamSize, should.Equal(10))
				assert.Loosely(t, tls.State.ArchiveIndexURL, should.Equal("gs://fake.index"))
				assert.Loosely(t, tls.State.ArchiveIndexSize, should.Equal(20))
			})

			t.Run(`Will mark the stream as partially archived if not complete.`, func(t *ftt.Test) {
				req.LogEntryCount = 13

				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, tls.Get(c), should.BeNil)
				assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
				assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchivedPartial))

				assert.Loosely(t, tls.State.Updated, should.Resemble(now))
				assert.Loosely(t, tls.State.ArchivalKey, should.BeNil)
				assert.Loosely(t, tls.State.TerminatedTime, should.Resemble(now))
				assert.Loosely(t, tls.State.ArchivedTime, should.Resemble(now))
				assert.Loosely(t, tls.State.TerminalIndex, should.Equal(13))
				assert.Loosely(t, tls.State.ArchiveLogEntryCount, should.Equal(13))
				assert.Loosely(t, tls.State.ArchiveStreamURL, should.Equal("gs://fake.stream"))
				assert.Loosely(t, tls.State.ArchiveStreamSize, should.Equal(10))
				assert.Loosely(t, tls.State.ArchiveIndexURL, should.Equal("gs://fake.index"))
				assert.Loosely(t, tls.State.ArchiveIndexSize, should.Equal(20))
			})

			t.Run(`Will refuse to process an invalid stream hash.`, func(t *ftt.Test) {
				req.Id = "!!!invalid!!!"
				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("Invalid ID"))
			})

			t.Run(`If index URL is missing, will refuse to mark the stream archived.`, func(t *ftt.Test) {
				req.IndexUrl = ""

				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
			})

			t.Run(`If stream URL is missing, will refuse to mark the stream archived.`, func(t *ftt.Test) {
				req.StreamUrl = ""

				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)())
			})

			t.Run(`If stream is already archived, will not update and return success.`, func(t *ftt.Test) {
				tls.State.TerminalIndex = 1337
				tls.State.ArchiveLogEntryCount = 42
				tls.State.ArchivedTime = now
				tls.State.TerminatedTime = now

				assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
				assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchivedPartial))
				assert.Loosely(t, tls.Put(c), should.BeNil)

				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, should.BeNil)

				tls.State.TerminalIndex = -1 // To make sure it reloaded.
				assert.Loosely(t, tls.Get(c), should.BeNil)
				assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
				assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchivedPartial))

				assert.Loosely(t, tls.State.TerminalIndex, should.Equal(1337))
				assert.Loosely(t, tls.State.ArchiveLogEntryCount, should.Equal(42))
			})

			t.Run(`If the archive has failed, it is archived as an empty stream.`, func(t *ftt.Test) {
				req.Error = "archive error"

				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, tls.Get(c), should.BeNil)
				assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchivedComplete))

				assert.Loosely(t, tls.State.ArchivalKey, should.BeNil)
				assert.Loosely(t, tls.State.TerminalIndex, should.Equal(-1))
				assert.Loosely(t, tls.State.ArchiveLogEntryCount, should.BeZero)
			})

			t.Run(`When datastore Get fails, returns internal error.`, func(t *ftt.Test) {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "GetMulti")

				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInternal)())
			})

			t.Run(`When datastore Put fails, returns internal error.`, func(t *ftt.Test) {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "PutMulti")

				_, err := svr.ArchiveStream(c, req)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInternal)())
			})
		})
	})
}
