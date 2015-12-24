// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"bytes"
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/services"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/appengine/ephelper/assertions"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRegisterStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		s := Service{
			ServiceBase: ephelper.ServiceBase{
				Middleware: ephelper.TestMode,
			},
		}

		c = ct.UseConfig(c, &services.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := s.RegisterStream(c, &RegisterStreamRequest{})
			So(err, ShouldBeForbiddenError)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			desc := ct.TestLogStreamDescriptor(c, "foo/bar")
			pb, err := proto.Marshal(desc)
			So(err, ShouldBeNil)

			secret := bytes.Repeat([]byte{0xAA}, types.StreamSecretLength)

			Convey(`A stream registration request for "testing/+/foo/bar"`, func() {
				req := RegisterStreamRequest{
					Path:         "testing/+/foo/bar",
					Secret:       secret,
					ProtoVersion: protocol.Version,
					Descriptor:   pb,
				}

				expResp := &RegisterStreamResponse{
					Path:   "testing/+/foo/bar",
					Secret: secret,
					State: &lep.LogStreamState{
						ProtoVersion:  protocol.Version,
						Created:       lep.ToRFC3339(coordinator.NormalizeTime(tc.Now().UTC())),
						Updated:       lep.ToRFC3339(coordinator.NormalizeTime(tc.Now().UTC())),
						TerminalIndex: -1,
					},
				}

				Convey(`Can register the stream.`, func() {
					resp, err := s.RegisterStream(c, &req)
					So(err, ShouldBeNil)
					So(resp, ShouldResembleV, expResp)

					Convey(`Can register the stream again (idempotent).`, func() {
						resp, err := s.RegisterStream(c, &req)
						So(err, ShouldBeNil)
						So(resp, ShouldResembleV, expResp)
					})

					Convey(`Will not re-register if scerets don't match.`, func() {
						req.Secret[0] = 0xAB
						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeConflictError, "log stream is already incompatibly registered")
					})

					Convey(`Will not re-register if descriptor data differs.`, func() {
						desc.Tags = append(desc.Tags, &protocol.LogStreamDescriptor_Tag{"testing", "value"})
						req.Descriptor, err = proto.Marshal(desc)
						So(err, ShouldBeNil)

						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeConflictError, "log stream is already incompatibly registered")
					})
				})

				Convey(`Returns internal server error if the datastore Get() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")

					_, err := s.RegisterStream(c, &req)
					So(err, ShouldBeInternalServerError)
				})

				Convey(`Returns internal server error if the datastore Put() fails.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "PutMulti")

					_, err := s.RegisterStream(c, &req)
					So(err, ShouldBeInternalServerError)
				})

				Convey(`Registration failure cases`, func() {
					Convey(`Will not register a stream with an invalid path.`, func() {
						req.Path = "has/no/name"
						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Invalid path")
					})

					Convey(`Will not register a stream without a protobuf version.`, func() {
						req.ProtoVersion = ""
						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "No protobuf version supplied.")
					})

					Convey(`Will not register a stream with an unknown protobuf version.`, func() {
						req.ProtoVersion = "unknown"
						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Unrecognized protobuf version.")
					})

					Convey(`Will not register a wrong-sized secret.`, func() {
						req.Secret = nil
						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Invalid secret length")
					})

					Convey(`Will not register with an empty descriptor.`, func() {
						req.Descriptor = nil
						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Missing log stream descriptor.")
					})

					Convey(`Will not register with an invalid descriptor.`, func() {
						req.Descriptor = []byte{0x00} // Invalid tag, "0".
						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Could not unmarshal Descriptor protobuf.")
					})

					Convey(`Will not register if the descriptor's Prefix doesn't match.`, func() {
						desc.Prefix = "different"
						req.Descriptor, err = proto.Marshal(desc)
						So(err, ShouldBeNil)

						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Descriptor prefix does not match path")
					})

					Convey(`Will not register if the descriptor's Name doesn't match.`, func() {
						desc.Name = "different"
						req.Descriptor, err = proto.Marshal(desc)
						So(err, ShouldBeNil)

						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Descriptor name does not match path")
					})

					Convey(`Will not register if the descriptor doesn't validate.`, func() {
						desc.ContentType = ""
						So(desc.Validate(true), ShouldNotBeNil)
						req.Descriptor, err = proto.Marshal(desc)
						So(err, ShouldBeNil)

						_, err := s.RegisterStream(c, &req)
						So(err, ShouldBeBadRequestError, "Invalid log stream descriptor")
					})
				})
			})
		})
	})
}
