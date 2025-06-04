// Copyright 2025 The LUCI Authors.
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

package bqexport

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/bqpb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/impl/model/graph"
)

// expandGroups returns the expanded version of all groups in the given AuthDB.
// All members, globs and nested subgroups included in a group (either directly
// or indirectly) are specified for each group.
func expandGroups(ctx context.Context, authDB *protocol.AuthDB, authDBRev int64,
	ts *timestamppb.Timestamp) ([]*bqpb.GroupRow, error) {
	sizeHint := len(authDB.Groups)

	if sizeHint == 0 {
		// Log a warning because it is very unlikely the AuthDB has no groups.
		logging.Warningf(ctx, "no groups in AuthDB")
		return []*bqpb.GroupRow{}, nil
	}

	groups := make([]model.GraphableGroup, sizeHint)
	directMemberships := make(map[string][]string, sizeHint)
	for i, group := range authDB.Groups {
		groups[i] = model.GraphableGroup(group)
		directMemberships[group.Name] = group.Members
	}
	groupsGraph := graph.NewGraph(groups)

	// Get all group names, including missing groups.
	names := groupsGraph.GroupNames()

	sizeHint = len(names)
	cache := &graph.ExpansionCache{
		Groups: make(map[string]*graph.ExpandedGroup, sizeHint),
	}
	result := make([]*bqpb.GroupRow, sizeHint)
	for i, name := range names {
		expanded, err := groupsGraph.GetExpandedGroup(ctx, name, true, cache)
		if err != nil {
			return nil, errors.Fmt("failed to expand group %q: %w", name, err)
		}
		var directMembers []string
		if dm, ok := directMemberships[name]; ok {
			directMembers = dm
		}
		result[i] = &bqpb.GroupRow{
			Name:          expanded.Name,
			Description:   expanded.Description,
			Owners:        expanded.Owners,
			Members:       expanded.Members.ToSortedSlice(),
			Globs:         expanded.Globs.ToSortedSlice(),
			Subgroups:     expanded.Nested.ToSortedSlice(),
			AuthdbRev:     authDBRev,
			ExportedAt:    ts,
			Missing:       expanded.Missing,
			DirectMembers: directMembers,
		}
	}

	return result, nil
}
