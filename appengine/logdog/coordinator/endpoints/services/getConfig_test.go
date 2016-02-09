// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package services

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/appengine/gaesettings"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/settings"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetConfig(t *testing.T) {
	t.Parallel()

	Convey(`With a testing configuration`, t, func() {
		c := memory.Use(context.Background())
		c = settings.Use(c, settings.New(&gaesettings.Storage{}))
		be := Server{}

		c = ct.UseConfig(c, &svcconfig.Coordinator{
			ServiceAuthGroup: "test-services",
		})
		fs := authtest.FakeState{}
		c = auth.WithState(c, &fs)

		Convey(`Returns Forbidden error if not a service.`, func() {
			_, err := be.GetConfig(c, nil)
			So(err, ShouldBeRPCPermissionDenied)
		})

		Convey(`When logged in as a service, can retrieve the configuration.`, func() {

			c = ct.UseConfig(c, &svcconfig.Coordinator{
				ServiceAuthGroup: "test-services",
			})
			fs := authtest.FakeState{}
			c = auth.WithState(c, &fs)
			fs.IdentityGroups = []string{"test-services"}

			gcfg, err := config.LoadGlobalConfig(c)
			So(err, ShouldBeRPCOK)

			cr, err := be.GetConfig(c, nil)
			So(err, ShouldBeRPCOK)
			So(cr, ShouldResemble, &services.GetConfigResponse{
				ConfigServiceUrl: gcfg.ConfigServiceURL,
				ConfigSet:        gcfg.ConfigSet,
				ConfigPath:       gcfg.ConfigPath,
			})
		})
	})
}
