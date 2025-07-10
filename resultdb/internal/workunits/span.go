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
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
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

func (w *WorkUnitRow) toMutation() *spanner.Mutation {
	// TODO: Normalise instruction names before writing.

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
		"Instructions":          spanutil.Compressed(pbutil.MustMarshal(w.Instructions)),
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
	// TODO: Normalise instruction names before writing.

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
		"Instructions": spanutil.Compressed(pbutil.MustMarshal(w.Instructions)),
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
