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

	"github.com/golang/protobuf/ptypes"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ByTimestamp queries invocations indexed by timestamp for results history
// purposes.
// Only the InvocationId and HistoryTime Fields of the Invocation will be filled.
func ByTimestamp(ctx context.Context, timeRange *pb.TimeRange, pageSize int, realm string) ([]*pb.Invocation, error) {
	if pageSize <= 0 {
		return nil, errors.Reason("pageSize is required to be a positive integer").Err()
	}
	ret := make([]*pb.Invocation, 0, pageSize)

	minTime, err := ptypes.Timestamp(timeRange.GetEarliest())
	if err != nil {
		return nil, err
	}
	maxTime, err := ptypes.Timestamp(timeRange.GetLatest())
	if err != nil {
		return nil, err
	}
	if maxTime.Before(minTime) {
		maxTime = time.Now()
	}

	st := spanner.NewStatement(`
		SELECT
		i.InvocationId,
		i.CreateTime,
		FROM Invocations@{FORCE_INDEX=InvocationsByTimestamp} i
		WHERE i.Realm = @realm AND i.HistoryTime < @maxTime AND i.HistoryTime >= @minTime
		ORDER BY i.HistoryTime DESC
		LIMIT @pageSize
	`)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"realm":    realm,
		"pageSize": pageSize,
		"minTime":  minTime,
		"maxTime":  maxTime,
	})
	var b spanutil.Buffer
	err = spanutil.Query(ctx, st, func(row *spanner.Row) error {

		var id ID
		inv := &pb.Invocation{}
		if err := b.FromSpanner(row, &id, &inv.CreateTime); err != nil {
			return err
		}
		inv.Name = pbutil.InvocationName(string(id))
		ret = append(ret, inv)
		return nil
	})
	return ret, err
}
