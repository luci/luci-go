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
	"slices"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/lhttp"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/constants"
)

// ipAllowlistNameRE is the regular expression for IP Allowlist Names.
var ipAllowlistNameRE = regexp.MustCompile(`^[0-9A-Za-z_\-\+\.\ ]{2,200}$`)

// validateAllowlist validates an ip_allowlist.cfg file.
func validateAllowlist(ctx *validation.Context, configSet, path string, content []byte) error {
	cfg := configspb.IPAllowlistConfig{}

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}

	ctx.SetFile(path)
	if err := validateAllowlistCfg(&cfg); err != nil {
		ctx.Error(err)
	}

	if _, err := GetSubnets(cfg.GetIpAllowlists()); err != nil {
		ctx.Error(err)
	}
	return nil
}

func validateAllowlistCfg(cfg *configspb.IPAllowlistConfig) error {
	// Allowlist validation.
	allowlists := stringset.New(len(cfg.GetIpAllowlists()))
	for _, a := range cfg.GetIpAllowlists() {
		switch name := a.GetName(); {
		case !ipAllowlistNameRE.MatchString(name):
			return errors.New(fmt.Sprintf("invalid IP allowlist name %s", name))
		case allowlists.Has(name):
			return errors.New(fmt.Sprintf("IP allowlist is defined twice %s", name))
		default:
			allowlists.Add(name)
		}

		// Validate subnets, check that the format is valid.
		// Either in CIDR format or just a textual representation
		// of an IP.
		// e.g. "192.0.0.1", "127.0.0.1/23"
		for _, subnet := range a.Subnets {
			if _, err := normalizeSubnet(subnet); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateOAuth(ctx *validation.Context, configSet, path string, content []byte) error {
	cfg := configspb.OAuthConfig{}

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}

	ctx.SetFile(path)
	if cfg.GetTokenServerUrl() != "" {
		if _, err := lhttp.ParseHostURL(cfg.GetTokenServerUrl()); err != nil {
			ctx.Error(err)
		}
	}

	return nil
}

func validateSecurityCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := protocol.SecurityConfig{}

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}

	ctx.Enter("internal_service_regexp")
	for i, re := range cfg.GetInternalServiceRegexp() {
		ctx.Enter("# %d", i)
		if _, err := regexp.Compile(fmt.Sprintf("^%s$", re)); err != nil {
			ctx.Error(err)
		}
		ctx.Exit()
	}
	ctx.Exit()

	return nil
}

func validateSettingsCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := configspb.SettingsCfg{}
	ctx.Enter("validating settings.cfg")
	defer ctx.Exit()

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
	}

	// Ensure the integrated UI URL starts with `https://` if it's specified.
	uiTarget := cfg.IntegratedUiUrl
	if uiTarget != "" && !strings.HasPrefix(uiTarget, "https://") {
		ctx.Error(errors.New("Integrated UI URL must start with https://"))
	}

	return nil
}

func validateImportsCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := configspb.GroupImporterConfig{}
	ctx.Enter("validating imports.cfg")
	defer ctx.Exit()

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
	}

	ctx.Enter("validating tarball_upload names...")
	tarballUploadNames := make(map[string]bool)
	for _, entry := range cfg.GetTarballUpload() {
		entryName := entry.GetName()
		if entryName == "" {
			ctx.Error(errors.New("Some tarball_upload entry doesn't have a name"))
		}

		if tarballUploadNames[entryName] {
			ctx.Errorf("tarball_upload entry %s is specified twice", entryName)
		}
		tarballUploadNames[entryName] = true

		authorizedUploader := entry.GetAuthorizedUploader()
		if authorizedUploader == nil {
			ctx.Errorf("authorized_uploader is required in tarball_upload entry %s", entryName)
		}

		for _, email := range authorizedUploader {
			_, err := identity.MakeIdentity(fmt.Sprintf("user:%s", email))
			if err != nil {
				ctx.Error(err)
			}
		}
	}
	ctx.Exit()

	ctx.Enter("validating systems")
	seenSystems := make(map[string]bool)
	seenSystems["external"] = true
	for _, entry := range cfg.GetTarballUpload() {
		title := fmt.Sprintf(`"tarball_upload" entry with name %q`, entry.GetName())
		if err := validateSystems(entry.GetSystems(), seenSystems, title); err != nil {
			ctx.Error(err)
		}
	}
	ctx.Exit()

	return nil
}

