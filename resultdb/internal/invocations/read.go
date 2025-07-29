// Copyright 2020 The LUCI Authors.
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

package invocations

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ReadColumns reads the specified columns from an invocation Spanner row.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
// For ptrMap see ReadRow comment in span/util.go.
func ReadColumns(ctx context.Context, id ID, ptrMap map[string]any) error {
	if id == "" {
		return errors.New("id is unspecified")
	}
	err := spanutil.ReadRow(ctx, "Invocations", id.Key(), ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return appstatus.Attachf(err, codes.NotFound, "%s not found", id.Name())

	case err != nil:
		return errors.Fmt("failed to fetch %s: %w", id.Name(), err)

	default:
		return nil
	}
}

type ReadMask int

const (
	// Read all invocation properties.
	AllFields ReadMask = iota
	// Read all invocation fields, except extended properties. As it's max size,
	// i.e. pbutil.MaxSizeInvocationExtendedProperties will be increased, when
	// reading a large IDSet, the total size may be too big to fit into memory.
	// So we want to exclude this field in certain cases.
	ExcludeExtendedProperties
)

func readMulti(ctx context.Context, ids IDSet, mask ReadMask, f func(id ID, inv *pb.Invocation) error) error {
	if len(ids) == 0 {
		return nil
	}

	// Determine if extended properties should be included
	includeExtendedProperties := true
	if mask == ExcludeExtendedProperties {
		includeExtendedProperties = false
	}

	baseCols := []string{
		"i.InvocationId",
		"i.State",
		"i.CreatedBy",
		"i.CreateTime",
		"i.FinalizeStartTime",
		"i.FinalizeTime",
		"i.Deadline",
		"i.Tags",
		"i.IsExportRoot",
		"i.BigQueryExports",
		"ARRAY(SELECT IncludedInvocationId FROM IncludedInvocations incl WHERE incl.InvocationID = i.InvocationId)",
		"i.ProducerResource",
		"i.Realm",
		"i.Properties",
		"i.Sources",
		"i.InheritSources",
		"i.IsSourceSpecFinal",
		"i.BaselineId",
		"i.Instructions",
		"i.TestResultVariantUnion",
	}

	if includeExtendedProperties {
		baseCols = append(baseCols, "i.ExtendedProperties")
	}

	st := spanner.NewStatement(fmt.Sprintf(`
		SELECT
		 %s
		FROM Invocations i
		WHERE i.InvocationID IN UNNEST(@invIDs)
	`, strings.Join(baseCols, ",")))
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"invIDs": ids,
	})
	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var id ID
		included := IDSet{}
		inv := &pb.Invocation{}

		var (
			createdBy          spanner.NullString
			isExportRoot       spanner.NullBool
			producerResource   spanner.NullString
			realm              spanner.NullString
			properties         spanutil.Compressed
			sources            spanutil.Compressed
			inheritSources     spanner.NullBool
			isSourceSpecFinal  spanner.NullBool
			baselineID         spanner.NullString
			instructions       spanutil.Compressed
			extendedProperties spanutil.Compressed
		)

		dest := []any{
			&id,
			&inv.State,
			&createdBy,
			&inv.CreateTime,
			&inv.FinalizeStartTime,
			&inv.FinalizeTime,
			&inv.Deadline,
			&inv.Tags,
			&isExportRoot,
			&inv.BigqueryExports,
			&included,
			&producerResource,
			&realm,
			&properties,
			&sources,
			&inheritSources,
			&isSourceSpecFinal,
			&baselineID,
			&instructions,
			&inv.TestResultVariantUnion,
		}
		if includeExtendedProperties {
			dest = append(dest, &extendedProperties)
		}

		err := b.FromSpanner(row, dest...)
		if err != nil {
			return err
		}

		inv.Name = pbutil.InvocationName(string(id))
		inv.IncludedInvocations = included.Names()
		inv.CreatedBy = createdBy.StringVal
		inv.ProducerResource = producerResource.StringVal
		inv.Realm = realm.StringVal

		if len(properties) != 0 {
			inv.Properties = &structpb.Struct{}
			if err := proto.Unmarshal(properties, inv.Properties); err != nil {
				return err
			}
		}

		if inheritSources.Valid || len(sources) > 0 {
			inv.SourceSpec = &pb.SourceSpec{}
			inv.SourceSpec.Inherit = inheritSources.Valid && inheritSources.Bool
			if len(sources) != 0 {
				inv.SourceSpec.Sources = &pb.Sources{}
				if err := proto.Unmarshal(sources, inv.SourceSpec.Sources); err != nil {
					return err
				}
			}
		}

		if isExportRoot.Valid && isExportRoot.Bool {
			inv.IsExportRoot = true
		}

		if isSourceSpecFinal.Valid && isSourceSpecFinal.Bool {
			inv.IsSourceSpecFinal = true
		}

		if baselineID.Valid {
			inv.BaselineId = baselineID.StringVal
		}

		if len(instructions) > 0 {
			inv.Instructions = &pb.Instructions{}
			if err := proto.Unmarshal(instructions, inv.Instructions); err != nil {
				return err
			}
			inv.Instructions = instructionutil.InstructionsWithNames(inv.Instructions, id.Name())
		}

		// Conditionally process ExtendedProperties.
		// Only process if the mask included it AND data was non-empty.
		if includeExtendedProperties && len(extendedProperties) > 0 {
			internalExtendedProperties := &invocationspb.ExtendedProperties{}
			if err := proto.Unmarshal(extendedProperties, internalExtendedProperties); err != nil {
				return errors.Fmt("failed to unmarshal ExtendedProperties for invocation %q: %w", id, err)
			}
			inv.ExtendedProperties = internalExtendedProperties.ExtendedProperties
		}

		return f(id, inv)
	})
}

