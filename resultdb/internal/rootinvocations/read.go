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

package rootinvocations

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tracing"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// readColumns reads the specified columns from a root invocation Spanner row.
// If the root invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
// For ptrMap see ReadRow comment in span/util.go.
func readColumns(ctx context.Context, id ID, ptrMap map[string]any) error {
	if id == "" {
		return errors.New("id is unspecified")
	}
	err := spanutil.ReadRow(ctx, "RootInvocations", id.Key(), ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return appstatus.Attachf(err, codes.NotFound, "%q not found", id.Name())

	case err != nil:
		return errors.Fmt("fetch %s: %w", id.Name(), err)

	default:
		return nil
	}
}

// readColumnsFromShard reads the specified columns from a root invocation shard Spanner row.
// If the root invocation shard does not exist, the returned error is annotated with
// NotFound GRPC code.
// For ptrMap see ReadRow comment in span/util.go.
func readColumnsFromShard(ctx context.Context, id ShardID, ptrMap map[string]any) error {
	if id.RootInvocationID == "" {
		return errors.New("root invocation id is unspecified")
	}
	err := spanutil.ReadRow(ctx, "RootInvocationShards", id.Key(), ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return appstatus.Attachf(err, codes.NotFound, "%q not found", id.RootInvocationID.Name())

	case err != nil:
		return errors.Fmt("fetch %s (shard %v): %w", id.RootInvocationID.Name(), id.ShardIndex, err)

	default:
		return nil
	}
}

// ReadFinalizationState reads the finalization state of the given root invocation.
// If the root invocation is not found, returns a NotFound appstatus error.
// Otherwise returns the internal error.
func ReadFinalizationState(ctx context.Context, id ID) (state pb.RootInvocation_FinalizationState, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/rootinvocations.ReadFinalizationState")
	defer func() { tracing.End(ts, err) }()

	err = readColumns(ctx, id, map[string]any{
		"FinalizationState": &state,
	})
	if err != nil {
		return 0, err
	}
	return state, nil
}

// ReadRealm reads the realm of the given root invocation. If the root invocation
// is not found, returns a NotFound appstatus error. Otherwise returns the internal
// error.
func ReadRealm(ctx context.Context, id ID) (realm string, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/rootinvocations.ReadRealm")
	defer func() { tracing.End(ts, err) }()

	err = readColumns(ctx, id, map[string]any{
		"Realm": &realm,
	})
	if err != nil {
		return "", err
	}
	return realm, nil
}

