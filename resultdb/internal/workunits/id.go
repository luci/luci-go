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

// Returns the spanner primary key of this work unit.
func (id ID) key() spanner.Key {
	return spanner.Key{id.rootInvocationShardID(), id.WorkUnitID}
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

// Determine which root invocation shard this work unit is stored in.
// rootInvocationShardID is part of the primary key in the spanner table.
func (id ID) rootInvocationShardID() string {
	// Use %q instead of %s which convert string to escaped Go string literal.
	// If we ever let these invocation IDs use colons, this make sure the input to the hashing is unique
	hash := sha256.Sum256([]byte(fmt.Sprintf("%q:%q", id.RootInvocationID, id.WorkUnitID)))
	val := binary.BigEndian.Uint32(hash[:4])
	shardIdx := val % uint32(rootinvocations.RootInvocationShardCount)
	return rootinvocations.ComputeRootInvocationShardID(id.RootInvocationID, int(shardIdx))
}

// Return the resource name of a work unit.
func (id ID) Name() string {
	return pbutil.WorkUnitName(string(id.RootInvocationID), id.WorkUnitID)
}

// IDFromRowID converts a Spanner-level row ID to an ID.
func IDFromRowID(rootInvocationShardID string, workUnitID string) ID {
	parts := strings.SplitN(rootInvocationShardID, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		panic(fmt.Sprintf("invalid rootInvocationShardID format: %q", rootInvocationShardID))
	}
	return ID{
		RootInvocationID: rootinvocations.ID(parts[1]),
		WorkUnitID:       workUnitID,
	}
}
