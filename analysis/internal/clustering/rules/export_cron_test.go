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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/analysis/internal/clustering/rules/exporter"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExportRules(t *testing.T) {
	Convey(`TestExportRules`, t, func() {
		exportTime, _ := time.Parse(time.RFC3339Nano, "2023-02-05T07:20:44.639000000Z")

		sortF := func(rows []*bqpb.FailureAssociationRulesHistoryRow) {
			sort.Slice(rows, func(i, j int) bool { return rows[i].RuleId < rows[j].RuleId })
		}

		Convey(`TestExportRulesFailedGetLastUpdated`, func() {
			mockClient := exporter.NewFakeClient().Errs([]error{errors.New("BOOM")})

			err := exportRules(testutil.IntegrationTestContext(t), mockClient, exportTime)
			So(err, ShouldErrLike, "BOOM")
		})

		Convey(`TestExportRulesFailedInsert`, func() {
			mockClient := exporter.NewFakeClient().Errs([]error{nil, errors.New("BOOM")})

			err := exportRules(testutil.IntegrationTestContext(t), mockClient, exportTime)
			So(err, ShouldErrLike, "BOOM")
		})

		Convey(`TestExportRulesEmptySpannerTable`, func() {
			mockClient := exporter.NewFakeClient()
			err := exportRules(testutil.IntegrationTestContext(t), mockClient, exportTime)

			So(err, ShouldBeNil)
			So(mockClient.Insertions, ShouldResembleProto, []*bqpb.FailureAssociationRulesHistoryRow{})
		})

		Convey(`TestExportRulesEmptyBqTable`, func() {
			mockClient := exporter.NewFakeClient()
			ctx := testutil.IntegrationTestContext(t)
			r1 := NewRule(1).Build()
			r2 := NewRule(2).Build()
			r3 := NewRule(3).Build()
			rulesInTable := []*FailureAssociationRule{r1, r2, r3}
			err := SetRulesForTesting(ctx, rulesInTable)
			So(err, ShouldBeNil)

			err = exportRules(ctx, mockClient, exportTime)
			So(err, ShouldBeNil)
			expected := []*bqpb.FailureAssociationRulesHistoryRow{
				toFailureAssociationRulesHistoryRow(r1, exportTime),
				toFailureAssociationRulesHistoryRow(r2, exportTime),
				toFailureAssociationRulesHistoryRow(r3, exportTime),
			}
			sortF(expected)
			sortF(mockClient.Insertions)
			So(mockClient.Insertions, ShouldResembleProto, expected)
		})

		Convey(`TestExportRulesInsertDeltaSinceLastUpdate`, func() {
			lastUpdatedTime, _ := time.Parse(time.RFC3339Nano, "2023-02-05T07:15:44.639000000Z")
			mockClient := exporter.NewFakeClient().LastUpdate(lastUpdatedTime)
			ctx := testutil.IntegrationTestContext(t)
			r1 := NewRule(1).WithLastUpdated(lastUpdatedTime.Add(-2 * time.Minute)).Build()
			r2 := NewRule(2).WithLastUpdated(lastUpdatedTime.Add(-2 * time.Minute)).Build()
			r3 := NewRule(3).WithLastUpdated(lastUpdatedTime.Add(time.Minute)).Build()
			r4 := NewRule(4).WithLastUpdated(lastUpdatedTime.Add(2 * time.Minute)).Build()
			rulesInTable := []*FailureAssociationRule{r1, r2, r3, r4}
			err := SetRulesForTesting(ctx, rulesInTable)
			So(err, ShouldBeNil)

			err = exportRules(ctx, mockClient, exportTime)
			So(err, ShouldBeNil)
			expected := []*bqpb.FailureAssociationRulesHistoryRow{
				toFailureAssociationRulesHistoryRow(r3, exportTime),
				toFailureAssociationRulesHistoryRow(r4, exportTime),
			}
			sortF(expected)
			sortF(mockClient.Insertions)
			So(mockClient.Insertions, ShouldResembleProto, expected)
		})
	})
}
