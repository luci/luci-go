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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
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
// units are not found, returns a NotFound appstatus error.
// Duplicate IDs are allowed.
func ReadRealms(ctx context.Context, ids []ID) (realms map[ID]string, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadRealms")
	defer func() { tracing.End(ts, err) }()

	var b spanutil.Buffer
	parseRow := func(r *spanner.Row) (ID, string, error) {
		var rootInvocationShardID string
		var workUnitID string
		var realm string
		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID, &realm)
		if err != nil {
			return ID{}, "", err
		}
		return IDFromRowID(rootInvocationShardID, workUnitID), realm, nil
	}

	resultMap, err := readRows(ctx, ids, []string{"Realm"}, parseRow)
	if err != nil {
		return nil, err
	}

	// Returns NotFound appstatus error if a row for an ID is missing.
	if err := expectIDs(resultMap, ids); err != nil {
		return nil, err
	}
	return resultMap, nil
}

// ReadStates reads the state of the given work units. If any of the work
// units are not found, returns a NotFound appstatus error. Returned state
// match 1:1 with the requested ids, i.e. result[i] corresponds to ids[i].
// Duplicate IDs are allowed.
func ReadStates(ctx context.Context, ids []ID) (states []pb.WorkUnit_State, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadStates")
	defer func() { tracing.End(ts, err) }()

	var b spanutil.Buffer
	columns := []string{"State"}
	parseRow := func(r *spanner.Row) (ID, pb.WorkUnit_State, error) {
		var rootInvocationShardID string
		var workUnitID string
		var state pb.WorkUnit_State
		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID, &state)
		if err != nil {
			return ID{}, pb.WorkUnit_STATE_UNSPECIFIED, err
		}
		return IDFromRowID(rootInvocationShardID, workUnitID), state, nil
	}

	resultMap, err := readRows(ctx, ids, columns, parseRow)
	if err != nil {
		return nil, err
	}
	// Returns NotFound appstatus error if a row for an ID is missing.
	return rowsForIDsMandatory(resultMap, ids)
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

	var b spanutil.Buffer
	columns := []string{"CreateRequestId", "CreatedBy"}
	parseRow := func(r *spanner.Row) (ID, RequestIDAndCreatedBy, error) {
		var rootInvocationShardID string
		var workUnitID string
		var requestID string
		var createdBy string
		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID, &requestID, &createdBy)
		if err != nil {
			return ID{}, RequestIDAndCreatedBy{}, err
		}
		result := RequestIDAndCreatedBy{
			RequestID: requestID,
			CreatedBy: createdBy,
		}
		return IDFromRowID(rootInvocationShardID, workUnitID), result, nil
	}

	resultMap, err := readRows(ctx, ids, columns, parseRow)
	if err != nil {
		return nil, err
	}
	return rowsForIDsOptional(resultMap, ids)
}

// TestResultInfo contains fields about the work unit that are useful to RPCs
// populating test results into the work unit.
type TestResultInfo struct {
	State pb.WorkUnit_State
	// The realm of the work unit.
	Realm string
	// The module associated with the work unit.
	ModuleID *pb.ModuleIdentifier
}

// ReadTestResultInfos reads the content info of the given work units.
// If any of the work units are not found, returns a NotFound appstatus error.
// Duplicate IDs are allowed.
func ReadTestResultInfos(ctx context.Context, ids []ID) (results map[ID]TestResultInfo, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.ReadContentInfo")
	defer func() { tracing.End(ts, err) }()

	var b spanutil.Buffer
	parseRow := func(r *spanner.Row) (ID, TestResultInfo, error) {
		var rootInvocationShardID string
		var workUnitID string
		var state pb.WorkUnit_State
		var realm string
		var moduleName spanner.NullString
		var moduleScheme spanner.NullString
		var moduleVariant *pb.Variant

		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID, &state, &realm, &moduleName, &moduleScheme, &moduleVariant)
		if err != nil {
			return ID{}, TestResultInfo{}, err
		}

		var moduleID *pb.ModuleIdentifier
		if moduleName.Valid != moduleScheme.Valid {
			panic("invariant violated: moduleName.Valid == moduleScheme.Valid, is there data corruption?")
		}
		if moduleName.Valid {
			moduleID = &pb.ModuleIdentifier{
				ModuleName:    moduleName.StringVal,
				ModuleScheme:  moduleScheme.StringVal,
				ModuleVariant: moduleVariant,
			}
			pbutil.PopulateModuleIdentifierHashes(moduleID)
		}

		result := TestResultInfo{
			State:    state,
			Realm:    realm,
			ModuleID: moduleID,
		}
		return IDFromRowID(rootInvocationShardID, workUnitID), result, nil
	}

	resultMap, err := readRows(ctx, ids, []string{"State", "Realm", "ModuleName", "ModuleScheme", "ModuleVariant"}, parseRow)
	if err != nil {
		return nil, err
	}
	// Expect all requested IDs are in the map, or return a NotFound appstatus error.
	if err := expectIDs(resultMap, ids); err != nil {
		return nil, err
	}
	return resultMap, nil
}

