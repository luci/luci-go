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

package workunits

import (
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Builder is a builder for WorkUnitRow for testing.
type Builder struct {
	row WorkUnitRow
}

// NewBuilder returns a new builder for a WorkUnitRow for testing.
// The builder is initialized with some default values.
func NewBuilder(rootInvocationID rootinvocations.ID, workUnitID string) *Builder {
	id := ID{RootInvocationID: rootInvocationID, WorkUnitID: workUnitID}
	var parentID spanner.NullString
	// Default to creating the work unit under the work unit "root".
	if workUnitID != "root" {
		parentID = spanner.NullString{Valid: true, StringVal: "root"}
	}
	return &Builder{
		// Default to populating all fields. This helps maximise test coverage.
		row: WorkUnitRow{
			ID:                    id,
			SecondaryIndexShardID: id.shardID(secondaryIndexShardCount),
			ParentWorkUnitID:      parentID,
			State:                 pb.WorkUnit_FINALIZED,
			Realm:                 "testproject:testrealm",
			CreateTime:            time.Date(2025, 4, 25, 1, 2, 3, 4000, time.UTC),
			CreatedBy:             "user:test@example.com",
			FinalizeStartTime:     spanner.NullTime{Valid: true, Time: time.Date(2025, 4, 26, 1, 2, 3, 4000, time.UTC)},
			FinalizeTime:          spanner.NullTime{Valid: true, Time: time.Date(2025, 4, 27, 1, 2, 3, 4000, time.UTC)},
			Deadline:              time.Date(2025, 4, 28, 1, 2, 3, 4000, time.UTC),
			CreateRequestID:       "test-request-id",
			ProducerResource:      "//builds.example.com/builds/123",
			Tags:                  pbutil.StringPairs("k1", "v1"),
			Properties: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"key": structpb.NewStringValue("value"),
				},
			},
			Instructions: &pb.Instructions{
				Instructions: []*pb.Instruction{
					{
						Id:   "step",
						Type: pb.InstructionType_STEP_INSTRUCTION,
					},
				},
			},
			ExtendedProperties: map[string]*structpb.Struct{
				"mykey": {
					Fields: map[string]*structpb.Value{
						"@type":       structpb.NewStringValue("foo.bar.com/x/_some.package.MyMessage"),
						"child_key_1": structpb.NewStringValue("child_value_1"),
					},
				},
			},
		},
	}
}

// WithMinimalFields clears as many fields as possible on the work unit while keeping
// it a valid work unit. This helps test code handles unset fields.
func (b *Builder) WithMinimalFields() *Builder {
	b.row = WorkUnitRow{
		ID:                    b.row.ID,
		ParentWorkUnitID:      b.row.ParentWorkUnitID,
		SecondaryIndexShardID: b.row.SecondaryIndexShardID,
		// Means the finalized time and start time will be cleared in Build() unless state is
		// subsequently overridden.
		State:             pb.WorkUnit_ACTIVE,
		Realm:             b.row.Realm,
		CreateTime:        b.row.CreateTime,
		CreatedBy:         b.row.CreatedBy,
		FinalizeStartTime: b.row.FinalizeStartTime,
		FinalizeTime:      b.row.FinalizeTime,
		Deadline:          b.row.Deadline,
		CreateRequestID:   b.row.CreateRequestID,
		// Prefer to use empty slice rather than nil (even though semantically identical)
		// as this what we always report in reads.
		Tags: []*pb.StringPair{},
	}
	return b
}

// WithID sets the work unit ID.
func (b *Builder) WithID(id ID) *Builder {
	b.row.ID = id
	b.row.SecondaryIndexShardID = id.shardID(secondaryIndexShardCount)
	return b
}

// WithParentWorkUnitID sets the parent work unit ID.
func (b *Builder) WithParentWorkUnitID(parentID string) *Builder {
	b.row.ParentWorkUnitID = spanner.NullString{StringVal: parentID, Valid: true}
	return b
}

// WithState sets the state of the work unit.
func (b *Builder) WithState(state pb.WorkUnit_State) *Builder {
	b.row.State = state
	return b
}

// WithRealm sets the realm of the work unit.
func (b *Builder) WithRealm(realm string) *Builder {
	b.row.Realm = realm
	return b
}

// WithCreateTime sets the creation time of the work unit.
func (b *Builder) WithCreateTime(t time.Time) *Builder {
	b.row.CreateTime = t
	return b
}

// WithCreatedBy sets the creator of the work unit.
func (b *Builder) WithCreatedBy(creator string) *Builder {
	b.row.CreatedBy = creator
	return b
}

// WithFinalizeStartTime sets the finalize start time.
func (b *Builder) WithFinalizeStartTime(t time.Time) *Builder {
	b.row.FinalizeStartTime = spanner.NullTime{Valid: true, Time: t}
	return b
}

// WithFinalizeTime sets the finalize time.
func (b *Builder) WithFinalizeTime(t time.Time) *Builder {
	b.row.FinalizeTime = spanner.NullTime{Valid: true, Time: t}
	return b
}

// WithDeadline sets the deadline of the work unit.
func (b *Builder) WithDeadline(t time.Time) *Builder {
	b.row.Deadline = t
	return b
}

// WithCreateRequestID sets the create request ID.
func (b *Builder) WithCreateRequestID(id string) *Builder {
	b.row.CreateRequestID = id
	return b
}

