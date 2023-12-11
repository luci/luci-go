// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rules

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/rules/exporter"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

type exporterClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.FailureAssociationRulesHistoryRow) error
	// NewestLastUpdated get the largest value in the lastUpdated field from the BigQuery table.
	NewestLastUpdated(ctx context.Context) (bigquery.NullTimestamp, error)
}

// ExportRulesCron is the entry-point to the export failure association rules. It is triggered by a cron job configured in
// cron.yaml.
func ExportRulesCron(ctx context.Context, projectID string) error {
	client, err := exporter.NewClient(ctx, projectID)
	if err != nil {
		logging.Errorf(ctx, "failed to create client for project %s: %s", projectID, err)
		return err
	}
	defer client.Close()
	return exportRules(ctx, client, time.Now())
}

func exportRules(ctx context.Context, client exporterClient, exportTime time.Time) error {
	lastUpdate, err := client.NewestLastUpdated(ctx)
	if err != nil {
		logging.Errorf(ctx, "failed to get newest lastUpdate: %s", err)
		return err
	}
	deltaSince := StartingEpoch
	if lastUpdate.Valid {
		deltaSince = lastUpdate.Timestamp
	}
	txn := span.Single(ctx)
	updatedRules, err := ReadDeltaAllProjects(txn, deltaSince)
	if err != nil {
		logging.Errorf(ctx, "failed to read updated rules for time since %s: %s", deltaSince, err)
		return err
	}
	bqRules := []*bqpb.FailureAssociationRulesHistoryRow{}
	for _, r := range updatedRules {
		bqRules = append(bqRules, toFailureAssociationRulesHistoryRow(r, exportTime))
	}
	if err := client.Insert(ctx, bqRules); err != nil {
		logging.Errorf(ctx, "failed to insert updated rules: %s", err)
		return err
	}
	return nil
}

func toFailureAssociationRulesHistoryRow(rule *Entry, exportTime time.Time) *bqpb.FailureAssociationRulesHistoryRow {
	return &bqpb.FailureAssociationRulesHistoryRow{
		Name:                    fmt.Sprintf("projects/%s/rules/%s", rule.Project, rule.RuleID),
		Project:                 rule.Project,
		RuleId:                  rule.RuleID,
		RuleDefinition:          rule.RuleDefinition,
		PredicateLastUpdateTime: timestamppb.New(rule.PredicateLastUpdateTime),
		Bug: &bqpb.FailureAssociationRulesHistoryRow_Bug{
			System: rule.BugID.System,
			Id:     rule.BugID.ID,
		},
		IsActive:                            rule.IsActive,
		IsManagingBug:                       rule.IsManagingBug,
		IsManagingBugPriority:               rule.IsManagingBugPriority,
		IsManagingBugPriorityLastUpdateTime: timestamppb.New(rule.IsManagingBugPriorityLastUpdateTime),
		SourceCluster: &analysispb.ClusterId{
			Algorithm: rule.SourceCluster.Algorithm,
			Id:        rule.SourceCluster.ID,
		},
		BugManagementState:      ToExternalBugManagementStatePB(rule.BugManagementState),
		CreateTime:              timestamppb.New(rule.CreateTime),
		LastAuditableUpdateTime: timestamppb.New(rule.LastAuditableUpdateTime),
		LastUpdateTime:          timestamppb.New(rule.LastUpdateTime),
		ExportedTime:            timestamppb.New(exportTime),
	}
}
