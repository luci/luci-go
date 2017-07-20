// Copyright 2016 The LUCI Authors.
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

package common

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	memcfg "github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/authtest"
	"github.com/luci/luci-go/server/auth/identity"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestACL(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
		c = gologger.StdConfig.Use(c)

		Convey("Set up projects", func() {
			c = testconfig.WithCommonClient(c, memcfg.New(aclConfgs))
			err := UpdateConsoles(c)
			So(err, ShouldBeNil)

			Convey("Anon wants to...", func() {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       identity.AnonymousIdentity,
					IdentityGroups: []string{"all"},
				})
				Convey("Read public project", func() {
					ok, err := IsAllowed(c, "opensource")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
				Convey("Read private project", func() {
					ok, err := IsAllowed(c, "secret")
					So(ok, ShouldEqual, false)
					So(err, ShouldBeNil)
				})

			})
			Convey("alicebob@google.com wants to...", func() {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       "user:alicebob@google.com",
					IdentityGroups: []string{"googlers", "all"},
				})
				Convey("Read private project", func() {
					ok, err := IsAllowed(c, "secret")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
			})

			Convey("eve@notgoogle.com wants to...", func() {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       "user:eve@notgoogle.com",
					IdentityGroups: []string{"all"},
				})
				Convey("Read public project", func() {
					ok, err := IsAllowed(c, "opensource")
					So(ok, ShouldEqual, true)
					So(err, ShouldBeNil)
				})
				Convey("Read private project", func() {
					ok, err := IsAllowed(c, "secret")
					So(ok, ShouldEqual, false)
					So(err, ShouldBeNil)
				})
			})
		})
	})
}

var secretProjectCfg = `
name: "secret"
access: "group:googlers"
`

var publicProjectCfg = `
name: "opensource"
access: "group:all"
`

var aclConfgs = map[string]memcfg.ConfigSet{
	"projects/secret": {
		"project.cfg": secretProjectCfg,
	},
	"projects/opensource": {
		"project.cfg": publicProjectCfg,
	},
}
