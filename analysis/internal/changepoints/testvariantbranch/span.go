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

package testvariantbranch

import (
	"context"

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"
)

// RefHash is used for RefHash field in TestVariantBranchKey.
type RefHash string

// TestVariantBranchKey denotes the primary keys for TestVariantBranch table.
type TestVariantBranchKey struct {
	Project     string
	TestID      string
	VariantHash string
	// Make this as a string here so it can be used as key in map.
	// Note that it is a sequence of bytes, not a sequence of characters.
	RefHash RefHash
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
		keys[i] = spanner.Key{tvbks[i].Project, tvbks[i].TestID, tvbks[i].VariantHash, []byte(tvbks[i].RefHash)}
	}
	keyset := spanner.KeySetFromKeys(keys...)
	cols := []string{
		"Project", "TestId", "VariantHash", "RefHash", "Variant", "SourceRef",
		"HotInputBuffer", "ColdInputBuffer", "FinalizingSegment", "FinalizedSegments", "Statistics",
	}
	err := span.Read(ctx, "TestVariantBranch", keyset, cols).Do(
		func(row *spanner.Row) error {
			tvb, err := SpannerRowToTestVariantBranch(row)
			if err != nil {
				return errors.Annotate(err, "convert spanner row to test variant branch").Err()
			}
			tvbk := TestVariantBranchKey{
				Project:     tvb.Project,
				TestID:      tvb.TestID,
				VariantHash: tvb.VariantHash,
				RefHash:     RefHash(tvb.RefHash),
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

func SpannerRowToTestVariantBranch(row *spanner.Row) (*TestVariantBranch, error) {
	tvb := &TestVariantBranch{}
	var b spanutil.Buffer
	var sourceRef []byte
	var hotBuffer []byte
	var coldBuffer []byte
	var finalizingSegment []byte
	var finalizedSegments []byte
	var statistics []byte

	err := b.FromSpanner(row, &tvb.Project, &tvb.TestID, &tvb.VariantHash, &tvb.RefHash, &tvb.Variant,
		&sourceRef, &hotBuffer, &coldBuffer, &finalizingSegment, &finalizedSegments, &statistics)
	if err != nil {
		return nil, errors.Annotate(err, "read values from spanner").Err()
	}

	// Source ref
	tvb.SourceRef, err = DecodeSourceRef(sourceRef)
	if err != nil {
		return nil, errors.Annotate(err, "decode source ref").Err()
	}

	tvb.InputBuffer = &inputbuffer.Buffer{
		HotBufferCapacity:  inputbuffer.DefaultHotBufferCapacity,
		ColdBufferCapacity: inputbuffer.DefaultColdBufferCapacity,
	}

	tvb.InputBuffer.HotBuffer, err = inputbuffer.DecodeHistory(hotBuffer)
	if err != nil {
		return nil, errors.Annotate(err, "decode hot history").Err()
	}
	tvb.InputBuffer.ColdBuffer, err = inputbuffer.DecodeHistory(coldBuffer)
	if err != nil {
		return nil, errors.Annotate(err, "decode cold history").Err()
	}

	// Process output buffer.
	tvb.FinalizingSegment, err = DecodeSegment(finalizingSegment)
	if err != nil {
		return nil, errors.Annotate(err, "decode finalizing segment").Err()
	}

	tvb.FinalizedSegments, err = DecodeSegments(finalizedSegments)
	if err != nil {
		return nil, errors.Annotate(err, "decode finalized segments").Err()
	}

	tvb.Statistics, err = DecodeStatistics(statistics)
	if err != nil {
		return nil, errors.Annotate(err, "decode statistics").Err()
	}

	return tvb, nil
}

// ToMutation returns a spanner Mutation to insert a TestVariantBranch to
// Spanner table.
func (tvb *TestVariantBranch) ToMutation() (*spanner.Mutation, error) {
	cols := []string{"Project", "TestId", "VariantHash", "RefHash", "LastUpdated"}
	values := []interface{}{tvb.Project, tvb.TestID, tvb.VariantHash, tvb.RefHash, spanner.CommitTimestamp}

	if tvb.IsNew {
		// Variant needs to be updated only once.
		cols = append(cols, "Variant")
		values = append(values, spanutil.ToSpanner(tvb.Variant))
		// SourceRef needs to be updated only once.
		cols = append(cols, "SourceRef")
		bytes, err := EncodeSourceRef(tvb.SourceRef)
		if err != nil {
			return nil, errors.Annotate(err, "encode source ref").Err()
		}
		values = append(values, bytes)
	}

	// Based on the flow, we should always update the hot buffer.
	cols = append(cols, "HotInputBuffer")
	values = append(values, inputbuffer.EncodeHistory(tvb.InputBuffer.HotBuffer))

	// We should only update the cold buffer if it is dirty, or if this is new
	// record.
	if tvb.InputBuffer.IsColdBufferDirty || tvb.IsNew {
		cols = append(cols, "ColdInputBuffer")
		values = append(values, inputbuffer.EncodeHistory(tvb.InputBuffer.ColdBuffer))
	}

	// Finalizing segment.
	// We only write finalizing segment if this is new (because FinalizingSegment
	// is NOT NULL), or there is an update.
	if tvb.IsFinalizingSegmentDirty || tvb.IsNew {
		cols = append(cols, "FinalizingSegment")
		bytes, err := EncodeSegment(tvb.FinalizingSegment)
		if err != nil {
			return nil, errors.Annotate(err, "encode finalizing segment").Err()
		}
		values = append(values, bytes)
	}

	// Finalized segments.
	// We only write finalized segments if this is new (because FinalizedSegments
	// is NOT NULL), or there is an update.
	if tvb.IsFinalizedSegmentsDirty || tvb.IsNew {
		cols = append(cols, "FinalizedSegments")
		bytes, err := EncodeSegments(tvb.FinalizedSegments)
		if err != nil {
			return nil, errors.Annotate(err, "encode finalized segments").Err()
		}
		values = append(values, bytes)
	}

	// Evicted verdict statistics.
	// We only write statistics if this is new (because Statistics
	// is NOT NULL), or there is an update.
	if tvb.IsStatisticsDirty || tvb.IsNew {
		cols = append(cols, "Statistics")
		bytes, err := EncodeStatistics(tvb.Statistics)
		if err != nil {
			return nil, errors.Annotate(err, "encode statistics").Err()
		}
		values = append(values, bytes)
	}

	// We don't use spanner.InsertOrUpdate here because the mutation need to
	// follow the constraint of insert (i.e. we need to provide values for all
	// non-null columns).
	if tvb.IsNew {
		return spanner.Insert("TestVariantBranch", cols, values), nil
	}
	return spanner.Update("TestVariantBranch", cols, values), nil
}
