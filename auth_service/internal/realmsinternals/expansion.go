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
	"iter"
	"maps"
	"slices"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/data/stringset"
	lucierr "go.chromium.org/luci/common/errors"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/constants"
	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/internal/projects"
)

var (
	// ErrFinalized is used when the ConditionsSet has already been finalized
	// and further modifications are attempted.
	ErrFinalized = errors.New("conditions set has already been finalized")

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
	// normalized is a mapping from a serialized normalized protocol.Condition
	// to a pair (normalized *protocol.Condition, its unique index).
	normalized map[string]conditionMapTuple

	// indexMapping from serialized Condition to its index.
	indexMapping map[*realmsconf.Condition]uint32

	// finalized is true if finalize() was called, see finalize for more info.
	finalized bool
}

// conditionMapTuple is to represent the entries of normalized, reflects
// what index a Condition is tied to.
type conditionMapTuple struct {
	cond *protocol.Condition
	idx  uint32
}

// addCond adds a *Condition from realms.cfg definition to the set if it's
// not already there.
//
// Returns ErrFinalized -- if set has already been finalized.
func (cs *ConditionsSet) addCond(cond *realmsconf.Condition) error {
	if cs.finalized {
		return ErrFinalized
	}
	if _, ok := cs.indexMapping[cond]; ok {
		return nil
	}

	norm := &protocol.Condition{}
	if r := cond.GetRestrict(); r != nil {
		condSet := stringset.NewFromSlice(r.GetValues()...)
		norm.Op = &protocol.Condition_Restrict{
			Restrict: &protocol.Condition_AttributeRestriction{
				Attribute: r.GetAttribute(),
				Values:    condSet.ToSortedSlice(),
			},
		}
	}

	// Get the key for this condition and add it to the set of conditions.
	ck := conditionKey(norm)
	idx := uint32(len(cs.normalized))
	if condTup, ok := cs.normalized[ck]; ok {
		idx = condTup.idx
	}
	cs.normalized[ck] = conditionMapTuple{norm, idx}
	cs.indexMapping[cond] = idx
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

// sortConditionEntries sorts a given conditionMapTuple slice by attribute first
// then by values.
func sortConditionEntries(entries []conditionMapTuple) {
	sort.Slice(entries, sortby.Chain{
		func(i, j int) bool {
			return entries[i].cond.GetRestrict().GetAttribute() < entries[j].cond.GetRestrict().GetAttribute()
		},
		func(i, j int) bool {
			iVals, jVals := entries[i].cond.GetRestrict().GetValues(), entries[j].cond.GetRestrict().GetValues()
			return slices.Compare(iVals, jVals) < 0
		},
	}.Use)
}

// finalize finalizes the set by preventing any future addCond calls.
//
// Sorts the list of stored conditions by attribute first then by values.
// returns the final sorted list of protocol.Condition.
//
// Returns nil if ConditionSet is already finalized or if
// ConditionsSet is empty.
//
// Indexes returned by indexes() will refer to the indexes in this list.
func (cs *ConditionsSet) finalize() []*protocol.Condition {
	if cs.finalized {
		return nil
	}
	cs.finalized = true

	count := len(cs.normalized)
	if count == 0 {
		return nil
	}

	// Make a slice of the entries in cs.normalized to sort.
	normalized := make([]conditionMapTuple, 0, count)
	for _, entry := range cs.normalized {
		normalized = append(normalized, entry)
	}
	sortConditionEntries(normalized)

	// Not needed any more.
	cs.normalized = nil

	// Build the map of {old index -> new index} now that the final set of
	// conditions has been ordered.
	oldToNew := make(map[uint32]uint32, len(normalized))
	for idx, entry := range normalized {
		oldToNew[entry.idx] = uint32(idx)
	}

	// Update the index mapping to use the new order.
	for key, old := range cs.indexMapping {
		cs.indexMapping[key] = oldToNew[old]
	}

	// Return the list of conditions in the final order.
	conds := make([]*protocol.Condition, count)
	for i, entry := range normalized {
		conds[i] = entry.cond
	}
	return conds
}

// indexes returns a sorted slice of indexes.
//
// Can be called only after finalize(). All given conditions must have
// previously been put into the set via addCond(). The returned indexes can have
// fewer elements if some conditions in conds are equivalent.
//
// The returned indexes is essentially a compact encoding of the overall AND
// condition expression in a binding.
func (cs *ConditionsSet) indexes(conds []*realmsconf.Condition) []uint32 {
	if !cs.finalized {
		panic("indexes() called before finalize()")
	}
	if len(conds) == 0 {
		return nil
	}
	if len(conds) == 1 {
		if idx, ok := cs.indexMapping[conds[0]]; ok {
			return []uint32{idx}
		}
		panic(fmt.Sprintf("unexpected condition not seen by addCond: %v", conds[0]))
	}

	indexesSet := emptyIndexSet(len(conds))

	for _, cond := range conds {
		v, ok := cs.indexMapping[cond]
		if !ok {
			panic(fmt.Sprintf("unexpected condition not seen by addCond: %v", cond))
		}
		indexesSet.add(v)
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
	// permissionsDB is a database of all known permissions.
	permissionsDB map[string]*protocol.Permission

	// builtinRoles is a mapping from roleName -> *permissions.Role
	// these are generated from the permissions.cfg and translated to permissions
	// db which is where these roles come from. If a role is not found here
	// then it has not been defined in the permissions.cfg. This is assumed
	// final state and should not be modified.
	builtinRoles map[string]*permissions.Role

	// customRoles is a mapping from roleName -> *realmsconf.CustomRole
	// this mapping will be generated from permissionsDB and is defined in
	// permissisions.go when the DB is initialized. This is assumed final
	// state and should not be modifed.
	customRoles map[string]*realmsconf.CustomRole

	// permissions is a mapping from permission name to the internal index
	// all permissions are converted to a uint32 index for faster queries
	// this is the list of all declared permissions, this is initially defined
	// in permissions.cfg and initialized in permissionsDB. This is assumed
	// final state and should not be modified.
	permissions map[string]uint32

	// roles contains role to permissions mapping, keyed by roleName
	// this mapping contains a set of all the permissions a given role
	// is associated with.
	roles map[string]indexSet
}

// permIndex returns an internal index that represents the given permission string.
func (re *RolesExpander) permIndex(name string) uint32 {
	idx, ok := re.permissions[name]
	if !ok {
		idx = uint32(len(re.permissions))
		re.permissions[name] = idx
	}
	return idx
}

// permIndexes returns internal indexes representing the given permissions.
func (re *RolesExpander) permIndexes(names iter.Seq[string]) indexSet {
	// Note sorting is needed for the test determinism, this is removed in
	// the next CL.
	nameList := slices.Sorted(names)
	set := emptyIndexSet(len(nameList))
	for _, name := range nameList {
		set.add(re.permIndex(name))
	}
	return set
}

// role returns an IndexSet of permissions for a given role.
//
// returns
//
// ErrRoleNotFound - if given roleName doesn't exist in permissionsDB
// ErrImpossibleRole - if roleName format is invalid
func (re *RolesExpander) role(roleName string) (indexSet, error) {
	if perms, ok := re.roles[roleName]; ok {
		return perms, nil
	}

	var perms indexSet
	if strings.HasPrefix(roleName, constants.PrefixBuiltinRole) {
		role, ok := re.builtinRoles[roleName]
		if !ok {
			return indexSet{}, lucierr.Annotate(ErrRoleNotFound, "builtinRole: %s", roleName)
		}
		perms = re.permIndexes(maps.Keys(role.Permissions))
	} else if strings.HasPrefix(roleName, constants.PrefixCustomRole) {
		customRole, ok := re.customRoles[roleName]
		if !ok {
			return indexSet{}, lucierr.Annotate(ErrRoleNotFound, "customRole: %s", roleName)
		}
		perms = re.permIndexes(slices.Values(customRole.GetPermissions()))
		for _, parent := range customRole.Extends {
			parentRole, err := re.role(parent)
			if err != nil {
				return indexSet{}, err
			}
			perms.update(parentRole)
		}
	} else {
		return indexSet{}, ErrImpossibleRole
	}

	re.roles[roleName] = perms
	return perms, nil
}

// sortedPermissions returns a sorted slice of permissions and slice
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
		oldIdx := re.permissions[perm.Name]
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
		// Set of permissions associated with this role.
		perms, err := rlme.rolesExpander.role(b.GetRole())
		if err != nil {
			return nil, lucierr.Annotate(err, "there was an issue fetching permissions for this binding role")
		}

		// Skip empty roles, they don't affect the end result.
		if len(perms.set) == 0 {
			continue
		}

		// Sorted conditions associated with this binding.
		conds := rlme.condsSet.indexes(b.GetConditions())
		for _, principal := range b.GetPrincipals() {
			bindings = append(bindings, principalBindings{principal, perms, conds})
		}
	}

	// Go through parents and get their bindings too.
	for _, parent := range parents(r) {
		var err error
		bindings, err = rlme.perPrincipalBindings(parent, bindings)
		if err != nil {
			return nil, fmt.Errorf("failed when getting parent bindings for %s", realm)
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
