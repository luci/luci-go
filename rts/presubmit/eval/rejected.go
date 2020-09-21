// Copyright 2020 The LUCI Authors.
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

package eval

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging"
)

// RejectedPatchSetRequest is a request to retrieve all patchsets rejected by CQ within the
// given date range, along with tests that caused the rejection.
type RejectedPatchSetRequest struct {
	Context       context.Context
	StartTime     time.Time
	EndTime       time.Time
	Authenticator *auth.Authenticator
}

// RejectedPatchSetProvider retrieves patchsets rejected by CQ.
type RejectedPatchSetProvider func(RejectedPatchSetRequest) ([]*RejectedPatchSet, error)

// rejectedPatchSetQuery retrieves rejected patchsets and the tests that caused
// the rejection.
type rejectedPatchSetQuery struct {
	*evalRun
}

func (q *rejectedPatchSetQuery) Read(ctx context.Context) ([]*RejectedPatchSet, error) {
	assert(isUTCDayStart(q.startTime))
	assert(isUTCDayStart(q.endTime))
	assert(q.startTime.Before(q.endTime))

	ret, start := q.readCache(ctx)
	if start.After(q.endTime) {
		return ret, nil
	}

	// Fetch the rest of rejected patchsets.
	logging.Infof(ctx, "Fetching rejected patchsets between %s and %s. It may take a couple of minutes...", start.Format(dayLayout), q.endTime.Format(dayLayout))
	ret, err := q.RejectedPatchSetProvider(RejectedPatchSetRequest{
		Context:       ctx,
		Authenticator: q.auth,
		StartTime:     start,
		EndTime:       q.endTime,
	})
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "Fetched %d patchsets", len(ret))

	// Cache results.
	byDay := map[time.Time][]*RejectedPatchSet{}
	for _, rp := range ret {
		aDay := rp.Timestamp.Truncate(day)
		byDay[aDay] = append(byDay[aDay], rp)
	}
	q.writeCache(ctx, byDay)

	return ret, nil
}

func (q *rejectedPatchSetQuery) readCache(ctx context.Context) (fps []*RejectedPatchSet, newStartTime time.Time) {
	start := q.startTime
	var cached []*RejectedPatchSet
	for !start.After(q.endTime) {
		cached = cached[:0]
		if !q.cacheFile(start).TryRead(ctx, &cached) {
			// We might have a gap in days, so read the rest from BigQuery.
			break
		}

		fps = append(fps, cached...)
		start = start.Add(day)
	}

	return fps, start
}

func (q *rejectedPatchSetQuery) writeCache(ctx context.Context, byDay map[time.Time][]*RejectedPatchSet) {
	// Sort days.
	days := make([]time.Time, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool {
		return days[i].Before(days[j])
	})

	// Write a file per day.
	for _, day := range days {
		q.cacheFile(day).TryWrite(ctx, byDay[day])
	}
}

func (q *rejectedPatchSetQuery) cacheFile(t time.Time) cacheFile {
	return cacheFile(filepath.Join(q.CacheDir, fmt.Sprintf("rejected-patch-sets/%s", t.Format(dayLayout))))
}
