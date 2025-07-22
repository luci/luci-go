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
	"context"
	"sort"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// readColumns reads the specified columns from a work unit Spanner row.
// If the work unit does not exist, the returned error is annotated with
// NotFound GRPC code.
// For ptrMap see ReadRow comment in span/util.go.
func readColumns(ctx context.Context, id ID, ptrMap map[string]any) error {
	if err := validateID(id); err != nil {
		return err
	}

	err := spanutil.ReadRow(ctx, "WorkUnits", id.Key(), ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return appstatus.Attachf(err, codes.NotFound, "%q not found", id.Name())

	case err != nil:
		return errors.Fmt("fetch %s: %w", id.Name(), err)

	default:
		return nil
	}
}

// ReadRealm reads the realm of the given work unit. If the work unit
// is not found, returns a NotFound appstatus error.
func ReadRealm(ctx context.Context, id ID) (realm string, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadRealm")
	defer func() { tracing.End(ts, err) }()

	err = readColumns(ctx, id, map[string]any{
		"Realm": &realm,
	})
	if err != nil {
		return "", err
	}
	return realm, nil
}

// ReadState reads the state of the given work unit.
// If the work unit is not found, returns a NotFound appstatus error.
// Otherwise returns the internal error.
func ReadState(ctx context.Context, id ID) (state pb.WorkUnit_State, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadState")
	defer func() { tracing.End(ts, err) }()

	err = readColumns(ctx, id, map[string]any{
		"State": &state,
	})
	if err != nil {
		return 0, err
	}
	return state, nil
}

// ReadRequestIDAndCreatedBy reads the request id and createdBy of the given work unit.
// If the work unit is not found, returns a NotFound appstatus error.
// Otherwise returns the internal error.
func ReadRequestIDAndCreatedBy(ctx context.Context, id ID) (requestID string, createdBy string, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadRequestIDAndCreatedBy")
	defer func() { tracing.End(ts, err) }()

	err = readColumns(ctx, id, map[string]any{
		"CreateRequestId": &requestID,
		"CreatedBy":       &createdBy,
	})
	if err != nil {
		return "", "", err
	}
	return requestID, createdBy, nil
}

func validateID(id ID) error {
	if id.RootInvocationID == "" {
		return errors.New("rootInvocationID: unspecified")
	}
	if id.WorkUnitID == "" {
		return errors.New("workUnitID: unspecified")
	}
	return nil
}

func validateIDs(ids []ID) error {
	for i, id := range ids {
		if err := validateID(id); err != nil {
			return errors.Fmt("ids[%d]: %w", i, err)
		}
	}
	return nil
}

// ReadRealms reads the realm of the given work units. If any of the work
// units are not found, returns a NotFound appstatus error. Returned realms
// match 1:1 with the requested ids, i.e. result[i] corresponds to ids[i].
// Duplicate IDs are allowed.
func ReadRealms(ctx context.Context, ids []ID) (realms []string, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadRealms")
	defer func() { tracing.End(ts, err) }()

	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	realms = make([]string, len(ids))

	// No need to dedup keys going into Spanner, Cloud Spanner always behaves
	// as if the key is only specified once.
	var keys []spanner.Key
	for _, id := range ids {
		keys = append(keys, id.Key())
	}

	resultMap := make(map[ID]string, len(ids))

	var b spanutil.Buffer
	columns := []string{"RootInvocationShardId", "WorkUnitId", "Realm"}
	err = span.Read(ctx, "WorkUnits", spanner.KeySetFromKeys(keys...), columns).Do(func(r *spanner.Row) error {
		var rootInvocationShardID string
		var workUnitID string
		var realm string
		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID, &realm)
		if err != nil {
			return errors.Fmt("read spanner row for work unit: %w", err)
		}
		id := IDFromRowID(rootInvocationShardID, workUnitID)
		resultMap[id] = realm
		return nil
	})
	if err != nil {
		return nil, err
	}

	for i, id := range ids {
		realm, ok := resultMap[id]
		if !ok {
			return nil, appstatus.Errorf(codes.NotFound, "%q not found", id.Name())
		}
		realms[i] = realm
	}
	return realms, nil
}

