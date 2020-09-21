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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// RejectedPatchSet is a patchset rejected due to test failures.
type RejectedPatchSet struct {
	Patchset  GerritPatchset `json:"patchset"`
	Timestamp time.Time      `json:"timestamp"`

	// FailedTests are the tests that caused the rejection.
	FailedTests []*Test `json:"failedTests"`
}

// RejectedPatchSetRequest is a request to retrieve all patchsets
// rejected due to test failures within the given time range,
// along with tests that caused the rejection.
type RejectedPatchSetRequest struct {
	Context       context.Context
	Authenticator *auth.Authenticator
	StartTime     time.Time
	EndTime       time.Time
}

// RejectedPatchSetProvider retrieves patchsets rejected due to test failures.
type RejectedPatchSetProvider func(RejectedPatchSetRequest) ([]*RejectedPatchSet, error)

// rejectedPatchSetSource retrieves rejected patchsets and the tests that caused
// the rejection.
type rejectedPatchSetSource struct {
	*evalRun
}

// Read returns patchsets rejected due to test failures, along with tests
// that caused the rejection.
func (s *rejectedPatchSetSource) Read(ctx context.Context) ([]*RejectedPatchSet, error) {
	switch {
	case !isUTCDayStart(s.startTime):
		return nil, errors.New("StartTime is not a UTC day start")
	case !isUTCDayStart(s.endTime):
		return nil, errors.New("EndTime is not a UTC day start")
	case !s.startTime.Before(s.endTime):
		return nil, errors.New("StartTime must be before EndTime")
	}

	ret, start := s.readCache(ctx)
	if start.After(s.endTime) {
		return ret, nil
	}

	// Fetch the rest of rejected patchsets.
	logging.Infof(ctx, "Fetching rejected patchsets between %s and %s. It may take a couple of minutes...", start.Format(dayLayout), s.endTime.Format(dayLayout))
	ret, err := s.RejectedPatchSetProvider(RejectedPatchSetRequest{
		Context:       ctx,
		Authenticator: s.auth,
		StartTime:     start,
		EndTime:       s.endTime,
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
	s.writeCache(ctx, byDay)

	return ret, nil
}

func (s *rejectedPatchSetSource) readCache(ctx context.Context) (fps []*RejectedPatchSet, newStartTime time.Time) {
	start := s.startTime
	var cached []*RejectedPatchSet
	for !start.After(s.endTime) {
		cached = cached[:0]
		if !s.cacheFile(start).TryRead(ctx, &cached) {
			// We might have a gap in days, so read the rest from BigQuery.
			break
		}

		fps = append(fps, cached...)
		start = start.Add(day)
	}

	return fps, start
}

func (s *rejectedPatchSetSource) writeCache(ctx context.Context, byDay map[time.Time][]*RejectedPatchSet) {
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
		s.cacheFile(day).TryWrite(ctx, byDay[day])
	}
}

func (s *rejectedPatchSetSource) cacheFile(t time.Time) cacheFile {
	return cacheFile(filepath.Join(s.CacheDir, fmt.Sprintf("rejected-patch-sets/%s", t.Format(dayLayout))))
}

const dayLayout = "2006-01-02"

func isUTCDayStart(t time.Time) bool {
	return t.Location() == time.UTC && t.Truncate(day).Equal(t)
}
