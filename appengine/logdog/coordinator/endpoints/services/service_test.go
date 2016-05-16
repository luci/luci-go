// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"testing"

	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/server/auth"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install()

		svr := New().(*logdog.DecoratedServices)

		Convey(`Will reject all traffic if no configuration is present.`, func() {
			env.ClearCoordinatorConfig(c)

			_, err := svr.Prelude(c, "test", nil)
			So(err, ShouldBeRPCInternal)
		})

		Convey(`With an application config installed`, func() {
			Convey(`Will reject users if there is an authentication error (no state).`, func() {
				c = auth.WithState(c, nil)

				_, err := svr.Prelude(c, "test", nil)
				So(err, ShouldBeRPCInternal)
			})

			Convey(`With an authentication state`, func() {
				Convey(`Will reject users who are not logged in.`, func() {
					_, err := svr.Prelude(c, "test", nil)
					So(err, ShouldBeRPCPermissionDenied)
				})

				Convey(`When a user is logged in`, func() {
					env.AuthState.Identity = "user:user@example.com"

					Convey(`Will reject users who are not members of the service group.`, func() {
						_, err := svr.Prelude(c, "test", nil)
						So(err, ShouldBeRPCPermissionDenied)
					})

					Convey(`Will allow users who are members of the service group.`, func() {
						env.JoinGroup("services")

						_, err := svr.Prelude(c, "test", nil)
						So(err, ShouldBeNil)
					})
				})
			})
		})
	})
}
