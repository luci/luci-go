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

package sweep

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/tq/internal/db"
	"go.chromium.org/luci/server/tq/internal/metrics"
	"go.chromium.org/luci/server/tq/internal/partition"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

// ScanParams contains parameters for the Scan call.
type ScanParams struct {
	DB            db.DB                // DB to use to fetch reminders
	Partition     *partition.Partition // the keyspace partition to scan
	KeySpaceBytes int                  // length of the reminder keys (usually 16)

	TasksPerScan        int // caps maximum number of reminders to process
	SecondaryScanShards int // caps the number of follow-up scans

	Level int // recursion level (0 == the root task)
}

// Scan scans the given partition of the Reminders' keyspace.
//
// Returns a list of stale reminders which likely match crashed AddTask calls.
// The caller is expected to eventually execute corresponding Cloud Tasks
// calls and delete these reminders, lest they'll be rediscovered during the
// next scan.
//
// If unable to complete the scan of the given part of the keyspace and Level is
// less than 2, it intelligently partitions the not-yet-scanned keyspace into
// several partitions for the follow up and returns them as well.
//
// Logs errors inside, but doesn't return them.
func Scan(ctx context.Context, p *ScanParams) ([]*reminder.Reminder, partition.SortedPartitions) {
	l, h := p.Partition.QueryBounds(p.KeySpaceBytes)

	startedAt := clock.Now(ctx)
	rs, err := p.DB.FetchRemindersMeta(ctx, l, h, p.TasksPerScan)
	// TODO(crbug.com/1201436): Use .Milliseconds() instead.
	durMS := float64(int64(clock.Now(ctx).Sub(startedAt)) / 1e6)

	status := ""
	needMoreScans := false
	switch {
	case len(rs) >= p.TasksPerScan:
		if len(rs) > p.TasksPerScan {
			logging.Errorf(ctx, "bug: %s.FetchRemindersMeta returned %d > limit %d",
				p.DB.Kind(), len(rs), p.TasksPerScan)
		}
		status = "limit"
		// There may be more items in the partition.
		needMoreScans = true
	case err == nil:
		// Scan covered everything.
		status = "OK"
	case ctx.Err() == context.DeadlineExceeded && err != nil:
		status = "timeout"
		// Nothing fetched before timeout should not happen frequently.
		// To avoid waiting until next SweepAll(), follow up with scans on
		// sub-partitions.
		needMoreScans = true
	default:
		status = "fail"
	}

	metrics.SweepFetchMetaDurationsMS.Add(ctx, durMS, status, p.Level, p.DB.Kind())
	metrics.SweepFetchMetaReminders.Add(ctx, int64(len(rs)), status, p.Level, p.DB.Kind())

	var scanParts partition.SortedPartitions

	if needMoreScans {
		if len(rs) == 0 {
			// We timed out before fetching anything at all. Divide the initial range
			// into smaller chunks.
			scanParts = p.Partition.Split(p.SecondaryScanShards)
		} else {
			// We fetched something but then hit the limit or timed out. Divide
			// the range after the last fetched Reminder.
			scanParts = p.Partition.EducatedSplitAfter(
				rs[len(rs)-1].ID,
				len(rs),
				// Aim to hit these many Reminders per follow up sweep task,
				p.TasksPerScan,
				// but create at most these many.
				p.SecondaryScanShards,
			)
		}
	}

	// Keep only sufficiently old reminders.
	filtered := filterOutTooFresh(ctx, rs, p.Level, p.DB.Kind())

	if err != nil {
		if len(filtered) == 0 && len(scanParts) == 0 {
			logging.Errorf(ctx, "Scan failed without returning any results: %s", err)
			return nil, nil
		}
		logging.Warningf(ctx, "Got %d reminders and %d follow-up ranges and then failed with: %s", len(filtered), len(scanParts), err)
	} else if len(filtered) != 0 || len(scanParts) != 0 {
		logging.Infof(ctx, "Got %d reminders and %d follow-up ranges", len(filtered), len(scanParts))
	}

	// Refuse to scan deeper than 2 levels.
	if p.Level >= 2 && len(scanParts) != 0 {
		logging.Errorf(ctx, "Refusing to recurse deeper, abandoning scans of %v", scanParts)
		scanParts = nil
	}

	return filtered, scanParts
}

// filterOutTooFresh throws away reminders that are too fresh.
//
// There's a high chance they will be processed on AddTask happy path, we
// shouldn't interfere.
//
// Mutates & re-uses the given Reminders slice. Updates metricReminderAge based
// on all fetched reminders.
//
// `lvl` and `db` used for metrics only.
func filterOutTooFresh(ctx context.Context, reminders []*reminder.Reminder, lvl int, db string) []*reminder.Reminder {
	now := clock.Now(ctx)
	filtered := reminders[:0]
	for _, r := range reminders {
		staleness := now.Sub(r.FreshUntil)
		// TODO(crbug.com/1201436): Use staleness.Milliseconds() instead.
		metrics.ReminderStalenessMS.Add(ctx, float64(int64(staleness)/1e6), lvl, db)
		if staleness >= 0 {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
