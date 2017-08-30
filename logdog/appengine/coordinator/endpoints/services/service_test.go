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
	"testing"

	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/server/auth"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install(true)

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
