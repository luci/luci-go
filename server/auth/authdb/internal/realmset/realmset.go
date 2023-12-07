// Copyright 2020 The LUCI Authors.
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

// Package realmset provides queryable representation of LUCI Realms DB.
//
// Used internally by authdb.Snapshot.
package realmset

import (
	"context"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/auth/authdb/internal/conds"
	"go.chromium.org/luci/server/auth/authdb/internal/graph"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"
)

// ExpectedAPIVersion is the supported value of api_version field.
//
// See Build implementation for details.
const ExpectedAPIVersion = 1

// Realms is a queryable representation of realms.Realms proto.
type Realms struct {
	perms  map[string]PermissionIndex     // permission name -> its index
	names  stringset.Set                  // just names of all defined realms
	realms map[realmAndPerm]Bindings      // <realm, perm> -> who has it under which conditions
	data   map[string]*protocol.RealmData // per-realm attached RealmData

	// Used by QueryBindings: perm -> project -> [(realm, bindings)].
	bindingsIdx map[PermissionIndex]map[string][]RealmBindings
}

// PermissionIndex is used in place of permission names.
//
// Note: should match an int type used in `permissions` field in the proto.
type PermissionIndex uint32

// Binding represents a set of principals and a condition when it can be used.
//
// See Bindings(...) method for more details.
type Binding struct {
	Condition *conds.Condition // nil if the binding is unconditional
	Groups    graph.SortedNodeSet
	Idents    stringset.Set
}

// badness is an overall heuristic score of how complex this binding to
// evaluate and how likely it will apply.
//
// 0 means "easy to evaluate, high likelihood of applying". Used to sort
// bindings in Bindings array returned by Bindings(...).
func (b *Binding) badness() int {
	// TODO(vadimsh): This can be improved. For example, bindings with groups
	// that contain globs (like `user:*`) are more likely to apply and should
	// have lower badness.
	if b.Condition == nil {
		return 0
	}
	return 1
}

// Bindings is a list of bindings in a single realm for a single permission.
type Bindings []Binding

// Check returns true of any of the bindings in the list are applying.
//
// Checks conditions on `attrs` and memberships of the identity represented by
// `q`.
func (b Bindings) Check(ctx context.Context, q *graph.MembershipsQueryCache, attrs realms.Attrs) bool {
	for _, binding := range b {
		if binding.Condition == nil || binding.Condition.Eval(ctx, attrs) {
			switch {
			case binding.Idents.Has(string(q.Identity)):
				return true // was granted the permission explicitly in the ACL
			case q.IsMemberOfAny(binding.Groups):
				return true // has the permission through a group
			}
		}
	}
	return false
}

// RealmBindings is a realm name plus bindings for a single permission there.
//
// Used as part of QueryBindings return value.
type RealmBindings struct {
	// Realms is a full realm name as "<project>:<name>".
	Realm string
	// Bindings is a list of bindings for a permission passed to QueryBindings.
	Bindings Bindings
}

// realmAndPerm is used as a composite key in `realms` map.
type realmAndPerm struct {
	realm string
	perm  PermissionIndex
}

// PermissionIndex returns an index of the given permission.
//
// It can be passed to Bindings(...). Returns (0, false) if there's no such
// permission in the Realms DB.
func (r *Realms) PermissionIndex(perm realms.Permission) (idx PermissionIndex, ok bool) {
	idx, ok = r.perms[perm.Name()]
	return
}

// HasRealm returns true if the given realm exists in the DB.
func (r *Realms) HasRealm(realm string) bool {
	return r.names.Has(realm)
}

// Data returns RealmData attached to a realm or nil if none.
func (r *Realms) Data(realm string) *protocol.RealmData {
	return r.data[realm]
}

// Bindings returns representation of bindings that define who has the requested
// permission in the given realm.
//
// Each returned binding is a tuple (condition, groups, identities):
//   - Condition: a predicate over realms.Attrs map that evaluates to true if
//     this binding is "active". Inactive bindings should be skipped.
//   - Groups: a set of groups with principals that have the permission,
//     represented by a sorted slice of group indexes in a graph.QueryableGraph
//     which was passed to Build().
//   - Identities: a set of identity strings that were specified in the realm
//     ACL directly (not via a group).
//
// The permission should be specified as its index obtained via PermissionIndex.
//
// The realm name is not validated. Unknown or invalid realms are silently
// treated as empty. No fallback to @root happens.
//
// Returns nil if the requested permission is not mentioned in any binding in
// the realm at all.
func (r *Realms) Bindings(realm string, perm PermissionIndex) Bindings {
	return r.realms[realmAndPerm{realm, perm}]
}

