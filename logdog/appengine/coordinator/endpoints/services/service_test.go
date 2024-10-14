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

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/server/auth"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestServiceAuth(t *testing.T) {
	t.Parallel()

	ftt.Run(`With a testing configuration`, t, func(t *ftt.Test) {
		c, env := ct.Install()

		svr := New(ServerSettings{NumQueues: 2}).(*logdog.DecoratedServices)

		t.Run(`With an application config installed`, func(t *ftt.Test) {
			t.Run(`Will reject users if there is an authentication error (no state).`, func(t *ftt.Test) {
				c = auth.WithState(c, nil)

				_, err := svr.Prelude(c, "test", nil)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInternal)())
			})

			t.Run(`With an authentication state`, func(t *ftt.Test) {
				t.Run(`Will reject users who are not logged in.`, func(t *ftt.Test) {
					_, err := svr.Prelude(c, "test", nil)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
				})

				t.Run(`When a user is logged in`, func(t *ftt.Test) {
					env.AuthState.Identity = "user:user@example.com"

					t.Run(`Will reject users who are not members of the service group.`, func(t *ftt.Test) {
						_, err := svr.Prelude(c, "test", nil)
						assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)())
					})

					t.Run(`Will allow users who are members of the service group.`, func(t *ftt.Test) {
						env.ActAsService()

						_, err := svr.Prelude(c, "test", nil)
						assert.Loosely(t, err, should.BeNil)
					})
				})
			})
		})
	})
}