// Read reads one invocation from Spanner.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
func Read(ctx context.Context, id ID, mask ReadMask) (*pb.Invocation, error) {
	var ret *pb.Invocation
	err := readMulti(ctx, NewIDSet(id), mask, func(id ID, inv *pb.Invocation) error {
		ret = inv
		return nil
	})

	switch {
	case err != nil:
		return nil, err
	case ret == nil:
		return nil, appstatus.Errorf(codes.NotFound, "%s not found", id.Name())
	default:
		return ret, nil
	}
}

// ReadBatch reads multiple invocations from Spanner.
// If any of them are not found, returns an error.
func ReadBatch(ctx context.Context, ids IDSet, mask ReadMask) (map[ID]*pb.Invocation, error) {
	ret := make(map[ID]*pb.Invocation, len(ids))
	err := readMulti(ctx, ids, mask, func(id ID, inv *pb.Invocation) error {
		if _, ok := ret[id]; ok {
			panic("query is incorrect; it returned duplicated invocation IDs")
		}
		ret[id] = inv
		return nil
	})
	if err != nil {
		return nil, err
	}
	for id := range ids {
		if _, ok := ret[id]; !ok {
			return nil, appstatus.Errorf(codes.NotFound, "%s not found", id.Name())
		}
	}
	return ret, nil
}

// ReadState returns the invocation's state.
func ReadState(ctx context.Context, id ID) (pb.Invocation_State, error) {
	var state pb.Invocation_State
	err := ReadColumns(ctx, id, map[string]any{"State": &state})
	return state, err
}

