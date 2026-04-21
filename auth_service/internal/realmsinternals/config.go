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
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	lucivalidation "go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/configs/validation"
	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/internal/projects"
)

const (
	// The services associated with Auth Service aka Chrome Infra Auth,
	// to get its own configs.
	Cria    = "services/chrome-infra-auth"
	CriaDev = "services/chrome-infra-auth-dev"
)

type realmsMap struct {
	mu     *sync.Mutex
	cfgMap map[string]*config.Config
}

// ExpandRealms expands a realmsconf.RealmsCfg into a flat protocol.Realms.
//
// The returned protocol.Realms contains realms and permissions of a single
// project only. Permissions not mentioned in the project's realms are omitted.
// All protocol.Permission messages have names only (no metadata). api_version
// field is omitted.
//
// All such protocol.Realms messages across all projects (plus a list of all
// defined permissions with all their metadata) are later merged together into
// a final universal protocol.Realms by merge() in the replication phase.
func ExpandRealms(ctx context.Context, db *permissions.PermissionsDB, projs *projects.Projects, projectID string, realmsCfg *realmsconf.RealmsCfg) (*protocol.Realms, error) {
	// internal is True when expanding internal realms (defined in a service
	// config file). Such realms can use internal roles and permissions and
	// they do not have implicit root bindings (since they are not associated
	// with any "project:<X>" identity used in implicit root bindings).
	internal := projectID == realms.InternalProject

	// Set on the staging instance of LUCI.
	isDev := validation.GetRealmsCfgPath(ctx) == validation.RealmsDevCfgPath

	// The server code could have changed since the config passed the validation
	// and realmsCfg may not be valid anymore. Verify it still is. The code
	// below depends crucially on the validity of realmsCfg.
	vctx := &lucivalidation.Context{Context: ctx}
	vctx.SetFile(fmt.Sprintf("realms for project %s", projectID))
	rv := validation.NewRealmsValidator(db, internal)
	rv.Validate(vctx, realmsCfg)
	if err := vctx.Finalize(); err != nil {
		return nil, errors.Fmt("invalid realms config: %w", err)
	}

	// Make sure @root realm exists and append implicit bindings to it. We need
	// to do this before enumerating the conditions below to actually instantiate
	// all Condition objects that we'll need to visit (some of them may come from
	// implicit bindings). Pre-instantiating them is important because we rely on
	// their pointer address as map keys for lookups.
	bindings := []*realmsconf.Binding{}
	if !internal {
		bindings = db.ImplicitRootBindings(projectID)
	}
	realmsMap := toRealmsMap(realmsCfg, bindings)

	// We will need to visit realms in sorted order twice. Sort once and remember.
	realmsList := make([]*realmsconf.Realm, 0, len(realmsMap))
	for _, v := range realmsMap {
		realmsList = append(realmsList, v)
	}
	sort.Slice(realmsList, func(i, j int) bool {
		return realmsList[i].GetName() < realmsList[j].GetName()
	})

	customRolesMap := make(map[string]*realmsconf.CustomRole, len(realmsCfg.GetCustomRoles()))
	for _, r := range realmsCfg.GetCustomRoles() {
		customRolesMap[r.GetName()] = r
	}

	condsSet := &ConditionsSet{
		indexMapping: make(map[*realmsconf.Condition]uint32),
		normalized:   make(map[string]conditionMapTuple),
	}

	// Prepopulate condsSet with all conditions mentioned in all bindings to
	// normalize, dedup and map them to integers. Integers are faster to work with
	// and we'll need them for the final proto message.
	for _, realm := range realmsList {
		for _, binding := range realm.Bindings {
			for _, cond := range binding.Conditions {
				if err := condsSet.addCond(cond); err != nil {
					return nil, err
				}
			}
		}
	}

	allConditions := condsSet.finalize()

	rolesExpander := &RolesExpander{
		permissionsDB: db.Permissions,
		builtinRoles:  db.Roles,
		customRoles:   customRolesMap,
		permissions:   map[string]uint32{},
		roles:         map[string]indexSet{},
	}

	realmsExpander := &RealmsExpander{
		rolesExpander: rolesExpander,
		condsSet:      condsSet,
		projectCfg:    projs.ProjectConfig(ctx, projectID, isDev),
		realms:        realmsMap,
		data:          map[string]*protocol.RealmData{},
	}

	// Visit all realms and build preliminary bindings as maps:
	//   (<conditions set>, <permissions set>) -> <principals set>
	//
	// The bindings are preliminary since we don't know final permission indexes
	// yet and instead use some internal indexes as generated by RolesExpander. We
	// need to finish this first pass to gather the list of ALL used permissions,
	// so we can calculate final indexes. This is done inside of rolesExpander.
	var realmsToReturn []*protocol.Realm
	var principalBindings []principalBindings
	for _, cfgRealm := range realmsList {
		// Get all elementary bindings based directly on what's in the config.
		// There will be a lot of duplicates we'll need to skip or group. Reuse
		// the slice capacity from the previous iteration.
		var err error
		principalBindings, err = realmsExpander.perPrincipalBindings(cfgRealm.GetName(), principalBindings[:0])
		if err != nil {
			return nil, err
		}

		// Build a mapping from a principal + conditions to the permissions set.
		//
		// Each map entry means the principal is granted the given set of
		// permissions if all given conditions allow it.
		//
		// This step essentially deduplicates permission bindings that result from
		// expanding realms and role inheritance chains.
		type principalAndConds struct {
			principal string
			conds     serializedIntList
		}
		type principalPerms struct {
			principal   string   // static, the same as in the key
			conds       []uint32 // static, the same as in the key
			permissions indexSet // updated in the loop
		}
		principalAndCondsToPerms := map[principalAndConds]principalPerms{}
		for _, principal := range principalBindings {
			key := principalAndConds{
				principal: principal.name,
				conds:     serializeIntList(principal.conditions),
			}
			if entry, ok := principalAndCondsToPerms[key]; ok {
				entry.permissions.update(principal.permissions)
			} else {
				principalAndCondsToPerms[key] = principalPerms{
					principal:   principal.name,
					conds:       principal.conditions,
					permissions: principal.permissions.clone(),
				}
			}
		}

		// Combine entries with the same set of permissions + conditions into one.
		//
		// Each map entry means all principals are granted all given permissions
		// if all given conditions allow it.
		//
		// This step merges principal sets of identical bindings to have a more
		// compact final representation.
		type permsAndConds struct {
			perms serializedIntList
			conds serializedIntList
		}
		type principalsSet struct {
			perms      []uint32      // static, the same as in the key
			conds      []uint32      // static, the same as in the key
			principals stringset.Set // updated in the loop
		}
		permsAndCondsToPrincipals := map[permsAndConds]principalsSet{}
		for _, permsEntry := range principalAndCondsToPerms {
			perms := permsEntry.permissions.toSortedSlice()
			key := permsAndConds{
				perms: serializeIntList(perms),
				conds: serializeIntList(permsEntry.conds),
			}
			if entry, ok := permsAndCondsToPrincipals[key]; ok {
				entry.principals.Add(permsEntry.principal)
			} else {
				permsAndCondsToPrincipals[key] = principalsSet{
					perms:      perms,
					conds:      permsEntry.conds,
					principals: stringset.NewFromSlice(permsEntry.principal),
				}
			}
		}

		// Convert principalsSet to *Binding.
		bindings := make([]*protocol.Binding, 0, len(permsAndCondsToPrincipals))
		for _, entry := range permsAndCondsToPrincipals {
			bindings = append(bindings, &protocol.Binding{
				Permissions: entry.perms,
				Conditions:  entry.conds,
				Principals:  entry.principals.ToSortedSlice(),
			})
		}
		realmsToReturn = append(realmsToReturn, &protocol.Realm{
			Name:     cfgRealm.GetName(),
			Bindings: bindings,
		})
	}

	// Remap permission indexes based on the final set of discovered permissions,
	// attach realm data and convert realm names to be global.
	perms, indexMap := rolesExpander.sortedPermissions()
	for _, r := range realmsToReturn {
		data, err := realmsExpander.realmData(r.Name, []*protocol.RealmData{})
		if err != nil {
			return nil, errors.Fmt("couldn't fetch realm data: %w", err)
		}
		r.Name = fmt.Sprintf("%s:%s", projectID, r.Name)
		r.Data = data
		normalizedBindings(r.Bindings, indexMap)
	}

	return &protocol.Realms{
		Permissions: perms,
		Conditions:  allConditions,
		Realms:      realmsToReturn,
	}, nil
}

