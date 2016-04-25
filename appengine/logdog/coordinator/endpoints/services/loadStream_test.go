// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"errors"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLoadStream(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		c = memory.Use(c)

		svcStub := ct.Services{}
		svcStub.InitConfig()
		svcStub.ServiceConfig.Coordinator.ServiceAuthGroup = "test-services"
		c = coordinator.WithServices(c, &svcStub)

		svr := New()

		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		// Register a test stream.
		desc := ct.TestLogStreamDescriptor(c, "foo/bar")
		ls := ct.TestLogStream(c, desc)
		if err := ds.Get(c).Put(ls); err != nil {
			panic(err)
		}

		// Prepare a request to load the test stream.
		req := logdog.LoadStreamRequest{
			Path: string(ls.Path()),
		}

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := svr.LoadStream(c, &logdog.LoadStreamRequest{})
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service`, func() {
			fs.IdentityGroups = []string{"test-services"}

			Convey(`Will succeed.`, func() {
				resp, err := svr.LoadStream(c, &req)
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &logdog.LoadStreamResponse{
					State: &logdog.LogStreamState{
						Path:          "testing/+/foo/bar",
						ProtoVersion:  "1",
						TerminalIndex: -1,
					},
				})
			})

			Convey(`Will return archival properties.`, func() {
				tc.Add(1 * time.Hour)
				ls.ArchivalKey = []byte("archival key")
				if err := ds.Get(c).Put(ls); err != nil {
					panic(err)
				}

				resp, err := svr.LoadStream(c, &req)
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &logdog.LoadStreamResponse{
					State: &logdog.LogStreamState{
						Path:          "testing/+/foo/bar",
						ProtoVersion:  "1",
						TerminalIndex: -1,
					},
					ArchivalKey: []byte("archival key"),
					Age:         google.NewDuration(1 * time.Hour),
				})
			})

			Convey(`Will succeed, and return the descriptor when requested.`, func() {
				req.Desc = true

				d, err := proto.Marshal(desc)
				if err != nil {
					panic(err)
				}

				resp, err := svr.LoadStream(c, &req)
				So(err, ShouldBeNil)
				So(resp, ShouldResemble, &logdog.LoadStreamResponse{
					State: &logdog.LogStreamState{
						Path:          "testing/+/foo/bar",
						ProtoVersion:  "1",
						TerminalIndex: -1,
					},
					Desc: d,
				})
			})

			Convey(`Will return InvalidArgument if the stream path is not valid.`, func() {
				req.Path = "no/stream/name"

				_, err := svr.LoadStream(c, &req)
				So(err, ShouldBeRPCInvalidArgument)
			})

			Convey(`Will return NotFound for non-existent streams.`, func() {
				req.Path = "this/stream/+/does/not/exist"

				_, err := svr.LoadStream(c, &req)
				So(err, ShouldBeRPCNotFound)
			})

			Convey(`Will return Internal for random datastore failures.`, func() {
				c, fb := featureBreaker.FilterRDS(c, nil)
				fb.BreakFeatures(errors.New("test error"), "GetMulti")

				_, err := svr.LoadStream(c, &req)
				So(err, ShouldBeRPCInternal)
			})
		})
	})
}
