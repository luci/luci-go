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

	"go.chromium.org/luci/resultdb/internal/spanutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Historical is a reference to an invocation paired with the ordinal fields
// that they are indexed under.
type Historical struct {
	ID        ID
	Timestamp *timestamp.Timestamp
	// TODO(crbug.com/1107680): Add support for commit position.
}

// ByTimestamp queries invocations in the results history timestamp index.
// Only fills the InvocationId and HistoryTime fields of the proto.
func ByTimestamp(ctx context.Context, timeRange *pb.TimeRange, realm string) ([]*Historical, error) {
	var err error
	ret := make([]*Historical, 0, 50)

	// We keep results for up to ~2 years, use this lower bound if one is not
	// given.
	minTime := time.Now().AddDate(-2, 0, 0)
	if timeRange.GetEarliest() != nil {
		minTime, err = ptypes.Timestamp(timeRange.GetEarliest())
		if err != nil {
			return nil, err
		}
	}

	// If unspecified, get results up to the present time.
	maxTime := time.Now()
	if timeRange.GetLatest() != nil {
		maxTime, err = ptypes.Timestamp(timeRange.GetLatest())
		if err != nil {
			return nil, err
		}

	}

	st := spanner.NewStatement(`
		SELECT
		i.InvocationId,
		i.HistoryTime,
		FROM Invocations@{FORCE_INDEX=InvocationsByTimestamp} i
		WHERE i.Realm = @realm AND i.HistoryTime < @maxTime AND i.HistoryTime >= @minTime
		ORDER BY i.HistoryTime DESC
		LIMIT @pageSize
	`)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"realm":    realm,
		"pageSize": 50,
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
