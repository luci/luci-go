// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/ephelper"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/logdog/services"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/appengine/ephelper/assertions"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)
		s := Service{
			ServiceBase: ephelper.ServiceBase{
				Middleware: ephelper.TestMode,
			},
		}

		desc := ct.TestLogStreamDescriptor(c, "foo/bar")
		ls, err := ct.TestLogStream(c, desc)
		So(err, ShouldBeNil)

		c = ct.UseConfig(c, &services.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := s.LoadStream(c, &LoadStreamRequest{})
			So(err, ShouldBeForbiddenError)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			Convey(`With a test log stream installed`, func() {
				So(ls.Put(ds.Get(c)), ShouldBeNil)

				d, err := proto.Marshal(desc)
				So(err, ShouldBeNil)

				Convey(`Can load the log stream by name.`, func() {
					lsr, err := s.LoadStream(c, &LoadStreamRequest{
						Path: "testing/+/foo/bar",
					})
					So(err, ShouldBeNil)
					So(lsr, ShouldResembleV, &LoadStreamResponse{
						Path:       "testing/+/foo/bar",
						State:      lep.LoadLogStreamState(ls),
						Descriptor: d,
					})
				})

				Convey(`Can load the log stream by hash.`, func() {
					lsr, err := s.LoadStream(c, &LoadStreamRequest{
						Path: ls.HashID(),
					})
					So(err, ShouldBeNil)

					So(lsr, ShouldResembleV, &LoadStreamResponse{
						Path:       "testing/+/foo/bar",
						State:      lep.LoadLogStreamState(ls),
						Descriptor: d,
					})
				})

				Convey(`Datastore failures return intenral server error.`, func() {
					c, fb := featureBreaker.FilterRDS(c, nil)
					fb.BreakFeatures(errors.New("test error"), "GetMulti")

					_, err := s.LoadStream(c, &LoadStreamRequest{
						Path: "testing/+/baz",
					})
					So(err, ShouldBeInternalServerError)
				})

				Convey(`Requests to non-existent log streams will fail.`, func() {
					_, err := s.LoadStream(c, &LoadStreamRequest{
						Path: "testing/+/baz",
					})
					So(err, ShouldBeNotFoundError)
				})
			})

			Convey(`Requests to non-path non-hash values return "bad request".`, func() {
				_, err := s.LoadStream(c, &LoadStreamRequest{
					Path: "prefix/but/no/name",
				})
				So(err, ShouldBeBadRequestError)
			})
		})
	})
}
