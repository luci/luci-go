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
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ReadSummary reads AnalyzedTestVariant rows by keys.
func ReadSummary(ctx context.Context, ks spanner.KeySet, f func(*atvpb.AnalyzedTestVariant) error) error {
	fields := []string{"Realm", "TestId", "VariantHash", "Status", "Tags", "TestMetadata"}
	var b spanutil.Buffer
	return span.Read(ctx, "AnalyzedTestVariants", ks, fields).Do(
		func(row *spanner.Row) error {
			var tmd spanutil.Compressed
			tv := &atvpb.AnalyzedTestVariant{}
			if err := b.FromSpanner(row, &tv.Realm, &tv.TestId, &tv.VariantHash, &tv.Status, &tv.Tags, &tmd); err != nil {
				return err
			}
			if len(tmd) > 0 {
				tv.TestMetadata = &pb.TestMetadata{}
				if err := proto.Unmarshal(tmd, tv.TestMetadata); err != nil {
					return errors.Annotate(err, "error unmarshalling test_metadata for %s", tv.Name).Err()
				}
			}
			return f(tv)
		},
	)
}

// StatusHistory contains all the information related to a test variant's status changes.
type StatusHistory struct {
	Status                    atvpb.Status
	StatusUpdateTime          time.Time
	PreviousStatuses          []atvpb.Status
	PreviousStatusUpdateTimes []time.Time
}

// ReadStatusHistory reads AnalyzedTestVariant rows by keys and returns the test variant's status related info.
func ReadStatusHistory(ctx context.Context, k spanner.Key) (*StatusHistory, spanner.NullTime, error) {
	fields := []string{"Status", "StatusUpdateTime", "NextUpdateTaskEnqueueTime", "PreviousStatuses", "PreviousStatusUpdateTimes"}
	var b spanutil.Buffer
	si := &StatusHistory{}
	var enqTime, t spanner.NullTime
	err := span.Read(ctx, "AnalyzedTestVariants", spanner.KeySets(k), fields).Do(
		func(row *spanner.Row) error {
			if err := b.FromSpanner(row, &si.Status, &t, &enqTime, &si.PreviousStatuses, &si.PreviousStatusUpdateTimes); err != nil {
				return err
			}
			if !t.Valid {
				return fmt.Errorf("invalid status update time")
			}
			si.StatusUpdateTime = t.Time
			return nil
		},
	)
	return si, enqTime, err
}

// ReadNextUpdateTaskEnqueueTime reads the NextUpdateTaskEnqueueTime from the
// requested test variant.
func ReadNextUpdateTaskEnqueueTime(ctx context.Context, k spanner.Key) (spanner.NullTime, error) {
	row, err := span.ReadRow(ctx, "AnalyzedTestVariants", k, []string{"NextUpdateTaskEnqueueTime"})
	if err != nil {
		return spanner.NullTime{}, err
	}

	var t spanner.NullTime
	err = row.Column(0, &t)
	return t, err
}

// QueryTestVariantsByBuilder queries AnalyzedTestVariants with unexpected
// results on the given builder.
func QueryTestVariantsByBuilder(ctx context.Context, realm, builder string, f func(*atvpb.AnalyzedTestVariant) error) error {
	st := spanner.NewStatement(`
		SELECT TestId, VariantHash
		FROM AnalyzedTestVariants@{FORCE_INDEX=AnalyzedTestVariantsByBuilderAndStatus, spanner_emulator.disable_query_null_filtered_index_check=true}
		WHERE Realm = @realm
		AND Builder = @builder
		AND Status in UNNEST(@statuses)
		ORDER BY TestId, VariantHash
	`)
	st.Params = map[string]any{
		"realm":    realm,
		"builder":  builder,
		"statuses": []int{int(atvpb.Status_FLAKY), int(atvpb.Status_CONSISTENTLY_UNEXPECTED), int(atvpb.Status_HAS_UNEXPECTED_RESULTS)},
	}

	var b spanutil.Buffer
	return span.Query(ctx, st).Do(
		func(row *spanner.Row) error {
			tv := &atvpb.AnalyzedTestVariant{}
			if err := b.FromSpanner(row, &tv.TestId, &tv.VariantHash); err != nil {
				return err
			}
			return f(tv)
		},
	)
}