// ReadStates reads the state of the given work units. If any of the work
// units are not found, returns a NotFound appstatus error. Returned state
// match 1:1 with the requested ids, i.e. result[i] corresponds to ids[i].
// Duplicate IDs are allowed.
func ReadStates(ctx context.Context, ids []ID) (states []pb.WorkUnit_State, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadStates")
	defer func() { tracing.End(ts, err) }()

	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	states = make([]pb.WorkUnit_State, len(ids))

	// No need to dedup keys going into Spanner, Cloud Spanner always behaves
	// as if the key is only specified once.
	var keys []spanner.Key
	for _, id := range ids {
		keys = append(keys, id.Key())
	}

	resultMap := make(map[ID]pb.WorkUnit_State, len(ids))

	var b spanutil.Buffer
	columns := []string{"RootInvocationShardId", "WorkUnitId", "State"}
	err = span.Read(ctx, "WorkUnits", spanner.KeySetFromKeys(keys...), columns).Do(func(r *spanner.Row) error {
		var rootInvocationShardID string
		var workUnitID string
		var state pb.WorkUnit_State
		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID, &state)
		if err != nil {
			return errors.Fmt("read spanner row for work unit: %w", err)
		}
		id := IDFromRowID(rootInvocationShardID, workUnitID)
		resultMap[id] = state
		return nil
	})
	if err != nil {
		return nil, err
	}

	for i, id := range ids {
		state, ok := resultMap[id]
		if !ok {
			return nil, appstatus.Errorf(codes.NotFound, "%q not found", id.Name())
		}
		states[i] = state
	}
	return states, nil
}

type RequestIDAndCreatedBy struct {
	RequestID string
	CreatedBy string
}