// QueryBindings returns **all** bindings for the given permission across all
// realms and projects.
//
// The result is a map "project name => list of (realm, bindings for the
// requested permission in this realm)". It includes only projects and realms
// that have bindings for the queried permission. The order of items in the list
// is not well-defined.
//
// This information is available only for permission flagged with
// UsedInQueryRealms. Returns `ok == false` if `perm` was not flagged.
func (r *Realms) QueryBindings(perm PermissionIndex) (map[string][]RealmBindings, bool) {
	res, ok := r.bindingsIdx[perm]
	return res, ok
}

// Build constructs Realms from the proto message, the group graph and
// permissions registered by the processes.
//
// Only registered permissions will be queriable. Bindings with all other
// permissions will be ignored to save RAM.
func Build(r *protocol.Realms, qg *graph.QueryableGraph, registered map[realms.Permission]realms.PermissionFlags) (*Realms, error) {
	// Do not use realms.Realms we don't understand. Better to go offline
	// completely than mistakenly allow access to something private by
	// misinterpreting realm rules (e.g. if a new hypothetical DENY rule is
	// misinterpreted as ALLOW).
	//
	// Bumping `api_version` (if it ever happens) should be done extremely
	// carefully in multiple stages:
	//   1. Update components.auth to understand both new and old api_version.
	//   2. Redeploy *everything*.
	//   3. Update Auth Service to generate realms.Realms using the new API.
	if r.ApiVersion != ExpectedAPIVersion {
		return nil, errors.Reason(
			"Realms proto has api_version %d not compatible with this service (it expects %d)",
			r.ApiVersion, ExpectedAPIVersion).Err()
	}

	// Build map: permission name -> its index (since Binding messages operate
	// with indexes). Using ints as keys is also slightly faster than strings.
	perms := make(map[string]PermissionIndex, len(r.Permissions))
	for idx, perm := range r.Permissions {
		perms[perm.Name] = PermissionIndex(idx)
	}

	// Build a set of permission indexes the process is interested in checking.
	// All other permissions will simply be ignored to avoid wasting RAM on them
	// (they won't be checked anyway).
	activePerms := make(map[PermissionIndex]struct{}, len(registered))
	for perm := range registered {
		if idx, ok := perms[perm.Name()]; ok {
			activePerms[idx] = struct{}{}
		}
	}

	// Gather names of all realms for HasRealm check.
	names := stringset.New(len(r.Realms))
	for _, realm := range r.Realms {
		names.Add(realm.Name)
	}

	// This is the `realms` map under construction. We'll shrink its memory
	// footprint at the end.
	type bindingKey struct {
		realmAndPerm
		cond *conds.Condition
	}
	realmsToBe := map[bindingKey]principalSet{}
	counts := map[realmAndPerm]int{}

	// A caching factory of Conditions for conditional bindings.
	conds := conds.NewBuilder(r.Conditions)

	// interner is used to deduplicate memory used to store identity names.
	interner := stringInterner{}

	// Visit all bindings in all realms and update principal sets in realmsToBe.
	for _, realm := range r.Realms {
		for _, binding := range realm.Bindings {
			// Categorize 'principals' into groups and identity strings.
			groups, idents := categorizePrincipals(binding.Principals, qg, interner)
			if len(groups) == 0 && len(idents) == 0 {
				continue
			}

			// Build a condition predicate (`nil` means "no condition"). If such
			// predicate was already seen before, returns the existing condition, so
			// using Condition pointers in map keys is OK. Returns an error if
			// a condition index in binding.Conditions is out of bounds or the
			// condition is malformed. This should not happen in a valid AuthDB.
			cond, err := conds.Condition(binding.Conditions)
			if err != nil {
				return nil, errors.Annotate(err, "invalid binding %q in realm %q", binding, realm.Name).Err()
			}

			// Add principals into the corresponding principal sets in realmsToBe.
			for _, permIdx := range binding.Permissions {
				permIdx := PermissionIndex(permIdx)
				if _, yes := activePerms[permIdx]; !yes {
					continue
				}
				key := bindingKey{realmAndPerm{realm.Name, permIdx}, cond}
				if ps, ok := realmsToBe[key]; ok {
					ps.add(groups, idents)
				} else {
					realmsToBe[key] = newPrincipalSet(groups, idents)
					counts[key.realmAndPerm] += 1
				}
			}
		}
	}

	// Replace identically looking group sets with references to a single copy.
	// Collect conditional bindings for the same (realm, perm) key into an array,
	// since we'll need to evaluate them sequentially when serving HasPermission
	// checks.
	realmMap := make(map[realmAndPerm]Bindings, len(counts))
	dedupper := graph.NodeSetDedupper{}
	for key, ps := range realmsToBe {
		groups, idents := ps.finalize(dedupper)
		if realmMap[key.realmAndPerm] == nil {
			realmMap[key.realmAndPerm] = make(Bindings, 0, counts[key.realmAndPerm])
		}
		realmMap[key.realmAndPerm] = append(realmMap[key.realmAndPerm], Binding{
			Condition: key.cond,
			Groups:    groups,
			Idents:    idents,
		})
	}

	// Order bindings by "badness" of checking (easiest to check first) and
	// chances of applying. Right now this uses a very simplistic heuristic:
	// unconditional bindings are easier to check and more likely to apply than
	// conditional ones.
	for _, bindings := range realmMap {
		sort.Slice(bindings, func(l, r int) bool {
			if bl, br := bindings[l].badness(), bindings[r].badness(); bl != br {
				return bl < br
			}
			// Order bindings of equal "badness" deterministically based on index of
			// their conditions (which ultimately depends on order of data in Realms
			// proto). This simplifies tests and makes HasPermission check
			// performance more deterministic too.
			idxLeft := 0
			if bindings[l].Condition != nil {
				idxLeft = bindings[l].Condition.Index() + 1
			}
			idxRight := 0
			if bindings[r].Condition != nil {
				idxRight = bindings[r].Condition.Index() + 1
			}
			return idxLeft < idxRight
		})
	}

	// Extract attached per-realm data into a queryable map.
	count := 0
	for _, realm := range r.Realms {
		if realm.Data != nil {
			count++
		}
	}
	data := make(map[string]*protocol.RealmData, count)
	for _, realm := range r.Realms {
		if realm.Data != nil {
			data[realm.Name] = realm.Data
		}
	}

	// For all permissions with UsedInQueryRealms flag, build a data set with all
	// realms that have this permission. This allows skipping unrelated realms
	// in QueryRealms. Note that we'll reuse Bindings slices from `realmMap`, so
	// we pay extra RAM only for actual mapping.
	bindingsIdx := make(map[PermissionIndex]map[string][]RealmBindings, len(registered))
	for perm, flags := range registered {
		if flags&realms.UsedInQueryRealms != 0 {
			if permIdx, ok := perms[perm.Name()]; ok {
				bindingsIdx[permIdx] = map[string][]RealmBindings{}
			}
		}
	}
	for realmAndPerm, bindings := range realmMap {
		if projToBindings, ok := bindingsIdx[realmAndPerm.perm]; ok {
			proj, _ := realms.Split(realmAndPerm.realm)
			projToBindings[proj] = append(projToBindings[proj], RealmBindings{
				Realm:    realmAndPerm.realm,
				Bindings: bindings,
			})
		}
	}

	return &Realms{
		perms:       perms,
		names:       names,
		realms:      realmMap,
		data:        data,
		bindingsIdx: bindingsIdx,
	}, nil
}

