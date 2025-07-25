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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// secondaryIndexShardCount is the number of values for the SecondaryIndexShardId column in the RootInvocations table
	// used for preventing hotspots in global secondary indexes. The range of values is [0, secondaryIndexShardCount-1].
	secondaryIndexShardCount = 100
)

// Create creates mutations of the following records.
//   - a new root invocation record,
//   - the corresponding legacy invocation record
//   - sharding records in RootInvocationShards
func Create(rootInvocation *RootInvocationRow) []*spanner.Mutation {
	if rootInvocation.RootInvocationID == "" {
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
	RootInvocationID                        ID
	SecondaryIndexShardID                   int64 // Output only.
	State                                   pb.RootInvocation_State
	Realm                                   string
	CreateTime                              time.Time // Output only.
	CreatedBy                               string
	FinalizeStartTime                       spanner.NullTime // Output only.
	FinalizeTime                            spanner.NullTime // Output only.
	Deadline                                time.Time
	UninterestingTestVerdictsExpirationTime spanner.NullTime
	CreateRequestID                         string
	ProducerResource                        string
	Tags                                    []*pb.StringPair
	Properties                              *structpb.Struct
	Sources                                 *pb.Sources
	IsSourcesFinal                          bool
	BaselineID                              string
	Submitted                               bool
}

// Clone makes a deep copy of the row.
func (r *RootInvocationRow) Clone() *RootInvocationRow {
	ret := *r
	if r.Tags != nil {
		ret.Tags = make([]*pb.StringPair, len(r.Tags))
		for i, tp := range r.Tags {
			ret.Tags[i] = proto.Clone(tp).(*pb.StringPair)
		}
	}
	if r.Properties != nil {
		ret.Properties = proto.Clone(r.Properties).(*structpb.Struct)
	}
	if r.Sources != nil {
		ret.Sources = proto.Clone(r.Sources).(*pb.Sources)
	}
	return &ret
}

// Convert the root invocation row to the canonical form.
func (r *RootInvocationRow) Normalize() {
	pbutil.SortStringPairs(r.Tags)

	changelists := r.Sources.GetChangelists()
	pbutil.SortGerritChanges(changelists)
}

func (r *RootInvocationRow) toMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"RootInvocationId":      r.RootInvocationID,
		"SecondaryIndexShardId": r.RootInvocationID.shardID(secondaryIndexShardCount),
		"State":                 r.State,
		"Realm":                 r.Realm,
		"CreateTime":            spanner.CommitTimestamp,
		"CreatedBy":             r.CreatedBy,
		"Deadline":              r.Deadline,
		"UninterestingTestVerdictsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateRequestId":                         r.CreateRequestID,
		"ProducerResource":                        r.ProducerResource,
		"Tags":                                    r.Tags,
		"Properties":                              spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"Sources":                                 spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourcesFinal":                          r.IsSourcesFinal,
		"BaselineId":                              r.BaselineID,
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
		"InvocationId":                      r.RootInvocationID.LegacyInvocationID(),
		"Type":                              invocations.Root,
		"ShardId":                           r.RootInvocationID.shardID(invocations.Shards),
		"State":                             r.State,
		"Realm":                             r.Realm,
		"InvocationExpirationTime":          time.Unix(0, 0), // unused field, but spanner schema enforce it to be not null.
		"ExpectedTestResultsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateTime":                        spanner.CommitTimestamp,
		"CreatedBy":                         r.CreatedBy,
		"Deadline":                          r.Deadline,
		"Tags":                              r.Tags,
		"CreateRequestId":                   r.CreateRequestID,
		"ProducerResource":                  r.ProducerResource,
		"Properties":                        spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"InheritSources":                    spanner.NullBool{Bool: false, Valid: true}, // A root invocation defines its own sources.
		"Sources":                           spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourceSpecFinal":                 r.IsSourcesFinal,
		"IsExportRoot":                      spanner.NullBool{Bool: true, Valid: true}, // Root invocations are always export roots.
		"BaselineId":                        r.BaselineID,
		"Submitted":                         r.Submitted,
	}

	if r.State == pb.RootInvocation_FINALIZING {
		// Invocation immediately transitioning to finalizing.
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("Invocations", row)
}

func (r *RootInvocationRow) toShardsMutations() []*spanner.Mutation {
	mutations := make([]*spanner.Mutation, RootInvocationShardCount)
	for i := 0; i < RootInvocationShardCount; i++ {
		row := map[string]interface{}{
			"RootInvocationShardId": ShardID{RootInvocationID: r.RootInvocationID, ShardIndex: i},
			"ShardIndex":            i,
			"RootInvocationId":      r.RootInvocationID,
			"State":                 r.State,
			"Realm":                 r.Realm,
			"CreateTime":            spanner.CommitTimestamp,
			"Sources":               spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
			"IsSourcesFinal":        r.IsSourcesFinal,
		}
		mutations[i] = spanutil.InsertMap("RootInvocationShards", row)
	}
	return mutations
}

func (r *RootInvocationRow) ToProto() *pb.RootInvocation {
	result := &pb.RootInvocation{
		Name:             r.RootInvocationID.Name(),
		RootInvocationId: string(r.RootInvocationID),
		State:            r.State,
		Realm:            r.Realm,
		CreateTime:       pbutil.MustTimestampProto(r.CreateTime),
		Creator:          r.CreatedBy,
		Deadline:         pbutil.MustTimestampProto(r.Deadline),
		ProducerResource: r.ProducerResource,
		Sources:          r.Sources,
		SourcesFinal:     r.IsSourcesFinal,
		Tags:             r.Tags,
		Properties:       r.Properties,
		BaselineId:       r.BaselineID,
	}
	if r.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(r.FinalizeStartTime.Time)
	}
	if r.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(r.FinalizeTime.Time)
	}
	return result
}

// MarkFinalizing creates mutations to mark the given root invocation as finalizing.
// The caller MUST check the root invocation is currently in ACTIVE state, or this
// may incorrectly overwrite the FinalizeStartTime.
func MarkFinalizing(id ID) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 2+RootInvocationShardCount)
	ms = append(ms, spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId":  id,
		"State":             pb.RootInvocation_FINALIZING,
		"FinalizeStartTime": spanner.CommitTimestamp,
	}))

	for i := 0; i < RootInvocationShardCount; i++ {
		ms = append(ms, spanutil.UpdateMap("RootInvocationShards", map[string]any{
			"RootInvocationShardId": ShardID{RootInvocationID: id, ShardIndex: i},
			"State":                 pb.RootInvocation_FINALIZING,
		}))
	}
	ms = append(ms, invocations.MarkFinalizing(id.LegacyInvocationID()))
	return ms
}

// MarkFinalized creates a mutation to mark the given root invocation as finalized.
// The caller MUST check the root invocation is currently in FINALIZING state, or this
// may incorrectly overwrite the FinalizeTime.
func MarkFinalized(id ID) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 2+RootInvocationShardCount)
	ms = append(ms, spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId": id,
		"State":            pb.RootInvocation_FINALIZED,
		"FinalizeTime":     spanner.CommitTimestamp,
	}))

	for i := 0; i < RootInvocationShardCount; i++ {
		ms = append(ms, spanutil.UpdateMap("RootInvocationShards", map[string]any{
			"RootInvocationShardId": ShardID{RootInvocationID: id, ShardIndex: i},
			"State":                 pb.RootInvocation_FINALIZED,
		}))
	}
	ms = append(ms, invocations.MarkFinalized(id.LegacyInvocationID()))
	return ms
}