// ReadRequestIDAndCreatedBy reads the request id and createdBy of the given root invocation.
// If the root invocation is not found, returns a NotFound appstatus error.
// Otherwise returns the internal error.
func ReadRequestIDAndCreatedBy(ctx context.Context, id ID) (requestID string, createdBy string, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/rootinvocations.ReadRequestIDAndCreatedBy")
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

// FinalizerTaskState is the state of the work unit finalizer task.
type FinalizerTaskState struct {
	// Pending indicates whether there is a pending work unit finalizer task.
	Pending bool
	// The sequence number of the latest work unit finalizer task that has been scheduled.
	Sequence int64
}

// ReadFinalizerTaskState reads the state of work unit finalizer.
func ReadFinalizerTaskState(ctx context.Context, id ID) (taskState FinalizerTaskState, err error) {
	var pending bool
	var seq int64

	err = readColumns(ctx, id, map[string]any{
		"FinalizerPending":  &pending,
		"FinalizerSequence": &seq,
	})
	if err != nil {
		return FinalizerTaskState{}, err
	}
	return FinalizerTaskState{Pending: pending, Sequence: seq}, nil
}

// ReadRealmFromShard reads the realm of the given root invocation shard. If the
// root invocation is not found, returns a NotFound appstatus error. Otherwise
// returns the internal error.
//
// This will return identical results to ReadRealm but can be used to avoid hotspotting
// the root invocation record.
func ReadRealmFromShard(ctx context.Context, id ShardID) (realm string, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/rootinvocations.ReadRealmFromShard")
	defer func() { tracing.End(ts, err) }()

	err = readColumnsFromShard(ctx, id, map[string]any{
		"Realm": &realm,
	})
	if err != nil {
		return "", err
	}
	return realm, nil
}

// readMulti reads multiple root invocations from Spanner.
func readMulti(ctx context.Context, ids IDSet, f func(inv *RootInvocationRow) error) error {
	if len(ids) == 0 {
		return nil
	}
	cols := []string{
		"RootInvocationId",
		"SecondaryIndexShardId",
		"FinalizationState",
		"State",
		"SummaryMarkdown",
		"Realm",
		"CreateTime",
		"CreatedBy",
		"LastUpdated",
		"FinalizeStartTime",
		"FinalizeTime",
		"UninterestingTestVerdictsExpirationTime",
		"CreateRequestId",
		"ProducerResource",
		"Tags",
		"Properties",
		"Sources",
		"BaselineId",
		"StreamingExportState",
		"Submitted",
		"FinalizerPending",
		"FinalizerSequence",
	}
	var b spanutil.Buffer
	return span.Read(ctx, "RootInvocations", ids.Keys(), cols).Do(func(row *spanner.Row) error {
		inv := &RootInvocationRow{}
		var (
			properties spanutil.Compressed
			sources    spanutil.Compressed
		)

		dest := []any{
			&inv.RootInvocationID,
			&inv.SecondaryIndexShardID,
			&inv.FinalizationState,
			&inv.State,
			&inv.SummaryMarkdown,
			&inv.Realm,
			&inv.CreateTime,
			&inv.CreatedBy,
			&inv.LastUpdated,
			&inv.FinalizeStartTime,
			&inv.FinalizeTime,
			&inv.UninterestingTestVerdictsExpirationTime,
			&inv.CreateRequestID,
			&inv.ProducerResource,
			&inv.Tags,
			&properties,
			&sources,
			&inv.BaselineID,
			&inv.StreamingExportState,
			&inv.Submitted,
			&inv.FinalizerPending,
			&inv.FinalizerSequence,
		}

		if err := b.FromSpanner(row, dest...); err != nil {
			return err
		}

		if len(properties) > 0 {
			inv.Properties = &structpb.Struct{}
			if err := proto.Unmarshal(properties, inv.Properties); err != nil {
				return err
			}
		}

		if len(sources) > 0 {
			inv.Sources = &pb.Sources{}
			if err := proto.Unmarshal(sources, inv.Sources); err != nil {
				return err
			}
		}

		return f(inv)
	})
}

// Read reads one root invocation from Spanner.
// If the invocation does not exist, the returned error is annotated with
// NotFound GRPC code.
func Read(ctx context.Context, id ID) (row *RootInvocationRow, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/rootinvocations.Read")
	defer func() { tracing.End(ts, err) }()

	var ret *RootInvocationRow
	err = readMulti(ctx, NewIDSet(id), func(inv *RootInvocationRow) error {
		ret = inv
		return nil
	})

	switch {
	case err != nil:
		return nil, err
	case ret == nil:
		return nil, appstatus.Errorf(codes.NotFound, "%q not found", pbutil.RootInvocationName(string(id)))
	default:
		return ret, nil
	}
}

// CheckRootInvocationUpdateRequestExist checks if the given root invocation has already been updated by the given
// user with the given request ID.
func CheckRootInvocationUpdateRequestExist(ctx context.Context, id ID, updatedBy, requestID string) (exist bool, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/resultdb/internal/rootinvocations.CheckRootInvocationUpdateRequestExist")
	defer func() { tracing.End(ts, err) }()

	if id == "" {
		return false, errors.New("id is unspecified")
	}
	// ReadRow needs to read something for it to succeed, but we just want to check for the existence of the row
	// The value read is not used.
	var readID ID
	ptrMap := map[string]any{"RootInvocationId": &readID}
	err = spanutil.ReadRow(ctx, "RootInvocationUpdateRequests", id.Key(updatedBy, requestID), ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return false, nil
	case err != nil:
		return false, errors.Fmt("fetch RootInvocationUpdateRequests: %w", err)
	default:
		return true, nil
	}
}
