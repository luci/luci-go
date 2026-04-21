// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package realmsinternals

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/constants"
	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/internal/projects"
)

var (
	// ErrRoleNotFound is used when a role requested is not found in the internal permissionsDB.
	ErrRoleNotFound = errors.New("role does not exist in internal representation")

	// ErrImpossibleRole is used when there is an attempt to expand a role that is not allowed.
	ErrImpossibleRole = errors.New("role is impossible, does not include one of the approved prefixes")
)

// ConditionsSet normalizes and dedups conditions, maps them to integers.
//
// Assumes all incoming realmsconf.Condition are immutable and dedups them by
// pointer, as well as by normalized values. Also assumes the set of all
// possible *objects* ever passed to indexes(...) was also passed to
// addCond(...) first (so it could build pointer => index map).
//
// This makes hot indexes(...) function fast by allowing to lookup pointers
// instead of (potentially huge) protobuf message values.
type ConditionsSet struct {
	// normalized is a map from a serialized normalized Condition to its details.
	//
	// Used only during construction.
	normalized map[string]*conditionDetails

	// conditions is a map from an added Condition (by address) to its details and
	// index in the finalize() list.
	//
	// Shares values with `normalized`. Multiple entries can map to the exact same
	// *conditionDetails value (if they normalize to the same condition).
	conditions map[*realmsconf.Condition]*conditionDetails

	// finalized is true if finalize() was called, see finalize for more info.
	finalized bool
}

// conditionDetails is a normalized condition and an attribute it examines.
type conditionDetails struct {
	// An index of this condition in the list returned by finalize().
	idx uint32
	// The condition in normalized form.
	cond *protocol.Condition
	// An attribute this condition examines.
	attr string
	// An associated attribute values, used exclusively for sorting.
	values []string
}

// addCond adds a *Condition from realms.cfg definition to the set if it's
// not already there.
//
// Panics if ConditionsSet is already finalized. Returns an error if the
// condition has unrecognized type.
func (cs *ConditionsSet) addCond(cond *realmsconf.Condition) error {
	if cs.finalized {
		panic("already finalized")
	}
	if _, ok := cs.conditions[cond]; ok {
		return nil
	}

	var norm *protocol.Condition
	var attr string
	var values []string
	switch op := cond.Op.(type) {
	case *realmsconf.Condition_Restrict:
		attr = op.Restrict.Attribute
		values = stringset.NewFromSlice(op.Restrict.Values...).ToSortedSlice()
		norm = &protocol.Condition{
			Op: &protocol.Condition_Restrict{
				Restrict: &protocol.Condition_AttributeRestriction{
					Attribute: attr,
					Values:    values,
				},
			},
		}
	default:
		return fmt.Errorf("unrecognized condition kind in %v", cond)
	}

	// If we've seen an equivalent of this condition already, point `cond` entry
	// to an existing *conditionDetails. Otherwise add a new one.
	ck := conditionKey(norm)
	details, ok := cs.normalized[ck]
	if !ok {
		details = &conditionDetails{
			cond:   norm,
			attr:   attr,
			values: values,
		}
		cs.normalized[ck] = details
	}
	cs.conditions[cond] = details
	return nil
}

// conditionKey generates a key by serializing a protocol.Condition.
func conditionKey(cond *protocol.Condition) string {
	key, err := proto.Marshal(cond)
	if err != nil {
		return ""
	}
	return string(key)
}

// finalize finalizes the set by preventing any future addCond calls.
//
// Sorts the list of stored conditions by attribute first then by values.
// Returns the final sorted list of protocol.Condition. Indexes returned by
// indexes() will refer to the indexes in this list.
//
// Panics of the ConditionsSet is already finalized.
func (cs *ConditionsSet) finalize() []*protocol.Condition {
	if cs.finalized {
		panic("already finalized")
	}
	cs.finalized = true

	count := len(cs.normalized)
	if count == 0 {
		return nil
	}

	// Convert `normalized` to a sorted slice of values.
	normalized := slices.SortedFunc(maps.Values(cs.normalized), func(a, b *conditionDetails) int {
		if res := cmp.Compare(a.attr, b.attr); res != 0 {
			return res
		}
		return slices.Compare(a.values, b.values)
	})

	// Not needed any more.
	cs.normalized = nil

	// Compose the list of conditions in the final order. Populate indexes in
	// *conditionDetail to allow indexes(...) to resolve a *Condition to its
	// index in the returned list.
	conds := make([]*protocol.Condition, count)
	for i, entry := range normalized {
		conds[i] = entry.cond
		entry.idx = uint32(i)
	}
	return conds
}

