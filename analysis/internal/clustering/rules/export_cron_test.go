// Copyright 2023 The LUCI Authors.
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

package rules

import (
	"errors"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules/exporter"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	analysispb "go.chromium.org/luci/analysis/proto/v1"
)

func TestExportRules(t *testing.T) {
	ftt.Run(`TestExportRules`, t, func(t *ftt.Test) {
		exportTime, _ := time.Parse(time.RFC3339Nano, "2023-02-05T07:20:44.639000000Z")

		sortF := func(rows []*bqpb.FailureAssociationRulesHistoryRow) {
			sort.Slice(rows, func(i, j int) bool { return rows[i].RuleId < rows[j].RuleId })
		}

		t.Run(`failed to get last updated`, func(t *ftt.Test) {
			mockClient := exporter.NewFakeClient().Errs([]error{errors.New("BOOM")})

			err := exportRules(testutil.IntegrationTestContext(t), mockClient, exportTime)
			assert.Loosely(t, err, should.ErrLike("BOOM"))
		})

		t.Run(`failed to insert`, func(t *ftt.Test) {
			mockClient := exporter.NewFakeClient().Errs([]error{nil, errors.New("BOOM")})

			err := exportRules(testutil.IntegrationTestContext(t), mockClient, exportTime)
			assert.Loosely(t, err, should.ErrLike("BOOM"))
		})

		t.Run(`empty Spanner rules table`, func(t *ftt.Test) {
			mockClient := exporter.NewFakeClient()
			err := exportRules(testutil.IntegrationTestContext(t), mockClient, exportTime)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, mockClient.Insertions, should.Resemble([]*bqpb.FailureAssociationRulesHistoryRow{}))
		})

		t.Run(`empty BigQuery table`, func(t *ftt.Test) {
			mockClient := exporter.NewFakeClient()
			ctx := testutil.IntegrationTestContext(t)
			r1 := NewRule(1).Build()
			r2 := NewRule(2).Build()
			r3 := NewRule(3).Build()
			rulesInTable := []*Entry{r1, r2, r3}
			err := SetForTesting(ctx, t, rulesInTable)
			assert.Loosely(t, err, should.BeNil)

			err = exportRules(ctx, mockClient, exportTime)
			assert.Loosely(t, err, should.BeNil)
			expected := []*bqpb.FailureAssociationRulesHistoryRow{
				toFailureAssociationRulesHistoryRow(r1, exportTime),
				toFailureAssociationRulesHistoryRow(r2, exportTime),
				toFailureAssociationRulesHistoryRow(r3, exportTime),
			}
			sortF(expected)
			sortF(mockClient.Insertions)
			assert.Loosely(t, mockClient.Insertions, should.Resemble(expected))
		})

		t.Run(`incremental export`, func(t *ftt.Test) {
			lastUpdatedTime, _ := time.Parse(time.RFC3339Nano, "2023-02-05T07:15:44.639000000Z")
			mockClient := exporter.NewFakeClient().LastUpdate(lastUpdatedTime)
			ctx := testutil.IntegrationTestContext(t)
			r1 := NewRule(1).WithLastUpdateTime(lastUpdatedTime.Add(-2 * time.Minute)).Build()
			r2 := NewRule(2).WithLastUpdateTime(lastUpdatedTime.Add(-2 * time.Minute)).Build()
			r3 := NewRule(3).WithLastUpdateTime(lastUpdatedTime.Add(time.Minute)).Build()
			r4 := NewRule(4).WithLastUpdateTime(lastUpdatedTime.Add(2 * time.Minute)).Build()
			rulesInTable := []*Entry{r1, r2, r3, r4}
			err := SetForTesting(ctx, t, rulesInTable)
			assert.Loosely(t, err, should.BeNil)

			err = exportRules(ctx, mockClient, exportTime)
			assert.Loosely(t, err, should.BeNil)
			expected := []*bqpb.FailureAssociationRulesHistoryRow{
				toFailureAssociationRulesHistoryRow(r3, exportTime),
				toFailureAssociationRulesHistoryRow(r4, exportTime),
			}
			sortF(expected)
			sortF(mockClient.Insertions)
			assert.Loosely(t, mockClient.Insertions, should.Resemble(expected))
		})
	})
	// The behaviour of this method is assumed in the tests above.
	ftt.Run(`toFailureAssociationRulesHistoryRow`, t, func(t *ftt.Test) {
		r := NewRule(1).
			WithProject("myproject").
			WithRuleID("11111111111111111111111111111111").
			WithRuleDefinition(`reason LIKE "%definition%"`).
			WithPredicateLastUpdateTime(time.Date(2001, 1, 1, 1, 1, 1, 1, time.UTC)).
			WithBug(bugs.BugID{System: bugs.BuganizerSystem, ID: "1"}).
			WithActive(true).
			WithBugManaged(true).
			WithBugPriorityManaged(true).
			WithBugPriorityManagedLastUpdateTime(time.Date(2002, 1, 1, 1, 1, 1, 1, time.UTC)).
			WithSourceCluster(clustering.ClusterID{
				Algorithm: "alg-1",
				ID:        "123456",
			}).
			WithBugManagementState(&bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"policy-a": {
						IsActive:           true,
						LastActivationTime: timestamppb.New(time.Date(1911, 1, 1, 1, 1, 1, 1, time.UTC)),
						ActivationNotified: true,
					},
				},
			}).
			WithCreateTime(time.Date(2003, 1, 1, 1, 1, 1, 1, time.UTC)).
			WithLastAuditableUpdateTime(time.Date(2004, 1, 1, 1, 1, 1, 1, time.UTC)).
			WithLastUpdateTime(time.Date(2005, 1, 1, 1, 1, 1, 1, time.UTC)).
			Build()

		exportTime, _ := time.Parse(time.RFC3339Nano, "2023-02-06T08:11:22.139000000Z")
		expected := &bqpb.FailureAssociationRulesHistoryRow{
			Name:                    "projects/myproject/rules/11111111111111111111111111111111",
			Project:                 "myproject",
			RuleId:                  "11111111111111111111111111111111",
			RuleDefinition:          `reason LIKE "%definition%"`,
			PredicateLastUpdateTime: timestamppb.New(time.Date(2001, 1, 1, 1, 1, 1, 1, time.UTC)),
			Bug: &bqpb.FailureAssociationRulesHistoryRow_Bug{
				System: "buganizer",
				Id:     "1",
			},
			IsActive:                            true,
			IsManagingBug:                       true,
			IsManagingBugPriority:               true,
			IsManagingBugPriorityLastUpdateTime: timestamppb.New(time.Date(2002, 1, 1, 1, 1, 1, 1, time.UTC)),
			SourceCluster: &analysispb.ClusterId{
				Algorithm: "alg-1",
				Id:        "123456",
			},
			BugManagementState: &analysispb.BugManagementState{
				PolicyState: []*analysispb.BugManagementState_PolicyState{
					{
						PolicyId:           "policy-a",
						IsActive:           true,
						LastActivationTime: timestamppb.New(time.Date(1911, 1, 1, 1, 1, 1, 1, time.UTC)),
					},
				},
			},
			CreateTime:              timestamppb.New(time.Date(2003, 1, 1, 1, 1, 1, 1, time.UTC)),
			LastAuditableUpdateTime: timestamppb.New(time.Date(2004, 1, 1, 1, 1, 1, 1, time.UTC)),
			LastUpdateTime:          timestamppb.New(time.Date(2005, 1, 1, 1, 1, 1, 1, time.UTC)),
			ExportedTime:            timestamppb.New(exportTime),
		}
		assert.Loosely(t, toFailureAssociationRulesHistoryRow(r, exportTime), should.Resemble(expected))
	})
}
