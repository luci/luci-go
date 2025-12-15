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
	"fmt"
	"maps"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

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

// Create returns mutations to create the following records.
//   - a new root invocation record,
//   - the corresponding legacy invocation record
//   - sharding records in RootInvocationShards
func Create(rootInvocation *RootInvocationRow) []*spanner.Mutation {
	if rootInvocation.RootInvocationID == "" {
		panic("do not create root invocations with empty id")
	}
	if rootInvocation.FinalizationState != pb.RootInvocation_FINALIZING && rootInvocation.FinalizationState != pb.RootInvocation_ACTIVE {
		// validateCreateInvocationRequest should have rejected any other states.
		panic("do not create root invocations in states other than active or finalizing")
	}
	if rootInvocation.Realm == "" {
		panic("do not create root invocations with empty realm")
	}
	if rootInvocation.CreatedBy == "" {
		panic("do not create root invocations with empty creator")
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
	FinalizationState                       pb.RootInvocation_FinalizationState
	State                                   pb.RootInvocation_State
	SummaryMarkdown                         string
	Realm                                   string
	CreateTime                              time.Time // Output only.
	CreatedBy                               string    // Output only.
	LastUpdated                             time.Time
	FinalizeStartTime                       spanner.NullTime // Output only.
	FinalizeTime                            spanner.NullTime // Output only.
	UninterestingTestVerdictsExpirationTime spanner.NullTime
	CreateRequestID                         string
	ProducerResource                        *pb.ProducerResource
	Definition                              *pb.RootInvocationDefinition
	Sources                                 *pb.Sources
	PrimaryBuild                            *pb.BuildDescriptor
	ExtraBuilds                             []*pb.BuildDescriptor
	Tags                                    []*pb.StringPair
	Properties                              *structpb.Struct
	BaselineID                              string
	StreamingExportState                    pb.RootInvocation_StreamingExportState
	Submitted                               bool
	FinalizerPending                        bool
	FinalizerSequence                       int64
	TestShardingAlgorithm                   TestShardingAlgorithmID
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
		"FinalizationState":     r.FinalizationState,
		"State":                 r.State,
		"SummaryMarkdown":       r.SummaryMarkdown,
		"Realm":                 r.Realm,
		"CreateTime":            spanner.CommitTimestamp,
		"CreatedBy":             r.CreatedBy,
		"LastUpdated":           spanner.CommitTimestamp,
		"UninterestingTestVerdictsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateRequestId":                         r.CreateRequestID,
		"ProducerResource":                        spanutil.Compressed(pbutil.MustMarshal(pbutil.RemoveProducerResourceOutputOnlyFields(r.ProducerResource))),
		"Sources":                                 spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"PrimaryBuild":                            spanutil.Compressed(pbutil.MustMarshal(removeBuildDescriptorOutputOnlyFields(r.PrimaryBuild))),
		"ExtraBuilds":                             serializeExtraBuilds(r.ExtraBuilds),
		"Tags":                                    r.Tags,
		"Properties":                              spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"StreamingExportState":                    r.StreamingExportState,
		"BaselineId":                              r.BaselineID,
		"Submitted":                               r.Submitted,
		"FinalizerPending":                        r.FinalizerPending,
		"FinalizerSequence":                       r.FinalizerSequence,
		"TestShardingAlgorithm":                   string(r.TestShardingAlgorithm),
	}
	if r.Definition != nil {
		row["DefinitionSystem"] = r.Definition.System
		row["DefinitionName"] = r.Definition.Name
		row["DefinitionProperties"] = r.Definition.Properties
	}

	if r.FinalizationState == pb.RootInvocation_FINALIZING {
		// Invocation immediately transitioning to finalizing.
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("RootInvocations", row)
}

func removeBuildDescriptorOutputOnlyFields(primaryBuild *pb.BuildDescriptor) *pb.BuildDescriptor {
	if primaryBuild == nil {
		return nil
	}
	// The URL field is output only and should not be stored in Spanner.
	// Rather, it should be computed based on the current service configuration
	// whenever it is returned.
	result := proto.Clone(primaryBuild).(*pb.BuildDescriptor)
	result.Url = ""
	return result
}

// serializeExtraBuilds serializes a slice of `*pb.BuildDescriptor`s into
// a slice of compressed bytes.
func serializeExtraBuilds(extraBuilds []*pb.BuildDescriptor) [][]byte {
	if len(extraBuilds) == 0 {
		// Write an empty slice instead of nil to avoid writing NULL to the database.
		return make([][]byte, 0)
	}

	result := make([][]byte, len(extraBuilds))
	for i, b := range extraBuilds {
		result[i] = spanutil.Compress(pbutil.MustMarshal(removeBuildDescriptorOutputOnlyFields(b)))
	}
	return result
}

// deserializeExtraBuilds deserializes a slice of compressed bytes into a slice of
// `*pb.BuildDescriptor`s.
func deserializeExtraBuilds(extraBuildsBytes [][]byte) ([]*pb.BuildDescriptor, error) {
	if len(extraBuildsBytes) == 0 {
		return nil, nil
	}

	result := make([]*pb.BuildDescriptor, 0, len(extraBuildsBytes))
	var decompressBuf []byte
	for i, b := range extraBuildsBytes {
		build := &pb.BuildDescriptor{}
		var err error
		decompressBuf, err = spanutil.Decompress(b, decompressBuf)
		if err != nil {
			return nil, errors.Fmt("decompress build descriptor: extra_builds[%v]: %w", i, err)
		}
		if err := proto.Unmarshal(decompressBuf, build); err != nil {
			return nil, errors.Fmt("unmarshal build descriptor: extra_builds[%v]: %w", i, err)
		}
		result = append(result, build)
	}
	return result, nil
}

func (r *RootInvocationRow) toLegacyInvocationMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"InvocationId":                      r.RootInvocationID.LegacyInvocationID(),
		"Type":                              invocations.Root,
		"ShardId":                           r.RootInvocationID.shardID(invocations.Shards),
		"State":                             toInvocationState(r.FinalizationState),
		"Realm":                             r.Realm,
		"InvocationExpirationTime":          time.Unix(0, 0), // unused field, but spanner schema enforce it to be not null.
		"ExpectedTestResultsExpirationTime": r.UninterestingTestVerdictsExpirationTime,
		"CreateTime":                        spanner.CommitTimestamp,
		"CreatedBy":                         r.CreatedBy,
		// set far into the future to avoid bothering the legacy deadline enforcer.
		"Deadline":          time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC),
		"Tags":              r.Tags,
		"CreateRequestId":   r.CreateRequestID,
		"Properties":        spanutil.Compressed(pbutil.MustMarshal(r.Properties)),
		"InheritSources":    spanner.NullBool{Bool: false, Valid: true}, // A root invocation defines its own sources.
		"Sources":           spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
		"IsSourceSpecFinal": r.StreamingExportState == pb.RootInvocation_METADATA_FINAL,
		"IsExportRoot":      spanner.NullBool{Bool: true, Valid: true}, // Root invocations are always export roots.
		"BaselineId":        r.BaselineID,
		"Submitted":         r.Submitted,
	}

	if r.FinalizationState == pb.RootInvocation_FINALIZING {
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
			"Realm":                 r.Realm,
			"CreateTime":            spanner.CommitTimestamp,
			"Sources":               spanutil.Compressed(pbutil.MustMarshal(r.Sources)),
			"TestShardingAlgorithm": string(r.TestShardingAlgorithm),
		}
		mutations[i] = spanutil.InsertMap("RootInvocationShards", row)
	}
	return mutations
}

