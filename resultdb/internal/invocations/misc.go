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
	"fmt"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/spanutil"
)

// Shards is the sharding level for the Invocations table.
// Column Invocations.ShardId is a value in range [0, Shards).
const Shards = 100

// CurrentMaxShard reads the highest shard id in the Invocations table.
// This may differ from the constant above when it has changed recently.
func CurrentMaxShard(ctx context.Context) (int, error) {
	var ret int64
	err := spanutil.QueryFirstRow(span.Single(ctx), spanner.NewStatement(`
		SELECT ShardId
		FROM Invocations@{FORCE_INDEX=InvocationsByInvocationExpiration}
		ORDER BY ShardID DESC
		LIMIT 1
	`), &ret)
	return int(ret), err
}

// ReadTestResultCount returns the total number of test results of requested
// invocations.
func ReadTestResultCount(ctx context.Context, ids IDSet) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	st := spanner.NewStatement(`
		SELECT SUM(TestResultCount)
		FROM Invocations
		WHERE InvocationId IN UNNEST(@invIDs)
	`)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"invIDs": ids,
	})
	var count spanner.NullInt64
	err := spanutil.QueryFirstRow(ctx, st, &count)
	return count.Int64, err
}

// IncrementTestResultCount increases the TestResultCount of the invocation.
func IncrementTestResultCount(ctx context.Context, id ID, delta int64) error {
	if delta == 0 {
		return nil
	}

	st := spanner.NewStatement(`
		UPDATE Invocations
		SET TestResultCount = TestResultCount + @delta
		WHERE InvocationId = @invID
	`)
	st.Params = spanutil.ToSpannerMap(map[string]interface{}{
		"invID": id,
		"delta": delta,
	})
	switch rowCount, err := span.Update(ctx, st); {
	case err != nil:
		return err
	case rowCount != 1:
		return fmt.Errorf("expected to update 1 row, updated %d row instead", rowCount)
	default:
		return nil
	}
}

// TokenToMap parses a page token to a map.
// The first component of the token is expected to be an invocation ID.
// Convenient to initialize Spanner statement parameters.
// Expects the token to be either empty or have len(keys) components.
// If the token is empty, sets map values to "".
func TokenToMap(token string, dest map[string]interface{}, keys ...string) error {
	if len(keys) == 0 {
		panic("keys is empty")
	}
	switch parts, err := pagination.ParseToken(token); {
	case err != nil:
		return err

	case len(parts) == 0:
		for i, k := range keys {
			if i == 0 {
				dest[k] = ID("")
			} else {
				dest[k] = ""
			}
		}
		return nil

	case len(parts) != len(keys):
		return pagination.InvalidToken(errors.Reason("expected %d components, got %q", len(keys), parts).Err())

	default:
		for i, k := range keys {
			if i == 0 {
				dest[k] = ID(parts[i])
			} else {
				dest[k] = parts[i]
			}
		}
		return nil
	}
}

// ShardStatement splits the statement into multiple shards by invocation
// parameter. For example, if a query retrieves test results from a
// set of invocations, then ShardQuery splits the set of invocations into
// subsets, and returns a statement for each of them.
// The returned statements are ordered by invocation row id.
//
// Users of this function must be careful with LIMIT and ORDER BY operator,
// because each statement would have its own LIMIT and order.
//
// st must have a parameter with the name invIDsParamName, and it must have a
// type IDSet.
//
// TODO(crbug.com/1113071): remove this function.
func ShardStatement(st spanner.Statement, invIDsParamName string) []spanner.Statement {
	idSet := st.Params[invIDsParamName].(IDSet)
	batches := idSet.Batches()
	ret := make([]spanner.Statement, len(batches))
	for i, batch := range idSet.Batches() {
		ret[i] = cloneStatement(st)
		ret[i].Params[invIDsParamName] = batch
	}
	return ret
}

// cloneStatement returns a shallowish copy of st: parameter values are not
// cloned.
func cloneStatement(st spanner.Statement) spanner.Statement {
	clone := spanner.Statement{
		SQL:    st.SQL,
		Params: make(map[string]interface{}, len(st.Params)),
	}
	for k, v := range st.Params {
		clone.Params[k] = v
	}
	return clone
}
