// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//  http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package rootinvocations defines functions to interact with spanner.
package workunits

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
)

// Represents a fully-qualified work unit ID.
type ID struct {
	RootInvocationID rootinvocations.ID
	WorkUnitID       string
}

// Returns the spanner primary Key of this work unit.
func (id ID) Key() spanner.Key {
	return spanner.Key{id.RootInvocationShardID().RowID(), id.WorkUnitID}
}

// shardID returns a value in [0,shardCount) deterministically based on the ID value.
func (id ID) shardID(shardCount int) int64 {
	// Use %q instead of %s which convert string to escaped Go string literal.
	// If we ever let these invocation IDs use colons, this make sure the input to the hashing is unique.
	return spanutil.ShardID(fmt.Sprintf("%q:%q", id.RootInvocationID, id.WorkUnitID), shardCount)
}

// Returns the corresponding Invocation ID of the work unit in the legacy invocation table.
func (id ID) LegacyInvocationID() invocations.ID {
	legacyInvocationID := fmt.Sprintf("workunit:%s:%s", id.RootInvocationID, id.WorkUnitID)
	return invocations.ID(legacyInvocationID)
}

// RootInvocationShardID returns the identifier of the root invocation shard this work unit
// is stored in. rootInvocationShardID is part of the primary key in the spanner table.
func (id ID) RootInvocationShardID() rootinvocations.ShardID {
	// Use %q instead of %s which convert string to escaped Go string literal.
	// If we ever let these invocation IDs use colons, this make sure the input to the hashing is unique.
	hash := sha256.Sum256([]byte(fmt.Sprintf("%q:%q", id.RootInvocationID, id.WorkUnitID)))
	val := binary.BigEndian.Uint32(hash[:4])
	shardIdx := val % uint32(rootinvocations.RootInvocationShardCount)
	return rootinvocations.ShardID{
		RootInvocationID: id.RootInvocationID,
		ShardIndex:       int(shardIdx),
	}
}

// Return the resource name of a work unit.
func (id ID) Name() string {
	return pbutil.WorkUnitName(string(id.RootInvocationID), id.WorkUnitID)
}

// IDFromRowID converts a Spanner-level row ID to an ID.
func IDFromRowID(rootInvocationShardID string, workUnitID string) ID {
	shardID := rootinvocations.ShardIDFromRowID(rootInvocationShardID)
	return ID{
		RootInvocationID: shardID.RootInvocationID,
		WorkUnitID:       workUnitID,
	}
}

// MustParseLegacyInvocationID parses the ID of an invocation representing
// the shadow record for a work unit into the ID of the work unit it is
// shadowing.
//
// If the invocation ID does not correspond to a work unit, it panics.
func MustParseLegacyInvocationID(id invocations.ID) ID {
	// The work unit ID may have a colon embedded in it, we do not want to
	// split this.
	parts := strings.SplitN(string(id), ":", 3)
	if len(parts) != 3 || parts[0] != "workunit" {
		panic(fmt.Sprintf("not a legacy invocation for a work unit: %q", id))
	}

	return ID{
		RootInvocationID: rootinvocations.ID(parts[1]),
		WorkUnitID:       parts[2],
	}
}

// ParseName parses a work unit resource name into its ID.
// An error is returned if parsing fails.
func ParseName(name string) (ID, error) {
	rootInvocationID, workUnitID, err := pbutil.ParseWorkUnitName(name)
	if err != nil {
		return ID{}, err
	}
	return ID{
		RootInvocationID: rootinvocations.ID(rootInvocationID),
		WorkUnitID:       workUnitID,
	}, nil
}

// MustParseName parses a work unit resource name into its ID.
// If the resource name is not valid, it panics.
func MustParseName(name string) ID {
	id, err := ParseName(name)
	if err != nil {
		panic(err)
	}
	return id
}

// IDSet is an unordered set of work unit ids.
type IDSet map[ID]struct{}

// NewIDSet creates an IDSet from members.
func NewIDSet(ids ...ID) IDSet {
	ret := make(IDSet, len(ids))
	for _, id := range ids {
		ret.Add(id)
	}
	return ret
}

// Add adds id to the set.
func (s IDSet) Add(id ID) {
	s[id] = struct{}{}
}

// Has returns true if id is in the set.
func (s IDSet) Has(id ID) bool {
	_, ok := s[id]
	return ok
}

// RemoveAll removes any ids present in other.
func (s IDSet) RemoveAll(other IDSet) {
	if len(s) > 0 {
		for id := range other {
			s.Remove(id)
		}
	}
}

// Remove removes id from the set if it was present.
func (s IDSet) Remove(id ID) {
	delete(s, id)
}

// SortedByRowID returns IDs in the set sorted by row id.
func (s IDSet) SortedByRowID() []ID {
	shardIDs := make(map[ID]string)
	for id := range s {
		shardIDs[id] = id.RootInvocationShardID().RowID()
	}

	ids := make([]ID, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if shardIDs[ids[i]] != shardIDs[ids[j]] {
			return shardIDs[ids[i]] < shardIDs[ids[j]]
		}
		return ids[i].WorkUnitID < ids[j].WorkUnitID
	})
	return ids
}

// SortedByID returns IDs in the set sorted by logical id.
func (s IDSet) SortedByID() []ID {
	ids := make([]ID, 0, len(s))
	for id := range s {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if ids[i].RootInvocationID != ids[j].RootInvocationID {
			return ids[i].RootInvocationID < ids[j].RootInvocationID
		}
		return ids[i].WorkUnitID < ids[j].WorkUnitID
	})
	return ids
}
