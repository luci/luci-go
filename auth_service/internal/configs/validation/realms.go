// Copyright 2024 The LUCI Authors.
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
	"regexp"
	"strings"

	"golang.org/x/exp/slices"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/auth_service/constants"
	"go.chromium.org/luci/auth_service/internal/permissions"
)

const rootRealm = "@root"

var (
	knownSpecialRealms = stringset.NewFromSlice(rootRealm, "@legacy", "@project")
	realmNameRE        = regexp.MustCompile(`^[a-z0-9_\.\-/]{1,400}$`)
)

type realmsValidator struct {
	db            *permissions.PermissionsDB
	allowInternal bool
}

// Validate checks the realms config is correctly formatted and does not
// reference undefined roles or permissions.
func (rv *realmsValidator) Validate(ctx *validation.Context, cfg *realmsconf.RealmsCfg) {
	validCustomRoles := rv.validateCustomRoles(ctx, cfg.GetCustomRoles())
	rv.validateRealms(ctx, cfg.GetRealms(), validCustomRoles)
}

func (rv *realmsValidator) validateCustomRoles(ctx *validation.Context, customRoles []*realmsconf.CustomRole) stringset.Set {
	ctx.Enter("validating custom roles")
	defer ctx.Exit()

	if len(customRoles) == 0 {
		// Custom roles are optional.
		return stringset.Set{}
	}

	customRoleNames := stringset.New(len(customRoles))
	for _, role := range customRoles {
		customRoleNames.Add(role.GetName())
	}

	// Map of custom role name to ordered slice of custom roles it extends.
	graph := map[string][]string{}
	for i, role := range customRoles {
		roleName := role.GetName()
		ctx.Enter("custom role #%d (%q)", i+1, roleName)

		if !strings.HasPrefix(roleName, constants.PrefixCustomRole) {
			ctx.Errorf("role name should have prefix %q", constants.PrefixCustomRole)
			ctx.Exit()
			continue
		}

		if _, ok := graph[roleName]; ok {
			ctx.Errorf("custom role already defined")
			ctx.Exit()
			continue
		}

		// All referenced permissions must be known and have correct visibility.
		for _, perm := range role.GetPermissions() {
			rv.validatePermission(ctx, perm)
		}

		// Validate `extends` relations.
		extended := role.GetExtends()
		parentCustomRoles := stringset.New(len(extended))
		for _, parent := range extended {
			if rv.validateRoleRef(ctx, parent, customRoleNames) && isCustomRole(parent) {
				if added := parentCustomRoles.Add(parent); !added {
					ctx.Errorf("the role %q is specified in `extends` more than once",
						parent)
				}
			}
		}
		// Make traversal order deterministic by storing the sorted result.
		graph[roleName] = parentCustomRoles.ToSortedSlice()
		ctx.Exit()
	}

	// Create an ordered slice of the processed roles to make the cycle-finding
	// deterministic.
	processedRoleNames := []string{}
	for role := range graph {
		processedRoleNames = append(processedRoleNames, role)
	}
	slices.Sort(processedRoleNames)

	validRoles := stringset.Set{}
	cyclicRoles := stringset.Set{}
	for _, role := range processedRoleNames {
		if cyclicRoles.Has(role) {
			// Already found; no need to report it again.
			continue
		}

		cycle, err := findCycle(role, graph)
		if err != nil {
			ctx.Error(err)
			continue
		}
		if len(cycle) > 0 {
			ctx.Errorf("custom role %q cyclically extends itself: %s",
				role, strings.Join(cycle, " -> "))
			cyclicRoles.AddAll(cycle)
			continue
		}

		// No cycles; the role is valid.
		validRoles.Add(role)
	}

	return validRoles
}

