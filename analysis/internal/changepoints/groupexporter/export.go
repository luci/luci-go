// Copyright 2024 The LUCI Authors.
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

package groupexporter

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/changepoints"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// InsertClient defines an interface for inserting rows into BigQuery.
type InsertClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.GroupedChangepointRow) error
}

// Exporter provides methods to batch insert grouped changepoint row to BigQuery.
type Exporter struct {
	client InsertClient
}

// NewExporter instantiates a new Exporter. The given client is used
// to insert rows into BigQuery.
func NewExporter(client InsertClient) *Exporter {
	return &Exporter{client: client}
}

// Export flattens and exports the grouped changepoints to BigQuery.
func (e *Exporter) Export(ctx context.Context, groups [][]*changepoints.ChangepointRow, version time.Time) error {
	rows := prepareExportRows(groups, version)

	if err := e.client.Insert(ctx, rows); err != nil {
		return errors.Annotate(err, "insert rows").Err()
	}
	return nil
}

// prepareExportRows prepares BigQuery export rows for grouped changepoints.
func prepareExportRows(groups [][]*changepoints.ChangepointRow, version time.Time) []*bqpb.GroupedChangepointRow {
	rows := make([]*bqpb.GroupedChangepointRow, 0)
	for _, g := range groups {

		groupKey := changepointGroupID(g)
		for _, r := range g {
			row := &bqpb.GroupedChangepointRow{
				Project:     r.Project,
				TestId:      r.TestID,
				VariantHash: r.VariantHash,
				RefHash:     r.RefHash,
				Variant:     r.Variant.JSONVal,
				Ref: &pb.SourceRef{
					System: &pb.SourceRef_Gitiles{
						Gitiles: &pb.GitilesRef{
							Host:    r.Ref.Gitiles.Host.String(),
							Project: r.Ref.Gitiles.Project.String(),
							Ref:     r.Ref.Gitiles.Ref.String(),
						},
					},
				},
				UnexpectedSourceVerdictRate:         r.UnexpectedVerdictRateAfter,
				PreviousUnexpectedSourceVerdictRate: r.UnexpectedVerdictRateBefore,
				PreviousNominalEndPosition:          r.PreviousNominalEndPosition,
				StartPosition:                       r.NominalStartPosition,
				StartPositionLowerBound_99Th:        r.LowerBound99th,
				StartPositionUpperBound_99Th:        r.UpperBound99th,
				StartHour:                           timestamppb.New(r.StartHour),
				StartHourWeek:                       timestamppb.New(changepoints.StartOfWeek(r.StartHour)),
				TestIdNum:                           r.TestIDNum,
				GroupId:                             groupKey,
				Version:                             timestamppb.New(version),
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// changepointGroupID is a concatenation of the lexicographically smallest
// (test_id, variant_hash, ref_hash, start_position) tuple within this group.
func changepointGroupID(cps []*changepoints.ChangepointRow) string {
	minimum := cps[0]
	for _, cp := range cps {
		if changepoints.CompareTestVariantBranchChangepoint(cp, minimum) {
			minimum = cp
		}
	}
	return ChangepointKey(minimum)
}

func ChangepointKey(cp *changepoints.ChangepointRow) string {
	return fmt.Sprintf("%s-%s-%s-%s-%d", cp.Project, cp.TestID, cp.VariantHash, cp.RefHash, cp.NominalStartPosition)
}
