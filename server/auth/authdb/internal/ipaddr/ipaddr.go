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

// Package ipaddr implements IP allowlist check.
package ipaddr

import (
	"fmt"
	"net"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth/service/protocol"
)

// Allowlist holds all named IP allowlist and the allowlist assignment map.
type Allowlist struct {
	assignments map[identity.Identity]string // identity => IP allowlist for it
	allowlists  map[string][]net.IPNet       // IP allowlist => subnets
}

// NewAllowlist creates new populated IP allowlist set.
func NewAllowlist(wl []*protocol.AuthIPWhitelist, as []*protocol.AuthIPWhitelistAssignment) (Allowlist, error) {
	assignments := make(map[identity.Identity]string, len(as))
	for _, a := range as {
		assignments[identity.Identity(a.Identity)] = a.IpWhitelist
	}

	allowlists := make(map[string][]net.IPNet, len(wl))
	for _, w := range wl {
		if len(w.Subnets) == 0 {
			continue
		}
		nets := make([]net.IPNet, len(w.Subnets))
		for i, subnet := range w.Subnets {
			_, ipnet, err := net.ParseCIDR(subnet)
			if err != nil {
				return Allowlist{}, fmt.Errorf("bad subnet %q in IP list %q - %s", subnet, w.Name, err)
			}
			nets[i] = *ipnet
		}
		allowlists[w.Name] = nets
	}

	return Allowlist{assignments, allowlists}, nil
}

// GetAllowlistForIdentity returns name of the IP allowlist to use to check
// IP of requests from given `ident`.
//
// Returns an empty string if the identity is not IP-restricted.
func (l Allowlist) GetAllowlistForIdentity(ident identity.Identity) string {
	return l.assignments[ident]
}

// IsAllowedIP returns true if IP address belongs to given named IP allowlist.
func (l Allowlist) IsAllowedIP(ip net.IP, allowlist string) bool {
	for _, ipnet := range l.allowlists[allowlist] {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}
