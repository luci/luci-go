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
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// secondaryIndexShardCount is the number of values for the SecondaryIndexShardId column in the WorkUnits table
	// used for preventing hotspots in global secondary indexes. The range of values is [0, secondaryIndexShardCount-1].
	secondaryIndexShardCount = 100

	// Work unit ID of the root work unit.
	RootWorkUnitID = "root"
)

// Extra information to support writing to legacy invocation table.
type LegacyCreateOptions struct {
	ExpectedTestResultsExpirationTime time.Time
}

// Create creates mutations of the following records.
//   - a new work unit record in the WorkUnits table,
//   - the corresponding legacy invocation record in the Invocations table.
func Create(workUnit *WorkUnitRow, opts LegacyCreateOptions) []*spanner.Mutation {
	if workUnit.ID.RootInvocationID == "" {
		panic("do not create work units with empty root invocation id")
	}
	if workUnit.ID.WorkUnitID == "" {
		panic("do not create work units with empty work unit id")
	}
	if workUnit.FinalizationState != pb.WorkUnit_FINALIZING && workUnit.FinalizationState != pb.WorkUnit_ACTIVE {
		panic("do not create work units in states other than active or finalizing")
	}
	if workUnit.Realm == "" {
		panic("do not create work units with empty realm")
	}
	if workUnit.CreatedBy == "" {
		panic("do not create work units with empty creator")
	}
	if workUnit.Deadline.IsZero() {
		panic("do not create work units with empty deadline")
	}
	workUnit.Normalize()

	var ms []*spanner.Mutation
	ms = append(ms, workUnit.toMutation())
	if workUnit.ParentWorkUnitID.Valid {
		ms = append(ms, workUnit.toChildWorkUnitsMutations()...)
	}
	ms = append(ms, workUnit.toLegacyInvocationMutation(opts))
	ms = append(ms, workUnit.toLegacyInclusionMutation())
	return ms
}

// Convert the work unit row to the canonical form.
func (w *WorkUnitRow) Normalize() {
	pbutil.SortStringPairs(w.Tags)
}

// WorkUnitRow is the logical representation of the schema of the WorkUnits Spanner table.
// The values for the output only fields are ignored during writing.
type WorkUnitRow struct {
	ID                     ID
	ParentWorkUnitID       spanner.NullString
	SecondaryIndexShardID  int64                         // Output only.
	FinalizationState      pb.WorkUnit_FinalizationState // Output only.
	State                  pb.WorkUnit_State
	SummaryMarkdown        string
	Realm                  string
	CreateTime             time.Time // Output only.
	CreatedBy              string
	LastUpdated            time.Time        // Output only
	FinalizeStartTime      spanner.NullTime // Output only.
	FinalizeTime           spanner.NullTime // Output only.
	FinalizerCandidateTime spanner.NullTime // Output only.
	Deadline               time.Time
	CreateRequestID        string
	ModuleID               *pb.ModuleIdentifier
	ProducerResource       string
	Tags                   []*pb.StringPair
	Properties             *structpb.Struct
	Instructions           *pb.Instructions
	ExtendedProperties     map[string]*structpb.Struct
	ChildWorkUnits         []ID             // Output only.
	ChildInvocations       []invocations.ID // Output only.
}

// Clone makes a deep copy of the row.
func (w *WorkUnitRow) Clone() *WorkUnitRow {
	ret := *w
	if w.Tags != nil {
		ret.Tags = make([]*pb.StringPair, len(w.Tags))
		for i, tp := range w.Tags {
			ret.Tags[i] = proto.Clone(tp).(*pb.StringPair)
		}
	}
	if w.Properties != nil {
		ret.Properties = proto.Clone(w.Properties).(*structpb.Struct)
	}
	if w.Instructions != nil {
		ret.Instructions = proto.Clone(w.Instructions).(*pb.Instructions)
	}
	if w.ModuleID != nil {
		ret.ModuleID = proto.Clone(w.ModuleID).(*pb.ModuleIdentifier)
	}
	if w.ExtendedProperties != nil {
		ret.ExtendedProperties = make(map[string]*structpb.Struct, len(w.ExtendedProperties))
		for k, v := range w.ExtendedProperties {
			ret.ExtendedProperties[k] = proto.Clone(v).(*structpb.Struct)
		}
	}
	if w.ChildWorkUnits != nil {
		ret.ChildWorkUnits = make([]ID, len(w.ChildWorkUnits))
		copy(ret.ChildWorkUnits, w.ChildWorkUnits)
	}
	if w.ChildInvocations != nil {
		ret.ChildInvocations = make([]invocations.ID, len(w.ChildInvocations))
		copy(ret.ChildInvocations, w.ChildInvocations)
	}
	return &ret
}

