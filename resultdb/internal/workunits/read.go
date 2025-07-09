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
	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// readColumns reads the specified columns from a work unit Spanner row.
// If the work unit does not exist, the returned error is annotated with
// NotFound GRPC code.
// For ptrMap see ReadRow comment in span/util.go.
func readColumns(ctx context.Context, id ID, ptrMap map[string]any) error {
	if id.RootInvocationID == "" {
		return errors.New("rootInvocationID is unspecified")
	}
	if id.WorkUnitID == "" {
		return errors.New("workUnitID is unspecified")
	}

	err := spanutil.ReadRow(ctx, "WorkUnits", id.key(), ptrMap)
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return appstatus.Attachf(err, codes.NotFound, "%s not found", id.Name())

	case err != nil:
		return errors.Fmt("fetch %s: %w", id.Name(), err)

	default:
		return nil
	}
}

// readMulti reads multiple work units from Spanner.
func readMulti(ctx context.Context, ids []ID, f func(wu *WorkUnitRow) error) error {
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
		"ExtendedProperties",
	}

	keys := spanner.KeySets()
	for _, id := range ids {
		keys = spanner.KeySets(keys, id.key())
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
			&extendedProperties,
		}

		if err := b.FromSpanner(row, dest...); err != nil {
			return err
		}
		wu.ID = IDFromRowID(rootInvocationShardID, workUnitID)

		if len(properties) > 0 {
			wu.Properties = &structpb.Struct{}
			if err := proto.Unmarshal(properties, wu.Properties); err != nil {
				return err
			}
		}

		if len(instructions) > 0 {
			wu.Instructions = &pb.Instructions{}
			if err := proto.Unmarshal(instructions, wu.Instructions); err != nil {
				return err
			}
			wu.Instructions = instructionutil.InstructionsWithNames(wu.Instructions, wu.ID.Name())
		}

		if len(extendedProperties) > 0 {
			internalExtendedProperties := &invocationspb.ExtendedProperties{}
			if err := proto.Unmarshal(extendedProperties, internalExtendedProperties); err != nil {
				return errors.Fmt("unmarshal ExtendedProperties for work unit %s: %w", wu.ID.Name(), err)
			}
			wu.ExtendedProperties = internalExtendedProperties.ExtendedProperties
		}

		return f(wu)
	})
}

// Read reads one work unit from Spanner.
// If the work unit does not exist, the returned error is annotated with
// NotFound GRPC code.
func Read(ctx context.Context, id ID) (*WorkUnitRow, error) {
	var ret *WorkUnitRow
	err := readMulti(ctx, []ID{id}, func(wu *WorkUnitRow) error {
		ret = wu
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
