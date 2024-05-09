// Copyright 2024 The LUCI Authors.
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

package rpcs

import (
	"context"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// CountBots implements the corresponding RPC method.
func (srv *BotsServer) CountBots(ctx context.Context, req *apipb.BotsCountRequest) (*apipb.BotsCount, error) {
	dims, err := model.NewFilter(req.Dimensions)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid dimensions: %s", err)
	}
	if err := CheckListingPerm(ctx, dims, acls.PermPoolsListBots); err != nil {
		return nil, err
	}

	out := &apipb.BotsCount{}

	// State filter => where to put the final count of bots matching it.
	type perStateQuery struct {
		filter model.StateFilter
		res    *int32
	}
	perState := []perStateQuery{
		{model.StateFilter{}, &out.Count},
		{model.StateFilter{Quarantined: apipb.NullableBool_TRUE}, &out.Quarantined},
		{model.StateFilter{InMaintenance: apipb.NullableBool_TRUE}, &out.Maintenance},
		{model.StateFilter{IsDead: apipb.NullableBool_TRUE}, &out.Dead},
		{model.StateFilter{IsBusy: apipb.NullableBool_TRUE}, &out.Busy},
	}

	queries := model.FilterBotsByDimensions(model.BotInfoQuery(), srv.BotQuerySplitMode, dims)

	// If there's only one query, we can upgrade it into an aggregation query and
	// do counting completely on the datastore side. We can't do that though if we
	// need to merge multiple queries with OR operator, since we won't be able to
	// avoid double counting entities that match multiple subqueries at the same
	// time. In that case we will need to manually run multiple keys-only regular
	// queries and merge their results locally (by counting only unique keys).
	//
	// Note that it is tempting to run a single projection query on `composite`
	// repeated field and group bots by state locally (since we need to visit them
	// all anyway to count the total number of bots). But such query surprisingly
	// returns 4x results: one entity per individual value of `composite` field
	// (and not one entity with 4 values in repeated `composite` field as one
	// would expect). So such query is actually ~4x slower in terms of wall clock
	// time than running 5 per-state keys-only queries in parallel (like we do
	// below).
	useAggregation := len(queries) == 1

	// Manually running many queries and merging their results can take a lot of
	// time (up to a minute in some cases). At least do it transactionally to get
	// a consistent snapshot of the state, since otherwise it can drift quite a
	// bit between individual queries. This appears to have no noticeable impact
	// on performance in production (the queries are equally slow either way).
	//
	// If we are going to use aggregation queries, run them non-transactionally,
	// since they are not supported in transactions. They are usually super fast
	// (under a second, even when counting a lot of bots) and the state doesn't
	// change much during such a short interval. We can tolerate "eventual
	// consistency" for such huge performance wins. Swarming CountBots API never
	// promised to be strongly consistent anyway (and never was before).
	err = maybeTxn(ctx, !useAggregation, func(ctx context.Context) error {
		eg, ectx := errgroup.WithContext(ctx)
		for _, subq := range perState {
			filter, res := subq.filter, subq.res
			eg.Go(func() error {
				var count int64
				var err error
				if useAggregation {
					// Note: len(queries) == 1 here, queries[0] is the only query to run.
					count, err = datastore.Count(ectx, model.FilterBotsByState(queries[0], filter).EventualConsistency(true))
				} else {
					// Apply the filter, enable firestore mode to run in the transaction.
					filtered := make([]*datastore.Query, len(queries))
					for i, q := range queries {
						filtered[i] = model.FilterBotsByState(q, filter).EventualConsistency(false).FirestoreMode(true)
					}
					count, err = datastore.CountMulti(ectx, filtered)
				}
				if err != nil {
					if !errors.Is(err, context.Canceled) {
						logging.Errorf(ctx, "Error in BotInfo query with filter %v: %s", filter, err)
					}
					return err
				}
				*res = int32(count)
				return nil
			})
		}
		return eg.Wait()
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "datastore error counting bots")
	}

	out.Now = timestamppb.New(clock.Now(ctx))
	return out, nil
}

// maybeTxn runs the callback either transactionally or not, depending on `txn`.
func maybeTxn(ctx context.Context, txn bool, cb func(ctx context.Context) error) error {
	if txn {
		return datastore.RunInTransaction(ctx, cb, &datastore.TransactionOptions{ReadOnly: true})
	}
	return cb(ctx)
}
