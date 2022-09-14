// Copyright 2022 The LUCI Authors.
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

package validation

import (
	"fmt"
	"net"
	"regexp"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"
)

// ipAllowlistNameRE is the regular expression for IP Allowlist Names.
var ipAllowlistNameRE = regexp.MustCompile(`^[0-9A-Za-z_\-\+\.\ ]{2,200}$`)

// validateAllowlist validates an ip_allowlist.cfg file.
//
// Validation result is returned via validation ctx, while error returned
// drectly implies only a bug in this code.
func validateAllowlist(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := configspb.IPAllowlistConfig{}

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}

	// Allowlist validation.
	allowlists := stringset.New(len(cfg.GetIpAllowlists()))
	for _, a := range cfg.GetIpAllowlists() {
		switch name := a.GetName(); {
		case !ipAllowlistNameRE.MatchString(name):
			ctx.Errorf("invalid ip allowlist name %s", name)
		case allowlists.Has(name):
			ctx.Errorf("ip allowlist is defined twice %s", name)
		default:
			allowlists.Add(name)
		}

		// Validate subnets, check that the format is valid.
		// Either in CIDR format or just a textual representation
		// of an IP.
		// e.g. "192.0.0.1", "127.0.0.1/23"
		for _, subnet := range a.Subnets {
			if strings.Contains(subnet, "/") {
				if _, _, err := net.ParseCIDR(subnet); err != nil {
					ctx.Error(err)
				}
			} else {
				if ip := net.ParseIP(subnet); ip == nil {
					ctx.Errorf("unable to parse ip for subnet: %s", subnet)
				}
			}
		}
	}

	// Assignment validation
	idents := stringset.New(len(cfg.GetAssignments()))
	for _, a := range cfg.GetAssignments() {
		ident := a.GetIdentity()
		alName := a.GetIpAllowlistName()

		// Checks if valid Identity.
		if _, err := identity.MakeIdentity(ident); err != nil {
			ctx.Error(err)
		}
		if !allowlists.Has(alName) {
			ctx.Errorf("unknown allowlist %s", alName)
		}
		if idents.Has(ident) {
			ctx.Errorf("identity %s defined twice", ident)
		}
		idents.Add(ident)
	}
	resolveAllowlistIncludes(ctx, cfg.GetIpAllowlists())
	return nil
}

// TODO(cjacomet): Abstract vctx out to be able to use this when updating the allowlist cfg in datastore.
// resolveAllowlistIncludes validates the includes of all allowlists and generates a map {allowlistName: []subnets}.
func resolveAllowlistIncludes(vctx *validation.Context, allowlists []*configspb.IPAllowlistConfig_IPAllowlist) {
	allowlistsByName := make(map[string]*configspb.IPAllowlistConfig_IPAllowlist, len(allowlists))
	for _, al := range allowlists {
		allowlistsByName[al.GetName()] = al
	}

	subnetMap := make(map[string][]string, len(allowlists))
	for _, al := range allowlists {
		subnetMap[al.GetName()] = getSubnetsRecursive(vctx, al, make([]string, 0, len(allowlists)), allowlistsByName, subnetMap)
	}
}

// getSubnetsRecursive does a depth first search traversal to find all transitively included subnets for a given allowlist.
func getSubnetsRecursive(vctx *validation.Context, al *configspb.IPAllowlistConfig_IPAllowlist, visiting []string, allowlistsByName map[string]*configspb.IPAllowlistConfig_IPAllowlist, subnetMap map[string][]string) []string {
	alName := al.GetName()

	// If we've already seen this allowlist before.
	if val, ok := subnetMap[alName]; ok {
		return val
	}

	// Cycle check.
	if contains(visiting, alName) {
		errorCycle := fmt.Sprintf("%s -> %s", strings.Join(visiting, " -> "), alName)
		vctx.Errorf("IP allowlist is part of an included cycle %s", errorCycle)
		return []string{}
	}

	visiting = append(visiting, alName)
	subnets := stringset.NewFromSlice(al.GetSubnets()...)
	for _, inc := range al.GetIncludes() {
		val, ok := allowlistsByName[inc]
		if !ok {
			vctx.Errorf("IP Allowlist contains unknown allowlist %s", inc)
			continue
		}

		resolved := getSubnetsRecursive(vctx, val, visiting, allowlistsByName, subnetMap)
		subnets.AddAll(resolved)
	}
	return subnets.ToSortedSlice()
}

func contains(s []string, val string) bool {
	for _, v := range s {
		if v == val {
			return true
		}
	}
	return false
}