// validatePermissionsCfg does basic validation that the permissions.cfg file
// has the proper format.
func validatePermissionsCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	ctx.SetFile(path)
	cfg := configspb.PermissionsConfig{}

	if err := prototext.Unmarshal(content, &cfg); err != nil {
		ctx.Error(err)
		return nil
	}

	permMap := make(map[string]*protocol.Permission, len(cfg.Permission))
	for idx, perm := range cfg.Permission {
		ctx.Enter("permission #%d %q", idx+1, perm.Name)
		validatePermission(ctx, perm, permMap)
		ctx.Exit()
	}

	roleMap := make(map[string]*configspb.PermissionsConfig_Role, len(cfg.GetRole()))
	for idx, role := range cfg.Role {
		ctx.Enter("role #%d %q", idx+1, role.Name)
		validateRole(ctx, role, permMap, roleMap)
		ctx.Exit()
	}

	inclusions := make(map[string][]string, len(roleMap))
	for _, role := range cfg.Role {
		inclusions[role.Name] = role.Includes
	}

	for idx, role := range cfg.Role {
		ctx.Enter("role #%d %q", idx+1, role.Name)
		validateRoleExpansion(ctx, role.Name, inclusions)
		ctx.Exit()
	}

	return nil
}

func validatePermission(ctx *validation.Context, perm *protocol.Permission, permMap map[string]*protocol.Permission) {
	if perm.Name == "" {
		ctx.Errorf("name is required")
	} else if permMap[perm.Name] != nil {
		ctx.Errorf("such permission was already declared")
	} else {
		if strings.Count(perm.Name, ".") != 2 {
			ctx.Errorf("permission name must have the form <service>.<subject>.<verb>")
		}
		permMap[perm.Name] = perm
	}
	for idx, attr := range perm.Attributes {
		if attr == "" {
			ctx.Errorf("attribute #%d: name is required", idx+1)
		}
	}
}

func validateRole(ctx *validation.Context,
	role *configspb.PermissionsConfig_Role,
	permMap map[string]*protocol.Permission,
	roleMap map[string]*configspb.PermissionsConfig_Role,
) {
	if role.Name == "" {
		ctx.Errorf("name is required")
	} else if roleMap[role.Name] != nil {
		ctx.Errorf("such role was already declared")
	} else {
		if !strings.HasPrefix(role.Name, constants.PrefixBuiltinRole) {
			ctx.Errorf("invalid role name, must start with %q", constants.PrefixBuiltinRole)
		}
		roleMap[role.Name] = role
	}

	for idx, perm := range role.Permissions {
		ctx.Enter("permission #%d %q", idx+1, perm.Name)
		if perm.Name == "" {
			ctx.Errorf("name is required")
		} else if def := permMap[perm.Name]; def != nil {
			if def.GetInternal() && !strings.HasPrefix(role.Name, constants.PrefixInternalRole) {
				ctx.Errorf("this is an internal permission, it can only be added to internal roles")
			}
		} else {
			ctx.Errorf("no such permission defined in the top-level permission list")
		}
		ctx.Exit()
	}
}

func validateRoleExpansion(ctx *validation.Context, role string, inclusions map[string][]string) {
	for _, inc := range inclusions[role] {
		if _, ok := inclusions[inc]; !ok {
			ctx.Errorf("unknown included role %q", inc)
		} else if isRoleIncludedBy(role, inc, inclusions) {
			ctx.Errorf("inclusion of %q introduces an inclusion cycle", inc)
		}
	}
}

// isRoleIncludedBy is true if role is in a transitive inclusion set of root.
func isRoleIncludedBy(role, root string, inclusionMap map[string][]string) bool {
	explored := stringset.New(0)
	var explore func(node string) bool
	explore = func(node string) bool {
		if node == role {
			return true
		}
		if !explored.Add(node) {
			return false // already explored or exploring now
		}
		for _, included := range inclusionMap[node] {
			if explore(included) {
				return true
			}
		}
		return false
	}
	return explore(root)
}

