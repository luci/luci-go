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
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRegisterStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install()
		env.ModServiceConfig(c, func(cfg *svcconfig.Coordinator) {
			cfg.ArchiveDelayMax = google.NewDuration(time.Hour)
		})
		ds.Get(c).Testable().Consistent(true)

		svr := New()

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := svr.RegisterStream(c, &logdog.RegisterStreamRequest{})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			env.JoinGroup("services")

			tls := ct.MakeStream(c, "proj-foo", "testing/+/foo/bar")

			Convey(`A stream registration request for "testing/+/foo/bar"`, func() {
				req := logdog.RegisterStreamRequest{
					Project:      string(tls.Project),
					Secret:       tls.Prefix.Secret,
					ProtoVersion: logpb.Version,
					Desc:         tls.DescBytes(),
				}

				expResp := &logdog.RegisterStreamResponse{
					Id: string(tls.Stream.ID),
					State: &logdog.LogStreamState{
						Secret:        tls.Prefix.Secret,
						ProtoVersion:  logpb.Version,
						TerminalIndex: -1,
					},
				}

				Convey(`Can register the stream.`, func() {
					created := ds.RoundTime(env.Clock.Now())

					resp, err := svr.RegisterStream(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, ShouldResemble, expResp)
					ds.Get(c).Testable().CatchupIndexes()
					env.DrainTumbleAll(c)

					So(tls.Get(c), ShouldBeNil)

					// Registers the log stream.
					So(tls.Stream.Created, ShouldResemble, created)

					// Registers the log stream state.
					So(tls.State.Created, ShouldResemble, created)
					So(tls.State.Updated, ShouldResemble, created)
					So(tls.State.Secret, ShouldResemble, req.Secret)
					So(tls.State.TerminalIndex, ShouldEqual, -1)
					So(tls.State.Terminated(), ShouldBeFalse)
					So(tls.State.ArchivalState(), ShouldEqual, coordinator.NotArchived)

					// Should also register the log stream Prefix.
					So(tls.Prefix.Created, ShouldResemble, created)
					So(tls.Prefix.Secret, ShouldResemble, req.Secret)

					// No archival request yet.
					So(env.ArchivalPublisher.Hashes(), ShouldResemble, []string{})

					// Should have name components.
					getNameComponents := func(b string) []string {
						l, err := hierarchy.Get(c, hierarchy.Request{Project: string(tls.Project), PathBase: b})
						if err != nil {
							panic(err)
						}
						names := make([]string, len(l.Comp))
						for i, e := range l.Comp {
							names[i] = e.Name
							if e.Stream {
								names[i] += "$"
							}
						}
						return names
					}
					So(getNameComponents(""), ShouldResemble, []string{"testing"})
					So(getNameComponents("testing"), ShouldResemble, []string{"+"})
					So(getNameComponents("testing/+"), ShouldResemble, []string{"foo"})
					So(getNameComponents("testing/+/foo"), ShouldResemble, []string{"bar$"})

					Convey(`Can register the stream again (idempotent).`, func() {
						env.Clock.Set(created.Add(10 * time.Minute))

						resp, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, ShouldResemble, expResp)

						tls.WithProjectNamespace(c, func(c context.Context) {
							So(ds.Get(c).GetMulti([]interface{}{tls.Stream, tls.State}), ShouldBeNil)
						})
						So(tls.State.Created, ShouldResemble, created)
						So(tls.Stream.Created, ShouldResemble, created)

						// No archival request yet.
						So(env.ArchivalPublisher.Hashes(), ShouldResemble, []string{})

						Convey(`Forces an archival request after first archive expiration.`, func() {
							env.Clock.Set(created.Add(time.Hour)) // 1 hour after initial registration.
							env.DrainTumbleAll(c)

							So(env.ArchivalPublisher.Hashes(), ShouldResemble, []string{string(tls.Stream.ID)})
						})
					})

					Convey(`Will not re-register if secrets don't match.`, func() {
						req.Secret[0] = 0xAB
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCAlreadyExists, "Log prefix is already registered")
					})
				})

				Convey(`Returns internal server error if the datastore Get() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")

					_, err := svr.RegisterStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Returns internal server error if the Prefix Put() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")

					_, err := svr.RegisterStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Registration failure cases`, func() {
					Convey(`Will not register a stream without a protobuf version.`, func() {
						req.ProtoVersion = ""
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Unrecognized protobuf version")
					})

					Convey(`Will not register a stream with an unknown protobuf version.`, func() {
						req.ProtoVersion = "unknown"
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Unrecognized protobuf version")
					})

					Convey(`Will not register a wrong-sized secret.`, func() {
						req.Secret = nil
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid prefix secret")
					})

					Convey(`Will not register with an empty descriptor.`, func() {
						req.Desc = nil

						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid log stream descriptor")
					})

					Convey(`Will not register if the descriptor doesn't validate.`, func() {
						tls.Desc.ContentType = ""
						So(tls.Desc.Validate(true), ShouldNotBeNil)
						req.Desc = tls.DescBytes()

						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid log stream descriptor")
					})
				})

				Convey(`The registerStreamMutation`, func() {
					rsm := registerStreamMutation{
						RegisterStreamRequest: &req,
						desc: tls.Desc,
						pfx:  tls.Prefix,
						ls:   tls.Stream,
						lst:  tls.State,
					}

					Convey(`Can RollForward.`, func() {
						_, err := rsm.RollForward(c)
						So(err, ShouldBeNil)
					})

					Convey(`Returns internal server error if the stream Put() fails (tumble).`, func() {
						c, fb := featureBreaker.FilterRDS(c, nil)
						fb.BreakFeatures(errors.New("test error"), "PutMulti")

						_, err := rsm.RollForward(c)
						So(err, ShouldBeRPCInternal)
					})
				})
			})
		})
	})
}