func toInvocationState(finalizationState pb.RootInvocation_FinalizationState) pb.Invocation_State {
	switch finalizationState {
	case pb.RootInvocation_ACTIVE:
		return pb.Invocation_ACTIVE
	case pb.RootInvocation_FINALIZING:
		return pb.Invocation_FINALIZING
	case pb.RootInvocation_FINALIZED:
		return pb.Invocation_FINALIZED
	default:
		panic(fmt.Sprintf("unknown root invocation state %s", finalizationState))
	}
}

// ToLegacyInvocationProto returns the expected legacy invocation representation.
// For testing only, this should not be returned over any API surface.
func (r *RootInvocationRow) ToLegacyInvocationProto() *pb.Invocation {
	var sourceSpec *pb.SourceSpec
	if r.Sources != nil {
		sourceSpec = &pb.SourceSpec{Sources: r.Sources}
	}
	result := &pb.Invocation{
		Name:                   r.RootInvocationID.LegacyInvocationID().Name(),
		State:                  toInvocationState(r.FinalizationState),
		Realm:                  r.Realm,
		CreateTime:             pbutil.MustTimestampProto(r.CreateTime),
		CreatedBy:              r.CreatedBy,
		Deadline:               timestamppb.New(time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)),
		SourceSpec:             sourceSpec,
		IsSourceSpecFinal:      r.StreamingExportState == pb.RootInvocation_METADATA_FINAL,
		Tags:                   r.Tags,
		IsExportRoot:           true,
		Properties:             r.Properties,
		BaselineId:             r.BaselineID,
		TestResultVariantUnion: &pb.Variant{},
	}
	if r.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(r.FinalizeStartTime.Time)
	}
	if r.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(r.FinalizeTime.Time)
	}
	return result
}

