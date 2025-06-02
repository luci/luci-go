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

package verdictingester

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	antsinvocationexporter "go.chromium.org/luci/analysis/internal/ants/invocations/exporter"
	ctrlpb "go.chromium.org/luci/analysis/internal/ingestion/control/proto"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	bqpblegacy "go.chromium.org/luci/analysis/proto/bq/legacy"
)

func TestExportAntsInvocation(t *testing.T) {
	ftt.Run("TestExportTestVerdicts", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		partitionTime := clock.Now(ctx).Add(-1 * time.Hour)
		invocationCreationTime := partitionTime.Add(-3 * time.Hour)

		payload := &taskspb.IngestTestVerdicts{
			IngestionId: "ingestion-id",
			Project:     "android",
			Invocation: &ctrlpb.InvocationResult{
				ResultdbHost: "rdb-host",
				InvocationId: invocationID,
				CreationTime: timestamppb.New(invocationCreationTime),
			},
			PartitionTime:        timestamppb.New(partitionTime),
			PageToken:            "expected_token",
			TaskIndex:            1,
			UseNewIngestionOrder: true,
		}
		input := Inputs{
			Invocation: &rdbpb.Invocation{
				Name:         testInvocation,
				Realm:        testRealm,
				IsExportRoot: true,
				FinalizeTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			},
			Verdicts: mockedQueryTestVariantsRsp().TestVariants,
			Payload:  payload,
			LastPage: true,
		}
		antsInvocationClient := antsinvocationexporter.NewFakeClient()
		ingester := AnTSTInvocationExporter{exporter: antsinvocationexporter.NewExporter(antsInvocationClient)}
		t.Run("baseline", func(t *ftt.Test) {
			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			expectedRow := &bqpblegacy.AntsInvocationRow{
				InvocationId: "build-87654321",
				Timing: &bqpblegacy.AntsInvocationRow_Timing{
					CreationTimestamp: 0,
					CompleteTimestamp: 1744070400000,
				},
				CompletionTime: timestamppb.New(time.Date(2025, 4, 8, 0, 0, 0, 0, time.UTC)),
			}
			assert.Loosely(t, antsInvocationClient.Insertion, should.Match(expectedRow), truth.LineContext())
		})

		t.Run("not android project", func(t *ftt.Test) {
			payload.Project = "other"

			err := ingester.Ingest(ctx, input)
			assert.NoErr(t, err)
			assert.Loosely(t, antsInvocationClient.Insertion, should.BeNil)
		})
	})
}
