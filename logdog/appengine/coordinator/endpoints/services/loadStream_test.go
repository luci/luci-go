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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
)

func TestLoadStream(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()

		svr := New(ServerSettings{NumQueues: 2})

		// Register a test stream.
		env.AddProject(c, "proj-foo")
		tls := ct.MakeStream(c, "proj-foo", "some-realm", "testing/+/foo/bar")
		if err := tls.Put(c); err != nil {
			panic(err)
		}

		// Prepare a request to load the test stream.
		req := &logdog.LoadStreamRequest{
			Project: string(tls.Project),
			Id:      string(tls.Stream.ID),
		}

		t.Run(`Returns Forbidden error if not a service.`, func(t *ftt.Test) {
			_, err := svr.LoadStream(c, &logdog.LoadStreamRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run(`When logged in as a service`, func(t *ftt.Test) {
			env.ActAsService()

			t.Run(`Will succeed.`, func(t *ftt.Test) {
				resp, err := svr.LoadStream(c, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Resemble(&logdog.LoadStreamResponse{
					State: &logdog.InternalLogStreamState{
						ProtoVersion:  "1",
						TerminalIndex: -1,
						Secret:        tls.State.Secret,
					},
					Age: durationpb.New(0),
				}))
			})

			t.Run(`Will return archival properties.`, func(t *ftt.Test) {
				// Add an hour to the clock. Created is +0, Updated is +1hr.
				env.Clock.Add(1 * time.Hour)
				tls.State.ArchivalKey = []byte("archival key")
				tls.Reload(c)
				if err := tls.Put(c); err != nil {
					panic(err)
				}

				// Set time to +2hr, age should now be 1hr.
				env.Clock.Add(1 * time.Hour)
				resp, err := svr.LoadStream(c, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Resemble(&logdog.LoadStreamResponse{
					State: &logdog.InternalLogStreamState{
						ProtoVersion:  "1",
						TerminalIndex: -1,
						Secret:        tls.State.Secret,
					},
					ArchivalKey: []byte("archival key"),
					Age:         durationpb.New(1 * time.Hour),
				}))
			})

			t.Run(`Will succeed, and return the descriptor when requested.`, func(t *ftt.Test) {
				req.Desc = true

				d, err := proto.Marshal(tls.Desc)
				if err != nil {
					panic(err)
				}

				resp, err := svr.LoadStream(c, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp, should.Resemble(&logdog.LoadStreamResponse{
					State: &logdog.InternalLogStreamState{
						ProtoVersion:  "1",
						TerminalIndex: -1,
						Secret:        tls.State.Secret,
					},
					Desc: d,
					Age:  durationpb.New(0),
				}))
			})

			t.Run(`Will return InvalidArgument if the stream hash is not valid.`, func(t *ftt.Test) {
				req.Id = string("!!! not a hash !!!")

				_, err := svr.LoadStream(c, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.Loosely(t, err, should.ErrLike("Invalid ID"))
			})

			t.Run(`Will return NotFound for non-existent streams.`, func(t *ftt.Test) {
				req.Id = string(coordinator.LogStreamID("this/stream/+/does/not/exist"))

				_, err := svr.LoadStream(c, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})

			t.Run(`Will return Internal for random datastore failures.`, func(t *ftt.Test) {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "GetMulti")

				_, err := svr.LoadStream(c, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
			})
		})
	})
}