// readRows reads selected columns for each of the given work units.
// Duplicate IDs are allowed.
//
// If any of the referenced work units do not exist, this method returns a NotFound error.
//
// The given parseRow function is used to parse a Spanner row. It has the following
// contract:
//   - The provided Spanner row will have as columns 1 and 2 the RootInvocationShardID
//     and workUnitID, followed by the user-specified columns in `columns`.
//   - The function shall return this ID, alongside the user-specified row type and
//     any error encountered.
func readRows[T any](ctx context.Context, ids []ID, columns []string, parseRow func(row *spanner.Row) (ID, T, error)) (map[ID]T, error) {
	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// No need to dedup keys going into Spanner, Cloud Spanner always behaves
	// as if the key is only specified once.
	var keys []spanner.Key
	for _, id := range ids {
		keys = append(keys, id.Key())
	}

	resultMap := make(map[ID]T, len(ids))

	columns = append([]string{"RootInvocationShardId", "WorkUnitId"}, columns...)
	err := span.Read(ctx, "WorkUnits", spanner.KeySetFromKeys(keys...), columns).Do(func(r *spanner.Row) error {
		id, row, err := parseRow(r)
		if err != nil {
			return errors.Fmt("parse row: %w", err)
		}
		resultMap[id] = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resultMap, nil
}

// rowsForIDsMandatory converts the given result map to a slice.
// The result slice corresponds 1:1 with the expectedIDs collection,
// i.e. resultMap[i] matches results[i].
//
// In the case of duplicate IDs, the returned rows are shallow copies
// (in case rows containing reference types, this means aliasing).
//
// If one of the IDs is not found in the resultMap, a NotFound appstatus error
// is returned.
func rowsForIDsMandatory[T any](resultMap map[ID]T, expectedIDs []ID) ([]T, error) {
	results := make([]T, len(expectedIDs))
	for i, id := range expectedIDs {
		row, ok := resultMap[id]
		if !ok {
			return nil, appstatus.Errorf(codes.NotFound, "%q not found", id.Name())
		}
		results[i] = row
	}
	return results, nil
}

// rowsForIDsOptional converts the given result map to a slice.
// The result slice corresponds 1:1 with the expectedIDs collection,
// i.e. resultMap[i] matches results[i].
//
// In the case of duplicate IDs, the returned rows are shallow copies
// (in case rows containing reference types, this means aliasing).
//
// If one of the IDs is not found in the resultMap, the entry on the
// results slice is left as nil.
func rowsForIDsOptional[T any](resultMap map[ID]T, ids []ID) ([]*T, error) {
	results := make([]*T, len(ids))
	for i, id := range ids {
		row, ok := resultMap[id]
		if !ok {
			results[i] = nil
		} else {
			results[i] = &row
		}
	}
	return results, nil
}

// expectIDs expects that each of the IDs in expectIDs is present in the
// given resultMap. If not, it returns a NotFound appstatus error.
func expectIDs[T any](resultMap map[ID]T, expectedIDs []ID) error {
	for _, id := range expectedIDs {
		if _, ok := resultMap[id]; !ok {
			return appstatus.Errorf(codes.NotFound, "%q not found", id.Name())
		}
	}
	return nil
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

	extraCols := ""
	if mask == AllFields {
		extraCols = "			ExtendedProperties,\n"
	}

	stmt := spanner.NewStatement(`
		SELECT
			w.RootInvocationShardId,
			w.WorkUnitId,
			w.ParentWorkUnitId,
			w.SecondaryIndexShardId,
			w.State,
			w.Realm,
			w.CreateTime,
			w.CreatedBy,
			w.LastUpdated,
			w.FinalizeStartTime,
			w.FinalizeTime,
			w.Deadline,
			w.ModuleName,
			w.ModuleScheme,
			w.ModuleVariant,
			w.CreateRequestId,
			w.ProducerResource,
			w.Tags,
			w.Properties,
			w.Instructions,` + extraCols + `
			ARRAY(
				SELECT c.ChildWorkUnitId
				FROM ChildWorkUnits c WHERE c.RootInvocationShardId = w.RootInvocationShardId AND c.WorkUnitId = w.WorkUnitId
				ORDER BY c.ChildWorkUnitId
			) as ChildWorkUnits,
			ARRAY(
				SELECT c.ChildInvocationId
				FROM ChildInvocations c WHERE c.RootInvocationShardId = w.RootInvocationShardId AND c.WorkUnitId = w.WorkUnitId
			) as ChildInvocations
		FROM WorkUnits w
		WHERE STRUCT(w.RootInvocationShardId, w.WorkUnitId) IN UNNEST(@ids)
	`)

	// Struct to use as Spanner Query Parameter.
	type workUnitID struct {
		RootInvocationShardId string
		WorkUnitId            string
	}

	var workUnitIDs []workUnitID
	for _, id := range ids {
		workUnitIDs = append(workUnitIDs, workUnitID{
			RootInvocationShardId: id.RootInvocationShardID().RowID(),
			WorkUnitId:            id.WorkUnitID,
		})
	}
	stmt.Params = map[string]any{
		"ids": workUnitIDs,
	}

	var b spanutil.Buffer
	return span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		wu := &WorkUnitRow{}

		var (
			rootInvocationShardID string
			workUnitID            string
			properties            spanutil.Compressed
			instructions          spanutil.Compressed
			extendedProperties    spanutil.Compressed
			childWorkUnitIDs      []string
			childInvocations      invocations.IDSet
			moduleName            spanner.NullString
			moduleScheme          spanner.NullString
			moduleVariant         *pb.Variant
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
			&wu.LastUpdated,
			&wu.FinalizeStartTime,
			&wu.FinalizeTime,
			&wu.Deadline,
			&moduleName,
			&moduleScheme,
			&moduleVariant,
			&wu.CreateRequestID,
			&wu.ProducerResource,
			&wu.Tags,
			&properties,
			&instructions,
		}
		if mask == AllFields {
			dest = append(dest, &extendedProperties)
		}
		dest = append(dest,
			&childWorkUnitIDs,
			&childInvocations)

		if err := b.FromSpanner(row, dest...); err != nil {
			return errors.Fmt("read spanner row for work unit: %w", err)
		}
		wu.ID = IDFromRowID(rootInvocationShardID, workUnitID)

		if moduleName.Valid != moduleScheme.Valid {
			panic("invariant violated: moduleName.Valid == moduleScheme.Valid, is there data corruption?")
		}
		if moduleName.Valid {
			wu.ModuleID = &pb.ModuleIdentifier{
				ModuleName:    moduleName.StringVal,
				ModuleScheme:  moduleScheme.StringVal,
				ModuleVariant: moduleVariant,
			}
			pbutil.PopulateModuleIdentifierHashes(wu.ModuleID)
		}

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

		if len(childWorkUnitIDs) > 0 {
			wu.ChildWorkUnits = make([]ID, len(childWorkUnitIDs))
			for i, childWorkUnitID := range childWorkUnitIDs {
				wu.ChildWorkUnits[i] = ID{RootInvocationID: wu.ID.RootInvocationID, WorkUnitID: childWorkUnitID}
			}
		}

		if len(childInvocations) > 0 {
			wu.ChildInvocations = childInvocations.SortedByID()
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

// CheckWorkUnitUpdateRequestsExist checks if the given work units have already been updated by the given user with the given request ID.
// Returns a map of work unit IDs to a boolean indicating whether the update request exists.
func CheckWorkUnitUpdateRequestsExist(ctx context.Context, ids []ID, updatedBy string, requestID string) (exists map[ID]bool, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.workunits.CheckWorkUnitUpdateRequestsExist")
	defer func() { tracing.End(ts, err) }()

	if err := validateIDs(ids); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	// No need to dedup keys going into Spanner, Cloud Spanner always behaves
	// as if the key is only specified once.
	var keys []spanner.Key
	for _, id := range ids {
		keys = append(keys, id.Key(updatedBy, requestID))
	}
	var b spanutil.Buffer
	columns := []string{"RootInvocationShardId", "WorkUnitId"}
	exists = make(map[ID]bool)
	err = span.Read(ctx, "WorkUnitUpdateRequests", spanner.KeySetFromKeys(keys...), columns).Do(func(r *spanner.Row) error {
		var rootInvocationShardID string
		var workUnitID string
		err := b.FromSpanner(r, &rootInvocationShardID, &workUnitID)
		if err != nil {
			return err
		}
		readID := IDFromRowID(rootInvocationShardID, workUnitID)
		exists[readID] = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return exists, nil
}
