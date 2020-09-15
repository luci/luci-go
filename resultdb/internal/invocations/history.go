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
	// HistoryWindowYears specifies how far back to query results history by
	// default.
	HistoryWindowYears = 2

	// ClockDriftBufferSeconds how many seconds to add to the current time when
	// when using as a default upper bound for the query window.
	ClockDriftBufferSeconds = 600
)

// Historical is a reference to a set of "simultaneous"[1] invocations
// paired with the ordinal fields that they are indexed under.
//
// To the extent that they share a commit position or an ancestor invocation.
type Historical struct {
	ID        ID
	Timestamp *timestamp.Timestamp
	// TODO(crbug.com/1107680): Add support for commit position.
}

// Concurrent determines if the other invocation would be indexed with the
// same ordinal field values as this one.
func (h *Historical) Concurrent(other *Historical) bool {
	return h.Timestamp == other.Timestamp
}

// ByTimestamp queries invocations in the results history timestamp index.
func ByTimestamp(ctx context.Context, timeRange *pb.TimeRange, realm string) ([]*Historical, error) {
	var err error

	// We keep results for up to ~2 years, use this lower bound if one is not
	// given.
	minTime := clock.Now(ctx).AddDate(-HistoryWindowYears, 0, 0)
	if timeRange.GetEarliest() != nil {
		if minTime, err = ptypes.Timestamp(timeRange.GetEarliest()); err != nil {
			return nil, errors.Annotate(err, "min_time").Err()
		}
	}

	// If unspecified, get results up to the present time.
	// Plus a buffer to acount for possible clock drift.
	maxTime := clock.Now(ctx).Add(ClockDriftBufferSeconds * time.Second)
	if timeRange.GetLatest() != nil {
		if maxTime, err = ptypes.Timestamp(timeRange.GetLatest()); err != nil {
			return nil, errors.Annotate(err, "max_time").Err()
		}
	}

	st := spanner.NewStatement(`
		SELECT
			i.InvocationId,
			i.HistoryTime,
		FROM Invocations@{FORCE_INDEX=InvocationsByTimestamp} i
		WHERE i.Realm = @realm AND i.HistoryTime BETWEEN @minTime AND @maxTime
		ORDER BY i.HistoryTime DESC
		LIMIT @pageSize
	`)
	ret := make([]*Historical, 0, 50)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"realm":    realm,
		"pageSize": cap(ret),
		"minTime":  minTime,
		"maxTime":  maxTime,
	})
	var b spanutil.Buffer
	err = spanutil.Query(ctx, st, func(row *spanner.Row) error {
		inv := &Historical{}
		if err := b.FromSpanner(row, &inv.ID, &inv.Timestamp); err != nil {
			return err
		}
		ret = append(ret, inv)
		return nil
	})
	return ret, err
}