// indexes returns a subset of `conds` with conditions that evaluate any of
// the given attributes.
//
// The subset is returned as a sorted set of indexes in the list returned by
// finalize(). Can be called only after finalize(). All given condition pointers
// must have previously been put into the set via addCond(), panics otherwise.
func (cs *ConditionsSet) indexes(conds []*realmsconf.Condition, attrs *attrSet) []uint32 {
	if !cs.finalized {
		panic("indexes() called before finalize()")
	}
	if len(conds) == 0 {
		return nil
	}
	if len(conds) == 1 {
		details, ok := cs.conditions[conds[0]]
		if !ok {
			panic(fmt.Sprintf("unexpected condition not seen by addCond: %v", conds[0]))
		}
		if attrs.has(details.attr) {
			return []uint32{details.idx}
		}
		return nil
	}

	indexesSet := emptyIndexSet(len(conds))

	for _, cond := range conds {
		details, ok := cs.conditions[cond]
		if !ok {
			panic(fmt.Sprintf("unexpected condition not seen by addCond: %v", cond))
		}
		if attrs.has(details.attr) {
			indexesSet.add(details.idx)
		}
	}

	return indexesSet.toSortedSlice()
}

// indexSet is a set data structure for managing indexes when expanding realms
// and permissions.
type indexSet struct {
	set map[uint32]struct{}
}

// indexSetFromSlice converts a given slice of indexes to be indexSet.
func indexSetFromSlice(src []uint32) indexSet {
	res := emptyIndexSet(len(src))
	for _, val := range src {
		res.set[val] = struct{}{}
	}
	return res
}

// emptyIndexSet initializes and returns an empty IndexSet.
func emptyIndexSet(capacity int) indexSet {
	return indexSet{make(map[uint32]struct{}, capacity)}
}

// add adds a given uint32 to the index set.
func (is indexSet) add(v uint32) {
	is.set[v] = struct{}{}
}

// update adds all indexes from other set.
func (is indexSet) update(other indexSet) {
	for k := range other.set {
		is.add(k)
	}
}

// toSlice converts an IndexSet to a slice and returns it.
func (is indexSet) toSlice() []uint32 {
	res := make([]uint32, 0, len(is.set))
	for k := range is.set {
		res = append(res, k)
	}
	return res
}

// toSortedSlice converts an IndexSet to a slice and then sorts the indexes,
// returning the result.
func (is indexSet) toSortedSlice() []uint32 {
	res := is.toSlice()
	slices.Sort(res)
	return res
}

// clone makes a clone of the index set.
func (is indexSet) clone() indexSet {
	return indexSet{set: maps.Clone(is.set)}
}

// RolesExpander keeps track of permissions and role -> [permission] expansions.
//
// Permissions are represented internally as integers to speed up set operations.
//
// Should be used only with validated realmsconf.RealmsCfg.
type RolesExpander struct {
	// permissionsDB is a database of all known permissions in normalized form.
	//
	// It is taken from PermissionsDB.Permissions. Attributes are sorted.
	permissionsDB map[string]*protocol.Permission

	// builtinRoles is a mapping from roleName -> *permissions.Role.
	//
	// These are generated from the permissions.cfg and translated to permissions
	// db which is where these roles come from. If a role is not found here
	// then it has not been defined in the permissions.cfg. This is assumed
	// final state and should not be modified.
	builtinRoles map[string]*permissions.Role

	// customRoles is a mapping from roleName -> *realmsconf.CustomRole.
	//
	// This mapping is generated from realms.cfg in ExpandRealms.
	customRoles map[string]*realmsconf.CustomRole

	// permissions is a mapping from permission name to its details, including
	// the internal index.
	//
	// All permissions are converted to a uint32 index for faster queries.
	// Populated on the fly via permissionDetails(...). All these indexes are
	// remapped at the end of ExpandRealms to match indexes in sortedPermissions()
	// list.
	permissions map[string]permissionDetails

	// roles contains role to permissions mapping, keyed by roleName.
	//
	// This mapping contains a set of all the permissions a given role
	// is associated with grouped by set of attributes they evaluate.
	//
	// Populated on the fly via roleDetails(...).
	roles map[string]roleDetails
}

// permissionDetails holds details of a permission in RolesExpander.
type permissionDetails struct {
	// A unique index assigned to this permission.
	idx uint32
	// Attributes evaluated by this permission.
	attrs attrSet
}