// ReadStateBatch reads the states of multiple invocations.
func ReadStateBatch(ctx context.Context, ids IDSet) (map[ID]pb.Invocation_State, error) {
	ret := make(map[ID]pb.Invocation_State)
	err := span.Read(ctx, "Invocations", ids.Keys(), []string{"InvocationID", "State"}).Do(func(r *spanner.Row) error {
		var id ID
		var s pb.Invocation_State
		if err := spanutil.FromSpanner(r, &id, &s); err != nil {
			return errors.Fmt("failed to fetch %s: %w", ids, err)
		}
		ret[id] = s
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// ReadRealm returns the invocation's realm.
func ReadRealm(ctx context.Context, id ID) (string, error) {
	var realm string
	err := ReadColumns(ctx, id, map[string]any{"Realm": &realm})
	return realm, err
}

// QueryRealms returns the invocations' realms where available from the
// Invocations table.
// Makes a single RPC.
func QueryRealms(ctx context.Context, ids IDSet) (realms map[ID]string, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.invocations.QueryRealms",
		attribute.Int("cr.dev.count", len(ids)),
	)
	defer func() { tracing.End(ts, err) }()

	realms = map[ID]string{}
	st := spanner.NewStatement(`
		SELECT
			i.InvocationId,
			i.Realm
		FROM UNNEST(@invIDs) inv
		JOIN Invocations i
		ON i.InvocationId = inv`)
	st.Params = spanutil.ToSpannerMap(map[string]any{
		"invIDs": ids,
	})
	b := &spanutil.Buffer{}
	err = spanutil.Query(ctx, st, func(r *spanner.Row) error {
		var invocationID ID
		var realm spanner.NullString
		if err := b.FromSpanner(r, &invocationID, &realm); err != nil {
			return err
		}
		realms[invocationID] = realm.StringVal
		return nil
	})
	return realms, err
}

// ReadRealms returns the invocations' realms.
// Returns a NotFound error if unable to get the realm for any of the requested
// invocations.
// Makes a single RPC.
func ReadRealms(ctx context.Context, ids IDSet) (realms map[ID]string, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.invocations.ReadRealms",
		attribute.Int("cr.dev.count", len(ids)),
	)
	defer func() { tracing.End(ts, err) }()

	realms, err = QueryRealms(ctx, ids)
	if err != nil {
		return nil, err
	}

	// Return a NotFound error if ret is missing a requested invocation.
	for id := range ids {
		if _, ok := realms[id]; !ok {
			return nil, appstatus.Errorf(codes.NotFound, "%s not found", id.Name())
		}
	}
	return realms, nil
}

// InclusionKey returns a spanner key for an Inclusion row.
func InclusionKey(including, included ID) spanner.Key {
	return spanner.Key{including.RowID(), included.RowID()}
}

// ReadIncluded reads ids of (directly) included invocations.
func ReadIncluded(ctx context.Context, id ID) (IDSet, error) {
	var ret IDSet
	var b spanutil.Buffer
	err := span.Read(ctx, "IncludedInvocations", id.Key().AsPrefix(), []string{"IncludedInvocationId"}).Do(func(row *spanner.Row) error {
		var included ID
		if err := b.FromSpanner(row, &included); err != nil {
			return err
		}
		if ret == nil {
			ret = make(IDSet)
		}
		ret.Add(included)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// ReadSubmitted returns the invocation's submitted status.
func ReadSubmitted(ctx context.Context, id ID) (bool, error) {
	var submitted spanner.NullBool
	if err := ReadColumns(ctx, id, map[string]any{"Submitted": &submitted}); err != nil {
		return false, err
	}
	// submitted is not a required field and so may be nil, in which we default to false.
	return submitted.Valid && submitted.Bool, nil
}

// ExportInfo captures information pertinent to exporting an invocation and
// propogating export roots.
type ExportInfo struct {
	// Whether the invocation is in FINALIZED or FINALIZING state.
	IsInvocationFinal bool
	// The realm of the invocation.
	Realm string
	// IsSourcesSpecFinalEffective whether the source information is immutable.
	// This is true if either IsSourceSpecFinal is set on the invocation, or
	// the invocation is in FINALIZING or FINALIZED state.
	IsSourceSpecFinalEffective bool
	// IsInheritingSources contains whether the invocation is inheriting sources
	// from its parent (including) invocation.
	IsInheritingSources bool
	// Sources are the concrete sources specific on the invocation (if any).
	Sources *pb.Sources
}

// ReadExportInfo reads information pertinent to exporting an invocation.
func ReadExportInfo(ctx context.Context, invID ID) (ExportInfo, error) {
	var state pb.Invocation_State
	var realm string
	var inheritingSources spanner.NullBool
	var isSourceSpecFinal spanner.NullBool
	var sourceCmp spanutil.Compressed

	// Read invocation source spec, invocation finalization and source finalization.
	err := ReadColumns(ctx, invID, map[string]any{
		"State":             &state,
		"Realm":             &realm,
		"InheritSources":    &inheritingSources,
		"Sources":           &sourceCmp,
		"IsSourceSpecFinal": &isSourceSpecFinal,
	})
	if err != nil {
		return ExportInfo{}, errors.Fmt("read columns: %w", err)
	}

	var result ExportInfo
	result.IsInvocationFinal = state == pb.Invocation_FINALIZING || state == pb.Invocation_FINALIZED
	result.Realm = realm
	if result.IsInvocationFinal || (isSourceSpecFinal.Valid && isSourceSpecFinal.Bool) {
		result.IsSourceSpecFinalEffective = true
	}
	if inheritingSources.Valid && inheritingSources.Bool {
		result.IsInheritingSources = true
	}
	if len(sourceCmp) > 0 {
		result.Sources = &pb.Sources{}
		if err := proto.Unmarshal(sourceCmp, result.Sources); err != nil {
			return ExportInfo{}, errors.Fmt("unmarshal sources: %w", err)
		}
	}

	return result, nil
}

// FinalizedNotificationInfo captures information for sending an invocation finalized notification.
type FinalizedNotificationInfo struct {
	// The realm of the invocation.
	Realm string
	// Whether this invocation is a root of the invocation graph for export purposes.
	IsExportRoot bool
	// When the invocation was created.
	CreateTime *timestamppb.Timestamp
}

// ReadFinalizedNotificationInfo reads information for sending an invocation finalized notification.
func ReadFinalizedNotificationInfo(ctx context.Context, invID ID) (FinalizedNotificationInfo, error) {
	var isExportRoot spanner.NullBool
	var info FinalizedNotificationInfo

	if err := ReadColumns(ctx, invID, map[string]any{
		"Realm":        &info.Realm,
		"IsExportRoot": &isExportRoot,
		"CreateTime":   &info.CreateTime,
	}); err != nil {
		return FinalizedNotificationInfo{}, errors.Fmt("read columns: %w", err)
	}
	info.IsExportRoot = isExportRoot.Valid && isExportRoot.Bool
	return info, nil
}