func validateSystems(systems []string, seenSystems map[string]bool, title string) error {
	if systems == nil {
		return errors.New(fmt.Sprintf(`%s needs a "systems" field`, title))
	}
	twice := []string{}
	for _, system := range systems {
		if seenSystems[system] {
			twice = append(twice, system)
		} else {
			seenSystems[system] = true
		}
	}
	if len(twice) > 0 {
		sort.Strings(twice)
		return errors.New(fmt.Sprintf("%s is specifying a duplicated system(s): %v", title, twice))
	}
	return nil
}

// GetSubnets validates the includes of all allowlists and generates a map {allowlistName: []subnets}.
func GetSubnets(allowlists []*configspb.IPAllowlistConfig_IPAllowlist) (map[string][]string, error) {
	allowlistsByName := make(map[string]*configspb.IPAllowlistConfig_IPAllowlist, len(allowlists))
	for _, al := range allowlists {
		allowlistsByName[al.GetName()] = al
	}

	subnetMap := make(map[string][]string, len(allowlists))
	for _, al := range allowlists {
		subnets, err := getSubnetsRecursive(al, make([]string, 0, len(allowlists)), allowlistsByName, subnetMap)
		if err != nil {
			return nil, err
		}
		subnetMap[al.GetName()] = subnets
	}
	return subnetMap, nil
}

// getSubnetsRecursive does a depth first search traversal to find all transitively included subnets for a given allowlist.
func getSubnetsRecursive(al *configspb.IPAllowlistConfig_IPAllowlist, visiting []string, allowlistsByName map[string]*configspb.IPAllowlistConfig_IPAllowlist, subnetMap map[string][]string) ([]string, error) {
	alName := al.GetName()

	// If we've already seen this allowlist before.
	if val, ok := subnetMap[alName]; ok {
		return val, nil
	}

	// Cycle check.
	if slices.Contains(visiting, alName) {
		errorCycle := fmt.Sprintf("%s -> %s", strings.Join(visiting, " -> "), alName)
		return nil, errors.New(fmt.Sprintf("IP allowlist is part of an included cycle %s", errorCycle))
	}

	visiting = append(visiting, alName)
	rawSubnets := al.GetSubnets()
	subnets := stringset.New(len(rawSubnets))
	for _, rawSubnet := range rawSubnets {
		subnet, err := normalizeSubnet(rawSubnet)
		if err != nil {
			// Ignore invalid subnets.
			continue
		}
		subnets.Add(subnet)
	}
	for _, inc := range al.GetIncludes() {
		val, ok := allowlistsByName[inc]
		if !ok {
			return nil, errors.New(fmt.Sprintf("IP allowlist contains unknown allowlist %s", inc))
		}

		resolved, err := getSubnetsRecursive(val, visiting, allowlistsByName, subnetMap)
		if err != nil {
			return nil, err
		}
		subnets.AddAll(resolved)
	}
	return subnets.ToSortedSlice(), nil
}

// normalizeSubnet attempts to normalize the given raw subnet string.
// Valid subnets are either in CIDR format or just a textual representation of
// an IP, e.g. "192.0.0.1", "127.0.0.1/23", "123:a:b:c::4567".
//
// Returns:
// - the normalized subnet as a string; or
// - an error if the raw subnet string is invalid.
func normalizeSubnet(raw string) (string, error) {
	if strings.Contains(raw, "/") {
		// Must be in CIDR format.
		_, ipNetwork, err := net.ParseCIDR(raw)
		if err != nil {
			return "", errors.Fmt("invalid subnet string: %w", err)
		}
		return ipNetwork.String(), nil
	}

	// Single IP, but could be either IPv4 or IPv6.
	ip := net.ParseIP(raw)
	if ip == nil {
		return "", errors.New(fmt.Sprintf("unable to parse IP for subnet: %s", raw))
	}

	// The number of mask bits depends on whether the IP is IPv4 or IPv6.
	maskBits := 128
	if ip.To4() != nil {
		maskBits = 32
	}
	return fmt.Sprintf("%s/%d", ip.String(), maskBits), nil
}
