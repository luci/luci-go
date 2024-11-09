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
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/logdog/common/types"
)

func TestRegisterStream(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()
		env.AddProject(c, "proj-foo")

		// By default, the testing user is a service.
		env.ActAsService()

		svr := New(ServerSettings{NumQueues: 2})

		// The testable TQ object.
		ts := taskqueue.GetTestable(c)
		ts.CreatePullQueue(RawArchiveQueueName(0))
		ts.CreatePullQueue(RawArchiveQueueName(1))

		t.Run(`Returns Forbidden error if not a service.`, func(t *ftt.Test) {
			env.ActAsNobody()

			_, err := svr.RegisterStream(c, &logdog.RegisterStreamRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
		})

		t.Run(`When registering a testing log sream, "testing/+/foo/bar"`, func(t *ftt.Test) {
			tls := ct.MakeStream(c, "proj-foo", "some-realm", "testing/+/foo/bar")

			req := logdog.RegisterStreamRequest{
				Project:       string(tls.Project),
				Secret:        tls.Prefix.Secret,
				ProtoVersion:  logpb.Version,
				Desc:          tls.DescBytes(),
				TerminalIndex: -1,
			}

			t.Run(`Returns FailedPrecondition when the Prefix is not registered.`, func(t *ftt.Test) {
				_, err := svr.RegisterStream(c, &req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
			})

			t.Run(`When the Prefix is registered`, func(t *ftt.Test) {
				tls.WithProjectNamespace(c, func(c context.Context) {
					if err := ds.Put(c, tls.Prefix); err != nil {
						panic(err)
					}
				})

				expResp := &logdog.RegisterStreamResponse{
					Id: string(tls.Stream.ID),
					State: &logdog.InternalLogStreamState{
						Secret:        tls.Prefix.Secret,
						ProtoVersion:  logpb.Version,
						TerminalIndex: -1,
					},
				}

				t.Run(`Can register the stream.`, func(t *ftt.Test) {
					created := ds.RoundTime(env.Clock.Now())

					resp, err := svr.RegisterStream(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, should.Resemble(expResp))
					ds.GetTestable(c).CatchupIndexes()

					assert.Loosely(t, tls.Get(c), should.BeNil)

					// Registers the log stream.
					assert.Loosely(t, tls.Stream.Created, should.Resemble(created))
					assert.Loosely(t, tls.Stream.ExpireAt, should.Resemble(created.Add(coordinator.LogStreamExpiry)))

					// Registers the log stream state.
					assert.Loosely(t, tls.State.Created, should.Resemble(created))
					assert.Loosely(t, tls.State.Updated, should.Resemble(created))
					assert.Loosely(t, tls.State.ExpireAt, should.Resemble(created.Add(coordinator.LogStreamStateExpiry)))
					assert.Loosely(t, tls.State.Secret, should.Resemble(req.Secret))
					assert.Loosely(t, tls.State.TerminalIndex, should.Equal(-1))
					assert.Loosely(t, tls.State.Terminated(), should.BeFalse)
					// Pessimistic archival is scheduled.
					assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchiveTasked))

					// Should also register the log stream Prefix.
					assert.Loosely(t, tls.Prefix.Created, should.Resemble(created))
					assert.Loosely(t, tls.Prefix.Secret, should.Resemble(req.Secret))

					t.Run(`Can register the stream again (idempotent).`, func(t *ftt.Test) {
						env.Clock.Set(created.Add(10 * time.Minute))

						resp, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
						assert.Loosely(t, resp, should.Resemble(expResp))

						tls.WithProjectNamespace(c, func(c context.Context) {
							assert.Loosely(t, ds.Get(c, tls.Stream, tls.State), should.BeNil)
						})
						assert.Loosely(t, tls.State.Created, should.Resemble(created))
						assert.Loosely(t, tls.State.ExpireAt, should.Resemble(created.Add(coordinator.LogStreamStateExpiry)))
						assert.Loosely(t, tls.Stream.Created, should.Resemble(created))
						assert.Loosely(t, tls.Stream.ExpireAt, should.Resemble(created.Add(coordinator.LogStreamExpiry)))

						t.Run(`Skips archival completely after 3 weeks`, func(t *ftt.Test) {
							// Three weeks and an hour later
							threeWeeks := (time.Hour * 24 * 7 * 3) + time.Hour
							env.Clock.Set(created.Add(threeWeeks))
							// Make it so that any 2s sleep timers progress.
							env.Clock.SetTimerCallback(func(d time.Duration, tmr clock.Timer) {
								env.Clock.Add(3 * time.Second)
							})

							tls.WithProjectNamespace(c, func(c context.Context) {
								assert.Loosely(t, ds.Get(c, tls.State), should.BeNil)
							})
							// XXX: why is this skipped?
							// assert.That(t, tls.State.ArchivedTime, should.HappenAfter(created.Add(threeWeeks)))
						})
					})

					t.Run(`Will not re-register if secrets don't match.`, func(t *ftt.Test) {
						req.Secret[0] = 0xAB
						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("invalid secret"))
					})
				})

				t.Run(`Can register a terminal stream.`, func(t *ftt.Test) {
					// Make it so that any 2s sleep timers progress.
					env.Clock.SetTimerCallback(func(d time.Duration, tmr clock.Timer) {
						env.Clock.Add(3 * time.Second)
					})
					prefixCreated := ds.RoundTime(env.Clock.Now())
					streamCreated := prefixCreated // same time as the prefix
					req.TerminalIndex = 1337
					expResp.State.TerminalIndex = 1337

					resp, err := svr.RegisterStream(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
					assert.Loosely(t, resp, should.Resemble(expResp))
					ds.GetTestable(c).CatchupIndexes()

					assert.Loosely(t, tls.Get(c), should.BeNil)

					// Registers the log stream.
					assert.Loosely(t, tls.Stream.Created, should.Resemble(streamCreated))
					assert.Loosely(t, tls.Stream.ExpireAt, should.Resemble(streamCreated.Add(coordinator.LogStreamExpiry)))

					// Registers the log stream state.
					assert.Loosely(t, tls.State.Created, should.Resemble(streamCreated))
					assert.Loosely(t, tls.State.ExpireAt, should.Resemble(streamCreated.Add(coordinator.LogStreamStateExpiry)))
					// Tasking for archival should happen after creation.
					assert.Loosely(t, tls.State.Updated.Before(streamCreated), should.BeFalse)
					assert.Loosely(t, tls.State.Secret, should.Resemble(req.Secret))
					assert.Loosely(t, tls.State.TerminalIndex, should.Equal(1337))
					assert.Loosely(t, tls.State.TerminatedTime, should.Resemble(streamCreated))
					assert.Loosely(t, tls.State.Terminated(), should.BeTrue)
					assert.Loosely(t, tls.State.ArchivalState(), should.Equal(coordinator.ArchiveTasked))

					// Should also register the log stream Prefix.
					assert.Loosely(t, tls.Prefix.Created, should.Resemble(prefixCreated))
					assert.Loosely(t, tls.Prefix.Secret, should.Resemble(req.Secret))

					// When we advance to our settle delay, an archival task is scheduled.
					env.Clock.Add(10 * time.Minute)
				})

				t.Run(`Will schedule the correct archival expiration delay`, func(t *ftt.Test) {
					t.Run(`When there is no project config delay.`, func(t *ftt.Test) {
						// Make it so that any 2s sleep timers progress.
						env.Clock.SetTimerCallback(func(d time.Duration, tmr clock.Timer) {
							env.Clock.Add(3 * time.Second)
						})

						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
						ds.GetTestable(c).CatchupIndexes()
					})

					t.Run(`When there is no service or project config delay.`, func(t *ftt.Test) {
						// Make it so that any 2s sleep timers progress.
						env.Clock.SetTimerCallback(func(d time.Duration, tmr clock.Timer) {
							env.Clock.Add(3 * time.Second)
						})

						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
						ds.GetTestable(c).CatchupIndexes()
					})
				})

				t.Run(`Returns internal server error if the datastore Get() fails.`, func(t *ftt.Test) {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")

					_, err := svr.RegisterStream(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				})

				t.Run(`Returns internal server error if the Prefix Put() fails.`, func(t *ftt.Test) {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")

					_, err := svr.RegisterStream(c, &req)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.Internal))
				})

				t.Run(`Registration failure cases`, func(t *ftt.Test) {
					t.Run(`Will not register a stream if its prefix has expired.`, func(t *ftt.Test) {
						env.Clock.Set(tls.Prefix.Expiration)

						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
						assert.Loosely(t, err, should.ErrLike("prefix has expired"))
					})

					t.Run(`Will not register a stream without a protobuf version.`, func(t *ftt.Test) {
						req.ProtoVersion = ""
						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("Unrecognized protobuf version"))
					})

					t.Run(`Will not register a stream with an unknown protobuf version.`, func(t *ftt.Test) {
						req.ProtoVersion = "unknown"
						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("Unrecognized protobuf version"))
					})

					t.Run(`Will not register with an empty descriptor.`, func(t *ftt.Test) {
						req.Desc = nil

						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("Invalid log stream descriptor"))
					})

					t.Run(`Will not register if the descriptor doesn't validate.`, func(t *ftt.Test) {
						tls.Desc.ContentType = ""
						assert.Loosely(t, tls.Desc.Validate(true), should.NotBeNil)
						req.Desc = tls.DescBytes()

						_, err := svr.RegisterStream(c, &req)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("Invalid log stream descriptor"))
					})
				})
			})
		})
	})
}

func BenchmarkRegisterStream(b *testing.B) {
	c, env := ct.Install()

	// By default, the testing user is a service.
	env.ActAsService()

	const (
		prefix  = types.StreamName("testing")
		project = "proj-foo"
	)

	env.AddProject(c, project)
	tls := ct.MakeStream(c, project, "some-realm", prefix.Join(types.StreamName("foo/bar")))
	tls.WithProjectNamespace(c, func(c context.Context) {
		if err := ds.Put(c, tls.Prefix); err != nil {
			b.Fatalf("failed to register prefix: %v", err)
		}
	})

	svr := New(ServerSettings{NumQueues: 2})

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tls := ct.MakeStream(c, project, "some-realm", prefix.Join(types.StreamName(fmt.Sprintf("foo/bar/%d", i))))

		req := logdog.RegisterStreamRequest{
			Project:       string(tls.Project),
			Secret:        tls.Prefix.Secret,
			ProtoVersion:  logpb.Version,
			Desc:          tls.DescBytes(),
			TerminalIndex: -1,
		}

		_, err := svr.RegisterStream(c, &req)
		if err != nil {
			b.Fatalf("failed to get OK response (%s)", err)
		}
	}
}