func (rv *realmsValidator) validateRealms(ctx *validation.Context, realms []*realmsconf.Realm, customRoles stringset.Set) {
	ctx.Enter("validating realms")
	defer ctx.Exit()

	if len(realms) == 0 {
		// While an empty realms config is technically valid, it is unexpected.
		logging.Warningf(ctx.Context, "realms config is empty")
		return
	}

	allRealmNames := stringset.New(len(realms))
	for _, realm := range realms {
		allRealmNames.Add(realm.GetName())
	}

	graph := map[string][]string{}
	for i, realm := range realms {
		realmName := realm.GetName()
		ctx.Enter("realm #%d (%q)", i+1, realmName)

		if !validateRealmName(ctx, realmName) {
			ctx.Exit()
			continue
		}

		if _, ok := graph[realmName]; ok {
			ctx.Errorf("realm already defined")
			ctx.Exit()
			continue
		}

		// Check bindings refer to known roles only.
		for j, binding := range realm.GetBindings() {
			ctx.Enter("binding #%d (role %q)", j+1, binding)
			rv.validateBinding(ctx, binding, customRoles)
			ctx.Exit()
		}

		// The root realm cannot have any parents.
		if realmName == rootRealm {
			if len(realm.GetExtends()) > 0 {
				ctx.Errorf("the root realm must not use `extends`")
			}
			graph[realmName] = []string{}
			ctx.Exit()
			continue
		}

		// Validate `extends` relations.
		extended := realm.GetExtends()
		parentRealms := stringset.New(len(extended))
		for _, parent := range extended {
			if !allRealmNames.Has(parent) {
				ctx.Errorf("referencing an undefined realm %q", parent)
			} else if parentRealms.Has(parent) {
				ctx.Errorf(
					"the realm %q is specified in `extends` more than once",
					parent)
			} else {
				parentRealms.Add(parent)
			}
		}
		// Make traversal order deterministic by storing the sorted result.
		graph[realmName] = parentRealms.ToSortedSlice()
		ctx.Exit()
	}

	// Create an ordered slice of the processed realms to make the cycle-finding
	// deterministic.
	processedRealmNames := []string{}
	for realm := range graph {
		processedRealmNames = append(processedRealmNames, realm)
	}
	slices.Sort(processedRealmNames)

	cyclicRealms := stringset.Set{}
	for _, realm := range processedRealmNames {
		if cyclicRealms.Has(realm) {
			// Already found; no need to report it again.
			continue
		}

		cycle, err := findCycle(realm, graph)
		if err != nil {
			ctx.Error(err)
			continue
		}
		if len(cycle) > 0 {
			ctx.Errorf("realm %q cyclically extends itself: %s",
				realm, strings.Join(cycle, " -> "))
			cyclicRealms.AddAll(cycle)
			continue
		}
	}
}

// validatePermission returns whether the permission name is valid.
// It registers an error in the validation context if the permission is not
// defined or has the incorrect visibility.
func (rv *realmsValidator) validatePermission(ctx *validation.Context, name string) bool {
	perm, ok := rv.db.Permissions[name]
	if !ok || perm == nil {
		ctx.Errorf("permission %q is not defined in permissions DB revision %q",
			name, rv.db.Rev)
		return false
	}

	if perm.Internal && !rv.allowInternal {
		ctx.Errorf("permission %q is internal; it can't be used in a project config",
			name)
		return false
	}

	return true
}

// validateRealmName returns whether the realm name is valid.
// It registers an error in the validation context if it is invalid.
func validateRealmName(ctx *validation.Context, name string) bool {
	if strings.HasPrefix(name, "@") {
		if knownSpecialRealms.Has(name) {
			return true
		}
		ctx.Errorf("unknown special realm name")
		return false
	}

	if !realmNameRE.MatchString(name) {
		ctx.Errorf("invalid realm name; it must match %s", realmNameRE.String())
		return false
	}

	return true
}