func toRealmsMap(realmsCfg *realmsconf.RealmsCfg, implicitRootBindings []*realmsconf.Binding) map[string]*realmsconf.Realm {
	realmsMap := map[string]*realmsconf.Realm{}
	for _, r := range realmsCfg.GetRealms() {
		realmsMap[r.GetName()] = r
	}

	root, _ := realmsMap[realms.RootRealm]

	// Make sure there's a root realm with implicit bindings. If there's a root
	// already, make a shallow copy to avoid mutating `Bindings` list of
	// the original.
	realmsMap[realms.RootRealm] = &realmsconf.Realm{
		Name:             realms.RootRealm,
		Extends:          root.GetExtends(),
		Bindings:         append(slices.Clip(root.GetBindings()), implicitRootBindings...),
		EnforceInService: root.GetEnforceInService(),
	}

	return realmsMap
}

// serializedIntList is used in map keys to represent []uint32.
type serializedIntList string

// serializeIntList converts a list of integers to a string to use in a map key.
func serializeIntList(list []uint32) serializedIntList {
	if len(list) == 0 {
		return ""
	}
	out := make([]byte, len(list)*4)
	for idx, val := range list {
		binary.LittleEndian.PutUint32(out[idx*4:], val)
	}
	return serializedIntList(out)
}

