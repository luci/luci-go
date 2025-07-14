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
	"go.chromium.org/luci/resultdb/internal/permissions/permissionstype"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// secondaryIndexShardCount is the number of values for the SecondaryIndexShardId column in the WorkUnits table
	// used for preventing hotspots in global secondary indexes. The range of values is [0, secondaryIndexShardCount-1].
	secondaryIndexShardCount = 100
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
	if workUnit.State != pb.WorkUnit_FINALIZING && workUnit.State != pb.WorkUnit_ACTIVE {
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

	workUnitMutation := workUnit.toMutation()
	invMutation := workUnit.toLegacyInvocationMutation(opts)
	return []*spanner.Mutation{workUnitMutation, invMutation}
}

// Convert the work unit row to the canonical form.
func (w *WorkUnitRow) Normalize() {
	pbutil.SortStringPairs(w.Tags)
}

// WorkUnitRow is the logical representation of the schema of the WorkUnits Spanner table.
// The values for the output only fields are ignored during writing.
type WorkUnitRow struct {
	ID                    ID
	ParentWorkUnitID      spanner.NullString
	SecondaryIndexShardID int64 // Output only.
	State                 pb.WorkUnit_State
	Realm                 string
	CreateTime            time.Time // Output only.
	CreatedBy             string
	FinalizeStartTime     spanner.NullTime // Output only.
	FinalizeTime          spanner.NullTime // Output only.
	Deadline              time.Time
	CreateRequestID       string
	ProducerResource      string
	Tags                  []*pb.StringPair
	Properties            *structpb.Struct
	Instructions          *pb.Instructions
	ExtendedProperties    map[string]*structpb.Struct
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
	if w.ExtendedProperties != nil {
		ret.ExtendedProperties = make(map[string]*structpb.Struct, len(w.ExtendedProperties))
		for k, v := range w.ExtendedProperties {
			ret.ExtendedProperties[k] = proto.Clone(v).(*structpb.Struct)
		}
	}
	return &ret
}

func (w *WorkUnitRow) toMutation() *spanner.Mutation {
	row := map[string]interface{}{
		"RootInvocationShardId": w.ID.RootInvocationShardID(),
		"WorkUnitId":            w.ID.WorkUnitID,
		"ParentWorkUnitId":      w.ParentWorkUnitID,
		"SecondaryIndexShardId": w.ID.shardID(secondaryIndexShardCount),
		"State":                 w.State,
		"Realm":                 w.Realm,
		"CreateTime":            spanner.CommitTimestamp,
		"CreatedBy":             w.CreatedBy,
		"Deadline":              w.Deadline,
		"CreateRequestId":       w.CreateRequestID,
		"ProducerResource":      w.ProducerResource,
		"Tags":                  w.Tags,
		"Properties":            spanutil.Compressed(pbutil.MustMarshal(w.Properties)),
		"Instructions":          spanutil.Compressed(pbutil.MustMarshal(instructionutil.RemoveInstructionsName(w.Instructions))),
	}
	// Wrap into luci.resultdb.internal.invocations.ExtendedProperties so that
	// it can be serialized as a single value to spanner.
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: w.ExtendedProperties,
	}
	row["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))

	if w.State == pb.WorkUnit_FINALIZING {
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("WorkUnits", row)
}

func (w *WorkUnitRow) toLegacyInvocationMutation(opts LegacyCreateOptions) *spanner.Mutation {
	row := map[string]interface{}{
		"InvocationId":                      w.ID.LegacyInvocationID(),
		"Type":                              invocations.WorkUnit,
		"ShardId":                           w.ID.shardID(invocations.Shards),
		"State":                             w.State,
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
		// Source are not set for work unit.
		"InheritSources": spanner.NullBool{Valid: false},
		// Work units are not export roots.
		"IsExportRoot": spanner.NullBool{Bool: false, Valid: true},
		"Instructions": spanutil.Compressed(pbutil.MustMarshal(instructionutil.RemoveInstructionsName(w.Instructions))),
	}

	// Wrap into luci.resultdb.internal.invocations.ExtendedProperties so that
	// it can be serialized as a single value to spanner.
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: w.ExtendedProperties,
	}
	row["ExtendedProperties"] = spanutil.Compressed(pbutil.MustMarshal(internalExtendedProperties))

	if w.State == pb.WorkUnit_FINALIZING {
		row["FinalizeStartTime"] = spanner.CommitTimestamp
	}
	return spanutil.InsertMap("Invocations", row)
}

func (w *WorkUnitRow) ToCompleteProto() *pb.WorkUnit {
	return w.ToProto(permissionstype.FullAccess, pb.WorkUnitView_WORK_UNIT_VIEW_FULL)
}

func (w *WorkUnitRow) ToProto(accessLevel permissionstype.AccessLevel, view pb.WorkUnitView) *pb.WorkUnit {
	// TODO: child work units, child invocations.
	result := &pb.WorkUnit{
		// Include metadata-only fields by default.
		Name:             w.ID.Name(),
		WorkUnitId:       w.ID.WorkUnitID,
		State:            w.State,
		Realm:            w.Realm,
		CreateTime:       pbutil.MustTimestampProto(w.CreateTime),
		Creator:          w.CreatedBy,
		Deadline:         pbutil.MustTimestampProto(w.Deadline),
		ProducerResource: w.ProducerResource,
		IsMasked:         true,
	}
	if accessLevel == permissionstype.FullAccess {
		result.Tags = w.Tags
		result.Properties = w.Properties
		result.Instructions = w.Instructions
		result.IsMasked = false

		if view == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
			result.ExtendedProperties = w.ExtendedProperties
		}
	}

	if w.ID.WorkUnitID == "root" {
		result.Parent = w.ID.RootInvocationID.Name()
	} else {
		if !w.ParentWorkUnitID.Valid {
			panic(fmt.Sprintf("invariant violated: parent work unit ID not set on non-root work unit %q", w.ID.Name()))
		}
		result.Parent = ID{
			RootInvocationID: w.ID.RootInvocationID,
			WorkUnitID:       w.ParentWorkUnitID.StringVal,
		}.Name()
	}
	if w.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(w.FinalizeStartTime.Time)
	}
	if w.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(w.FinalizeTime.Time)
	}
	return result
}
