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

// ReadState reads the state of the given root invocation.
// If the root invocation is not found, returns a NotFound appstatus error.
// Otherwise returns the internal error.
func ReadState(ctx context.Context, id ID) (state pb.RootInvocation_State, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.rootinvocations.ReadState")
	defer func() { tracing.End(ts, err) }()

	err = readColumns(ctx, id, map[string]any{
		"State": &state,
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
	ctx, ts := tracing.Start(ctx, "resultdb.rootinvocations.ReadRealm")
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
	ctx, ts := tracing.Start(ctx, "resultdb.rootinvocations.ReadRequestIDAndCreatedBy")
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

// ReadRealmFromShard reads the realm of the given root invocation shard. If the
// root invocation is not found, returns a NotFound appstatus error. Otherwise
// returns the internal error.
//
// This will return identical results to ReadRealm but can be used to avoid hotspotting
// the root invocation record.
func ReadRealmFromShard(ctx context.Context, id ShardID) (realm string, err error) {
	ctx, ts := tracing.Start(ctx, "resultdb.rootinvocations.ReadRealmFromShard")
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
		"State",
		"Realm",
		"CreateTime",
		"CreatedBy",
		"FinalizeStartTime",
		"FinalizeTime",
		"Deadline",
		"UninterestingTestVerdictsExpirationTime",
		"CreateRequestId",
		"ProducerResource",
		"Tags",
		"Properties",
		"Sources",
		"IsSourcesFinal",
		"BaselineId",
		"Submitted",
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
			&inv.State,
			&inv.Realm,
			&inv.CreateTime,
			&inv.CreatedBy,
			&inv.FinalizeStartTime,
			&inv.FinalizeTime,
			&inv.Deadline,
			&inv.UninterestingTestVerdictsExpirationTime,
			&inv.CreateRequestID,
			&inv.ProducerResource,
			&inv.Tags,
			&properties,
			&sources,
			&inv.IsSourcesFinal,
			&inv.BaselineID,
			&inv.Submitted,
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
	ctx, ts := tracing.Start(ctx, "resultdb.rootinvocations.Read")
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
