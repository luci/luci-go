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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	"go.chromium.org/luci/analysis/internal/pagination"
	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// RefHash is used for RefHash field in TestVariantBranchKey.
type RefHash string

// Key denotes the primary key for the TestVariantBranch table.
type Key struct {
	Project     string
	TestID      string
	VariantHash string
	// Make this as a string here so it can be used as key in map.
	// Note that it is a sequence of bytes, not a sequence of characters.
	RefHash RefHash
}

// ReadF fetches rows from TestVariantBranch spanner table
// and calls the specified callback function for each row.
//
// Important: The caller must not retain a reference to the
// provided *Entry after each call, as the object may be
// re-used for in the next call. If an entry must be retained,
// it should be copied.
//
// The callback function will be passed the read row and
// the corresponding index in the key list that was read.
// If an item does not exist, the provided callback function
// will be called with the value 'nil'.
// Absent any errors, the callback function will be called
// exactly once for each key.
// The callback function may not be called in order of keys,
// as Spanner does not return items in order.
//
// This function assumes that it is running inside a transaction.
func ReadF(ctx context.Context, ks []Key, f func(i int, e *Entry) error) error {
	// Map keys back to key index.
	// This is because spanner does not return ordered results.
	keyMap := make(map[Key]int, len(ks))

	// Create the keyset.
	spannerKeys := make([]spanner.Key, len(ks))
	for i := 0; i < len(ks); i++ {
		spannerKeys[i] = spanner.Key{ks[i].Project, ks[i].TestID, ks[i].VariantHash, []byte(ks[i].RefHash)}
		keyMap[ks[i]] = i
	}
	keyset := spanner.KeySetFromKeys(spannerKeys...)
	cols := []string{
		"Project", "TestId", "VariantHash", "RefHash", "Variant", "SourceRef",
		"HotInputBuffer", "ColdInputBuffer", "FinalizingSegment", "FinalizedSegments", "Statistics",
	}

	// Re-use the same buffers for processing each test variant branch. This
	// avoids creating excessive memory pressure.
	tvb := New()
	var hs inputbuffer.HistorySerializer

	err := span.Read(ctx, "TestVariantBranch", keyset, cols).Do(func(r *spanner.Row) error {
		err := tvb.PopulateFromSpannerRow(r, &hs)
		if err != nil {
			return errors.Fmt("convert spanner row to test variant branch: %w", err)
		}
		key := Key{
			Project:     tvb.Project,
			TestID:      tvb.TestID,
			VariantHash: tvb.VariantHash,
			RefHash:     RefHash(tvb.RefHash),
		}
		index, ok := keyMap[key]
		if !ok {
			return errors.New("spanner returned unexpected key")
		}
		// Remove items which were read from the keyMap, so that
		// only unread items remain.
		delete(keyMap, key)
		return f(index, tvb)
	})
	if err != nil {
		return err
	}
	// Call callback function for items which did not exist.
	for _, i := range keyMap {
		if err := f(i, nil); err != nil {
			return err
		}
	}

	return nil
}

