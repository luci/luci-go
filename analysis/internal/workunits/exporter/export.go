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

// Package exporter contains methods to export work units to BigQuery.
package exporter

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/bqutil"
	analysispbutil "go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
)

// InsertClient is the interface implemented by Client to insert rows into BigQuery.
type InsertClient interface {
	Insert(ctx context.Context, rows []*bqpb.WorkUnitRow, dest ExportDestination) error
}

// Exporter provides methods to export work units to BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter initialises a new Exporter instance that uses the given client
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

type Options struct {
	// The LUCI Project.
	Project string
}

func (e *Exporter) Export(ctx context.Context, notification *rdbpb.WorkUnitsNotification, dest ExportDestination, opts Options) error {
	// Use the same timestamp for all rows exported in the same batch.
	insertTime := clock.Now(ctx)

	rows, err := prepareExportRows(notification, opts, insertTime)
	if err != nil {
		return errors.Fmt("prepare rows: %w", err)
	}

	if len(rows) == 0 {
		return nil
	}

	if err := e.client.Insert(ctx, rows, dest); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

func prepareExportRows(notification *rdbpb.WorkUnitsNotification, opts Options, insertTime time.Time) ([]*bqpb.WorkUnitRow, error) {
	if opts.Project == "" {
		return nil, errors.New("project must be specified")
	}

	var out []*bqpb.WorkUnitRow

	for _, wuDetails := range notification.WorkUnits {
		wu := wuDetails.WorkUnit
		if wu == nil {
			return nil, errors.New("work_unit is missing in details")
		}

		rootInvID, wuID, err := pbutil.ParseWorkUnitName(wu.Name)
		if err != nil {
			return nil, errors.Fmt("parse work unit name %q: %w", wu.Name, err)
		}

		mergedPropertiesJSON, err := bqutil.MarshalStructPB(wuDetails.MergedInheritedProperties)
		if err != nil {
			return nil, errors.Fmt("marshal merged inherited properties: %w", err)
		}

		propertiesJSON, err := bqutil.MarshalStructPB(wu.Properties)
		if err != nil {
			return nil, errors.Fmt("marshal properties: %w", err)
		}

		inheritedPropertiesJSON, err := bqutil.MarshalStructPB(wu.InheritedProperties)
		if err != nil {
			return nil, errors.Fmt("marshal inherited properties: %w", err)
		}

		var bqModuleID *bqpb.ModuleIdentifier
		if wu.ModuleId != nil {
			variantJSON, err := bqutil.VariantJSON(wu.ModuleId.ModuleVariant)
			if err != nil {
				return nil, errors.Fmt("marshal module variant: %w", err)
			}
			bqModuleID = &bqpb.ModuleIdentifier{
				ModuleName:        wu.ModuleId.ModuleName,
				ModuleScheme:      wu.ModuleId.ModuleScheme,
				ModuleVariant:     variantJSON,
				ModuleVariantHash: wu.ModuleId.ModuleVariantHash,
			}
		}

		var parentWorkUnit string
		if _, _, ok := pbutil.TryParseWorkUnitName(wu.Parent); ok {
			parentWorkUnit = wu.Parent
		}

		out = append(out, &bqpb.WorkUnitRow{
			Project:                   opts.Project,
			RootInvocationId:          rootInvID,
			WorkUnitId:                wuID,
			Name:                      wu.Name,
			ParentWorkUnit:            parentWorkUnit,
			Kind:                      wu.Kind,
			State:                     wu.State,
			SummaryMarkdown:           wu.SummaryMarkdown,
			FinalizationState:         wu.FinalizationState,
			Realm:                     wu.Realm,
			CreateTime:                wu.CreateTime,
			LastUpdated:               wu.LastUpdated,
			FinalizeStartTime:         wu.FinalizeStartTime,
			FinalizeTime:              wu.FinalizeTime,
			Deadline:                  wu.Deadline,
			ModuleId:                  bqModuleID,
			ModuleShardKey:            wu.ModuleShardKey,
			ProducerResource:          wu.ProducerResource,
			Tags:                      analysispbutil.StringPairFromResultDB(wu.Tags),
			Properties:                propertiesJSON,
			InheritedProperties:       inheritedPropertiesJSON,
			MergedInheritedProperties: mergedPropertiesJSON,
			ChildWorkUnits:            wu.ChildWorkUnits,
			ChildInvocations:          wu.ChildInvocations,
			InsertTime:                timestamppb.New(insertTime),
			PartitionTime:             notification.GetRootInvocationMetadata().GetCreateTime(),
		})
	}
	return out, nil
}
