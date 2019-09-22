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

// Package ipaddr implements IP whitelist check.
package ipaddr

import (
	"fmt"
	"net"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth/service/protocol"
)

// Whitelist holds all named IP whitelists and the whitelist assignment map.
type Whitelist struct {
	assignments map[identity.Identity]string // identity => IP whitelist for it
	whitelists  map[string][]net.IPNet       // IP whitelist => subnets
}

// NewWhitelist creates new populated IP whitelist.
func NewWhitelist(wl []*protocol.AuthIPWhitelist, as []*protocol.AuthIPWhitelistAssignment) (Whitelist, error) {
	assignments := make(map[identity.Identity]string, len(as))
	for _, a := range as {
		assignments[identity.Identity(a.Identity)] = a.IpWhitelist
	}

	whitelists := make(map[string][]net.IPNet, len(wl))
	for _, w := range wl {
		if len(w.Subnets) == 0 {
			continue
		}
		nets := make([]net.IPNet, len(w.Subnets))
		for i, subnet := range w.Subnets {
			_, ipnet, err := net.ParseCIDR(subnet)
			if err != nil {
				return Whitelist{}, fmt.Errorf("bad subnet %q in IP list %q - %s", subnet, w.Name, err)
			}
			nets[i] = *ipnet
		}
		whitelists[w.Name] = nets
	}

	return Whitelist{assignments, whitelists}, nil
}

// GetWhitelistForIdentity returns name of the IP whitelist to use to check
// IP of requests from given `ident`.
//
// Returns an empty string if the identity is not IP-restricted.
func (wl Whitelist) GetWhitelistForIdentity(ident identity.Identity) string {
	return wl.assignments[ident]
}

// IsInWhitelist returns true if IP address belongs to given named IP whitelist.
func (wl Whitelist) IsInWhitelist(ip net.IP, whitelist string) bool {
	for _, ipnet := range wl.whitelists[whitelist] {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}