// WithProducerResource sets the producer resource.
func (b *Builder) WithProducerResource(res string) *Builder {
	b.row.ProducerResource = res
	return b
}

// WithTags sets the tags.
func (b *Builder) WithTags(tags []*pb.StringPair) *Builder {
	b.row.Tags = tags
	return b
}

// WithProperties sets the properties.
func (b *Builder) WithProperties(p *structpb.Struct) *Builder {
	b.row.Properties = p
	return b
}

// WithInstructions sets the instructions.
func (b *Builder) WithInstructions(i *pb.Instructions) *Builder {
	b.row.Instructions = i
	return b
}

// WithExtendedProperties sets the extended properties.
func (b *Builder) WithExtendedProperties(ep map[string]*structpb.Struct) *Builder {
	b.row.ExtendedProperties = ep
	return b
}

// Build returns the constructed WorkUnitRow.
func (b *Builder) Build() *WorkUnitRow {
	// Return a copy to avoid changes to the returned object
	// flowing back into the builder.
	r := b.row.Clone()

	// Populate output-only fields on instructions.
	r.Instructions = instructionutil.InstructionsWithNames(r.Instructions, r.ID.Name())

	if r.State == pb.WorkUnit_ACTIVE {
		r.FinalizeStartTime = spanner.NullTime{}
		r.FinalizeTime = spanner.NullTime{}
	}
	if r.State == pb.WorkUnit_FINALIZING {
		r.FinalizeTime = spanner.NullTime{}
	}
	return r
}

// InsertForTesting inserts the work unit record and its corresponding
// legacy invocation record for testing purposes.
func InsertForTesting(w *WorkUnitRow) []*spanner.Mutation {
	workUnitMutation := spanutil.InsertMap("WorkUnits", map[string]any{
		"RootInvocationShardId": w.ID.RootInvocationShardID(),
		"WorkUnitId":            w.ID.WorkUnitID,
		"ParentWorkUnitId":      w.ParentWorkUnitID,
		"SecondaryIndexShardId": w.SecondaryIndexShardID,
		"State":                 w.State,
		"Realm":                 w.Realm,
		"CreateTime":            w.CreateTime,
		"CreatedBy":             w.CreatedBy,
		"FinalizeStartTime":     w.FinalizeStartTime,
		"FinalizeTime":          w.FinalizeTime,
		"Deadline":              w.Deadline,
		"CreateRequestId":       w.CreateRequestID,
		"ProducerResource":      w.ProducerResource,
		"Tags":                  w.Tags,
		"Properties":            spanutil.Compressed(pbutil.MustMarshal(w.Properties)),
		"Instructions":          spanutil.Compressed(pbutil.MustMarshal(instructionutil.RemoveInstructionsName(w.Instructions))),
		"ExtendedProperties":    spanutil.Compressed(pbutil.MustMarshal(&invocationspb.ExtendedProperties{ExtendedProperties: w.ExtendedProperties})),
	})

	legacyInvMutation := spanutil.InsertMap("Invocations", map[string]any{
		"InvocationId":                      w.ID.LegacyInvocationID(),
		"Type":                              invocations.WorkUnit,
		"ShardId":                           w.ID.shardID(invocations.Shards),
		"State":                             w.State,
		"Realm":                             w.Realm,
		"InvocationExpirationTime":          time.Unix(0, 0), // unused field, but spanner schema enforce it to be not null.
		"ExpectedTestResultsExpirationTime": w.CreateTime.Add(90 * 24 * time.Hour),
		"CreateTime":                        w.CreateTime,
		"CreatedBy":                         w.CreatedBy,
		"FinalizeStartTime":                 w.FinalizeStartTime,
		"FinalizeTime":                      w.FinalizeTime,
		"Deadline":                          w.Deadline,
		"Tags":                              w.Tags,
		"CreateRequestId":                   w.CreateRequestID,
		"ProducerResource":                  w.ProducerResource,
		"Properties":                        spanutil.Compressed(pbutil.MustMarshal(w.Properties)),
		"InheritSources":                    spanner.NullBool{Valid: false},
		"IsExportRoot":                      spanner.NullBool{Bool: false, Valid: true},
		"Instructions":                      spanutil.Compressed(pbutil.MustMarshal(instructionutil.RemoveInstructionsName(w.Instructions))),
		"ExtendedProperties":                spanutil.Compressed(pbutil.MustMarshal(&invocationspb.ExtendedProperties{ExtendedProperties: w.ExtendedProperties})),
	})

	var parentInvocationID invocations.ID
	if w.ParentWorkUnitID.Valid {
		// The parent is another work unit.
		parentInvocationID = ID{RootInvocationID: w.ID.RootInvocationID, WorkUnitID: w.ParentWorkUnitID.StringVal}.LegacyInvocationID()
	} else {
		// The parent is the root invocation.
		parentInvocationID = w.ID.RootInvocationID.LegacyInvocationID()
	}
	includeMutation := spanutil.InsertMap("IncludedInvocations", map[string]any{
		"InvocationId":         parentInvocationID,
		"IncludedInvocationId": w.ID.LegacyInvocationID(),
	})

	return []*spanner.Mutation{workUnitMutation, legacyInvMutation, includeMutation}
}