// permissionGroup is a group of permissions the all belong to the same role
// and which all evaluate the exact same set of attributes.
type permissionGroup struct {
	// Indexes of permissions in the group.
	perms indexSet
	// Attributes all these permissions evaluate (each one evaluates all `attrs`).
	attrs attrSet
}

// roleDetails holds details of an expanded role.
type roleDetails struct {
	// Permissions grouped by set of attributes they evaluate.
	groups []permissionGroup
}

// attrSet is an immutable set of attribute names.
type attrSet struct {
	// The set of attribute names as a sorted list.
	asSet []string
	// A serialized form of `asSet` to use as a map key.
	asKey string
}

// newAttrSet constructs an attrSet from a sorted list of attributes.
func newAttrSet(attrs []string) attrSet {
	if !slices.IsSorted(attrs) {
		panic(fmt.Sprintf("attributes must be sorted: %v", attrs))
	}
	return attrSet{
		asSet: attrs,
		asKey: strings.Join(attrs, "\x00"),
	}
}

// has returns true if `attr` is in the set.
func (s *attrSet) has(attr string) bool {
	_, found := slices.BinarySearch(s.asSet, attr)
	return found
}

// permissionDetails returns details of the given permission.
func (re *RolesExpander) permissionDetails(name string) permissionDetails {
	if details, ok := re.permissions[name]; ok {
		return details
	}
	details := permissionDetails{
		idx:   uint32(len(re.permissions)),
		attrs: newAttrSet(re.permissionsDB[name].GetAttributes()),
	}
	re.permissions[name] = details
	return details
}

// rolePermissions adds all permissions in the role to the given set.
func (re *RolesExpander) rolePermissions(roleName string, perms stringset.Set) error {
	if strings.HasPrefix(roleName, constants.PrefixBuiltinRole) {
		role, ok := re.builtinRoles[roleName]
		if !ok {
			return fmt.Errorf("%w: builtinRole: %s", ErrRoleNotFound, roleName)
		}
		for perm := range role.Permissions {
			perms.Add(perm)
		}
		return nil
	}

	if strings.HasPrefix(roleName, constants.PrefixCustomRole) {
		customRole, ok := re.customRoles[roleName]
		if !ok {
			return fmt.Errorf("%w: customRole: %s", ErrRoleNotFound, roleName)
		}

		perms.AddAll(customRole.Permissions)

		for _, parent := range customRole.Extends {
			if err := re.rolePermissions(parent, perms); err != nil {
				return err
			}
		}
		return nil
	}

	return ErrImpossibleRole
}

// roleDetails returns all permissions in a role grouped by attributes.
//
// Returns:
//
//	ErrRoleNotFound - if given roleName is not defined.
//	ErrImpossibleRole - if roleName format is invalid.
func (re *RolesExpander) roleDetails(roleName string) (roleDetails, error) {
	if details, ok := re.roles[roleName]; ok {
		return details, nil
	}

	// All permission in the role (as strings).
	permsByName := stringset.New(0)
	if err := re.rolePermissions(roleName, permsByName); err != nil {
		return roleDetails{}, err
	}

	// attrSet.asKey -> permissions that all evaluate these attributes.
	groups := make(map[string]permissionGroup, 1)

	for perm := range permsByName {
		details := re.permissionDetails(perm)
		if group, ok := groups[details.attrs.asKey]; ok {
			group.perms.add(details.idx)
		} else {
			groups[details.attrs.asKey] = permissionGroup{
				perms: indexSetFromSlice([]uint32{details.idx}),
				attrs: details.attrs,
			}
		}
	}

	role := roleDetails{groups: slices.Collect(maps.Values(groups))}
	re.roles[roleName] = role
	return role, nil
}

// sortedPermissions returns a sorted slice of permissions and a slice
// mapping old -> new indexes.
func (re *RolesExpander) sortedPermissions() ([]*protocol.Permission, []uint32) {
	perms := make([]*protocol.Permission, 0, len(re.permissions))
	for k := range re.permissions {
		perms = append(perms, re.permissionsDB[k])
	}
	slices.SortFunc(perms, func(a, b *protocol.Permission) int {
		return cmp.Compare(a.Name, b.Name)
	})
	mapping := make([]uint32, len(perms))
	for newIdx, perm := range perms {
		oldIdx := re.permissions[perm.Name].idx
		mapping[oldIdx] = uint32(newIdx)
	}
	return perms, mapping
}