func (rv *realmsValidator) validateBinding(ctx *validation.Context, binding *realmsconf.Binding, customRoles stringset.Set) {
	// Check the role name is valid.
	rv.validateRoleRef(ctx, binding.GetRole(), customRoles)

	// Check the principals are valid.
	for _, principal := range binding.GetPrincipals() {
		if strings.HasPrefix(principal, "group:") {
			groupName := strings.TrimPrefix(principal, "group:")
			if !auth.IsValidGroupName(groupName) {
				ctx.Errorf("invalid group name %q", groupName)
			}
			continue
		}

		if _, err := identity.MakeIdentity(principal); err != nil {
			ctx.Errorf("invalid principal format %q", principal)
		}
	}

	// Check the conditions are valid.
	for i, condition := range binding.GetConditions() {
		ctx.Enter("condition #%d", i+1)

		restrict := condition.GetRestrict()
		if restrict == nil {
			ctx.Errorf("invalid empty condition")
		} else {
			if !rv.db.HasAttribute(restrict.Attribute) {
				ctx.Errorf("unknown attribute %q", restrict.Attribute)
			}
		}

		ctx.Exit()
	}
}

// validateRoleRef returns whether the role name is valid.
// It registers an error in the validation context if the role is unrecognized.
func (rv *realmsValidator) validateRoleRef(ctx *validation.Context, name string, customRoles stringset.Set) bool {
	if isCustomRole(name) {
		valid := customRoles.Has(name)
		if !valid {
			ctx.Errorf("referencing a custom role %q not defined in the realms config",
				name)
		}
		return valid
	}

	if isBuiltinRole(name) {
		if isInternalRole(name) && !rv.allowInternal {
			ctx.Errorf("the role %q is internal; it can't be used in a project config",
				name)
			return false
		}

		_, valid := rv.db.Roles[name]
		if !valid {
			ctx.Errorf("referencing a role %q not defined in permissions DB revision %q",
				name, rv.db.Rev)
		}
		return valid
	}

	ctx.Errorf(
		"bad role reference %q: "+
			"must be either a predefined role (\"%s...\"), or "+
			"a custom role defined somewhere in this file (\"%s...\")",
		name, constants.PrefixBuiltinRole, constants.PrefixCustomRole)
	return false
}

func isBuiltinRole(roleName string) bool {
	return strings.HasPrefix(roleName, constants.PrefixBuiltinRole)
}

func isCustomRole(roleName string) bool {
	return strings.HasPrefix(roleName, constants.PrefixCustomRole)
}

func isInternalRole(roleName string) bool {
	return strings.HasPrefix(roleName, constants.PrefixInternalRole)
}

// findCycle finds a path from `start` to itself in the directed graph.
//
// Note: if the graph has other cycles (that don't have `start`), they are
// ignored.
func findCycle(start string, graph map[string][]string) ([]string, error) {
	explored := stringset.Set{} // Roots of totally explored trees.
	visiting := []string{}      // Stack of nodes currently being traversed.

	var doVisit func(node string) (bool, error)
	doVisit = func(node string) (bool, error) {
		if explored.Has(node) {
			// Been there already; no cycles there that have `start` in them.
			return false, nil
		}

		if contains(visiting, node) {
			// Found a cycle that starts and ends with `node`. Return true if it
			// is a `start` cycle; we don't care otherwise.
			return node == start, nil
		}

		visiting = append(visiting, node)
		parentNodes, ok := graph[node]
		if !ok {
			return false, fmt.Errorf("node %q is unrecognized in the graph", node)
		}
		for _, parent := range parentNodes {
			hasCycle, err := doVisit(parent)
			if err != nil {
				return false, err
			}
			if hasCycle {
				// Found a cycle!
				return true, nil
			}
		}

		lastIndex := len(visiting) - 1
		if lastIndex < 0 {
			return false, errors.New("error finding cycles; visiting stack corrupted")
		}
		var lastVisited string
		visiting, lastVisited = visiting[:lastIndex], visiting[lastIndex]
		if lastVisited != node {
			return false, errors.New("error finding cycles; visiting stack order corrupted")
		}

		// Record the root of the cycle-free subgraph.
		explored.Add(node)
		return false, nil
	}

	hasCycle, err := doVisit(start)
	if err != nil {
		return nil, err
	}
	if !hasCycle {
		return []string{}, nil
	}

	// Close the loop.
	visiting = append(visiting, start)
	return visiting, nil
}