// normalizedBindings produces a sorted slice of *protocol.Binding.
//
// Modifies given []*protocol.Binding in place by remapping permission indexes
// according to indexMapping (old -> new) and sorting the final list of
// bindings.
//
// Conditions are specified as indexes in ConditionSet, we use them as they are,
// since by construction of ConditionsSet all conditions are in use and we don't
// need any extra filtering (and consequently index remapping to skip gaps) as
// we do for permissions.
func normalizedBindings(bindings []*protocol.Binding, indexMapping []uint32) {
	for _, b := range bindings {
		for i, oldPermIdx := range b.Permissions {
			b.Permissions[i] = indexMapping[oldPermIdx]
		}
		slices.Sort(b.Permissions)
	}
	sort.Slice(bindings, sortby.Chain{
		func(i, j int) bool { return sliceCompare(bindings[i].Permissions, bindings[j].Permissions) },
		func(i, j int) bool { return sliceCompare(bindings[i].Conditions, bindings[j].Conditions) },
		func(i, j int) bool { return sliceCompare(bindings[i].Principals, bindings[j].Principals) },
	}.Use)
}

func sliceCompare[T string | uint32](sli []T, slj []T) bool {
	sliceLen := min(len(sli), len(slj))
	for idx := range sliceLen {
		if sli[idx] != slj[idx] {
			return sli[idx] < slj[idx]
		}
	}
	return len(sli) < len(slj)
}

// FetchLatestRealmsConfigs fetches the latest configs from luci-cfg
// concurrently.
//
// Errors:
//   - ErrNoConfig if config is not found
//   - annotated error for all other errors
func FetchLatestRealmsConfigs(ctx context.Context) (map[string]*config.Config, error) {
	targetCfgPath := validation.GetRealmsCfgPath(ctx)
	projects, err := cfgclient.ProjectsWithConfig(ctx, targetCfgPath)
	if err != nil {
		return nil, err
	}
	logging.Debugf(ctx, "%d projects with %s: %s", len(projects), targetCfgPath, projects)

	// Client to fetch configs.
	client := cfgclient.Client(ctx)

	latestMap := realmsMap{
		mu:     &sync.Mutex{},
		cfgMap: make(map[string]*config.Config, len(projects)+1),
	}

	selfProject := Cria
	if targetCfgPath == validation.RealmsDevCfgPath {
		selfProject = CriaDev
	}

	// Get self config i.e. services/chrome-infra-auth-dev/realms-dev.cfg
	// or services/chrome-infra-auth/realms.cfg.
	eg, childCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return latestMap.getLatestConfig(childCtx, client, selfProject)
	})

	// Get other projects' realms configs.
	for _, project := range projects {
		eg.Go(func() error {
			return latestMap.getLatestConfig(childCtx, client, project)
		})
	}

	err = eg.Wait()
	if err != nil {
		return nil, err
	}

	return latestMap.cfgMap, nil
}

// getLatestConfig fetches the most up to date realms.cfg for a given project, unless
// fetching the config for self, in which case it fetches the service config. The configs are
// written to a map mapping K: project name (string) -> V: *config.Config.
func (r *realmsMap) getLatestConfig(ctx context.Context, client config.Interface, project string) error {
	project, cfgSet, err := r.cfgSet(project)
	if err != nil {
		return err
	}

	targetCfgPath := validation.GetRealmsCfgPath(ctx)
	cfg, err := client.GetConfig(ctx, cfgSet, targetCfgPath, false)
	if err != nil {
		return errors.Fmt("failed to fetch %s for %s: %w", targetCfgPath, project, err)
	}

	r.mu.Lock()
	r.cfgMap[project] = cfg
	r.mu.Unlock()

	return nil
}

// cfgSet is a helper function to know which configSet to use, this is necessary for
// getting the realms cfg for CrIA or CrIADev since the realms.cfg is stored as
// a service config instead of a project config.
func (r *realmsMap) cfgSet(project string) (string, config.Set, error) {
	if project == Cria || project == CriaDev {
		r.mu.Lock()
		defer r.mu.Unlock()
		if _, ok := r.cfgMap[realms.InternalProject]; ok {
			return "", "", fmt.Errorf("unexpected LUCI Project: %s", realms.InternalProject)
		}
		return realms.InternalProject, config.Set(project), nil
	}

	ps, err := config.ProjectSet(project)
	if err != nil {
		return "", "", err
	}
	return project, ps, nil
}