// ReadRequestIDsAndCreatedBys reads the requestID and createBy of the given work units.
// Returned results match 1:1 with the requested ids, i.e. results[i] corresponds to ids[i].
// A nil in results[i] indicate that ids[i] not found in spanner.
// Duplicate IDs are allowed.
func ReadRequestIDsAndCreatedBys(ctx context.Context, ids []ID) (results []*RequestIDAndCreatedBy, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadRequestIDsAndCreatedBys")
	defer func() { tracing.End(ts, err) }()

	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, err
	}

	// No need to dedup keys going into Spanner, Cloud Spanner always behaves
	// as if the key is only specified once.
	var keys []spanner.Key
	for _, id := range ids {
		keys = append(keys, id.Key())
	}
	resultMap := make(map[ID]RequestIDAndCreatedBy)

	var b spanutil.Buffer
	columns := []string{"RootInvocationShardId", "WorkUnitId", "CreateRequestId", "CreatedBy"}
	err = span.Read(ctx, "WorkUnits", spanner.KeySetFromKeys(keys...), columns).Do(func(r *spanner.Row) error {
		var rootInvocationShardID string
		var workUnitID string
		var requestID string
		var createdBy string
		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID, &requestID, &createdBy)
		if err != nil {
			return errors.Fmt("read spanner row for work unit: %w", err)
		}
		id := IDFromRowID(rootInvocationShardID, workUnitID)
		resultMap[id] = RequestIDAndCreatedBy{
			RequestID: requestID,
			CreatedBy: createdBy,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	results = make([]*RequestIDAndCreatedBy, len(ids))
	for i, id := range ids {
		r, ok := resultMap[id]
		if !ok {
			results[i] = nil
		} else {
			// Create a copy of r to avoid aliasing the same object in
			// result twice (in case of the same ID being requested twice),
			// as the caller might not expect aliasing.
			copyR := r
			results[i] = &copyR
		}
	}
	return results, nil
}

// ReadMask controls what fields to read.
type ReadMask int

const (
	// Read all work unit properties.
	AllFields ReadMask = iota
	// Read all work unit fields, except extended properties.
	// As extended properties can be quite large (megabytes per row), when
	// reading many rows this can be too much data to query.
	ExcludeExtendedProperties
)

// readBatchInternal reads multiple work units from Spanner.
func readBatchInternal(ctx context.Context, ids []ID, mask ReadMask, f func(wu *WorkUnitRow) error) error {
	if len(ids) == 0 {
		return nil
	}

	cols := []string{
		"RootInvocationShardId",
		"WorkUnitId",
		"ParentWorkUnitId",
		"SecondaryIndexShardId",
		"State",
		"Realm",
		"CreateTime",
		"CreatedBy",
		"FinalizeStartTime",
		"FinalizeTime",
		"Deadline",
		"CreateRequestId",
		"ProducerResource",
		"Tags",
		"Properties",
		"Instructions",
	}
	if mask == AllFields {
		cols = append(cols, "ExtendedProperties")
	}

	keys := spanner.KeySets()
	for _, id := range ids {
		keys = spanner.KeySets(keys, id.Key())
	}

	var b spanutil.Buffer
	return span.Read(ctx, "WorkUnits", keys, cols).Do(func(row *spanner.Row) error {
		wu := &WorkUnitRow{}

		var (
			rootInvocationShardID string
			workUnitID            string
			properties            spanutil.Compressed
			instructions          spanutil.Compressed
			extendedProperties    spanutil.Compressed
		)

		dest := []any{
			&rootInvocationShardID,
			&workUnitID,
			&wu.ParentWorkUnitID,
			&wu.SecondaryIndexShardID,
			&wu.State,
			&wu.Realm,
			&wu.CreateTime,
			&wu.CreatedBy,
			&wu.FinalizeStartTime,
			&wu.FinalizeTime,
			&wu.Deadline,
			&wu.CreateRequestID,
			&wu.ProducerResource,
			&wu.Tags,
			&properties,
			&instructions,
		}
		if mask == AllFields {
			dest = append(dest, &extendedProperties)
		}

		if err := b.FromSpanner(row, dest...); err != nil {
			return errors.Fmt("read spanner row for work unit: %w", err)
		}
		wu.ID = IDFromRowID(rootInvocationShardID, workUnitID)

		if len(properties) > 0 {
			wu.Properties = &structpb.Struct{}
			if err := proto.Unmarshal(properties, wu.Properties); err != nil {
				return errors.Fmt("unmarshal properties for work unit %s: %w", wu.ID.Name(), err)
			}
		}

		if len(instructions) > 0 {
			wu.Instructions = &pb.Instructions{}
			if err := proto.Unmarshal(instructions, wu.Instructions); err != nil {
				return errors.Fmt("unmarshal instructions for work unit %s: %w", wu.ID.Name(), err)
			}
			// Populate output-only fields.
			wu.Instructions = instructionutil.InstructionsWithNames(wu.Instructions, wu.ID.Name())
		}

		if len(extendedProperties) > 0 {
			internalExtendedProperties := &invocationspb.ExtendedProperties{}
			if err := proto.Unmarshal(extendedProperties, internalExtendedProperties); err != nil {
				return errors.Fmt("unmarshal extended properties for work unit %s: %w", wu.ID.Name(), err)
			}
			wu.ExtendedProperties = internalExtendedProperties.ExtendedProperties
		}

		return f(wu)
	})
}

// Read reads one work unit from Spanner.
// If the work unit does not exist, the returned error is annotated with
// NotFound GRPC code.
func Read(ctx context.Context, id ID, mask ReadMask) (ret *WorkUnitRow, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.Read")
	defer func() { tracing.End(ts, err) }()
	if err := validateID(id); err != nil {
		return nil, err
	}

	err = readBatchInternal(ctx, []ID{id}, mask, func(wu *WorkUnitRow) error {
		ret = wu
		return nil
	})

	switch {
	case err != nil:
		return nil, err
	case ret == nil:
		return nil, appstatus.Errorf(codes.NotFound, "%q not found", id.Name())
	default:
		return ret, nil
	}
}

// ReadBatch reads the given work units. If any of the work units are not found,
// returns a NotFound appstatus error. Returned realms match 1:1 with the requested
// ids, i.e. result[i] corresponds to ids[i].
// Duplicate IDs are allowed.
func ReadBatch(ctx context.Context, ids []ID, mask ReadMask) (ret []*WorkUnitRow, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadBatch")
	defer func() { tracing.End(ts, err) }()
	if err := validateIDs(ids); err != nil {
		return nil, err
	}

	resultMap := make(map[ID]*WorkUnitRow, len(ids))
	err = readBatchInternal(ctx, ids, mask, func(wu *WorkUnitRow) error {
		resultMap[wu.ID] = wu
		return nil
	})
	if err != nil {
		return nil, err
	}

	ret = make([]*WorkUnitRow, len(ids))
	for i, id := range ids {
		row, ok := resultMap[id]
		if !ok {
			return nil, appstatus.Errorf(codes.NotFound, "%q not found", id.Name())
		}
		// Clone the row to avoid aliasing the same result row object in
		// result twice (in case of the same ID being requested twice),
		// as the caller might not expect aliasing.
		ret[i] = row.Clone()
	}
	return ret, err
}

// ReadChildren reads the child work unit IDs of a work unit.
func ReadChildren(ctx context.Context, id ID) (ret []ID, err error) {
	results, err := ReadChildrenBatch(ctx, []ID{id})
	if err != nil {
		return nil, err
	}
	return results[0], nil
}

// ReadChildrenBatch reads the child work units IDs of a batch of work units.
// The work units must be in the same root invocation.
// results[i] corresponds to ids[i]. Duplicate IDs are allowed.
// This method does not check the existence of the work units for which children
// are being read.
func ReadChildrenBatch(ctx context.Context, ids []ID) (results [][]ID, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadChildrenBatch")
	defer func() { tracing.End(ts, err) }()

	if len(ids) == 0 {
		return nil, nil
	}

	rootInvocation := ids[0].RootInvocationID
	var workUnitIDs []string
	for _, id := range ids {
		if id.RootInvocationID != rootInvocation {
			return nil, errors.New("all work units must belong to the same root invocation")
		}
		workUnitIDs = append(workUnitIDs, id.WorkUnitID)
	}

	st := spanner.NewStatement(`
		SELECT
			RootInvocationShardId,
			ParentWorkUnitId,
			WorkUnitId
		FROM WorkUnits
		WHERE RootInvocationShardId IN UNNEST(@rootInvocationShardIds) AND ParentWorkUnitId IN UNNEST(@parentWorkUnitIds)`)

	st.Params = map[string]any{
		"rootInvocationShardIds": rootInvocation.AllShardIDs(),
		"parentWorkUnitIds":      workUnitIDs,
	}

	resultsByParent := make(map[ID][]ID)
	var b spanutil.Buffer
	err = spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var rootInvocationShardID string
		var parentWorkUnitID spanner.NullString
		var workUnitID string
		if err := b.FromSpanner(row, &rootInvocationShardID, &parentWorkUnitID, &workUnitID); err != nil {
			return err
		}
		if !parentWorkUnitID.Valid {
			return errors.New("logic error: this query design should never return the root (parentless) work unit")
		}
		parentID := IDFromRowID(rootInvocationShardID, parentWorkUnitID.StringVal)
		resultsByParent[parentID] = append(resultsByParent[parentID], IDFromRowID(rootInvocationShardID, workUnitID))
		return nil
	})
	if err != nil {
		return nil, err
	}

	results = make([][]ID, len(ids))
	for i, id := range ids {
		children := resultsByParent[id]
		// Copy the results to avoid returning slices which are aliased
		// (references to same object) in case the duplicate IDs are
		// provided in the request. This might trip the caller up if they
		// ever modify the slices.
		copiedChildren := make([]ID, len(children))
		copy(copiedChildren, children)

		// Sort children by WorkUnitID for deterministic output.
		// This is important for testing.
		sort.Slice(copiedChildren, func(j, k int) bool {
			return copiedChildren[j].WorkUnitID < copiedChildren[k].WorkUnitID
		})
		results[i] = copiedChildren
	}
	return results, err
}