func (w *WorkUnitRow) toMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"RootInvocationShardId": w.ID.RootInvocationShardID(),
		"WorkUnitId":            w.ID.WorkUnitID,
		"ParentWorkUnitId":      w.ParentWorkUnitID,
		"SecondaryIndexShardId": w.ID.shardID(secondaryIndexShardCount),
		"FinalizationState":     w.FinalizationState,
		"State":                 w.State,
		"SummaryMarkdown":       w.SummaryMarkdown,
		"Realm":                 w.Realm,
		"CreateTime":            spanner.CommitTimestamp,
		"CreatedBy":             w.CreatedBy,
		"LastUpdated":           spanner.CommitTimestamp,
		"Deadline":              w.Deadline,
		"CreateRequestId":       w.CreateRequestID,
		"ProducerResource":      w.ProducerResource,
		"Tags":                  w.Tags,
		"Properties":            spanutil.Compressed(pbutil.MustMarshal(w.Properties)),
		"Instructions":          spanutil.Compressed(pbutil.MustMarshal(instructionutil.RemoveInstructionsName(w.Instructions))),
	}
	if w.ModuleID != nil {
		row["ModuleName"] = w.ModuleID.ModuleName
		row["ModuleScheme"] = w.ModuleID.ModuleScheme
		row["ModuleVariant"] = w.ModuleID.ModuleVariant
		row["ModuleVariantHash"] = pbutil.VariantHash(w.ModuleID.ModuleVariant)
	}
	// Wrap into luci.resultdb.internal.invocations.ExtendedProperties so that
	// it can be serialized as a single value to spanner.
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: w.ExtendedProperties,
	}
	row["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))

	if w.FinalizationState == pb.WorkUnit_FINALIZING {
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("WorkUnits", row)
}

func (w *WorkUnitRow) toChildWorkUnitsMutations() []*spanner.Mutation {
	if !w.ParentWorkUnitID.Valid {
		if w.ID.WorkUnitID != RootWorkUnitID {
			panic("only root work unit can have empty parent work unit ID")
		}
		panic("cannot create ChildWorkUnits mutation; work unit does not have a parent")
	}
	// Include an entry in ChildWorkUnits for the parent work unit.
	parentID := ID{
		RootInvocationID: w.ID.RootInvocationID,
		WorkUnitID:       w.ParentWorkUnitID.StringVal,
	}
	// Mark the parent work unit updated, per semantics of LastUpdated.
	parentRow := map[string]interface{}{
		"RootInvocationShardId": parentID.RootInvocationShardID(),
		"WorkUnitId":            parentID.WorkUnitID,
		"LastUpdated":           spanner.CommitTimestamp,
	}
	inclusionRow := map[string]interface{}{
		"RootInvocationShardId":      parentID.RootInvocationShardID(),
		"WorkUnitId":                 parentID.WorkUnitID,
		"ChildRootInvocationShardId": w.ID.RootInvocationShardID(),
		"ChildWorkUnitId":            w.ID.WorkUnitID,
	}
	var ms []*spanner.Mutation
	ms = append(ms, spanutil.UpdateMap("WorkUnits", parentRow))
	ms = append(ms, spanutil.InsertMap("ChildWorkUnits", inclusionRow))
	return ms
}

func (w *WorkUnitRow) toLegacyInvocationMutation(opts LegacyCreateOptions) *spanner.Mutation {
	row := map[string]interface{}{
		"InvocationId":                      w.ID.LegacyInvocationID(),
		"Type":                              invocations.WorkUnit,
		"ShardId":                           w.ID.shardID(invocations.Shards),
		"State":                             toInvocationState(w.FinalizationState),
		"Realm":                             w.Realm,
		"InvocationExpirationTime":          time.Unix(0, 0), // unused field, but spanner schema enforce it to be not null.
		"ExpectedTestResultsExpirationTime": opts.ExpectedTestResultsExpirationTime,
		"CreateTime":                        spanner.CommitTimestamp,
		"CreatedBy":                         w.CreatedBy,
		"Deadline":                          w.Deadline,
		"Tags":                              w.Tags,
		"CreateRequestId":                   w.CreateRequestID,
		"ProducerResource":                  w.ProducerResource,
		"Properties":                        spanutil.Compressed(pbutil.MustMarshal(w.Properties)),
		// Work unit always inherits source from root invocation.
		"InheritSources":    spanner.NullBool{Valid: true, Bool: true},
		"IsSourceSpecFinal": true,
		// Work units are not export roots.
		"IsExportRoot": spanner.NullBool{Bool: false, Valid: true},
		"Instructions": spanutil.Compressed(pbutil.MustMarshal(instructionutil.RemoveInstructionsName(w.Instructions))),
	}
	if w.ModuleID != nil {
		row["ModuleName"] = w.ModuleID.ModuleName
		row["ModuleScheme"] = w.ModuleID.ModuleScheme
		row["ModuleVariant"] = w.ModuleID.ModuleVariant
		row["ModuleVariantHash"] = pbutil.VariantHash(w.ModuleID.ModuleVariant)

		// Replicate the ModuleVariant to the TestResultVariantUnion, since this is no
		// longer computed at test result upload time.
		row["TestResultVariantUnion"] = w.ModuleID.ModuleVariant
	}
	// Wrap into luci.resultdb.internal.invocations.ExtendedProperties so that
	// it can be serialized as a single value to spanner.
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: w.ExtendedProperties,
	}
	row["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))

	if w.FinalizationState == pb.WorkUnit_FINALIZING {
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("Invocations", row)
}

func toInvocationState(finalizationState pb.WorkUnit_FinalizationState) pb.Invocation_State {
	switch finalizationState {
	case pb.WorkUnit_ACTIVE:
		return pb.Invocation_ACTIVE
	case pb.WorkUnit_FINALIZING:
		return pb.Invocation_FINALIZING
	case pb.WorkUnit_FINALIZED:
		return pb.Invocation_FINALIZED
	default:
		panic(fmt.Sprintf("unknown work unit state %s", finalizationState))
	}
}

func (w *WorkUnitRow) ToLegacyInvocationProto() *pb.Invocation {
	ret := &pb.Invocation{
		Name:                   w.ID.LegacyInvocationID().Name(),
		State:                  toInvocationState(w.FinalizationState),
		Realm:                  w.Realm,
		ModuleId:               w.ModuleID,
		CreateTime:             pbutil.MustTimestampProto(w.CreateTime),
		CreatedBy:              w.CreatedBy,
		Deadline:               pbutil.MustTimestampProto(w.Deadline),
		Tags:                   w.Tags,
		Properties:             w.Properties,
		SourceSpec:             &pb.SourceSpec{Inherit: true},
		IsSourceSpecFinal:      true,
		ProducerResource:       w.ProducerResource,
		Instructions:           instructionutil.InstructionsWithNames(w.Instructions, w.ID.LegacyInvocationID().Name()),
		ExtendedProperties:     w.ExtendedProperties,
		TestResultVariantUnion: &pb.Variant{},
	}
	if w.FinalizeStartTime.Valid {
		ret.FinalizeStartTime = pbutil.MustTimestampProto(w.FinalizeStartTime.Time)
	}
	if w.FinalizeTime.Valid {
		ret.FinalizeTime = pbutil.MustTimestampProto(w.FinalizeTime.Time)
	}
	return ret
}

func (w *WorkUnitRow) toLegacyInclusionMutation() *spanner.Mutation {
	var parentLegacyInvocationID invocations.ID
	if !w.ParentWorkUnitID.Valid {
		if w.ID.WorkUnitID != RootWorkUnitID {
			panic("only root work unit can have empty parent work unit ID")
		}
		// For root work unit, the parent should be the root invocation.
		parentLegacyInvocationID = w.ID.RootInvocationID.LegacyInvocationID()
	} else {
		parentID := ID{
			RootInvocationID: w.ID.RootInvocationID,
			WorkUnitID:       w.ParentWorkUnitID.StringVal,
		}
		parentLegacyInvocationID = parentID.LegacyInvocationID()
	}
	return spanutil.InsertMap("IncludedInvocations", map[string]any{
		"InvocationId":         parentLegacyInvocationID,
		"IncludedInvocationId": w.ID.LegacyInvocationID(),
	})
}

// MarkFinalized creates mutations to mark the given work unit as finalized.
// The caller MUST check the work unit is currently in FINALIZING state, or this
// may incorrectly overwrite the FinalizeTime.
func MarkFinalized(id ID) []*spanner.Mutation {
	ms := make([]*spanner.Mutation, 0, 2)
	ms = append(ms, spanutil.UpdateMap("WorkUnits", map[string]any{
		"RootInvocationShardId":  id.RootInvocationShardID(),
		"WorkUnitId":             id.WorkUnitID,
		"FinalizationState":      pb.WorkUnit_FINALIZED,
		"LastUpdated":            spanner.CommitTimestamp,
		"FinalizeTime":           spanner.CommitTimestamp,
		"FinalizerCandidateTime": nil,
	}))
	ms = append(ms, invocations.MarkFinalized(id.LegacyInvocationID()))
	return ms
}

func CreateWorkUnitUpdateRequest(id ID, updatedBy, requestID string) *spanner.Mutation {
	return spanutil.InsertMap("WorkUnitUpdateRequests", map[string]interface{}{
		"RootInvocationShardId": id.RootInvocationShardID(),
		"WorkUnitId":            id.WorkUnitID,
		"UpdatedBy":             updatedBy,
		"UpdateRequestId":       requestID,
		"CreateTime":            spanner.CommitTimestamp,
	})
}

// SetFinalizerCandidateTime creates a mutation to set the FinalizerCandidateTime for a work unit.
func SetFinalizerCandidateTime(id ID) *spanner.Mutation {
	return spanutil.UpdateMap("WorkUnits", map[string]any{
		"RootInvocationShardId":  id.RootInvocationShardID(),
		"WorkUnitId":             id.WorkUnitID,
		"FinalizerCandidateTime": spanner.CommitTimestamp,
	})
}

// ResetFinalizerCandidateTime conditionally reset the ResetFinalizerCandidateTime for work units
// when the read FinalizerCandidateTime matches the input.
// The reset is conditional to avoid clearing the candidate time
// if the work unit became a candidate (again) since the task read the state of work units.
func ResetFinalizerCandidateTime(candidates []FinalizerCandidate) spanner.Statement {
	st := spanner.NewStatement(`
			UPDATE WorkUnits
			SET
				FinalizerCandidateTime = NULL
			WHERE
				STRUCT(RootInvocationShardId, WorkUnitId, FinalizerCandidateTime) IN UNNEST(@candidates)
		`)
	// Struct to use as Spanner Query Parameter.
	type candiate struct {
		RootInvocationShardId  string
		WorkUnitId             string
		FinalizerCandidateTime time.Time
	}
	candidateParam := make([]candiate, 0, len(candidates))
	for _, candidate := range candidates {
		candidateParam = append(candidateParam, candiate{
			RootInvocationShardId:  candidate.ID.RootInvocationShardID().RowID(),
			WorkUnitId:             candidate.ID.WorkUnitID,
			FinalizerCandidateTime: candidate.FinalizerCandidateTime,
		})
	}
	st.Params = map[string]interface{}{
		"candidates": candidateParam,
	}
	return st
}

// MutationBuilder is a helper to construct mutations to update a work unit.
type MutationBuilder struct {
	id                     ID
	values                 map[string]any
	legacyInvocationValues map[string]any
	// A mutation builder used to replicate certain work unit fields to the
	// the root invocation. Only set if this work unit is the root work unit.
	rootInvocation *rootinvocations.MutationBuilder
}

// NewMutationBuilder creates a new MutationBuilder for the given work unit.
func NewMutationBuilder(id ID) *MutationBuilder {
	b := &MutationBuilder{
		id: id,
		values: map[string]any{
			"RootInvocationShardId": id.RootInvocationShardID(),
			"WorkUnitId":            id.WorkUnitID,
		},
		legacyInvocationValues: map[string]any{
			"InvocationId": id.LegacyInvocationID(),
		},
	}
	if id.WorkUnitID == RootWorkUnitID {
		b.rootInvocation = rootinvocations.NewMutationBuilder(id.RootInvocationID)
	}
	return b
}

// Build returns the mutations to update the work unit.
func (b *MutationBuilder) Build() []*spanner.Mutation {
	var mutations []*spanner.Mutation
	// The `values` map is initialized with the 2 primary key columns of a WorkUnit.
	// There is no update if no other columns are added.
	if len(b.values) > 2 {
		b.values["LastUpdated"] = spanner.CommitTimestamp
		mutations = append(mutations, spanutil.UpdateMap("WorkUnits", b.values))
		mutations = append(mutations, spanutil.UpdateMap("Invocations", b.legacyInvocationValues))

		if b.rootInvocation != nil {
			mutations = append(mutations, b.rootInvocation.Build()...)
		}
	}
	return mutations
}

// UpdateState updates the state of the work unit.
// If the new state is a terminal state, the work unit is transitioned to a FINALIZING finalization state.
// The caller MUST check the root invocation is currently in ACTIVE finalization state, or this
// may incorrectly overwrite the FinalizeStartTime.
func (b *MutationBuilder) UpdateState(state pb.WorkUnit_State) {
	b.values["State"] = state

	if pbutil.IsFinalWorkUnitState(state) {
		// We are transitioning to a terminal state. Begin finalizing
		// the work unit.
		b.values["FinalizationState"] = pb.WorkUnit_FINALIZING
		b.values["FinalizeStartTime"] = spanner.CommitTimestamp
		b.values["FinalizerCandidateTime"] = spanner.CommitTimestamp
		b.legacyInvocationValues["State"] = pb.Invocation_FINALIZING
		b.legacyInvocationValues["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	if b.rootInvocation != nil {
		// The state of the root work unit is replicated to the root invocation.
		b.rootInvocation.UpdateState(pbutil.WorkUnitToRootInvocationState(state))
	}
}

// UpdateSummaryMarkdown updates the summary markdown of the work unit.
func (b *MutationBuilder) UpdateSummaryMarkdown(summaryMarkdown string) {
	b.values["SummaryMarkdown"] = summaryMarkdown
	if b.rootInvocation != nil {
		// The summary markdown of the root work unit is replicated to the root invocation.
		b.rootInvocation.UpdateSummaryMarkdown(summaryMarkdown)
	}
}

// UpdateDeadline updates the deadline of the work unit.
func (b *MutationBuilder) UpdateDeadline(deadline time.Time) {
	b.values["Deadline"] = deadline
	b.legacyInvocationValues["Deadline"] = deadline
}

// UpdateModuleID updates the module ID of the work unit.
func (b *MutationBuilder) UpdateModuleID(moduleID *pb.ModuleIdentifier) {
	b.values["ModuleName"] = moduleID.ModuleName
	b.values["ModuleScheme"] = moduleID.ModuleScheme
	b.values["ModuleVariant"] = moduleID.ModuleVariant
	b.values["ModuleVariantHash"] = pbutil.VariantHash(moduleID.ModuleVariant)

	b.legacyInvocationValues["ModuleName"] = moduleID.ModuleName
	b.legacyInvocationValues["ModuleScheme"] = moduleID.ModuleScheme
	b.legacyInvocationValues["ModuleVariant"] = moduleID.ModuleVariant
	b.legacyInvocationValues["ModuleVariantHash"] = pbutil.VariantHash(moduleID.ModuleVariant)
}

// UpdateProperties updates the properties of the work unit.
func (b *MutationBuilder) UpdateProperties(properties *structpb.Struct) {
	b.values["Properties"] = spanutil.Compressed(pbutil.MustMarshal(properties))
	b.legacyInvocationValues["Properties"] = spanutil.Compressed(pbutil.MustMarshal(properties))
}

// UpdateTags updates the tags of the work unit.
func (b *MutationBuilder) UpdateTags(tags []*pb.StringPair) {
	b.values["Tags"] = tags
	b.legacyInvocationValues["Tags"] = tags
}

// UpdateExtendedProperties updates the extended properties of the work unit.
func (b *MutationBuilder) UpdateExtendedProperties(extendedProperties map[string]*structpb.Struct) {
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: extendedProperties,
	}
	b.values["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))
	b.legacyInvocationValues["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))
}

// UpdateInstructions updates the instructions of the work unit.
func (b *MutationBuilder) UpdateInstructions(instructions *pb.Instructions) {
	ins := instructionutil.RemoveInstructionsName(instructions)
	b.values["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(ins))
	b.legacyInvocationValues["Instructions"] = spanutil.Compressed(pbutil.MustMarshal(ins))
}
