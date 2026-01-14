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

package exporter

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/ants/utils"
	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows *bqpb.AntsInvocationRow) error
}

// Exporter provides methods to stream rows into BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

const (
	// Tag key to record the runner field.
	runnerTagKey = "runner"
	// Tag key to record the trigger field.
	triggerTagKey = "trigger"
	// Tag key to record the users fidld, there can be multiple matches.
	userTagKey = "user"
	// Tag key to record the test_label field, there can be multiple matches.
	testLabelTagKey = "test_label"
	// Tag key to record device_tracing_enable flag.
	deviceTracingEnableTagKey = "device_tracing_enable"
)

// ExportLegacy exports the given invocation to BigQuery.
func (e *Exporter) ExportLegacy(ctx context.Context, inv *rdbpb.Invocation) error {
	invID, err := pbutil.ParseInvocationName(inv.Name)
	if err != nil {
		// Should not happen
		return errors.Fmt("parse invocation name: %w", err)
	}

	exportRow := &bqpb.AntsInvocationRow{
		InvocationId:   invID,
		SchedulerState: bqpb.SchedulerState_SCHEDULER_STATE_UNSPECIFIED,
		Timing: &bqpb.Timing{
			CreationTimestamp: inv.CreateTime.AsTime().UnixMilli(),
			CompleteTimestamp: inv.FinalizeTime.AsTime().UnixMilli(),
		},
		CompletionTime: inv.FinalizeTime,
	}
	if err := e.client.Insert(ctx, exportRow); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

// Export exports the given root invocation to BigQuery.
func (e *Exporter) Export(ctx context.Context, rootInv *rdbpb.RootInvocation) error {
	row, err := prepareExportRows(rootInv)
	if err != nil {
		return errors.Fmt("prepare rows: %w", err)
	}

	if err := e.client.Insert(ctx, row); err != nil {
		return errors.Fmt("insert rows: %w", err)
	}
	return nil
}

func prepareExportRows(rootInv *rdbpb.RootInvocation) (*bqpb.AntsInvocationRow, error) {
	rootInvID, err := pbutil.ParseRootInvocationName(rootInv.Name)
	if err != nil {
		// Should not happen
		return nil, errors.Fmt("parse root invocation name: %w", err)
	}

	exportRow := &bqpb.AntsInvocationRow{
		InvocationId:   rootInvID,
		SchedulerState: toSchedulerState(rootInv.State),
		Timing: &bqpb.Timing{
			CreationTimestamp: rootInv.CreateTime.AsTime().UnixMilli(),
			CompleteTimestamp: rootInv.FinalizeTime.AsTime().UnixMilli(),
		},
		TestDefinitionId: "", // TODO: replicate the hash.
		TestLabels:       utils.FindKeysFromTags(testLabelTagKey, rootInv.Tags),
		Properties:       utils.ConvertToAnTSStringPair(rootInv.Tags),
		Summary:          rootInv.SummaryMarkdown,
		Runner:           utils.FindKeyFromTags(runnerTagKey, rootInv.Tags),
		Trigger:          utils.FindKeyFromTags(triggerTagKey, rootInv.Tags),
		Scheduler:        rootInv.ProducerResource.System,
		Users:            users(rootInv),
		Tags:             []string{},
		CompletionTime:   rootInv.FinalizeTime,
	}

	// Primary build.
	if rootInv.PrimaryBuild != nil {
		exportRow.PrimaryBuild = toAntsAndroidBuild(rootInv.PrimaryBuild)
		if exportRow.PrimaryBuild != nil {
			exportRow.BuildId = exportRow.PrimaryBuild.BuildId
		}
	}

	// Extra builds.
	if len(rootInv.ExtraBuilds) > 0 {
		exportRow.ExtraBuilds = make([]*bqpb.AntsInvocationRow_AndroidBuild, 0, len(rootInv.ExtraBuilds))
		for _, b := range rootInv.ExtraBuilds {
			if ab := toAntsAndroidBuild(b); ab != nil {
				exportRow.ExtraBuilds = append(exportRow.ExtraBuilds, ab)
			}
		}
	}
	// Test definition.
	if rootInv.Definition != nil {
		exportRow.Test = &bqpb.Test{
			Name:       rootInv.Definition.Name,
			Properties: utils.ConvertMapToAnTSStringPair(rootInv.Definition.Properties.Def),
		}
	}

	// Tags.
	deviceTracingEnable := utils.FindKeyFromTags(deviceTracingEnableTagKey, rootInv.Tags)
	if deviceTracingEnable == "true" {
		exportRow.Tags = append(exportRow.Tags, "device_tracing_enable")
	}
	if rootInv.ProducerResource.System == "tradefed" {
		exportRow.Tags = append(exportRow.Tags, "tf_invocation")
	}
	return exportRow, nil
}

func toAntsAndroidBuild(bd *rdbpb.BuildDescriptor) *bqpb.AntsInvocationRow_AndroidBuild {
	ab := bd.GetAndroidBuild()
	if ab == nil {
		return nil
	}
	return &bqpb.AntsInvocationRow_AndroidBuild{
		BuildProvider: "androidbuild",
		Branch:        ab.Branch,
		BuildTarget:   ab.BuildTarget,
		BuildId:       ab.BuildId,
	}
}

func toSchedulerState(state rdbpb.RootInvocation_State) bqpb.SchedulerState {
	switch state {
	case rdbpb.RootInvocation_PENDING:
		return bqpb.SchedulerState_SCHEDULER_STATE_PENDING
	case rdbpb.RootInvocation_RUNNING:
		return bqpb.SchedulerState_RUNNING
	case rdbpb.RootInvocation_SUCCEEDED:
		return bqpb.SchedulerState_COMPLETED
	case rdbpb.RootInvocation_FAILED:
		return bqpb.SchedulerState_ERROR
	case rdbpb.RootInvocation_CANCELLED:
		return bqpb.SchedulerState_CANCELLED
	case rdbpb.RootInvocation_SKIPPED:
		return bqpb.SchedulerState_SKIPPED
	default:
		return bqpb.SchedulerState_SCHEDULER_STATE_UNSPECIFIED
	}
}

func users(rootInv *rdbpb.RootInvocation) []string {
	users := utils.FindKeysFromTags(userTagKey, rootInv.Tags)
	if rootInv.Creator != "" {
		users = append(users, rootInv.Creator)
	}
	return users
}