// MarkFinalized creates a mutation to mark the given root invocation as finalized.
// The caller MUST check the root invocation is currently in FINALIZING state, or this
// may incorrectly overwrite the FinalizeTime.
func MarkFinalized(id ID) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 2)
	ms = append(ms, spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId":  id,
		"FinalizationState": pb.RootInvocation_FINALIZED,
		"LastUpdated":       spanner.CommitTimestamp,
		"FinalizeTime":      spanner.CommitTimestamp,
	}))
	ms = append(ms, invocations.MarkFinalized(id.LegacyInvocationID()))
	return ms
}

func CreateRootInvocationUpdateRequest(id ID, updatedBy, requestID string) *spanner.Mutation {
	return spanutil.InsertMap("RootInvocationUpdateRequests", map[string]interface{}{
		"RootInvocationId": id,
		"UpdatedBy":        updatedBy,
		"UpdateRequestId":  requestID,
		"CreateTime":       spanner.CommitTimestamp,
	})
}

// SetFinalizerPending returns mutations to set FinalizerPending to true,
// and sets the new sequence number.
// Note: The LastUpdated time is not updated because FinalizerPending and
// FinalizerSequence are internal control fields for managing background tasks.
func SetFinalizerPending(id ID, newSeq int64) *spanner.Mutation {
	return spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId":  id,
		"FinalizerPending":  true,
		"FinalizerSequence": newSeq,
	})
}

// ResetFinalizerPending returns a mutation which set FinalizerPending to false.
// Note: The LastUpdated time is not updated because FinalizerPending is an
// internal control field for managing background tasks.
func ResetFinalizerPending(id ID) *spanner.Mutation {
	return spanutil.UpdateMap("RootInvocations", map[string]any{
		"RootInvocationId": id,
		"FinalizerPending": false,
	})
}

// MutationBuilder is a helper to construct mutations to update a root invocation.
type MutationBuilder struct {
	id ID
	// values represents a partially-constructed mutation for the root invocation.
	values map[string]any
	// legacyInvocationValues represents a partially-constructed mtuation for the
	// legacy invocation that corresponds to the root invocation.
	legacyInvocationValues map[string]any
	// shardValues represents a partially-constructed mutation for the
	// root invocation shards.
	shardValues map[string]any
}

// NewMutationBuilder creates a new MutationBuilder for the given root invocation.
func NewMutationBuilder(id ID) *MutationBuilder {
	return &MutationBuilder{
		id: id,
		values: map[string]any{
			"RootInvocationId": id,
		},
		legacyInvocationValues: map[string]any{
			"InvocationId": id.LegacyInvocationID(),
		},
		shardValues: map[string]any{},
	}
}

// Build returns the mutations to update the root invocation.
func (b *MutationBuilder) Build() []*spanner.Mutation {
	var mutations []*spanner.Mutation
	// The `values` map is initialized with the primary key.
	// There is no update if no other columns are added.
	if len(b.values) > 1 {
		b.values["LastUpdated"] = spanner.CommitTimestamp
		mutations = append(mutations, spanutil.UpdateMap("RootInvocations", b.values))
		mutations = append(mutations, spanutil.UpdateMap("Invocations", b.legacyInvocationValues))

		// Check if any update is needed to the RootInvocationShards table.
		if len(b.shardValues) > 0 {
			// Update all records for this root invocation in RootInvocationShards table.
			for shardID := range b.id.AllShardIDs() {
				shardUpdate := make(map[string]any, len(b.shardValues)+1)
				maps.Copy(shardUpdate, b.shardValues)
				shardUpdate["RootInvocationShardId"] = shardID
				mutations = append(mutations, spanutil.UpdateMap("RootInvocationShards", shardUpdate))
			}
		}
	}
	return mutations
}

