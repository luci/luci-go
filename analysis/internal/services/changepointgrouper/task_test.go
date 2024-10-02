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

package changepointgrouper

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/changepoints/groupexporter"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestGroupChangepoints(t *testing.T) {
	ftt.Run("TestGroupChangepoints", t, func(t *ftt.Test) {
		ctx := context.Background()
		now := time.Date(2024, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)

		exportClient := groupexporter.NewFakeClient()
		changepointClient := fakeChangepointClient{}
		grouper := &changepointGrouper{
			changepointClient: &changepointClient,
			exporter:          *groupexporter.NewExporter(exportClient),
		}
		payload := &taskspb.GroupChangepoints{
			Week:         timestamppb.New(time.Date(2024, 9, 8, 0, 0, 0, 0, time.UTC)),
			ScheduleTime: timestamppb.New(now),
		}

		t.Run("drop old task", func(t *ftt.Test) {
			payload.ScheduleTime = timestamppb.New(now.Add(-11 * time.Minute))

			err := grouper.run(ctx, payload)
			assert.That(t, err, should.ErrLike("drop task older than 10 minutes"))
			assert.That(t, tq.Fatal.In(err), should.BeTrue)
		})
		t.Run("e2e", func(t *ftt.Test) {
			cp1 := makeChangepointRow(1, 2, 4, "chromium")
			cp2 := makeChangepointRow(2, 2, 3, "chromium")
			cp3 := makeChangepointRow(2, 10, 12, "chromium")
			cp4 := makeChangepointRow(1, 2, 4, "chromeos")
			changepointClient.ReadChangepointsResult = []*changepoints.ChangepointDetailRow{cp1, cp2, cp3, cp4}

			err := grouper.run(ctx, payload)
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, exportClient.Insertions, should.Match([]*bqpb.GroupedChangepointRow{
				makeGroupedChangepointRow(1, 2, 4, "chromeos", groupexporter.ChangepointKey(cp4)),
				makeGroupedChangepointRow(2, 2, 3, "chromium", groupexporter.ChangepointKey(cp1)),
				makeGroupedChangepointRow(1, 2, 4, "chromium", groupexporter.ChangepointKey(cp1)),
				makeGroupedChangepointRow(2, 10, 12, "chromium", groupexporter.ChangepointKey(cp3)),
			}))
		})
	})
}

func makeChangepointRow(testIDNum, lowerBound, upperBound int64, project string) *changepoints.ChangepointDetailRow {
	return &changepoints.ChangepointDetailRow{
		Project:     project,
		TestIDNum:   testIDNum,
		TestID:      fmt.Sprintf("test%d", testIDNum),
		VariantHash: "5097aaaaaaaaaaaa",
		Variant: bigquery.NullJSON{
			JSONVal: "{\"var\":\"abc\",\"varr\":\"xyx\"}",
			Valid:   true,
		},
		Ref: &changepoints.Ref{
			Gitiles: &changepoints.Gitiles{
				Host:    bigquery.NullString{Valid: true, StringVal: "host"},
				Project: bigquery.NullString{Valid: true, StringVal: "project"},
				Ref:     bigquery.NullString{Valid: true, StringVal: "ref"},
			},
		},
		RefHash:                            "b920ffffffffffff",
		UnexpectedSourceVerdictRateCurrent: 0,
		UnexpectedSourceVerdictRateAfter:   0.99,
		UnexpectedSourceVerdictRateBefore:  0.3,
		StartHour:                          time.Date(2024, time.April, 4, 0, 0, 0, 0, time.UTC),
		LowerBound99th:                     lowerBound,
		UpperBound99th:                     upperBound,
		NominalStartPosition:               (lowerBound + upperBound) / 2,
		PreviousNominalEndPosition:         100,
	}
}

func makeGroupedChangepointRow(testIDNum, lowerBound, upperBound int64, project, groupID string) *bqpb.GroupedChangepointRow {
	return &bqpb.GroupedChangepointRow{
		Project:     project,
		TestId:      fmt.Sprintf("test%d", testIDNum),
		VariantHash: "5097aaaaaaaaaaaa",
		RefHash:     "b920ffffffffffff",
		Variant:     "{\"var\":\"abc\",\"varr\":\"xyx\"}",
		Ref: &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "project",
					Ref:     "ref",
				},
			},
		},
		UnexpectedSourceVerdictRate:         0.99,
		PreviousUnexpectedSourceVerdictRate: 0.3,
		PreviousNominalEndPosition:          100,
		StartPosition:                       (lowerBound + upperBound) / 2,
		StartPositionLowerBound_99Th:        lowerBound,
		StartPositionUpperBound_99Th:        upperBound,
		StartHour:                           timestamppb.New(time.Date(2024, time.April, 4, 0, 0, 0, 0, time.UTC)),
		StartHourWeek:                       timestamppb.New(time.Date(2024, time.March, 31, 0, 0, 0, 0, time.UTC)),
		TestIdNum:                           testIDNum,
		GroupId:                             groupID,
		Version:                             timestamppb.New(time.Date(2024, time.April, 4, 4, 4, 4, 4, time.UTC)),
	}
}

type fakeChangepointClient struct {
	ReadChangepointsResult []*changepoints.ChangepointDetailRow
}

func (f *fakeChangepointClient) ReadChangepointsRealtime(ctx context.Context, week time.Time) ([]*changepoints.ChangepointDetailRow, error) {
	return f.ReadChangepointsResult, nil
}
