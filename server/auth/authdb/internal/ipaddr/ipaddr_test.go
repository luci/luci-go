// Copyright 2019 The LUCI Authors.
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

package ipaddr

import (
	"net"
	"testing"

	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestWhitelist(t *testing.T) {
	Convey("Works", t, func() {
		wl, _ := NewWhitelist(
			[]*protocol.AuthIPWhitelist{
				{
					Name: "whitelist",
					Subnets: []string{
						"1.2.3.4/32",
						"10.0.0.0/8",
					},
				},
				{
					Name: "empty",
				},
			},
			[]*protocol.AuthIPWhitelistAssignment{
				{
					Identity:    "user:abc@example.com",
					IpWhitelist: "whitelist",
				},
			},
		)

		Convey("IP assignment", func() {
			So(wl.GetWhitelistForIdentity("user:abc@example.com"), ShouldEqual, "whitelist")
			So(wl.GetWhitelistForIdentity("user:unknown@example.com"), ShouldEqual, "")
		})

		Convey("Whitelist", func() {
			call := func(ip, whitelist string) bool {
				ipaddr := net.ParseIP(ip)
				So(ipaddr, ShouldNotBeNil)
				return wl.IsInWhitelist(ipaddr, whitelist)
			}

			So(call("1.2.3.4", "whitelist"), ShouldBeTrue)
			So(call("10.255.255.255", "whitelist"), ShouldBeTrue)
			So(call("9.255.255.255", "whitelist"), ShouldBeFalse)
			So(call("1.2.3.4", "empty"), ShouldBeFalse)
		})
	})

	Convey("Bad subnet", t, func() {
		_, err := NewWhitelist(
			[]*protocol.AuthIPWhitelist{
				{
					Name:    "whitelist",
					Subnets: []string{"1.2.3.4/456"},
				},
			}, nil,
		)
		So(err, ShouldErrLike, `bad subnet "1.2.3.4/456" in IP list "whitelist" - invalid CIDR address: 1.2.3.4/456`)
	})
}
