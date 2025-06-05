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
	"go.chromium.org/luci/common/logging"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	lucirealms "go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/bqpb"
	customerrors "go.chromium.org/luci/auth_service/impl/errors"
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

// analyzeRealmsCfgRealms analyzes the given config for all realms.
func analyzeRealmsCfgRealms(ctx context.Context, cfg *realmsconf.RealmsCfg) []*bqpb.RealmSourceRow {
	realms := map[string][]*bqpb.RealmSourceRow{}
	for _, r := range cfg.GetRealms() {
		uniqueBindings := make(map[string]*bqpb.RealmSourceRow)

		// Explicit bindings defined in this realm.
		for _, b := range r.Bindings {
			row := &bqpb.RealmSourceRow{
				Name:       r.Name,
				Role:       b.Role,
				Source:     r.Name,
				Principals: b.Principals,
				Conditions: handleConditions(b.Conditions),
			}
			rowKey := realmSourceRowKey(row)
			if _, ok := uniqueBindings[rowKey]; !ok {
				uniqueBindings[rowKey] = row
			}
		}

		var parents []string
		// All realms inherit @root realms implicitly (except @root itself).
		if r.Name != lucirealms.RootRealm {
			parents = append(parents, lucirealms.RootRealm)
		}
		parents = append(parents, r.Extends...)

		for _, parent := range parents {
			parentBindings, ok := realms[parent]
			if !ok {
				logging.Warningf(ctx, "skipping realm extension - missing realm %q", parent)
				continue
			}
			for _, b := range parentBindings {
				// Copy the realm source row data, but associate it with this realm.
				row := &bqpb.RealmSourceRow{
					Name:       r.Name,
					Role:       b.Role,
					Source:     b.Source,
					Principals: b.Principals,
					Conditions: b.Conditions,
				}
				rowKey := realmSourceRowKey(row)
				if _, ok := uniqueBindings[rowKey]; !ok {
					uniqueBindings[rowKey] = row
				}
			}
		}

		realmBindings := make([]*bqpb.RealmSourceRow, 0, len(uniqueBindings))
		for _, u := range uniqueBindings {
			realmBindings = append(realmBindings, u)
		}
		realms[r.Name] = realmBindings
	}

	var rows []*bqpb.RealmSourceRow
	for _, realmRows := range realms {
		rows = append(rows, realmRows...)
	}
	return rows
}

// expandLatestRealms expands the latest realms configs so the source realm
// which defines each binding can be found.
func expandLatestRealms(ctx context.Context,
	latestRealms map[string]*ViewableConfig[*realmsconf.RealmsCfg],
	ts *timestamppb.Timestamp) []*bqpb.RealmSourceRow {
	var rows []*bqpb.RealmSourceRow
	for project, cfg := range latestRealms {
		projectRows := analyzeRealmsCfgRealms(ctx, cfg.Config)
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

	return rows
}
