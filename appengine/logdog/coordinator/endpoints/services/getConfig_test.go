// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"testing"

	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetConfig(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c, env := ct.Install()

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
				ConfigServiceUrl: env.GlobalConfig.ConfigServiceURL,
				ConfigSet:        env.GlobalConfig.ConfigSet,
				ConfigPath:       env.GlobalConfig.ConfigPath,
			})
		})
	})
}
