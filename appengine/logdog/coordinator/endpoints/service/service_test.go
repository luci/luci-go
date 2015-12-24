// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package service

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/proto/logdog/services"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/appengine/ephelper/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c = memory.Use(c)

		c = auth.SetAuthenticator(c, auth.Authenticator{&authtest.FakeAuth{}})
		Convey(`Will reject all traffic if no configuration is present.`, func() {
			So(Auth(c), ShouldBeInternalServerError)
		})

		Convey(`With an application config installed`, func() {
			c := ct.UseConfig(c, &services.Coordinator{
				ServiceAuthGroup: "test-services",
			})

			Convey(`Will reject users if there is an authentication error (no state).`, func() {
				So(Auth(c), ShouldBeInternalServerError)
			})

			Convey(`With an authentication state`, func() {
				fs := authtest.FakeState{}
				c = auth.WithState(c, &fs)

				Convey(`Will reject users who are not logged in.`, func() {
					So(Auth(c), ShouldBeForbiddenError)
				})

				Convey(`When a user is logged in`, func() {
					fs.Identity = "user:user@example.com"

					Convey(`Will reject users who are not members of the service group.`, func() {
						So(Auth(c), ShouldBeForbiddenError)
					})

					Convey(`Will allow users who are members of the service group.`, func() {
						fs.IdentityGroups = []string{"test-services"}
						So(Auth(c), ShouldBeNil)
					})
				})
			})
		})
	})
}
