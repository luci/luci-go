// Copyright 2026 The LUCI Authors.
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

package exporter

import (
	"context"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/ants/utils"
	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.AntsWorkUnitRow) error
}

// Exporter provides methods to export work units to BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter creates a new Exporter.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// ExportOptions captures context which will be exported
// alongside the work units.
type ExportOptions struct {
	RootInvocation *rdbpb.RootInvocation
}

// Export inserts the provided work units into BigQuery.
func (e *Exporter) Export(ctx context.Context, workUnits []*rdbpb.WorkUnit, opts ExportOptions) error {
	exportRows, err := prepareExportRows(workUnits, opts)
	if err != nil {
		return errors.Fmt("prepare rows: %w", err)
	}

	if err := e.client.Insert(ctx, exportRows); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

// prepareExportRows converts ResultDB WorkUnit protos to AntsWorkUnitRow BigQuery protos.
func prepareExportRows(workUnits []*rdbpb.WorkUnit, opts ExportOptions) ([]*bqpb.AntsWorkUnitRow, error) {
	rows := make([]*bqpb.AntsWorkUnitRow, 0, len(workUnits))
	for _, wu := range workUnits {
		row := &bqpb.AntsWorkUnitRow{
			// We instruct uploader to encode any semantic information currently captured in the AnTS `name` field in the
			// ResultdB work_unit_id.
			Name:         wu.WorkUnitId,
			Id:           wu.WorkUnitId,
			InvocationId: opts.RootInvocation.RootInvocationId,
			Timing: &bqpb.Timing{
				CreationTimestamp: wu.CreateTime.AsTime().UnixMilli(),
				CompleteTimestamp: wu.FinalizeTime.AsTime().UnixMilli(),
				CreationMonth:     wu.CreateTime.AsTime().Format("2006-01"),
			},
			State:      toSchedulerState(wu.State),
			Properties: utils.ConvertToAnTSStringPair(wu.Tags),
			Type:       wu.Kind,
			// Completion time is the finalize time of the root invocation.
			CompletionTime: opts.RootInvocation.FinalizeTime,
		}

		// Parent ID.
		if wu.Parent != "" {
			_, row.ParentId, _ = pbutil.ParseWorkUnitName(wu.Parent)
		}

		// Debug info
		if wu.State == rdbpb.WorkUnit_FAILED || wu.State == rdbpb.WorkUnit_CANCELLED {
			row.DebugInfo = &bqpb.DebugInfo{
				ErrorMessage: wu.SummaryMarkdown,
			}
		}

		// Skipped reason
		if wu.State == rdbpb.WorkUnit_SKIPPED {
			row.SkippedReason = &bqpb.SkippedReason{
				ReasonMessage: wu.SummaryMarkdown,
			}
		}

		// Aconfig flag overrides
		var err error
		if row.AconfigFlagOverrides, err = extractAconfigFlagOverrides(wu.Properties); err != nil {
			return nil, errors.Fmt("extract aconfig flag overrides: %w", err)
		}

		// Populate Module info if available
		if wu.ModuleId != nil {
			row.ModuleInfo = &bqpb.AntsWorkUnitRow_ModuleInfo{
				Name:             wu.ModuleId.ModuleName,
				ModuleParameters: utils.ModuleParametersFromVariants(wu.ModuleId.ModuleVariant),
			}
		}

		// Populate build info if available
		buildDesc := opts.RootInvocation.PrimaryBuild.GetAndroidBuild()
		if buildDesc != nil {
			row.BuildType = utils.BuildTypeFromBuildID(buildDesc.BuildId)
			row.BuildId = buildDesc.BuildId
			row.BuildProvider = "androidbuild"
			row.Branch = buildDesc.Branch
			row.BuildTarget = buildDesc.BuildTarget
		}

		rows = append(rows, row)
	}
	return rows, nil
}

func extractAconfigFlagOverrides(properties *structpb.Struct) (*bqpb.AconfigFlagOverrides, error) {
	if properties == nil {
		return nil, nil
	}
	propertiesType := "type.googleapis.com/wireless.android.busytown.proto.WorkUnitProperties"
	if properties.Fields["@type"].GetStringValue() != propertiesType {
		// Properties is not the supported WorkUnitProperties type.
		return nil, nil
	}
	// Marshal the field value to JSON.
	b, err := protojson.Marshal(properties)
	if err != nil {
		return nil, errors.Fmt("marshal properties to json: %w", err)
	}

	// Unmarshal JSON to the target proto.
	workUnitProperties := &bqpb.WorkUnitProperties{}
	unmarshaler := protojson.UnmarshalOptions{DiscardUnknown: true}
	if err := unmarshaler.Unmarshal(b, workUnitProperties); err != nil {
		return nil, errors.Fmt("unmarshal work unit properties from json: %w", err)
	}
	return workUnitProperties.AconfigFlagOverrides, nil
}

func toSchedulerState(state rdbpb.WorkUnit_State) bqpb.SchedulerState {
	switch state {
	case rdbpb.WorkUnit_PENDING:
		return bqpb.SchedulerState_SCHEDULER_STATE_PENDING
	case rdbpb.WorkUnit_RUNNING:
		return bqpb.SchedulerState_RUNNING
	case rdbpb.WorkUnit_SUCCEEDED:
		return bqpb.SchedulerState_COMPLETED
	case rdbpb.WorkUnit_FAILED:
		return bqpb.SchedulerState_ERROR
	case rdbpb.WorkUnit_CANCELLED:
		return bqpb.SchedulerState_CANCELLED
	case rdbpb.WorkUnit_SKIPPED:
		return bqpb.SchedulerState_SKIPPED
	default:
		return bqpb.SchedulerState_SCHEDULER_STATE_UNSPECIFIED
	}
}
