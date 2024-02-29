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
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

// CountTasks returns the latest task count for the given request.
func (srv *TasksServer) CountTasks(ctx context.Context, req *apipb.TasksCountRequest) (*apipb.TasksCount, error) {
	if req.Start == nil {
		return nil, status.Errorf(codes.InvalidArgument, "start timestamp is required")
	}
	// Set the End time if none is provided.
	if req.End == nil {
		req.End = timestamppb.New(clock.Now(ctx))
	}

	// If tags has length 0, tagsFilter and pools will both just be empty.
	tagsSP := make([]*apipb.StringPair, len(req.Tags))
	for i, tag := range req.Tags {
		parts := strings.SplitN(tag, ":", 2)
		tagsSP[i] = &apipb.StringPair{Key: parts[0], Value: parts[1]}
	}
	tagsFilter, err := model.NewFilter(tagsSP)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid tags: %s", err)
	}
	pools := tagsFilter.Pools()

	// ACL Check
	// If the caller has global permission, they can access all tasks
	// Otherwise, they are required to provide a pool dimension to check ACL.
	var res acls.CheckResult
	state := State(ctx)
	if len(pools) != 0 {
		res = state.ACL.CheckAllPoolsPerm(ctx, pools, acls.PermPoolsListTasks)
	} else {
		res = state.ACL.CheckServerPerm(ctx, acls.PermPoolsListTasks)
	}
	if !res.Permitted {
		return nil, res.ToGrpcErr()
	}
	queries, err := model.GetTaskResultSummaryQueries(
		ctx,
		&model.TaskResultSummaryQueryOptions{
			Start:      req.Start,
			End:        req.End,
			State:      req.State,
			TagsFilter: &tagsFilter,
		},
		srv.TaskQuerySplitMode,
	)
	if err != nil {
		logging.Errorf(ctx, "Error creating TaskResultSummary query: %s", err)
		return nil, status.Errorf(codes.Internal, "error in query creation")
	}
	// If we only have one query to run, we can make use of an aggregation query and
	// utilize the datastore server-side counting. Otherwise, we need to count locally
	// using datastore.CountMulti().
	useAggregation := len(queries) == 1
	var count int64
	if useAggregation {
		count, err = datastore.Count(ctx, queries[0].EventualConsistency(true))
	} else {
		qs := make([]*datastore.Query, len(queries))
		for i, q := range queries {
			// FirestoreMode ensures all quries are strongly consistent. This
			// ensures a more accurate count.
			qs[i] = q.FirestoreMode(true)
		}
		err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
			count, err = datastore.CountMulti(ctx, qs)
			return err
		}, &datastore.TransactionOptions{ReadOnly: true})
	}
	if err != nil {
		logging.Errorf(ctx, "Error in TaskResultSummary query: %s", err)
		return nil, status.Errorf(codes.Internal, "error in query")
	}
	return &apipb.TasksCount{
		Count: int32(count),
		Now:   timestamppb.New(clock.Now(ctx)),
	}, nil
}