// Read fetches rows from TestVariantBranch spanner table
// and returns the objects fetched.
// The returned slice will have the same length and order as the
// TestVariantBranchKey slices. If a record is not found, the corresponding
// element will be set to nil.
// This function assumes that it is running inside a transaction.
func Read(ctx context.Context, ks []Key) (rs []*Entry, retErr error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch.Read")
	defer func() { tracing.End(s, retErr) }()

	result := make([]*Entry, len(ks))
	err := ReadF(ctx, ks, func(i int, e *Entry) error {
		// ReadF re-uses buffers between entries, so if we want to
		// keep a reference to an entry, we must copy it.
		result[i] = e.Copy()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// QueryVariantBranchesOptions specifies options for QueryVariantBranches().
type QueryVariantBranchesOptions struct {
	PageSize  int
	PageToken string
}

// parseQueryVariantBranchesPageToken parses the variant hash position from the page token.
func parseQueryVariantBranchesPageToken(pageToken string) (afterVariantHash string, err error) {
	tokens, err := pagination.ParseToken(pageToken)
	if err != nil {
		return "", err
	}

	if len(tokens) != 1 {
		return "", pagination.InvalidToken(errors.Fmt("expected 1 components, got %d", len(tokens)))
	}

	return tokens[0], nil
}

func QueryVariantBranches(ctx context.Context, project, testID string, refHash []byte, opts QueryVariantBranchesOptions) (entries []*Entry, nextPageToken string, err error) {
	stmt := spanner.NewStatement(`
		SELECT
			Project,
			TestId,
			VariantHash,
			RefHash,
			Variant,
			SourceRef,
			HotInputBuffer,
			ColdInputBuffer,
			FinalizingSegment,
			FinalizedSegments,
			Statistics
	FROM TestVariantBranch
	WHERE
		Project = @project
			AND TestId = @testId
			AND RefHash = @RefHash
			AND VariantHash > @paginationVariantHash
	ORDER BY VariantHash ASC
	LIMIT @limit
`)
	paginationVariantHash := ""
	if opts.PageToken != "" {
		paginationVariantHash, err = parseQueryVariantBranchesPageToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
	}
	stmt.Params = map[string]any{
		"project": project,
		"testId":  testID,
		"refHash": refHash,

		// Control pagination.
		"limit":                 opts.PageSize,
		"paginationVariantHash": paginationVariantHash,
	}

	tvbs := make([]*Entry, 0, opts.PageSize)
	tvb := New()
	var hs inputbuffer.HistorySerializer
	err = span.Query(ctx, stmt).Do(func(row *spanner.Row) error {
		err := tvb.PopulateFromSpannerRow(row, &hs)
		if err != nil {
			return errors.Fmt("populate from spanner row: %w", err)
		}
		tvbs = append(tvbs, tvb.Copy())
		return nil
	})
	if err != nil {
		return nil, "", errors.Fmt("run query: %w", err)
	}

	if opts.PageSize != 0 && len(tvbs) == opts.PageSize {
		lastVariantHash := tvbs[len(tvbs)-1].VariantHash
		nextPageToken = pagination.Token(lastVariantHash)
	}
	return tvbs, nextPageToken, nil
}

func (tvb *Entry) PopulateFromSpannerRow(row *spanner.Row, hs *inputbuffer.HistorySerializer) error {
	// Avoid leaking state from the previous spanner row.
	tvb.Clear()

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
		return errors.Fmt("read values from spanner: %w", err)
	}

	// Source ref.
	tvb.SourceRef, err = DecodeSourceRef(sourceRef)
	if err != nil {
		return errors.Fmt("decode source ref: %w", err)
	}

	err = hs.DecodeInto(&tvb.InputBuffer.HotBuffer, hotBuffer)
	if err != nil {
		return errors.Fmt("decode hot history: %w", err)
	}
	err = hs.DecodeInto(&tvb.InputBuffer.ColdBuffer, coldBuffer)
	if err != nil {
		return errors.Fmt("decode cold history: %w", err)
	}

	// Process output buffer.
	tvb.FinalizingSegment, err = DecodeSegment(finalizingSegment)
	if err != nil {
		return errors.Fmt("decode finalizing segment: %w", err)
	}

	tvb.FinalizedSegments, err = DecodeSegments(finalizedSegments)
	if err != nil {
		return errors.Fmt("decode finalized segments: %w", err)
	}

	tvb.Statistics, err = DecodeStatistics(statistics)
	if err != nil {
		return errors.Fmt("decode statistics: %w", err)
	}

	return nil
}

// ToMutation returns a spanner Mutation to insert a TestVariantBranch to
// Spanner table.
func (tvb *Entry) ToMutation(hs *inputbuffer.HistorySerializer) (*spanner.Mutation, error) {
	cols := []string{"Project", "TestId", "VariantHash", "RefHash", "LastUpdated"}
	values := []any{tvb.Project, tvb.TestID, tvb.VariantHash, tvb.RefHash, spanner.CommitTimestamp}

	if tvb.IsNew {
		// Variant needs to be updated only once.
		cols = append(cols, "Variant")
		values = append(values, spanutil.ToSpanner(tvb.Variant))
		// SourceRef needs to be updated only once.
		cols = append(cols, "SourceRef")
		bytes, err := EncodeSourceRef(tvb.SourceRef)
		if err != nil {
			return nil, errors.Fmt("encode source ref: %w", err)
		}
		values = append(values, bytes)
	}

	// Based on the flow, we should always update the hot buffer.
	cols = append(cols, "HotInputBuffer")
	values = append(values, hs.Encode(tvb.InputBuffer.HotBuffer))

	// We should only update the cold buffer if it is dirty, or if this is new
	// record.
	if tvb.InputBuffer.IsColdBufferDirty || tvb.IsNew {
		cols = append(cols, "ColdInputBuffer")
		values = append(values, hs.Encode(tvb.InputBuffer.ColdBuffer))
	}

	// Finalizing segment.
	// We only write finalizing segment if this is new (because FinalizingSegment
	// is NOT NULL), or there is an update.
	if tvb.IsFinalizingSegmentDirty || tvb.IsNew {
		cols = append(cols, "FinalizingSegment")
		bytes, err := EncodeSegment(tvb.FinalizingSegment)
		if err != nil {
			return nil, errors.Fmt("encode finalizing segment: %w", err)
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
			return nil, errors.Fmt("encode finalized segments: %w", err)
		}
		values = append(values, bytes)
	}

	// Evicted run statistics.
	// We only write statistics if this is new (because Statistics
	// is NOT NULL), or there is an update.
	if tvb.IsStatisticsDirty || tvb.IsNew {
		cols = append(cols, "Statistics")
		bytes, err := EncodeStatistics(tvb.Statistics)
		if err != nil {
			return nil, errors.Fmt("encode statistics: %w", err)
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