// stringInterner implements string interning to save some memory.
type stringInterner map[string]string

// intern returns an interned copy of 's'.
func (si stringInterner) intern(s string) string {
	if existing, ok := si[s]; ok {
		return existing
	}
	si[s] = s
	return s
}

// categorizePrincipals splits a list of principals into a list of groups
// (identified by their indexes in a QueryableGraph) and list of identity names.
//
// Unknown groups are silently skipped.
func categorizePrincipals(p []string, qg *graph.QueryableGraph, interner stringInterner) (groups []graph.NodeIndex, idents []string) {
	for _, principal := range p {
		if strings.HasPrefix(principal, "group:") {
			if idx, ok := qg.GroupIndex(strings.TrimPrefix(principal, "group:")); ok {
				groups = append(groups, idx)
			}
		} else {
			idents = append(idents, interner.intern(principal))
		}
	}
	return
}

// principalSet represents a set of groups and identities.
//
// It is used transiently when constructing the final memory-optimized set in
// Build.
type principalSet struct {
	groups graph.NodeSet
	idents stringset.Set
}

func newPrincipalSet(groups []graph.NodeIndex, idents []string) principalSet {
	ps := principalSet{
		groups: make(graph.NodeSet, len(groups)),
		idents: stringset.New(len(idents)),
	}
	ps.add(groups, idents)
	return ps
}

func (ps principalSet) add(groups []graph.NodeIndex, idents []string) {
	for _, idx := range groups {
		ps.groups.Add(idx)
	}
	for _, ident := range idents {
		ps.idents.Add(ident)
	}
}

// finalize produces a memory-optimized representation of this principal set.
//
// It replace identically looking group sets with references to a single copy
// using the given dedupper. It also throws away zero-length sets replacing them
// with nils.
//
// Non-empty identity sets are kept as is without any dedupping, assuming using
// identities in Realm ACLs directly is rare and not worth optimizing for (on
// top of the string interning optimization we've already done).
func (ps principalSet) finalize(dedupper graph.NodeSetDedupper) (graph.SortedNodeSet, stringset.Set) {
	var groups graph.SortedNodeSet
	if len(ps.groups) > 0 {
		groups = dedupper.Dedup(ps.groups)
	}
	var idents stringset.Set
	if ps.idents.Len() > 0 {
		idents = ps.idents
	}
	return groups, idents
}
