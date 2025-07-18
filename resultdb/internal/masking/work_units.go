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

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// WorkUnitFields represents the inputs required to construct a pb.WorkUnit.
type WorkUnitFields struct {
	// The work unit row.
	Row *workunits.WorkUnitRow
	// The child work units.
	ChildWorkUnits []workunits.ID
	// The child invocations.
	ChildInvocations []invocations.ID
}

// WorkUnit constructs a *pb.WorkUnit from the given fields, applying masking
// appropriate to the access level and selected view.
func WorkUnit(inputs WorkUnitFields, accessLevel permissions.AccessLevel, view pb.WorkUnitView) *pb.WorkUnit {
	if accessLevel == permissions.NoAccess {
		return nil
	}

	r := inputs.Row
	result := &pb.WorkUnit{
		// Include metadata-only fields by default.
		Name:             r.ID.Name(),
		WorkUnitId:       r.ID.WorkUnitID,
		State:            r.State,
		Realm:            r.Realm,
		CreateTime:       pbutil.MustTimestampProto(r.CreateTime),
		Creator:          r.CreatedBy,
		Deadline:         pbutil.MustTimestampProto(r.Deadline),
		ProducerResource: r.ProducerResource,
		IsMasked:         true,
	}
	result.ChildWorkUnits = make([]string, 0, len(inputs.ChildWorkUnits))
	for _, child := range inputs.ChildWorkUnits {
		result.ChildWorkUnits = append(result.ChildWorkUnits, child.Name())
	}
	result.ChildInvocations = make([]string, 0, len(inputs.ChildInvocations))
	for _, child := range inputs.ChildInvocations {
		// Hide legacy invocations that correspond to the work units above.
		if !child.IsWorkUnit() {
			result.ChildInvocations = append(result.ChildInvocations, child.Name())
		}
	}

	if accessLevel == permissions.FullAccess {
		result.Tags = r.Tags
		result.Properties = r.Properties
		result.Instructions = r.Instructions
		result.IsMasked = false

		if view == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
			result.ExtendedProperties = r.ExtendedProperties
		}
	}

	if r.ID.WorkUnitID == "root" {
		result.Parent = r.ID.RootInvocationID.Name()
	} else {
		if !r.ParentWorkUnitID.Valid {
			panic(fmt.Sprintf("invariant violated: parent work unit ID not set on non-root work unit %q", r.ID.Name()))
		}
		result.Parent = workunits.ID{
			RootInvocationID: r.ID.RootInvocationID,
			WorkUnitID:       r.ParentWorkUnitID.StringVal,
		}.Name()
	}
	if r.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(r.FinalizeStartTime.Time)
	}
	if r.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(r.FinalizeTime.Time)
	}
	return result
}
