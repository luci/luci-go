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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/gob"
	"fmt"
	"math"
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
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/internal/permissions"
)

const (
	// The services associated with Auth Service aka Chrome Infra Auth,
	// to get its own configs.
	Cria    = "services/chrome-infra-auth"
	CriaDev = "services/chrome-infra-auth-dev"

	// The AppID of the deployed development environment, so the correct
	// config path will be used.
	DevAppID = "chrome-infra-auth-dev"

	// Paths to use within a project or service's folder when looking
	// for realms configs.
	RealmsCfgPath    = "realms.cfg"
	RealmsDevCfgPath = "realms-dev.cfg"
)

type realmsMap struct {
	mu     *sync.Mutex
	cfgMap map[string]*config.Config
}

// ExpandRealms expands a realmsconf.RealmsCfg into a flat protocol.Realms.
//
// The returned protocol.Realms contains realms and permissions of a single
// project only. Permissions not mentioned in the project's realms are omitted.
// All protocol.Permission messages have names only (no metadata). api_version field
// is omitted.
//
// All such protocol.Realms messages across all projects (plus a list of all
// defined permissions with all their metadata) are later merged together into
// a final universal protocol.Realms by merge() in the replication phase.
func ExpandRealms(db *permissions.PermissionsDB, projectID string, realmsCfg *realmsconf.RealmsCfg) (*protocol.Realms, error) {
	// internal is True when expanding internal realms (defined in a service
	// config file). Such realms can use internal roles and permissions and
	// they do not have implicit root bindings (since they are not associated
	// with any "project:<X>" identity used in implicit root bindings).
	internal := projectID == realms.InternalProject

	// TODO(cjacomet): Add extra validation step to ensure code hasn't changed

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
		normalized:   make(map[string]*conditionMapTuple),
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
		builtinRoles: db.Roles,
		customRoles:  customRolesMap,
		permissions:  map[string]uint32{},
		roles:        map[string]*indexSet{},
	}

	realmsExpander := &RealmsExpander{
		rolesExpander: rolesExpander,
		condsSet:      condsSet,
		realms:        realmsMap,
		data:          map[string]*protocol.RealmData{},
	}

	type realmMappingObj struct {
		name      string
		permTuple map[string]stringset.Set
	}

	realmsToReturn := []*realmMappingObj{}
	var permsToPrincipal map[string]stringset.Set

	// Visit all realms and build preliminary bindings as pairs of
	// (permission indexes, a list of principals who have them). The
	// bindings are preliminary since we don't know final permission indexes yet
	// and instead use some internal indexes as generated by RolesExpander. We need
	// to finish this first pass to gather the list of ALL used permissions, so we
	// can calculate final indexes. This is done inside of rolesExpander.
	for _, cfgRealm := range realmsList {
		// Build a mapping from a principal + conditions to the permissions set.
		//
		// Each map entry ---- means principal is granted the given set of permissions
		// if all given conditions allow it.
		//
		// This step essentially deduplicates permission bindings that result from
		// expanding realms and role inheritance chains.
		principalToPerms := map[string]*indexSet{}
		principalBindings, err := realmsExpander.perPrincipalBindings(cfgRealm.GetName())
		if err != nil {
			return nil, err
		}
		for _, principal := range principalBindings {
			key := toKey(principalPerms{Principal: principal.name, Conds: principal.conditions})
			if _, ok := principalToPerms[key]; !ok {
				principalToPerms[key] = emptyIndexSet()
			}
			principalToPerms[key].update(principal.permissions)
		}

		// Combine entries with the same set of permissions + conditions into one.
		//
		// Each map entry ---- means all principals are granted all given permissions
		// if all given conditions allow it.
		//
		// This step merges principal sets of identical bindings to have a more compact
		// final representation.
		permsToPrincipal = map[string]stringset.Set{}
		for key, perms := range principalToPerms {
			principalToPermsObj := toEntry(key)
			permsNorm := perms.toSortedSlice()
			permsToPrincipalObj := principalPerms{
				Conds: principalToPermsObj.Conds,
				Perms: permsNorm,
			}
			key := toKey(permsToPrincipalObj)
			if permsToPrincipal[key] == nil {
				permsToPrincipal[key] = stringset.Set{}
			}
			permsToPrincipal[key].Add(principalToPermsObj.Principal)
		}
		realmsToReturn = append(realmsToReturn, &realmMappingObj{cfgRealm.GetName(), permsToPrincipal})
	}

	perms, indexMap := rolesExpander.sortedPermissions()

	permsSorted := make([]*protocol.Permission, 0, len(perms))
	for _, p := range perms {
		permsSorted = append(permsSorted, &protocol.Permission{
			Name:     p,
			Internal: internal,
		})
	}

	realmsReturned := make([]*protocol.Realm, 0, len(realmsToReturn))
	for _, r := range realmsToReturn {
		data, err := realmsExpander.realmData(r.name, []*protocol.RealmData{})
		if err != nil {
			return nil, errors.Annotate(err, "couldn't fetch realm data").Err()
		}
		realmsReturned = append(realmsReturned, &protocol.Realm{
			Name:     fmt.Sprintf("%s:%s", projectID, r.name),
			Bindings: toNormalizedBindings(r.permTuple, indexMap),
			Data:     data,
		})
	}

	return &protocol.Realms{
		Permissions: permsSorted,
		Conditions:  allConditions,
		Realms:      realmsReturned,
	}, nil
}

