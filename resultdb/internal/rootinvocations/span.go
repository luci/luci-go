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
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// rootInvocationShardCount is the fixed number of shards per Root Invocation
	// for RootInvocationShards. As per the schema, this is currently 16.
	rootInvocationShardCount = 16

	// secondaryIndexShardCount is the number of values for the SecondaryIndexShardId column in the RootInvocations table
	// used for preventing hotspots in global secondary indexes. The range of values is [0, secondaryIndexShardCount-1].
	secondaryIndexShardCount = 100
)

// Create creates mutations of the following records.
//   - a new root invocation record,
//   - the corresponding legacy invocation record
//   - sharding records in RootInvocationShards
func Create(rootInvocation *RootInvocationRow) []*spanner.Mutation {
	if rootInvocation.RootInvocationId == "" {
		panic("do not create root invocations with empty id")
	}
	if rootInvocation.State != pb.RootInvocation_FINALIZING && rootInvocation.State != pb.RootInvocation_ACTIVE {
		// validateCreateInvocationRequest should have rejected any other states.
		panic("do not create root invocations in states other than active or finalizing")
	}
	if rootInvocation.Realm == "" {
		panic("do not create root invocations with empty realm")
	}
	if rootInvocation.CreatedBy == "" {
		panic("do not create root invocations with empty creator")
	}
	if rootInvocation.Deadline.IsZero() {
		panic("do not create root invocations with empty deadline")
	}
	if !rootInvocation.UninterestingTestVerdictsExpirationTime.Valid {
		panic("do not create root invocations with empty UninterestingTestVerdictsExpirationTime")

	}
	rootInvocation.Normalize()
	// Create mutation for the RootInvocations table record.
	rootInvMutation := rootInvocation.toMutation()

	// Create mutation for the legacy invocation table record.
	invMutation := rootInvocation.toLegacyInvocationMutation()

	// Create mutation for the RootInvocationShards table records.
	shardMutations := rootInvocation.toShardsMutations()

	mutations := []*spanner.Mutation{rootInvMutation, invMutation}
	mutations = append(mutations, shardMutations...)
	return mutations
}

// RootInvocationRow corresponds to the schema of the RootInvocations Spanner table.
// The values for the output only fields are ignored during writing.
type RootInvocationRow struct {
	RootInvocationId                        ID
	SecondaryIndexShardId                   int64 // Output only.
	State                                   pb.RootInvocation_State
	Realm                                   string
	CreateTime                              time.Time // Output only.
	CreatedBy                               string
	FinalizeStartTime                       spanner.NullTime // Output only.
	FinalizeTime                            spanner.NullTime // Output only.
	Deadline                                time.Time
	UninterestingTestVerdictsExpirationTime spanner.NullTime
	CreateRequestId                         string
	ProducerResource                        string
	Tags                                    []*pb.StringPair
	Properties                              *structpb.Struct
	Sources                                 *pb.Sources
	IsSourcesFinal                          bool
	BaselineId                              string
	Submitted                               bool
}

// Convert the root invocation row to the canonical form.
func (r *RootInvocationRow) Normalize() {
	pbutil.SortStringPairs(r.Tags)

	changelists := r.Sources.GetChangelists()
	pbutil.SortGerritChanges(changelists)
}

func (r *RootInvocationRow) toMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"RootInvocationId":      r.RootInvocationId,
		"SecondaryIndexShardId": r.RootInvocationId.shardID(secondaryIndexShardCount),
		"State":                 r.State,
		"Realm":                 r.Realm,
		"CreateTime":            spanner.CommitTimestamp,
		"CreatedBy":             r.CreatedBy,
		"Deadline":              r.Deadline,
		"UninterestingTestVerdictsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateRequestId":                         r.CreateRequestId,
		"ProducerResource":                        r.ProducerResource,
		"Tags":                                    r.Tags,
		"Properties":                              spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"Sources":                                 spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourcesFinal":                          r.IsSourcesFinal,
		"BaselineId":                              r.BaselineId,
		"Submitted":                               r.Submitted,
	}

	if r.State == pb.RootInvocation_FINALIZING {
		// Invocation immediately transitioning to finalizing.
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("RootInvocations", row)
}

func (r *RootInvocationRow) toLegacyInvocationMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"InvocationId":                      invocations.ID(fmt.Sprintf("root:%s", r.RootInvocationId)),
		"Type":                              invocations.Root,
		"ShardId":                           r.RootInvocationId.shardID(invocations.Shards),
		"State":                             r.State,
		"Realm":                             r.Realm,
		"InvocationExpirationTime":          time.Unix(0, 0), // unused field, but spanner schema enforce it to be not null.
		"ExpectedTestResultsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateTime":                        spanner.CommitTimestamp,
		"CreatedBy":                         r.CreatedBy,
		"Deadline":                          r.Deadline,
		"Tags":                              r.Tags,
		"CreateRequestId":                   r.CreateRequestId,
		"ProducerResource":                  r.ProducerResource,
		"Properties":                        spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"InheritSources":                    spanner.NullBool{Bool: false, Valid: true}, // A root invocation defines its own sources.
		"Sources":                           spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourceSpecFinal":                 r.IsSourcesFinal,
		"IsExportRoot":                      spanner.NullBool{Bool: true, Valid: true}, // Root invocations are always export roots.
		"BaselineId":                        r.BaselineId,
		"Submitted":                         r.Submitted,
	}

	if r.State == pb.RootInvocation_FINALIZING {
		// Invocation immediately transitioning to finalizing.
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("Invocations", row)
}

func (r *RootInvocationRow) toShardsMutations() []*spanner.Mutation {
	mutations := make([]*spanner.Mutation, rootInvocationShardCount)
	for i := 0; i < rootInvocationShardCount; i++ {
		row := map[string]interface{}{
			"RootInvocationShardId": computeRootInvocationShardID(r.RootInvocationId, i),
			"ShardIndex":            i,
			"RootInvocationId":      r.RootInvocationId,
			"CreateTime":            spanner.CommitTimestamp,
		}
		mutations[i] = spanutil.InsertMap("RootInvocationShards", row)
	}
	return mutations
}

// computeRootInvocationShardID implements the shard ID generation logic
// described in the RootInvocationShards schema.
func computeRootInvocationShardID(id ID, shardIndex int) string {
	if shardIndex < 0 || shardIndex >= rootInvocationShardCount {
		panic(fmt.Sprintf("shardIndex %d out of range [0, %d)", shardIndex, rootInvocationShardCount))
	}

	// shard_step is (2^32 / N)
	const shardStep = uint32(1 << 32 / rootInvocationShardCount)

	// shard_base is Mod(ToIntBigEndian(sha256(invocation_id)[:4]), shard_step).
	h := sha256.Sum256([]byte(id))
	hashPrefixAsInt := binary.BigEndian.Uint32(h[:4])
	shardBase := hashPrefixAsInt % shardStep

	// shard_key = shard_base + shard_index * shard_step
	shardKey := shardBase + uint32(shardIndex)*shardStep

	// Format the 4-byte shardKey as an 8-character hex string.
	var shardKeyBytes [4]byte
	binary.BigEndian.PutUint32(shardKeyBytes[:], shardKey)

	// Final Format: "${hex(shard_key)}:${user_provided_invocation_id}"
	return fmt.Sprintf("%x:%s", shardKeyBytes, id)
}
