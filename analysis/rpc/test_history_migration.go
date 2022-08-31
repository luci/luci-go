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

package rpc

import (
	"context"
	"time"

	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/testresults"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/server/span"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// The partition time from which the new database (luci-analysis(-dev))
// should be used as the source of truth. Prior to this date, the old
// database should be used as the source of truth.
var splitTime = time.Date(2022, time.September, 1, 0, 0, 0, 0, time.UTC)

// testHistoryBackend supports migration from chops-weetbix(-dev)
// to luci-analysis(-dev) by routing test history reads to one of
// two databases, depending on the time range read.
type testHistoryBackend struct {
	// oldDatabase creates a context to use to access to the
	// old database.
	oldDatabaseCtx func(context.Context) context.Context
}

// ReadTestHistory reads verdicts from the spanner database.
func (b *testHistoryBackend) ReadTestHistory(ctx context.Context, opts testresults.ReadTestHistoryOptions) ([]*pb.TestVerdict, string, error) {
	// Identify the time range the caller has asked us to query.
	// Start is inclusive, end is exclusive.
	fullRangeStart := testresults.MinSpannerTimestamp
	fullRangeEnd := testresults.MaxSpannerTimestamp
	if opts.TimeRange.GetEarliest() != nil {
		fullRangeStart = opts.TimeRange.Earliest.AsTime()
	}
	if opts.TimeRange.GetLatest() != nil {
		fullRangeEnd = opts.TimeRange.Latest.AsTime()
	}

	// Split the fullRange into two sub-ranges:
	// - oldRange is the time range to query from the old database
	// - newRange is the time range to query from the new database
	oldRangeStart := earliestOf(fullRangeStart, splitTime)
	oldRangeEnd := earliestOf(fullRangeEnd, splitTime)
	newRangeStart := latestOf(fullRangeStart, splitTime)
	newRangeEnd := latestOf(fullRangeEnd, splitTime)

	// Whether we have paged to before the date range handle by the new database.
	// Note that we return the most recent results first, so we are paging back
	// to the past.
	pagedBeforeSplit := false

	if opts.PageToken != "" {
		token, err := pagination.ParseToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
		// The first part of the page token is the paginationTime.
		paginationTime, err := time.Parse(time.RFC3339Nano, token[0])
		if err != nil {
			return nil, "", err
		}
		// A time before the splitTime means we have paged
		// past the split point.
		pagedBeforeSplit = paginationTime.Before(splitTime)
	}

	var results []*pb.TestVerdict
	var nextPageToken string
	var err error

	// Query from the newer split first, assuming the time range to be queried is non-empty.
	if !pagedBeforeSplit && newRangeEnd.After(newRangeStart) {
		newSplitOptions := opts
		newSplitOptions.TimeRange = &pb.TimeRange{
			Earliest: timestamppb.New(newRangeStart),
			Latest:   timestamppb.New(newRangeEnd),
		}

		results, nextPageToken, err = testresults.ReadTestHistory(span.Single(ctx), newSplitOptions)
		if err != nil {
			return nil, "", err
		}
	}
	// Then query from the older split, assuming the time range to be queried is non-empty
	// (and we have not already got the desired number of results).
	if len(results) < opts.PageSize && oldRangeEnd.After(oldRangeStart) {
		oldSplitOptions := opts
		oldSplitOptions.TimeRange = &pb.TimeRange{
			Earliest: timestamppb.New(oldRangeStart),
			Latest:   timestamppb.New(oldRangeEnd),
		}
		if !pagedBeforeSplit {
			// First time querying before the split. Do not pass the
			// page token from one part of the split to the other.
			oldSplitOptions.PageToken = ""
		}
		// Only query as many results as needed to meet the desired page size.
		oldSplitOptions.PageSize = opts.PageSize - len(results)

		// Route the query to the old Spanner database.
		oldDBCtx := b.oldDatabaseCtx(ctx)

		var additionalResults []*pb.TestVerdict
		additionalResults, nextPageToken, err = testresults.ReadTestHistory(span.Single(oldDBCtx), oldSplitOptions)
		if err != nil {
			return nil, "", err
		}
		results = append(results, additionalResults...)
	}
	return results, nextPageToken, nil
}

// ReadTestHistoryStats reads stats of verdicts grouped by UTC dates from the
// spanner database.
func (b *testHistoryBackend) ReadTestHistoryStats(ctx context.Context, opts testresults.ReadTestHistoryOptions) ([]*pb.QueryTestHistoryStatsResponse_Group, string, error) {
	// Identify the time range the caller has asked us to query.
	// Start is inclusive, end is exclusive.
	fullRangeStart := testresults.MinSpannerTimestamp
	fullRangeEnd := testresults.MaxSpannerTimestamp
	if opts.TimeRange.GetEarliest() != nil {
		fullRangeStart = opts.TimeRange.Earliest.AsTime()
	}
	if opts.TimeRange.GetLatest() != nil {
		fullRangeEnd = opts.TimeRange.Latest.AsTime()
	}

	// Split the fullRange into two sub-ranges:
	// - oldRange is the time range to query from the old database
	// - newRange is the time range to query from the new database
	oldRangeStart := earliestOf(fullRangeStart, splitTime)
	oldRangeEnd := earliestOf(fullRangeEnd, splitTime)
	newRangeStart := latestOf(fullRangeStart, splitTime)
	newRangeEnd := latestOf(fullRangeEnd, splitTime)

	// Whether we have paged to before the date range handle by the new database.
	// Note that we return the most recent results first, so we are paging back
	// to the past.
	pagedBeforeSplit := false

	if opts.PageToken != "" {
		token, err := pagination.ParseToken(opts.PageToken)
		if err != nil {
			return nil, "", err
		}
		// The first part of the page token is the paginationTime.
		paginationTime, err := time.Parse(time.RFC3339Nano, token[0])
		if err != nil {
			return nil, "", err
		}
		// A time before the splitTime means we have paged
		// past the split point.
		pagedBeforeSplit = paginationTime.Before(splitTime)
	}

	var results []*pb.QueryTestHistoryStatsResponse_Group
	var nextPageToken string
	var err error

	// Query from the newer split first, assuming the time range to be queried is non-empty.
	if !pagedBeforeSplit && newRangeEnd.After(newRangeStart) {
		newSplitOptions := opts
		newSplitOptions.TimeRange = &pb.TimeRange{
			Earliest: timestamppb.New(newRangeStart),
			Latest:   timestamppb.New(newRangeEnd),
		}

		results, nextPageToken, err = testresults.ReadTestHistoryStats(span.Single(ctx), newSplitOptions)
		if err != nil {
			return nil, "", err
		}
	}
	// Then query from the older split, assuming the time range to be queried is non-empty
	// (and we have not already got the desired number of results).
	if len(results) < opts.PageSize && oldRangeEnd.After(oldRangeStart) {
		oldSplitOptions := opts
		oldSplitOptions.TimeRange = &pb.TimeRange{
			Earliest: timestamppb.New(oldRangeStart),
			Latest:   timestamppb.New(oldRangeEnd),
		}
		if !pagedBeforeSplit {
			// First time querying the older split. Do not pass the
			// page token from one part of the split to the other.
			oldSplitOptions.PageToken = ""
		}
		// Only query as many results as needed to meet the desired page size.
		oldSplitOptions.PageSize = opts.PageSize - len(results)

		// Route the query to the old Spanner database.
		oldDBCtx := b.oldDatabaseCtx(ctx)

		var additionalResults []*pb.QueryTestHistoryStatsResponse_Group
		additionalResults, nextPageToken, err = testresults.ReadTestHistoryStats(span.Single(oldDBCtx), oldSplitOptions)
		if err != nil {
			return nil, "", err
		}
		results = append(results, additionalResults...)
	}
	return results, nextPageToken, nil
}

// earliestOf returns the earlier of two times. If they are equal,
// it returns either one.
func earliestOf(a time.Time, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// earliestOf returns the latest of two times. If they are equal,
// it returns either one.
func latestOf(a time.Time, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
