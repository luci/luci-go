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

package analyzedtestvariants

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
)

func purge(ctx context.Context) (int64, error) {
	st := spanner.NewStatement(`
		DELETE FROM AnalyzedTestVariants
		WHERE Status in UNNEST(@statuses)
		AND StatusUpdateTime < TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 31 DAY)
	`)
	st.Params = map[string]any{
		"statuses": []int{int(atvpb.Status_NO_NEW_RESULTS), int(atvpb.Status_CONSISTENTLY_EXPECTED)},
	}
	return span.PartitionedUpdate(ctx, st)
}

// Purge deletes AnalyzedTestVariants rows that have been in NO_NEW_RESULTS or
// CONSISTENTLY_EXPECTED status for over 1 month.
//
// Because Verdicts are interleaved with AnalyzedTestVariants, deleting
// AnalyzedTestVariants rows also deletes their verdicts.
func Purge(ctx context.Context) error {
	purged, err := purge(ctx)
	logging.Infof(ctx, "Purged %d test variants", purged)
	return err
}
