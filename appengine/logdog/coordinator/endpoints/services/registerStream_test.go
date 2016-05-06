// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/luci/gae/filter/featureBreaker"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logdog/types"
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
		env.ModConfig(func(cfg *svcconfig.Coordinator) {
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

			desc := ct.TestLogStreamDescriptor(c, "foo/bar")
			secret := bytes.Repeat([]byte{0xAA}, types.PrefixSecretLength)

			Convey(`A stream registration request for "testing/+/foo/bar"`, func() {
				const project config.ProjectName = "proj-foo"

				req := logdog.RegisterStreamRequest{
					Project:      string(project),
					Path:         "testing/+/foo/bar",
					Secret:       secret,
					ProtoVersion: logpb.Version,
					Desc:         desc,
				}

				expResp := &logdog.RegisterStreamResponse{
					State: &logdog.LogStreamState{
						Project:       string(project),
						Path:          "testing/+/foo/bar",
						ProtoVersion:  logpb.Version,
						TerminalIndex: -1,
					},
					Secret: secret,
				}

				Convey(`Can register the stream.`, func() {
					created := ds.RoundTime(env.Clock.Now())

					resp, err := svr.RegisterStream(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, ShouldResemble, expResp)
					ds.Get(c).Testable().CatchupIndexes()
					env.DrainTumbleAll(c)

					ls := coordinator.LogStreamFromPath(types.StreamPath(req.Path))
					ct.WithProjectNamespace(c, project, func(c context.Context) {
						So(ds.Get(c).Get(ls), ShouldBeNil)
					})
					So(ls.Created, ShouldResemble, created)
					So(ls.Secret, ShouldResemble, req.Secret)

					// Should also register the log stream Prefix.
					pfx := ls.LogPrefix()
					ct.WithProjectNamespace(c, project, func(c context.Context) {
						So(ds.Get(c).Get(pfx), ShouldBeNil)
					})
					So(pfx.Created, ShouldResemble, created)
					So(pfx.Secret, ShouldResemble, req.Secret)

					// No archival request yet.
					So(env.ArchivalPublisher.StreamNames(), ShouldResemble, []string{})

					// Should have name components.
					getNameComponents := func(b string) []string {
						l, err := hierarchy.Get(c, hierarchy.Request{Project: "proj-foo", PathBase: b})
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

						ls := coordinator.LogStreamFromPath(types.StreamPath(req.Path))
						ct.WithProjectNamespace(c, project, func(c context.Context) {
							So(ds.Get(c).Get(ls), ShouldBeNil)
						})
						So(ls.Created, ShouldResemble, created)

						// No archival request yet.
						So(env.ArchivalPublisher.StreamNames(), ShouldResemble, []string{})

						Convey(`Forces an archival request after first archive expiration.`, func() {
							env.Clock.Set(created.Add(time.Hour)) // 1 hour after initial registration.
							env.DrainTumbleAll(c)

							So(env.ArchivalPublisher.StreamNames(), ShouldResemble, []string{ls.Name})
						})
					})

					Convey(`Will not re-register if secrets don't match.`, func() {
						req.Secret[0] = 0xAB
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCAlreadyExists, "Log prefix is already registered")
					})

					Convey(`Will not re-register if descriptor data differs.`, func() {
						req.Desc.Tags = map[string]string{
							"testing": "value",
						}
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCAlreadyExists, "Log stream is already registered")
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
					Convey(`Will not register a stream with an invalid path.`, func() {
						req.Path = "has/no/name"
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid path")
					})

					Convey(`Will not register a stream without a protobuf version.`, func() {
						req.ProtoVersion = ""
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "No protobuf version supplied.")
					})

					Convey(`Will not register a stream with an unknown protobuf version.`, func() {
						req.ProtoVersion = "unknown"
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Unrecognized protobuf version.")
					})

					Convey(`Will not register a wrong-sized secret.`, func() {
						req.Secret = nil
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid prefix secret")
					})

					Convey(`Will not register with an empty descriptor.`, func() {
						req.Desc = nil
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Missing log stream descriptor.")
					})

					Convey(`Will not register if the descriptor's Prefix doesn't match.`, func() {
						req.Desc.Prefix = "different"
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Descriptor prefix does not match path")
					})

					Convey(`Will not register if the descriptor's Name doesn't match.`, func() {
						req.Desc.Name = "different"
						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Descriptor name does not match path")
					})

					Convey(`Will not register if the descriptor doesn't validate.`, func() {
						req.Desc.ContentType = ""
						So(req.Desc.Validate(true), ShouldNotBeNil)

						_, err := svr.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid log stream descriptor")
					})
				})

				Convey(`The registerStreamMutation`, func() {
					ls := ct.TestLogStream(c, desc)
					pfx := ct.TestLogPrefix(c, desc)

					rsm := registerStreamMutation{
						LogStream: ls,
						req:       &req,
						pfx:       pfx,
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
