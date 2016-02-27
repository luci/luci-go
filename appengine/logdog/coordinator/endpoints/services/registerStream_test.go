// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"bytes"
	"errors"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	ds "github.com/luci/gae/service/datastore"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/appengine/logdog/coordinator/hierarchy"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRegisterStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		tt := tumble.NewTesting()
		c := tt.Context()
		ds.Get(c).Testable().Consistent(true)
		be := Server{}

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := be.RegisterStream(c, &services.RegisterStreamRequest{})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			desc := ct.TestLogStreamDescriptor(c, "foo/bar")
			secret := bytes.Repeat([]byte{0xAA}, types.StreamSecretLength)

			Convey(`A stream registration request for "testing/+/foo/bar"`, func() {
				req := services.RegisterStreamRequest{
					Path:         "testing/+/foo/bar",
					Secret:       secret,
					ProtoVersion: logpb.Version,
					Desc:         desc,
				}

				expResp := &services.LogStreamState{
					Path:          "testing/+/foo/bar",
					Secret:        secret,
					ProtoVersion:  logpb.Version,
					TerminalIndex: -1,
				}

				Convey(`Can register the stream.`, func() {
					resp, err := be.RegisterStream(c, &req)
					So(err, ShouldBeRPCOK)
					So(resp, ShouldResemble, expResp)
					ds.Get(c).Testable().CatchupIndexes()
					tt.Drain(c)

					// Should have name components.
					getNameComponents := func(b string) []string {
						l, err := hierarchy.Get(ds.Get(c), hierarchy.Request{Base: b})
						if err != nil {
							panic(err)
						}
						names := make([]string, len(l.Comp))
						for i, e := range l.Comp {
							names[i] = e.Name
							if e.Stream != "" {
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
						resp, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCOK)
						So(resp, ShouldResemble, expResp)
					})

					Convey(`Will not re-register if scerets don't match.`, func() {
						req.Secret[0] = 0xAB
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCAlreadyExists, "Log stream is already incompatibly registered")
					})

					Convey(`Will not re-register if descriptor data differs.`, func() {
						req.Desc.Tags = map[string]string{
							"testing": "value",
						}
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCAlreadyExists, "Log stream is already incompatibly registered")
					})
				})

				Convey(`Returns internal server error if the datastore Get() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")

					_, err := be.RegisterStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Returns internal server error if the datastore Put() fails (in tumble).`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")

					_, err := be.RegisterStream(c, &req)
					So(err, ShouldBeRPCInternal)
				})

				Convey(`Registration failure cases`, func() {
					Convey(`Will not register a stream with an invalid path.`, func() {
						req.Path = "has/no/name"
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid path")
					})

					Convey(`Will not register a stream without a protobuf version.`, func() {
						req.ProtoVersion = ""
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "No protobuf version supplied.")
					})

					Convey(`Will not register a stream with an unknown protobuf version.`, func() {
						req.ProtoVersion = "unknown"
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Unrecognized protobuf version.")
					})

					Convey(`Will not register a wrong-sized secret.`, func() {
						req.Secret = nil
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid secret length")
					})

					Convey(`Will not register with an empty descriptor.`, func() {
						req.Desc = nil
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Missing log stream descriptor.")
					})

					Convey(`Will not register if the descriptor's Prefix doesn't match.`, func() {
						req.Desc.Prefix = "different"
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Descriptor prefix does not match path")
					})

					Convey(`Will not register if the descriptor's Name doesn't match.`, func() {
						req.Desc.Name = "different"
						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Descriptor name does not match path")
					})

					Convey(`Will not register if the descriptor doesn't validate.`, func() {
						req.Desc.ContentType = ""
						So(req.Desc.Validate(true), ShouldNotBeNil)

						_, err := be.RegisterStream(c, &req)
						So(err, ShouldBeRPCInvalidArgument, "Invalid log stream descriptor")
					})
				})
			})
		})
	})
}
