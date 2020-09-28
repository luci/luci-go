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
	"path/filepath"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

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

	// Read what we can from cache.
	ret, start := s.readCache(ctx)

	// Fetch the rest if needed.
	if start.Before(s.endTime) {
		logging.Infof(ctx, "Fetching rejected patchsets between %s and %s. It may take a couple of minutes...", start.Format(dayLayout), s.endTime.Format(dayLayout))
		fetched, err := s.Backend.RejectedPatchSets(RejectedPatchSetsRequest{
			Context:       ctx,
			Authenticator: s.auth,
			StartTime:     start,
			EndTime:       s.endTime,
		})
		if err != nil {
			return nil, err
		}
		logging.Infof(ctx, "Fetched %d patchsets", len(fetched))
		s.writeCache(ctx, fetched)
		ret = append(ret, fetched...)
	}

	return ret, nil
}

// readCache reads rejected patchsets from the on-disk cache.
//
// The returned newStartTime is the date after which the cache is missing
// data. The caller should fetch the rest.
func (s *rejectedPatchSetSource) readCache(ctx context.Context) (rps []*RejectedPatchSet, newStartTime time.Time) {
	start := s.startTime
	for !start.After(s.endTime) {
		var cached []*RejectedPatchSet
		if !s.cacheFile(start).TryRead(ctx, &cached) {
			// We might have a gap in days, so fetch the rest.
			break
		}

		rps = append(rps, cached...)
		start = start.Add(day)
	}

	return rps, start
}

// writeCache puts patchSets to the on-disk cache.
//
// If a patchSet for day D is present, then all patchSets for D must be also
// present.
func (s *rejectedPatchSetSource) writeCache(ctx context.Context, patchSets []*RejectedPatchSet) {
	// Group by day.
	byDay := map[time.Time][]*RejectedPatchSet{}
	for _, rp := range patchSets {
		psDay := rp.Timestamp.Truncate(day)
		byDay[psDay] = append(byDay[psDay], rp)
	}

	for day, rps := range byDay {
		s.cacheFile(day).TryWrite(ctx, rps)
	}
}

func (s *rejectedPatchSetSource) cacheFile(t time.Time) cacheFile {
	return cacheFile(filepath.Join(s.CacheDir, "rejected-patch-sets", s.Backend.Name(), t.Format(dayLayout)))
}

const dayLayout = "2006-01-02"

func isUTCDayStart(t time.Time) bool {
	return t.Location() == time.UTC && t.Truncate(day).Equal(t)
}
