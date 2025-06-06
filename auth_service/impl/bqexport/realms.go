// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	lucirealms "go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/bqpb"
	customerrors "go.chromium.org/luci/auth_service/impl/errors"
)

var (
	// ErrUnknownRealm is returned by RealmsGraph.GetRealmBindings if the given
	// realm name is not in the realms graph.
	ErrUnknownRealm = errors.New("unknown realm")
)

// parseRealms parses the realms from the given AuthDB to construct realm rows
// which can then be exported to BQ.
func parseRealms(ctx context.Context, authDB *protocol.AuthDB,
	authDBRev int64, ts *timestamppb.Timestamp) ([]*bqpb.RealmRow, error) {
	realms := authDB.Realms
	if realms == nil {
		return []*bqpb.RealmRow{}, customerrors.ErrAuthDBMissingRealms
	}
	sizeHint := len(realms.Realms)
	if sizeHint == 0 {
		// Log a warning because it is very unlikely the AuthDB has no realms.
		logging.Warningf(ctx, "no realms in AuthDB")
		return []*bqpb.RealmRow{}, nil
	}
	toPermissionNames := func(indices []uint32) []string {
		names := make([]string, len(indices))
		for i, permIndex := range indices {
			names[i] = realms.Permissions[permIndex].Name
		}
		return names
	}
	// Process each condition to the (attribute, value) pairs associated with it.
	conditions := make([][]string, len(realms.Conditions))
	for i, condition := range realms.Conditions {
		restriction := condition.GetRestrict()
		values := stringset.New(len(restriction.Values))
		for _, value := range restriction.Values {
			values.Add(fmt.Sprintf("%s==%s", restriction.Attribute, value))
		}
		conditions[i] = values.ToSortedSlice()
	}
	toConditionValues := func(indices []uint32) []string {
		// Set the initial capacity assuming every condition has at least one value.
		values := stringset.New(len(indices))
		for _, condIndex := range indices {
			values.AddAll(conditions[condIndex])
		}
		return values.ToSortedSlice()
	}
	// Set the initial capacity assuming every realm has at least one binding.
	realmRows := make([]*bqpb.RealmRow, 0, sizeHint)
	for _, realm := range realms.Realms {
		for i, binding := range realm.Bindings {
			realmRows = append(realmRows, &bqpb.RealmRow{
				Name:        realm.Name,
				BindingId:   int64(i),
				Permissions: toPermissionNames(binding.Permissions),
				Principals:  binding.Principals,
				Conditions:  toConditionValues(binding.Conditions),
				AuthdbRev:   authDBRev,
				ExportedAt:  ts,
			})
		}
	}
	return realmRows, nil
}

func handleConditions(conditions []*realmsconf.Condition) []string {
	out := make([]string, len(conditions))
	for i, c := range conditions {
		r := c.GetRestrict()
		values := stringset.NewFromSlice(r.GetValues()...)

		out[i] = fmt.Sprintf("%s==(%s)",
			r.GetAttribute(), strings.Join(values.ToSortedSlice(), "||"))
	}
	return out
}

func realmSourceRowKey(r *bqpb.RealmSourceRow) string {
	principals := strings.Join(stringset.NewFromSlice(r.Principals...).ToSortedSlice(), ",")
	conditions := strings.Join(stringset.NewFromSlice(r.Conditions...).ToSortedSlice(), "&&")
	return strings.Join([]string{r.Role, r.Source, principals, conditions}, "*")
}

// RealmNode represents a single realm in a realms config.
type RealmNode struct {
	Realm   *realmsconf.Realm
	Parents []*RealmNode

	DirectBindings []*bqpb.RealmSourceRow
}

// RealmsGraph represents all realms in a realms config.
type RealmsGraph struct {
	Realms map[string]*RealmNode
}

// NewRealmsGraph creates a new RealmsGraph based on the given config.
func NewRealmsGraph(ctx context.Context, cfg *realmsconf.RealmsCfg) *RealmsGraph {
	allRealms := cfg.GetRealms()
	rg := &RealmsGraph{
		Realms: make(map[string]*RealmNode, len(allRealms)),
	}
	rg.initializeNodes(ctx, allRealms)

	return rg
}

