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

package invocations

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	year = 365 * 24 * time.Hour

	// HistoryWindow specifies how far back to query results history by
	// default.
	HistoryWindow = 2 * year

	// ClockDriftBuffer how much time to add to the current time when
	// using it as a default upper bound for the query window.
	ClockDriftBuffer = 5 * time.Minute
)

// ByTimestamp queries indexed invocations in a given time range.
// It executes the callback once for each row, starting with the most recent.
func ByTimestamp(ctx context.Context, realm string, timeRange *pb.TimeRange, callback func(inv ID, ts *timestamp.Timestamp) error) error {
	var err error
	now := clock.Now(ctx)

	// We keep results for up to ~2 years, use this lower bound if one is not
	// given.
	minTime := now.Add(-HistoryWindow)
	if timeRange.GetEarliest() != nil {
		if minTime, err = ptypes.Timestamp(timeRange.GetEarliest()); err != nil {
			return errors.Annotate(err, "timeRange.earliest").Err()
		}
	}

	// If unspecified, get results up to the present time.
	// Plus a buffer to account for possible clock drift.
	maxTime := now.Add(ClockDriftBuffer)
	if timeRange.GetLatest() != nil {
		if maxTime, err = ptypes.Timestamp(timeRange.GetLatest()); err != nil {
			return errors.Annotate(err, "timeRange.latest").Err()
		}
	}

	st := spanner.NewStatement(`
		SELECT
			i.InvocationId,
			i.HistoryTime,
		FROM Invocations@{FORCE_INDEX=InvocationsByTimestamp} i
		WHERE i.Realm = @realm AND i.HistoryTime BETWEEN @minTime AND @maxTime
		ORDER BY i.HistoryTime DESC
	`)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"realm":   realm,
		"minTime": minTime,
		"maxTime": maxTime,
	})

	var b spanutil.Buffer
	return spanutil.Query(ctx, st, func(row *spanner.Row) error {
		var inv ID
		var ts *timestamp.Timestamp
		if err := b.FromSpanner(row, &inv, &ts); err != nil {
			return err
		}
		return callback(inv, ts)
	})
}
