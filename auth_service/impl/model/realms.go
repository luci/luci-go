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

package model

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/data/sortby"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth/service/protocol"
)

// RealmsAPIVersion is the currently acceptable version of Realms API.
// See api_version in realms.proto.
const RealmsAPIVersion = 1

// conditionSet dedups identical conditions, maps them to integer indexes.
//
// Most often identical conditions appear from implicit root bindings that are
// similar across all projects.
//
// Assumes all incoming protocol.Condition are immutable and already
// normalized. Retains the order in which they were added.
type conditionSet struct {
	// all contains the unique Conditions added, preserving the order in which
	// they were first added.
	all []*protocol.Condition

	// indexMapping is the mapping of Condition keys to the Condition's index
	// within all.
	indexMapping map[string]uint32
}

// add attempts to add the given Condition to the set, skipping if an
// equivalent Condition already exists in the set.
func (cs *conditionSet) add(cond *protocol.Condition) error {
	condKey, err := conditionKey(cond)
	if err != nil {
		return err
	}

	if _, ok := cs.indexMapping[condKey]; ok {
		// duplicate condition; no need to add it.
		return nil
	}

	// This is a new Condition which needs to be recorded.
	cs.indexMapping[condKey] = uint32(len(cs.all))
	cs.all = append(cs.all, cond)
	return nil
}

// relabel calculates the indices of equivalent Conditions in all, given target
// Conditions. The target conditions are specified by providing a slice of
// Conditions and the target Conditions' indices within that slice.
func (cs *conditionSet) relabel(localConds []*protocol.Condition, localIndices []uint32) ([]uint32, error) {
	maxIndex := uint32(len(localConds) - 1)
	globalIndices := make([]uint32, 0, len(localIndices))
	errs := []error{}
	for _, localIndex := range localIndices {
		if localIndex > maxIndex {
			errs = append(errs, fmt.Errorf("index %d > max index %d",
				localIndex, maxIndex))
			continue
		}

		targetKey, err := conditionKey(localConds[localIndex])
		if err != nil {
			errs = append(errs, err)
			continue
		}

		globalIndex, ok := cs.indexMapping[targetKey]
		if !ok {
			errs = append(errs, fmt.Errorf("unknown condition at index %d",
				localIndex))
			continue
		}
		globalIndices = append(globalIndices, globalIndex)
	}

	return globalIndices, errors.NewMultiError(errs...).AsError()
}

// conditionKey generates a key by serializing a protocol.Condition.
func conditionKey(cond *protocol.Condition) (string, error) {
	key, err := proto.Marshal(cond)
	if err != nil {
		return "", errors.Fmt("error generating key: %w", err)
	}
	return string(key), nil
}

