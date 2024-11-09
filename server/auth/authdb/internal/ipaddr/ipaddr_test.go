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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/service/protocol"
)

func TestAllowlist(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
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

		t.Run("IP assignment", func(t *ftt.Test) {
			assert.Loosely(t, l.GetAllowlistForIdentity("user:abc@example.com"), should.Equal("allowlist"))
			assert.Loosely(t, l.GetAllowlistForIdentity("user:unknown@example.com"), should.BeEmpty)
		})

		t.Run("Allowlist", func(t *ftt.Test) {
			call := func(ip, allowlist string) bool {
				ipaddr := net.ParseIP(ip)
				assert.Loosely(t, ipaddr, should.NotBeNil)
				return l.IsAllowedIP(ipaddr, allowlist)
			}

			assert.Loosely(t, call("1.2.3.4", "allowlist"), should.BeTrue)
			assert.Loosely(t, call("10.255.255.255", "allowlist"), should.BeTrue)
			assert.Loosely(t, call("9.255.255.255", "allowlist"), should.BeFalse)
			assert.Loosely(t, call("1.2.3.4", "empty"), should.BeFalse)
		})
	})

	ftt.Run("Bad subnet", t, func(t *ftt.Test) {
		_, err := NewAllowlist(
			[]*protocol.AuthIPWhitelist{
				{
					Name:    "allowlist",
					Subnets: []string{"1.2.3.4/456"},
				},
			}, nil,
		)
		assert.Loosely(t, err, should.ErrLike(`bad subnet "1.2.3.4/456" in IP list "allowlist" - invalid CIDR address: 1.2.3.4/456`))
	})
}
