// Copyright 2022 The LUCI Authors.
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

package verdicts

import (
	"context"
	"fmt"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/server/span"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/analysis/internal"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
)

func statusCalculationDuration(du *durationpb.Duration) int {
	return int(du.AsDuration().Hours())
}

// ComputeTestVariantStatusFromVerdicts computes the test variant's status based
// on its verdicts within a time range.
//
// Currently the time range is the past one day, but should be configurable.
// TODO(crbug.com/1259374): Use the value in configurations.
func ComputeTestVariantStatusFromVerdicts(ctx context.Context, tvKey *taskspb.TestVariantKey, du *durationpb.Duration) (atvpb.Status, error) {
	st := spanner.NewStatement(`
		SELECT Status
		FROM Verdicts@{FORCE_INDEX=VerdictsByTestVariantAndIngestionTime, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE Realm = @realm
		AND TestId = @testID
		AND VariantHash = @variantHash
		AND IngestionTime >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL @numHours HOUR)
	`)
	st.Params = map[string]interface{}{
		"realm":       tvKey.Realm,
		"testID":      tvKey.TestId,
		"variantHash": tvKey.VariantHash,
		"numHours":    statusCalculationDuration(du),
	}

	totalCount := 0
	unexpectedCount := 0

	itr := span.Query(ctx, st)
	defer itr.Stop()
	var b spanutil.Buffer
	for {
		row, err := itr.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return atvpb.Status_STATUS_UNSPECIFIED, err
		}
		var verdictStatus internal.VerdictStatus
		if err = b.FromSpanner(row, &verdictStatus); err != nil {
			return atvpb.Status_STATUS_UNSPECIFIED, err
		}

		totalCount++
		switch verdictStatus {
		case internal.VerdictStatus_VERDICT_FLAKY:
			// Any flaky verdict means the test variant is flaky.
			// Return status right away.
			itr.Stop()
			return atvpb.Status_FLAKY, nil
		case internal.VerdictStatus_UNEXPECTED:
			unexpectedCount++
		case internal.VerdictStatus_EXPECTED:
		default:
			panic(fmt.Sprintf("got unsupported verdict status %d", int(verdictStatus)))
		}
	}

	return computeTestVariantStatus(totalCount, unexpectedCount), nil
}

func computeTestVariantStatus(total, unexpected int) atvpb.Status {
	switch {
	case total == 0:
		// No new results of the test variant.
		return atvpb.Status_NO_NEW_RESULTS
	case unexpected == 0:
		return atvpb.Status_CONSISTENTLY_EXPECTED
	case unexpected == total:
		return atvpb.Status_CONSISTENTLY_UNEXPECTED
	default:
		return atvpb.Status_HAS_UNEXPECTED_RESULTS
	}
}
