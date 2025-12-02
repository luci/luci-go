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
	"regexp"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// limitedSummaryLength is the length to which the work unit's summary
// will be truncated for a TestResult when the caller only has
// limited access to the work unit.
const limitedSummaryLength = 140

// WorkUnit constructs a *pb.WorkUnit from the given fields, applying masking
// appropriate to the access level and selected view.
// The producer URL is computed based on the service configuration.
func WorkUnit(row *workunits.WorkUnitRow, accessLevel permissions.AccessLevel, view pb.WorkUnitView, cfg *config.CompiledServiceConfig) *pb.WorkUnit {
	if accessLevel == permissions.NoAccess {
		return nil
	}

	result := &pb.WorkUnit{
		// Include metadata-only fields by default.
		Name:              row.ID.Name(),
		WorkUnitId:        row.ID.WorkUnitID,
		Kind:              row.Kind,
		State:             row.State,
		FinalizationState: row.FinalizationState,
		Realm:             row.Realm,
		CreateTime:        pbutil.MustTimestampProto(row.CreateTime),
		Creator:           row.CreatedBy,
		LastUpdated:       pbutil.MustTimestampProto(row.LastUpdated),
		Deadline:          pbutil.MustTimestampProto(row.Deadline),
		ModuleShardKey:    row.ModuleShardKey,
		IsMasked:          true,
		Etag:              WorkUnitETag(row, accessLevel, view),
	}
	if row.ProducerResource != nil {
		// Clone to avoid modifying the original row.
		result.ProducerResource = proto.Clone(row.ProducerResource).(*pb.ProducerResource)
		result.ProducerResource.Url = producerResourceURL(result.ProducerResource, cfg)
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
		result.SummaryMarkdown = row.SummaryMarkdown
		result.Tags = row.Tags
		result.Properties = row.Properties
		result.Instructions = row.Instructions
		result.ModuleId = row.ModuleID
		result.IsMasked = false

		if view == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
			result.ExtendedProperties = row.ExtendedProperties
		}
	} else {
		// Include a truncated version of the summary.
		result.SummaryMarkdown = pbutil.TruncateString(row.SummaryMarkdown, limitedSummaryLength)
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

func producerResourceURL(pr *pb.ProducerResource, cfg *config.CompiledServiceConfig) string {
	if ps, ok := cfg.ProducerSystems[pr.System]; ok {
		url, err := ps.GenerateURL(pr)
		if err == nil {
			return url
		}
		// While the ProducerResource was valid at upload time, due to a
		// configuration change, it might be invalid now. It is better
		// to degrade gracefully and simply show no URL than fail the request.
		return ""
	}
	// The producer system was not be configured.
	return ""
}

// WorkUnitETag returns the HTTP ETag for the given work unit.
func WorkUnitETag(wu *workunits.WorkUnitRow, accessLevel permissions.AccessLevel, view pb.WorkUnitView) string {
	accessLevelFilter := ""
	switch accessLevel {
	case permissions.LimitedAccess:
		accessLevelFilter = "+l"
	case permissions.FullAccess:
		// Default case, keep accessLevelFilter empty.
	default:
		panic("invariant violated: invalid access level for etag generation")
	}

	viewFilter := ""
	switch view {
	case pb.WorkUnitView_WORK_UNIT_VIEW_BASIC, pb.WorkUnitView_WORK_UNIT_VIEW_UNSPECIFIED:
		// Default case, keep viewFilter empty.
	case pb.WorkUnitView_WORK_UNIT_VIEW_FULL:
		viewFilter = "+f"
	default:
		panic("invariant violated: invalid view for etag generation")
	}

	// The ETag must be a function of the resource representation according to (AIP-154).
	// The representation of a work unit depends on the access level and view,
	// so we include them in the ETag.
	return fmt.Sprintf(`W/"%s%s/%s"`, accessLevelFilter, viewFilter, wu.LastUpdated.UTC().Format(time.RFC3339Nano))
}

// etagRegexp extracts the work unit's last updated timestamp from a work unit ETag.
var etagRegexp = regexp.MustCompile(`^W/"(?:\+[lf])*/(.*)"$`)

// ParseWorkUnitETag validate the etag and returns the embedded lastUpdated time.
func ParseWorkUnitETag(etag string) (lastUpdated string, err error) {
	m := etagRegexp.FindStringSubmatch(etag)
	if len(m) < 2 {
		return "", errors.Fmt("malformated etag")
	}
	return m[1], nil
}

// IsWorkUnitETagMatch determines if the Etag is consistent with the specified
// work unit version.
func IsWorkUnitETagMatch(wu *workunits.WorkUnitRow, etag string) (bool, error) {
	lastUpdated, err := ParseWorkUnitETag(etag)
	if err != nil {
		return false, err
	}
	return lastUpdated == wu.LastUpdated.UTC().Format(time.RFC3339Nano), nil
}
