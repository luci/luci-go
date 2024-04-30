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

package resultdb

import (
	"context"

	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/resultcount"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// QueryTestResultStatistics implements pb.ResultDBServer.
func (s *resultDBServer) QueryTestResultStatistics(ctx context.Context, in *pb.QueryTestResultStatisticsRequest) (*pb.QueryTestResultStatisticsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := permissions.VerifyInvocationsByName(ctx, in.Invocations, rdbperms.PermListTestResults); err != nil {
		return nil, err
	}

	if err := validateQueryRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	// Query is valid - increment the queryInvocationsCount metric
	queryInvocationsCount.Add(ctx, 1, "QueryTestResultStatistics", len(in.Invocations))

	// Get the transitive closure.
	invs, err := graph.Reachable(ctx, invocations.MustParseNames(in.Invocations))
	if err != nil {
		return nil, err
	}
	invocationIds, err := invs.IDSet()
	if err != nil {
		return nil, err
	}
	totalNum, err := resultcount.ReadTestResultCount(ctx, invocationIds)
	if err != nil {
		return nil, err
	}

	return &pb.QueryTestResultStatisticsResponse{
		TotalTestResults: totalNum,
	}, nil
}