// principalPerms is a wrapper struct to represent a relationship
// between a principal and permissions + conditions. The encoded
// form of this struct is used as a key to deduplicate.
type principalPerms struct {
	Principal string
	Conds     []uint32
	Perms     []uint32
}

// toKey converts a principalPerms struct to a key.
// this is useful for deduplicating principal to permissions
// bindings.
func toKey(p principalPerms) string {
	b := bytes.Buffer{}
	e := gob.NewEncoder(&b)
	err := e.Encode(p)
	if err != nil {
		fmt.Println(`failed gob Encode`, err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

// toEntry converts the key to an equivalent principalPerms
// struct.
func toEntry(key string) principalPerms {
	m := principalPerms{}
	by, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		fmt.Println(`failed base64 Decode`, err)
	}
	b := bytes.Buffer{}
	b.Write(by)
	d := gob.NewDecoder(&b)
	err = d.Decode(&m)
	if err != nil {
		fmt.Println(`failed gob Decode`, err)
	}
	return m
}

func toRealmsMap(realmsCfg *realmsconf.RealmsCfg, implicitRootBindings []*realmsconf.Binding) map[string]*realmsconf.Realm {
	realmsMap := map[string]*realmsconf.Realm{}
	for _, r := range realmsCfg.GetRealms() {
		realmsMap[r.GetName()] = r
	}
	root := &realmsconf.Realm{Name: realms.RootRealm}
	if res, ok := realmsMap[realms.RootRealm]; ok {
		root = res
	}
	root.Bindings = append(root.Bindings, implicitRootBindings...)
	realmsMap[realms.RootRealm] = root
	return realmsMap
}

type normalizedStruct struct {
	permsSorted []uint32
	conds       []uint32
	princ       []string
}

// toNormalizedBindings produces a sorted slice of *protocol.Binding.
//
// Bindings are given as a map from principalPerms -> list of principles
// that should have all given permission if all given conditions allow. In
// the principalPerms only the permissions and conditions are filled.
//
// Conditions are specified as indexes in ConditionSet, we use them as they are,
// since by consruction of ConditionsSet all conditions are in use and we don't
// need any extra filtering (and consequently index remapping to skip gaps) as we
// do for permissions.
//
// permsToPrincipal is a map mapping {Conds, Perms} -> principals.
// indexMapping defines how to remap permission indexes (old -> new).
func toNormalizedBindings(permsToPrincipal map[string]stringset.Set, indexMapping []uint32) []*protocol.Binding {
	normalized := []*normalizedStruct{}

	for key, principals := range permsToPrincipal {
		permsConds := toEntry(key)
		principalsCopy := principals.ToSortedSlice()

		idxSet := emptyIndexSet()
		for _, oldPermIdx := range permsConds.Perms {
			idxSet.add(indexMapping[oldPermIdx])
		}
		normalized = append(normalized, &normalizedStruct{
			permsSorted: idxSet.toSortedSlice(),
			conds:       permsConds.Conds,
			princ:       principalsCopy,
		})
	}
	bindings := []*protocol.Binding{}

	sort.Slice(normalized, sortby.Chain{
		func(i, j int) bool { return sliceCompare(normalized[i].permsSorted, normalized[j].permsSorted) },
		func(i, j int) bool { return sliceCompare(normalized[i].conds, normalized[j].conds) },
		func(i, j int) bool { return sliceCompare(normalized[i].princ, normalized[j].princ) },
	}.Use)

	for _, k := range normalized {
		bindings = append(bindings, &protocol.Binding{
			Permissions: k.permsSorted,
			Principals:  k.princ,
			Conditions:  k.conds,
		})
	}

	return bindings
}

func sliceCompare[T string | uint32](sli []T, slj []T) bool {
	sliceLen := int(math.Min(float64(len(sli)), float64(len(slj))))
	for idx := 0; idx < sliceLen; idx++ {
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
	targetCfgPath := cfgPath(ctx)
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

	self := func(ctx context.Context) string {
		if cfgPath(ctx) == RealmsDevCfgPath {
			return CriaDev
		}
		return Cria
	}

	// Get self config i.e. services/chrome-infra-auth-dev/realms-dev.cfg
	// or services/chrome-infra-auth/realms.cfg.
	eg, childCtx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		return latestMap.getLatestConfig(childCtx, client, self(ctx))
	})

	// Get other projects' realms configs.
	for _, project := range projects {
		project := project
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

	targetCfgPath := cfgPath(ctx)
	cfg, err := client.GetConfig(ctx, cfgSet, targetCfgPath, false)
	if err != nil {
		return errors.Annotate(err, "failed to fetch %s for %s", targetCfgPath, project).Err()
	}

	r.mu.Lock()
	r.cfgMap[project] = cfg
	r.mu.Unlock()

	return nil
}

// cfgPath is a helper function to know which cfg, depending on dev or prod env.
func cfgPath(ctx context.Context) string {
	if info.IsDevAppServer(ctx) || info.AppID(ctx) == DevAppID {
		return RealmsDevCfgPath
	}
	return RealmsCfgPath
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
