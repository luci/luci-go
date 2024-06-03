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

package testvariantbranch

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/changepoints/inputbuffer"
	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
)

// sourceVerdict represents an aggregation of all test results
// at a source position.
type sourceVerdict struct {
	// The position along the source ref (e.g. git branch).
	CommitPosition int64
	// The most recent hour for which a test result was recorded.
	LastHour time.Time
	// The number of expected (non-skipped) results at the source position.
	ExpectedResults int64
	// The number of unexpected (non-skipped) results at the source position.
	UnexpectedResults int64
}

// runStreamAggregator aggregates a stream of runs (in ascending
// commit position order) into a stream of source verdicts.
//
// This is effectively implementing a 'GROUP BY commit_position'
// over a stream of runs with O(1) memory requirements. It can do
// so because the stream of runs is in ascending order.
type runStreamAggregator struct {
	// Whether `partial` is set, i.e. whether any runs have been
	// aggregated into a partial source verdict.
	// In practice, this is only false when the stream is waiting
	// for the first run to be inserted.
	hasPartial bool
	// The partial source verdict aggregated from runs inserted
	// into the stream.
	// This partial verdict will become a complete verdict when
	// the stream of runs advances past the commit position this
	// is aggregating.
	partial sourceVerdict
}

// NewRunStreamAggregator intialises a new streaming aggregator
// for converting test runs (in ascending commit position order)
// to source verdicts, optionally recovering the aggregation state
// from a partial source verdict.
func NewRunStreamAggregator(partialVerdict *cpb.PartialSourceVerdict) *runStreamAggregator {
	result := &runStreamAggregator{}
	if partialVerdict != nil {
		result.hasPartial = true
		result.partial = sourceVerdict{
			CommitPosition:    partialVerdict.CommitPosition,
			LastHour:          partialVerdict.LastHour.AsTime(),
			ExpectedResults:   partialVerdict.ExpectedResults,
			UnexpectedResults: partialVerdict.UnexpectedResults,
		}
	}
	return result
}

// Insert inserts a run into the aggregator.
// If an aggregated source verdict is available as a result of the
// insertion, it is yielded, with OK set to true. Otherwise, ok
// is set to false and yield should be ignored.
func (s *runStreamAggregator) Insert(run inputbuffer.Run) (yield sourceVerdict, ok bool) {
	var toYield sourceVerdict
	var shouldYield bool
	if s.hasPartial && s.partial.CommitPosition != run.CommitPosition {
		// We have advanced past the commit position of the partial
		// source verdict. The partial source verdict is now complete.
		if run.CommitPosition < s.partial.CommitPosition {
			panic("run commit position should not advance backwards")
		}
		// The current source verdict is final.
		toYield = s.partial
		shouldYield = true

		s.partial = sourceVerdict{}
		s.hasPartial = false
	}
	if !s.hasPartial {
		// Start a new source verdict for this commit position.
		s.hasPartial = true
		s.partial = sourceVerdict{
			CommitPosition: run.CommitPosition,
			LastHour:       run.Hour,
		}
	}
	if run.Hour.After(s.partial.LastHour) {
		s.partial.LastHour = run.Hour
	}

	s.partial.ExpectedResults += int64(run.Expected.Count())
	s.partial.UnexpectedResults += int64(run.Unexpected.Count())

	// Use a struct + bool rather than a pointer to avoid unnecessary
	// memory allocations.
	return toYield, shouldYield
}

// SaveState finishes the use of the streaming aggregator, returning the
// aggregation state (by means of a partial source verdict).
// If the last run has been inserted into the aggregator, the partial
// source verdict may be treated as complete.
func (s *runStreamAggregator) SaveState() *cpb.PartialSourceVerdict {
	if !s.hasPartial {
		return nil
	}
	return &cpb.PartialSourceVerdict{
		CommitPosition:    s.partial.CommitPosition,
		LastHour:          timestamppb.New(s.partial.LastHour),
		ExpectedResults:   s.partial.ExpectedResults,
		UnexpectedResults: s.partial.UnexpectedResults,
	}
}