// MergeRealms merges all the project realms into one realms definition, using
// the permissions in the given AuthRealmsGlobals as an authoritative source of
// valid permissions.
//
// If some realm uses a permission not in the list, it will be silently dropped
// from the bindings. This can potentially happen due to the asynchronous nature
// of realms config updates (e.g. a role change that deletes some permissions
// can be committed into the AuthDB before realms are reevaluated). Eventually,
// the state should converge to be 100% consistent.
//
// Returns a single Realms object, which represents all projects' realms if
// there were no issues when merging, otherwise nil and an annotated error.
//
// An error may occur when:
// * unmarshalling project realms;
// * merging conditions into a single set;
// * processing a single realm that doesn't have the project ID as a prefix;
// * relabelling condition indices relative to the new set of all conditions.
func MergeRealms(
	ctx context.Context,
	realmsGlobals *AuthRealmsGlobals,
	allAuthProjectRealms []*AuthProjectRealms) (*protocol.Realms, error) {
	result := &protocol.Realms{
		ApiVersion: RealmsAPIVersion,
	}

	var permissions []*protocol.Permission
	if realmsGlobals != nil {
		permissions = realmsGlobals.PermissionsList.GetPermissions()
	}
	result.Permissions = permissions

	// Permission name -> its index in the merged realms' permissions.
	globalPermIndex := make(map[string]int, len(permissions))
	for i, p := range permissions {
		globalPermIndex[p.GetName()] = i
	}

	projectIDs := make([]string, len(allAuthProjectRealms))
	realmsByProject := make(map[string]*protocol.Realms, len(allAuthProjectRealms))
	for i, authProjectRealms := range allAuthProjectRealms {
		projRealms, err := authProjectRealms.RealmsToProto()
		if err != nil {
			return nil, errors.Fmt("error parsing Realms from AuthProjectRealms: %w", err)

		}
		projectIDs[i] = authProjectRealms.ID
		realmsByProject[authProjectRealms.ID] = projRealms
	}
	sort.Strings(projectIDs)

	// Make the set of Conditions across all projects.
	condSet := &conditionSet{
		all:          []*protocol.Condition{},
		indexMapping: make(map[string]uint32),
	}
	for _, projectID := range projectIDs {
		for _, cond := range realmsByProject[projectID].Conditions {
			if err := condSet.add(cond); err != nil {
				return nil, errors.Fmt("error merging conditions: %w", err)

			}
		}
	}
	result.Conditions = condSet.all

	for _, projectID := range projectIDs {
		expectedPrefix := fmt.Sprintf("%s:", projectID)
		projectRealms := realmsByProject[projectID]

		// Map the permission index in the project realms to the
		// permission index in the merged realms.
		oldToNewPermIndex := make([]int, len(projectRealms.Permissions))
		for oldIndex, perm := range projectRealms.Permissions {
			newIndex, ok := globalPermIndex[perm.GetName()]
			if !ok {
				newIndex = -1
			}
			oldToNewPermIndex[oldIndex] = newIndex
		}

		// Visit all bindings in all realms.
		for _, oldRealm := range projectRealms.Realms {
			realmName := oldRealm.GetName()
			if !strings.HasPrefix(realmName, expectedPrefix) {
				return nil, fmt.Errorf(
					"project '%s' has realm with incorrectly prefixed name '%s'",
					projectID, realmName)
			}

			newBindings := []*protocol.Binding{}
			for _, oldBinding := range oldRealm.Bindings {
				permIndices := make([]uint32, 0, len(oldBinding.Permissions))
				for _, oldIndex := range oldBinding.Permissions {
					if newIndex := oldToNewPermIndex[oldIndex]; newIndex >= 0 {
						permIndices = append(permIndices, uint32(newIndex))
					}
				}

				if len(permIndices) > 0 {
					condIndices, err := condSet.relabel(projectRealms.Conditions, oldBinding.Conditions)
					if err != nil {
						return nil, errors.Fmt("error relabelling conditions: %w", err)

					}
					// Permissions and Conditions in a protocol.Binding must be
					// in ascending order. See
					// go.chromium.org/luci/server/auth/service/protocol#Binding
					sortIndices(permIndices)
					sortIndices(condIndices)
					newBinding := &protocol.Binding{
						Permissions: permIndices,
						Conditions:  condIndices,
						Principals:  oldBinding.Principals,
					}
					newBindings = append(newBindings, newBinding)
				}
			}

			// Bindings in a protocol.Realm must be in lexicographical order.
			// See go.chromium.org/luci/server/auth/service/protocol#Realm
			sortBindings(newBindings)
			newRealm := &protocol.Realm{
				Name:     realmName,
				Bindings: newBindings,
				Data:     oldRealm.GetData(),
			}
			result.Realms = append(result.Realms, newRealm)
		}
	}

	return result, nil
}

// /////////////////////////////////////////////////////////////////////
// ///////////////////// Sorting helper functions //////////////////////
// /////////////////////////////////////////////////////////////////////

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func sliceCompare[T string | uint32](sliceA []T, sliceB []T) bool {
	maxCommonIndex := min(len(sliceA), len(sliceB))
	for idx := range maxCommonIndex {
		if sliceA[idx] != sliceB[idx] {
			return sliceA[idx] < sliceB[idx]
		}
	}
	return len(sliceA) < len(sliceB)
}

func sortIndices(indices []uint32) {
	sort.Slice(indices, func(i, j int) bool {
		return indices[i] < indices[j]
	})
}

func sortBindings(bindings []*protocol.Binding) {
	// Sort by Permissions primarily. Fallback to comparing Conditions,
	// then Principals.
	sort.Slice(bindings, sortby.Chain{
		func(i, j int) bool {
			return sliceCompare(bindings[i].Permissions, bindings[j].Permissions)
		},
		func(i, j int) bool {
			return sliceCompare(bindings[i].Conditions, bindings[j].Conditions)
		},
		func(i, j int) bool {
			return sliceCompare(bindings[i].Principals, bindings[j].Principals)
		},
	}.Use)
}

///////////////////////////////////////////////////////////////////////
