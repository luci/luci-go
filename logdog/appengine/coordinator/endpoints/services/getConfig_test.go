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

	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	ct "go.chromium.org/luci/logdog/appengine/coordinator/coordinatorTest"
	"go.chromium.org/luci/luci_config/appengine/gaeconfig"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetConfig(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install()

		s := gaeconfig.Settings{
			ConfigServiceHost: "example.com",
		}
		So(s.SetIfChanged(c, "test", "test"), ShouldBeNil)

		svr := New()

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := svr.GetConfig(c, nil)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service, can retrieve the configuration.`, func() {
			env.JoinGroup("services")

			cr, err := svr.GetConfig(c, nil)
			So(err, ShouldBeRPCOK)
			So(cr, ShouldResemble, &logdog.GetConfigResponse{
				ConfigServiceUrl:  "test://example.com",
				ConfigSet:         "services/app",
				ServiceConfigPath: svcconfig.ServiceConfigPath,
				ConfigServiceHost: "example.com",
			})
		})
	})
}