// RealmsExpander helps traverse the realm inheritance graph.
type RealmsExpander struct {
	// rolesExpander will handle role expansion for the realms.
	rolesExpander *RolesExpander
	// condsSet will handle the expansion for conditions in realms.
	condsSet *ConditionsSet
	// projectCfg is a extracted from projects.cfg service config.
	projectCfg *projects.ProjectConfig
	// realms is a mapping from realm name -> *realmsconf.Realm.
	realms map[string]*realmsconf.Realm
	// data is a mapping from realm name -> *protocol.RealmData.
	data map[string]*protocol.RealmData
}

// parents returns the list of immediate parents given a realm.
// includes @root realm by default since all realms implicitly
// inherit from it.
func parents(realm *realmsconf.Realm) []string {
	if realm.GetName() == realms.RootRealm {
		return nil
	}
	pRealms := make([]string, 0, 1+len(realm.Extends))
	pRealms = append(pRealms, realms.RootRealm)
	for _, name := range realm.Extends {
		if name != realms.RootRealm {
			pRealms = append(pRealms, name)
		}
	}
	return pRealms
}

// principalBindings binds a principal to a set of permissions and conditions.
type principalBindings struct {
	// name is the name of this principal, can be a user or a group.
	name string
	// permissions contains the indexes of permissions bound to this principal.
	permissions indexSet
	// conditions contains the sorted indexes of conditions guarding this binding.
	conditions []uint32
}

// perPrincipalBindings visits all bindings in the realm and its parent realms
// and appends them to the given slice (returning it in the end).
//
// Produces a lot of duplicates. It's the caller's job to skip them.
func (rlme *RealmsExpander) perPrincipalBindings(realm string, bindings []principalBindings) ([]principalBindings, error) {
	r, ok := rlme.realms[realm]
	if !ok {
		return nil, fmt.Errorf("realm %s not found in RealmsExpander", realm)
	}
	if r.GetName() != realm {
		return nil, fmt.Errorf("given realm: %s does not match name found internally: %s", realm, r.GetName())
	}

	for _, b := range r.Bindings {
		// Role details, including permissions grouped by attributes they evaluate.
		role, err := rlme.rolesExpander.roleDetails(b.Role)
		if err != nil {
			return nil, err
		}
		// For each permission group pick only conditions on attributes relevant
		// to this permission group. All other conditions won't affect evaluation
		// of this particular permission group and can be omitted.
		for _, permGroup := range role.groups {
			conds := rlme.condsSet.indexes(b.Conditions, &permGroup.attrs)
			for _, principal := range b.Principals {
				bindings = append(bindings, principalBindings{
					principal,
					permGroup.perms,
					conds,
				})
			}
		}
	}

	// Go through parents and get their bindings too.
	for _, parent := range parents(r) {
		var err error
		bindings, err = rlme.perPrincipalBindings(parent, bindings)
		if err != nil {
			return nil, fmt.Errorf("parent realm %q: %w", realm, err)
		}
	}
	return bindings, nil
}

// realmData returns calculated protocol.RealmData for a given realm.
func (rlme *RealmsExpander) realmData(name string, extends []*protocol.RealmData) (*protocol.RealmData, error) {
	_, ok := rlme.data[name]
	if !ok {
		rlm, found := rlme.realms[name]
		if !found {
			return nil, fmt.Errorf("realm %s not found in realms mapping", name)
		}
		for _, p := range parents(rlm) {
			data, err := rlme.realmData(p, extends)
			if err != nil {
				return nil, err
			}
			extends = append(extends, data)
		}
		data := deriveRealmData(rlm, extends)
		if name == realms.RootRealm && !rlme.projectCfg.IsEmpty() {
			if data == nil {
				data = &protocol.RealmData{}
			}
			populateRootRealmData(data, rlme.projectCfg)
		}
		rlme.data[name] = data
	}
	return rlme.data[name], nil
}

// deriveRealmData calculates the protocol.RealmData from the realm config and parent data.
func deriveRealmData(realm *realmsconf.Realm, extends []*protocol.RealmData) *protocol.RealmData {
	enforceInService := stringset.NewFromSlice(realm.EnforceInService...)
	for _, d := range extends {
		enforceInService.AddAll(d.GetEnforceInService())
	}
	if len(enforceInService) == 0 {
		return nil
	}
	return &protocol.RealmData{
		EnforceInService: enforceInService.ToSortedSlice(),
	}
}

// populateRootRealmData updates RealmData of the root realm based on a config.
func populateRootRealmData(d *protocol.RealmData, cfg *projects.ProjectConfig) {
	d.ProjectScopedAccount = cfg.ProjectScopedAccount
	d.BillingCloudProjectId = cfg.BillingCloudProjectID
}
