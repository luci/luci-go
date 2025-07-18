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

// Package rootinvocations defines functions to interact with spanner.
package rootinvocations

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/resultdb/internal/spanutil"
)

const (
	// RootInvocationShardCount is the fixed number of shards per Root Invocation
	// for RootInvocationShards. As per the schema, this is currently 16.
	RootInvocationShardCount = 16

	// The number of hash prefix bytes on the root invocation shard ID.
	shardIDHashPrefixBytes = 4
)

// Represents the identifier of a root invocation-shard. Each shard contains
// a portion of the root invocation's work units, test results, test exonerations
// and artifacts.
//
// Each such entity (e.g. work units, test results) determines the placement
// of which shards results should be written to, based its particular requirements
// (including need to optimise write throughput by spreading data over many shards
// while in any given transaction not using too many transaction participants).
type ShardID struct {
	RootInvocationID ID
	ShardIndex       int
}

// ToSpanner implements spanutil.Value.
func (id ShardID) ToSpanner() any {
	return id.RowID()
}

// SpannerPtr implements spanutil.Ptr.
func (id *ShardID) SpannerPtr(b *spanutil.Buffer) any {
	return &b.NullString
}

// FromSpanner implements spanutil.Ptr.
func (id *ShardID) FromSpanner(b *spanutil.Buffer) error {
	*id = ShardID{}
	if b.NullString.Valid {
		*id = ShardIDFromRowID(b.NullString.StringVal)
	}
	return nil
}

// ShardIDFromRowID converts a Spanner-level row ID to a ShardID.
func ShardIDFromRowID(rowID string) ShardID {
	prefix, suffix := splitHashPrefix(rowID)
	return ShardID{
		RootInvocationID: ID(suffix),
		ShardIndex:       shardIndexFromHashedPrefix(prefix),
	}
}

// RowID returns the root invocation shard ID used in Spanner rows.
// If id is empty, returns "".
func (id ShardID) RowID() string {
	if id.ShardIndex < 0 || id.ShardIndex >= RootInvocationShardCount {
		panic(fmt.Sprintf("ShardIndex %d out of range [0, %d)", id.ShardIndex, RootInvocationShardCount))
	}
	if id.RootInvocationID == "" {
		return ""
	}

	// shard_step is (2^32 / N)
	const shardStep = uint32(1 << 32 / RootInvocationShardCount)

	// shard_base is Mod(ToIntBigEndian(sha256(invocation_id)[:4]), shard_step).
	h := sha256.Sum256([]byte(string(id.RootInvocationID)))
	hashPrefixAsInt := binary.BigEndian.Uint32(h[:4])
	shardBase := hashPrefixAsInt % shardStep

	// shard_key = shard_base + shard_index * shard_step
	shardKey := shardBase + uint32(id.ShardIndex)*shardStep

	// Format the 4-byte shardKey as an 8-character hex string.
	var shardKeyBytes [shardIDHashPrefixBytes]byte
	binary.BigEndian.PutUint32(shardKeyBytes[:], shardKey)

	// Final Format: "${hex(shard_key)}:${user_provided_invocation_id}"
	return fmt.Sprintf("%x:%s", shardKeyBytes, string(id.RootInvocationID))
}

// Key returns a root invocation shard spanner key.
func (id ShardID) Key(suffix ...any) spanner.Key {
	ret := make(spanner.Key, 1+len(suffix))
	ret[0] = id.RowID()
	copy(ret[1:], suffix)
	return ret
}

func splitHashPrefix(s string) (prefix, suffix string) {
	expectedPrefixLen := hex.EncodedLen(shardIDHashPrefixBytes) + 1 // +1 for separator
	if len(s) < expectedPrefixLen {
		panic(fmt.Sprintf("%q is too short", s))
	}
	return s[:expectedPrefixLen-1], s[expectedPrefixLen:]
}

// shardIndexFromHashedPrefix extracts the shard index from a hashed prefix.
func shardIndexFromHashedPrefix(hashedPrefix string) int {
	decoded, err := hex.DecodeString(hashedPrefix)
	if err != nil {
		panic(fmt.Sprintf("invalid hex string as key prefix %q: %s", hashedPrefix, err))
	}
	if len(decoded) != shardIDHashPrefixBytes {
		panic(fmt.Sprintf("invalid hashed prefix length %d, expected %d", len(decoded), shardIDHashPrefixBytes))
	}
	val := binary.BigEndian.Uint32(decoded)

	// shard_step is (2^32 / N)
	const shardStep = uint32(1 << 32 / RootInvocationShardCount)

	// We reverse the shard key calculation. We know:
	// - shard_key = shard_base + shard_index * shard_step
	// - shard_base is [0,shard_step)
	// Thus:
	// shard_index = shard_key / shard_step.
	result := int(val / shardStep)
	if result >= RootInvocationShardCount || result < 0 {
		// This indicates a logic error, this should not be possible.
		panic(fmt.Sprintf("logic error: obtained shard index %d from hash prefix %s", result, hashedPrefix))
	}
	return result
}

// Represents a set of Shard IDs.
type ShardIDSet map[ShardID]struct{}

// NewShardIDSet creates an ShardIDSet from members.
func NewShardIDSet(ids ...ShardID) ShardIDSet {
	ret := make(ShardIDSet, len(ids))
	for _, id := range ids {
		ret.Add(id)
	}
	return ret
}

// Add adds id to the set.
func (s ShardIDSet) Add(id ShardID) {
	s[id] = struct{}{}
}

// ToSpanner implements span.Value.
func (s ShardIDSet) ToSpanner() any {
	ret := make([]string, 0, len(s))
	for id := range s {
		ret = append(ret, id.RowID())
	}
	sort.Strings(ret)
	return ret
}