func (rg *RealmsGraph) initializeNodes(ctx context.Context, allRealms []*realmsconf.Realm) {
	for _, r := range allRealms {
		bindings := r.GetBindings()
		node := &RealmNode{
			Realm:          r,
			Parents:        make([]*RealmNode, 0, len(r.GetExtends())+1),
			DirectBindings: make([]*bqpb.RealmSourceRow, len(bindings)),
		}
		for i, b := range bindings {
			node.DirectBindings[i] = &bqpb.RealmSourceRow{
				Name:       r.Name,
				Role:       b.Role,
				Source:     r.Name,
				Principals: b.Principals,
				Conditions: handleConditions(b.Conditions),
			}
		}
		rg.Realms[r.GetName()] = node
	}

	// Populate parents.
	for _, child := range allRealms {
		childName := child.GetName()
		extends := child.GetExtends()
		parentNames := make([]string, 0, len(extends)+1)
		// All realms implicitly inherit from the @root realm (except for @root).
		if child.GetName() != lucirealms.RootRealm {
			parentNames = append(parentNames, lucirealms.RootRealm)
		}
		parentNames = append(parentNames, extends...)
		for _, parentName := range parentNames {
			parent, ok := rg.Realms[parentName]
			if !ok {
				logging.Warningf(ctx, "skipping %q realm extension - missing realm %q",
					childName, parentName)
				continue
			}
			rg.Realms[childName].Parents = append(rg.Realms[childName].Parents, parent)
		}
	}
}

func (rg *RealmsGraph) doRealmsExpansion(ctx context.Context, node *RealmNode, cache map[string][]*bqpb.RealmSourceRow) ([]*bqpb.RealmSourceRow, error) {
	name := node.Realm.Name

	// Check the cache first.
	if cachedResult, ok := cache[name]; ok {
		return cachedResult, nil
	}

	// Initialize this realm's bindings.
	uniqueBindings := make(map[string]*bqpb.RealmSourceRow, len(node.DirectBindings))
	for _, row := range node.DirectBindings {
		uniqueBindings[realmSourceRowKey(row)] = row
	}

	for _, parentNode := range node.Parents {
		parentBindings, err := rg.doRealmsExpansion(ctx, parentNode, cache)
		if err != nil {
			return nil, err
		}
		for _, row := range parentBindings {
			uniqueBindings[realmSourceRowKey(row)] = &bqpb.RealmSourceRow{
				Name:       name,
				Role:       row.Role,
				Source:     row.Source,
				Principals: row.Principals,
				Conditions: row.Conditions,
			}
		}
	}

	result := make([]*bqpb.RealmSourceRow, 0, len(uniqueBindings))
	for _, row := range uniqueBindings {
		result = append(result, row)
	}

	// Add to the cache.
	cache[name] = result
	return result, nil
}

// GetRealmBindings returns the direct and indirect realm bindings associated
// with the given realm.
func (rg *RealmsGraph) GetRealmBindings(ctx context.Context, name string, cache map[string][]*bqpb.RealmSourceRow) ([]*bqpb.RealmSourceRow, error) {
	root, ok := rg.Realms[name]
	if !ok {
		return nil, errors.Fmt("%q: %w", name, ErrUnknownRealm)
	}

	if cache == nil {
		cache = map[string][]*bqpb.RealmSourceRow{}
	}

	return rg.doRealmsExpansion(ctx, root, cache)
}

// analyzeRealmsCfgRealms analyzes the given config for all realms.
func analyzeRealmsCfgRealms(ctx context.Context, cfg *realmsconf.RealmsCfg) ([]*bqpb.RealmSourceRow, error) {
	rg := NewRealmsGraph(ctx, cfg)
	cache := make(map[string][]*bqpb.RealmSourceRow, len(rg.Realms))
	var rows []*bqpb.RealmSourceRow
	for realm := range rg.Realms {
		realmRows, err := rg.GetRealmBindings(ctx, realm, cache)
		if err != nil {
			return nil, err
		}
		rows = append(rows, realmRows...)
	}
	return rows, nil
}

// expandLatestRealms expands the latest realms configs so the source realm
// which defines each binding can be found.
func expandLatestRealms(ctx context.Context,
	latestRealms map[string]*ViewableConfig[*realmsconf.RealmsCfg],
	ts *timestamppb.Timestamp) ([]*bqpb.RealmSourceRow, error) {
	var rows []*bqpb.RealmSourceRow
	for project, cfg := range latestRealms {
		logging.Debugf(ctx, "analyzing realms for project %q", project)
		projectRows, err := analyzeRealmsCfgRealms(ctx, cfg.Config)
		if err != nil {
			return nil, err
		}
		// Change the realm names to the full format of <project>:<realm>, set the
		// common view URL, and the job-level export time.
		for _, row := range projectRows {
			row.Name = fmt.Sprintf("%s:%s", project, row.Name)
			row.Source = fmt.Sprintf("%s:%s", project, row.Source)
			row.Url = cfg.ViewURL
			row.ExportedAt = ts
		}
		rows = append(rows, projectRows...)
	}

	return rows, nil
}
