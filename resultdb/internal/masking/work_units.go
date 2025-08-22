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

// Package masking provides methods for converting data access layer records
// to service response protos, masking the returned data based on the user's
// access level and selected view.
package masking

import (
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// WorkUnit constructs a *pb.WorkUnit from the given fields, applying masking
// appropriate to the access level and selected view.
func WorkUnit(row *workunits.WorkUnitRow, accessLevel permissions.AccessLevel, view pb.WorkUnitView) *pb.WorkUnit {
	if accessLevel == permissions.NoAccess {
		return nil
	}

	result := &pb.WorkUnit{
		// Include metadata-only fields by default.
		Name:             row.ID.Name(),
		WorkUnitId:       row.ID.WorkUnitID,
		State:            row.State,
		Realm:            row.Realm,
		CreateTime:       pbutil.MustTimestampProto(row.CreateTime),
		Creator:          row.CreatedBy,
		LastUpdated:      pbutil.MustTimestampProto(row.LastUpdated),
		Deadline:         pbutil.MustTimestampProto(row.Deadline),
		ProducerResource: row.ProducerResource,
		IsMasked:         true,
	}
	result.ChildWorkUnits = make([]string, 0, len(row.ChildWorkUnits))
	for _, child := range row.ChildWorkUnits {
		result.ChildWorkUnits = append(result.ChildWorkUnits, child.Name())
	}
	result.ChildInvocations = make([]string, 0, len(row.ChildInvocations))
	for _, child := range row.ChildInvocations {
		result.ChildInvocations = append(result.ChildInvocations, child.Name())
	}

	if accessLevel == permissions.FullAccess {
		result.Tags = row.Tags
		result.Properties = row.Properties
		result.Instructions = row.Instructions
		result.ModuleId = row.ModuleID
		result.IsMasked = false

		if view == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
			result.ExtendedProperties = row.ExtendedProperties
		}
	} else {
		// Include a masked version of the module identifier.
		if row.ModuleID != nil {
			moduleID := proto.Clone(row.ModuleID).(*pb.ModuleIdentifier)
			moduleID.ModuleVariant = nil
			result.ModuleId = moduleID
		}
	}

	if row.ID.WorkUnitID == "root" {
		result.Parent = row.ID.RootInvocationID.Name()
	} else {
		if !row.ParentWorkUnitID.Valid {
			panic(fmt.Sprintf("invariant violated: parent work unit ID not set on non-root work unit %q", row.ID.Name()))
		}
		result.Parent = workunits.ID{
			RootInvocationID: row.ID.RootInvocationID,
			WorkUnitID:       row.ParentWorkUnitID.StringVal,
		}.Name()
	}
	if row.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(row.FinalizeStartTime.Time)
	}
	if row.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(row.FinalizeTime.Time)
	}
	return result
}