// UpdateState updates the state of the root invocation.
// If the new state is a terminal state, the root invocation is transitioned to a FINALIZING finalization state.
// The caller MUST check the root invocation is currently in ACTIVE finalization state, or this
// may incorrectly overwrite the FinalizeStartTime.
func (b *MutationBuilder) UpdateState(state pb.RootInvocation_State) {
	b.values["State"] = state
	if pbutil.IsFinalRootInvocationState(state) {
		b.values["FinalizationState"] = pb.RootInvocation_FINALIZING
		b.values["FinalizeStartTime"] = spanner.CommitTimestamp
		b.legacyInvocationValues["State"] = pb.Invocation_FINALIZING
		b.legacyInvocationValues["FinalizeStartTime"] = spanner.CommitTimestamp
	}
}

// UpdateSummaryMarkdown updates the summary markdown of the root invocation.
func (b *MutationBuilder) UpdateSummaryMarkdown(summaryMarkdown string) {
	b.values["SummaryMarkdown"] = summaryMarkdown
}

// UpdateDefinition updates the definition of the root invocation.
func (b *MutationBuilder) UpdateDefinition(definition *pb.RootInvocationDefinition) {
	if definition != nil {
		b.values["DefinitionSystem"] = definition.System
		b.values["DefinitionName"] = definition.Name
		b.values["DefinitionProperties"] = definition.Properties
	} else {
		b.values["DefinitionSystem"] = spanner.NullString{}
		b.values["DefinitionName"] = spanner.NullString{}
		b.values["DefinitionProperties"] = ([]string)(nil)
	}
}

// UpdateSources updates the sources of the root invocation.
func (b *MutationBuilder) UpdateSources(sources *pb.Sources) {
	compressedSources := spanutil.Compressed(pbutil.MustMarshal(sources))
	b.values["Sources"] = compressedSources
	b.legacyInvocationValues["Sources"] = compressedSources
	b.shardValues["Sources"] = compressedSources
}

// UpdatePrimaryBuild updates the primary build of the root invocation.
func (b *MutationBuilder) UpdatePrimaryBuild(primaryBuild *pb.BuildDescriptor) {
	b.values["PrimaryBuild"] = spanutil.Compressed(pbutil.MustMarshal(removeBuildDescriptorOutputOnlyFields(primaryBuild)))
}

// UpdateExtraBuilds updates the extra builds of the root invocation.
func (b *MutationBuilder) UpdateExtraBuilds(extraBuilds []*pb.BuildDescriptor) {
	if extraBuilds == nil {
		// Set an empty slice instead of nil to ensure the value written is NOT NULL.
		extraBuilds = make([]*pb.BuildDescriptor, 0)
	}
	b.values["ExtraBuilds"] = serializeExtraBuilds(extraBuilds)
}

// UpdateStreamingExportState updates the streaming export state of the root invocation.
func (b *MutationBuilder) UpdateStreamingExportState(state pb.RootInvocation_StreamingExportState) {
	b.values["StreamingExportState"] = state
	b.legacyInvocationValues["IsSourceSpecFinal"] = spanner.NullBool{Valid: true, Bool: true}
}

// UpdateTags updates the tags of the root invocation.
func (b *MutationBuilder) UpdateTags(tags []*pb.StringPair) {
	b.values["Tags"] = tags
	b.legacyInvocationValues["Tags"] = tags
}

// UpdateProperties updates the properties of the root invocation.
func (b *MutationBuilder) UpdateProperties(properties *structpb.Struct) {
	compressedProps := spanutil.Compressed(pbutil.MustMarshal(properties))
	b.values["Properties"] = compressedProps
	b.legacyInvocationValues["Properties"] = compressedProps
}

// UpdateBaselineID updates the baseline ID of the root invocation.
func (b *MutationBuilder) UpdateBaselineID(baselineID string) {
	b.values["BaselineId"] = baselineID
	b.legacyInvocationValues["BaselineId"] = baselineID
}
