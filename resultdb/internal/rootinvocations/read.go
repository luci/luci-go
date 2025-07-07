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
		return appstatus.Attachf(err, codes.NotFound, "%s not found", id.Name())

	case err != nil:
		return errors.Fmt("fetch %s: %w", id.Name(), err)

	default:
		return nil
	}
}

// readMulti reads multiple root invocations from Spanner.
func readMulti(ctx context.Context, ids IDSet, f func(inv *RootInvocationRow) error) error {
	if len(ids) == 0 {
		return nil
	}
	cols := []string{
		"RootInvocationId",
		"ShardId",
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
			&inv.RootInvocationId,
			&inv.ShardId,
			&inv.State,
			&inv.Realm,
			&inv.CreateTime,
			&inv.CreatedBy,
			&inv.FinalizeStartTime,
			&inv.FinalizeTime,
			&inv.Deadline,
			&inv.UninterestingTestVerdictsExpirationTime,
			&inv.CreateRequestId,
			&inv.ProducerResource,
			&inv.Tags,
			&properties,
			&sources,
			&inv.IsSourcesFinal,
			&inv.BaselineId,
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
func Read(ctx context.Context, id ID) (*RootInvocationRow, error) {
	var ret *RootInvocationRow
	err := readMulti(ctx, NewIDSet(id), func(inv *RootInvocationRow) error {
		ret = inv
		return nil
	})

	switch {
	case err != nil:
		return nil, err
	case ret == nil:
		return nil, appstatus.Errorf(codes.NotFound, "%s not found", pbutil.RootInvocationName(string(id)))
	default:
		return ret, nil
	}
}
