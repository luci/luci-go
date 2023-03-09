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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// TestVariantBranch represents one row in the TestVariantBranch spanner table.
// See go/luci-test-variant-analysis-design for details.
type TestVariantBranch struct {
	// IsNew is a boolean to denote if the TestVariantBranch is new or already
	// existed in Spanner.
	// It is used for reducing the number of mutations. For example, the Variant
	// field is only inserted once.
	IsNew                  bool
	Project                string
	TestID                 string
	VariantHash            string
	GitReferenceHash       string
	Variant                *pb.Variant
	InputBuffer            *InputBuffer
	RecentChangepointCount int64
	// TODO (nqmtuan): Add output buffer.
}

// TestVariantBranchKey denotes the primary keys for TestVariantBranch table.
type TestVariantBranchKey struct {
	Project          string
	TestID           string
	VariantHash      string
	GitReferenceHash string
}

// ReadTestVariantBranches fetches rows from TestVariantBranch spanner table
// and returns the objects fetched.
// The returned slice will have the same length and order as the
// TestVariantBranchKey slices. If a record is not found, the corresponding
// element will be set to nil.
// This function assumes that it is running inside a transaction.
func ReadTestVariantBranches(ctx context.Context, tvbks []TestVariantBranchKey) ([]*TestVariantBranch, error) {
	// Map keys to TestVariantBranch.
	// This is because spanner does not return ordered results.
	keyMap := map[TestVariantBranchKey]*TestVariantBranch{}

	// Create the keyset.
	keys := make([]spanner.Key, len(tvbks))
	for i := 0; i < len(tvbks); i++ {
		keys[i] = spanner.Key{tvbks[i].Project, tvbks[i].TestID, tvbks[i].VariantHash, tvbks[i].GitReferenceHash}
	}
	keyset := spanner.KeySetFromKeys(keys...)
	cols := []string{"Project", "TestId", "VariantHash", "GitReferenceHash", "Variant", "HotInputBuffer", "ColdInputBuffer", "RecentChangepointCount"}
	err := span.Read(ctx, "TestVariantBranch", keyset, cols).Do(
		func(row *spanner.Row) error {
			tvb, err := spannerRowToTestVariantBranch(row)
			if err != nil {
				return errors.Annotate(err, "convert spanner row to test variant branch").Err()
			}
			tvbk := TestVariantBranchKey{
				Project:          tvb.Project,
				TestID:           tvb.TestID,
				VariantHash:      tvb.VariantHash,
				GitReferenceHash: tvb.GitReferenceHash,
			}
			keyMap[tvbk] = tvb
			return nil
		},
	)

	if err != nil {
		return nil, err
	}

	result := make([]*TestVariantBranch, len(tvbks))
	for i, tvbk := range tvbks {
		tvb, ok := keyMap[tvbk]
		if ok {
			result[i] = tvb
		}
	}

	return result, nil
}

func spannerRowToTestVariantBranch(row *spanner.Row) (*TestVariantBranch, error) {
	tvb := &TestVariantBranch{}
	var b spanutil.Buffer
	var hotBuffer []byte
	var coldBuffer []byte
	var recentChangepointCount int64

	if err := b.FromSpanner(row, &tvb.Project, &tvb.TestID, &tvb.VariantHash, &tvb.GitReferenceHash, &tvb.Variant, &hotBuffer, &coldBuffer, &recentChangepointCount); err != nil {
		return nil, errors.Annotate(err, "read values from spanner").Err()
	}

	tvb.RecentChangepointCount = recentChangepointCount
	tvb.InputBuffer = &InputBuffer{
		HotBufferCapacity:  defaultHotBufferCapacity,
		ColdBufferCapacity: defaultColdBufferCapacity,
	}

	var err error
	tvb.InputBuffer.HotBuffer, err = DecodeHistory(hotBuffer)
	if err != nil {
		return nil, errors.Annotate(err, "decode hot history").Err()
	}
	tvb.InputBuffer.ColdBuffer, err = DecodeHistory(coldBuffer)
	if err != nil {
		return nil, errors.Annotate(err, "decode cold history").Err()
	}

	return tvb, nil
}

// ToMutation returns a spanner Mutation to insert a TestVariantBranch to
// Spanner table.
func (tvb *TestVariantBranch) ToMutation() *spanner.Mutation {
	cols := []string{"Project", "TestId", "VariantHash", "GitReferenceHash", "LastUpdated", "RecentChangepointCount"}
	values := []interface{}{tvb.Project, tvb.TestID, tvb.VariantHash, tvb.GitReferenceHash, spanner.CommitTimestamp, tvb.RecentChangepointCount}

	if tvb.IsNew {
		// Variant needs to be updated only once.
		// FinalizingSegment and FinalizedSegments are NOT NULL.
		cols = append(cols, []string{"Variant", "FinalizingSegment", "FinalizedSegments"}...)
		values = append(values, []interface{}{spanutil.ToSpanner(tvb.Variant), []byte{}, []byte{}}...)
	}

	// Based on the flow, we should always update the hot buffer.
	cols = append(cols, "HotInputBuffer")
	values = append(values, EncodeHistory(tvb.InputBuffer.HotBuffer))

	// We should only update the cold buffer if it is dirty, or if this is new
	// record.
	if tvb.InputBuffer.IsColdBufferDirty || tvb.IsNew {
		cols = append(cols, "ColdInputBuffer")
		values = append(values, EncodeHistory(tvb.InputBuffer.ColdBuffer))
	}

	// TODO (nqmtuan): Handle the mutation for output buffer.

	// We don't use spanner.InsertOrUpdate here because the mutation need to
	// follow the constraint of insert (i.e. we need to provide values for all
	// non-null columns).
	if tvb.IsNew {
		return spanner.Insert("TestVariantBranch", cols, values)
	}
	return spanner.Update("TestVariantBranch", cols, values)
}
