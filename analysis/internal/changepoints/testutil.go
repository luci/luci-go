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

package changepoints

import (
	"context"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
)

func FetchTestVariantBranches(ctx context.Context) ([]*testvariantbranch.Entry, error) {
	st := spanner.NewStatement(`
			SELECT
				Project, TestId, VariantHash, RefHash, Variant, SourceRef,
				HotInputBuffer, ColdInputBuffer, FinalizingSegment, FinalizedSegments, Statistics
			FROM TestVariantBranch
			ORDER BY TestId
		`)
	it := span.Query(span.Single(ctx), st)
	results := []*testvariantbranch.Entry{}
	var hs inputbuffer.HistorySerializer
	err := it.Do(func(r *spanner.Row) error {
		tvb := testvariantbranch.New()
		err := tvb.PopulateFromSpannerRow(r, &hs)
		if err != nil {
			return err
		}
		results = append(results, tvb)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return results, nil
}
