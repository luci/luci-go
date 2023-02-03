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

func TestAllowlist(t *testing.T) {
	Convey("Works", t, func() {
		l, _ := NewAllowlist(
			[]*protocol.AuthIPWhitelist{
				{
					Name: "allowlist",
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
					IpWhitelist: "allowlist",
				},
			},
		)

		Convey("IP assignment", func() {
			So(l.GetAllowlistForIdentity("user:abc@example.com"), ShouldEqual, "allowlist")
			So(l.GetAllowlistForIdentity("user:unknown@example.com"), ShouldEqual, "")
		})

		Convey("Allowlist", func() {
			call := func(ip, allowlist string) bool {
				ipaddr := net.ParseIP(ip)
				So(ipaddr, ShouldNotBeNil)
				return l.IsAllowedIP(ipaddr, allowlist)
			}

			So(call("1.2.3.4", "allowlist"), ShouldBeTrue)
			So(call("10.255.255.255", "allowlist"), ShouldBeTrue)
			So(call("9.255.255.255", "allowlist"), ShouldBeFalse)
			So(call("1.2.3.4", "empty"), ShouldBeFalse)
		})
	})

	Convey("Bad subnet", t, func() {
		_, err := NewAllowlist(
			[]*protocol.AuthIPWhitelist{
				{
					Name:    "allowlist",
					Subnets: []string{"1.2.3.4/456"},
				},
			}, nil,
		)
		So(err, ShouldErrLike, `bad subnet "1.2.3.4/456" in IP list "allowlist" - invalid CIDR address: 1.2.3.4/456`)
	})
}
