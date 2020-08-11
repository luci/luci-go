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

	"go.chromium.org/luci/common/errors"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// GetTestResultHistory implements pb.ResultDBServer.
func (s *resultDBServer) GetTestResultHistory(ctx context.Context, in *pb.GetTestResultHistoryRequest) (*pb.GetTestResultHistoryResponse, error) {
	// Read realm, check permission
	// validate request
	// TODO(): Support token field
	// tl_invocations, order, more_tl_inv, err := getTopLevelInvocationsForRequest(ctx, in)
	// invocations, more_inv := getClosure(tl_invocations, in.Predicate, after_inv, page_size)
	// test_results, more_results := getMatchingResultsForInvocations(invocations, in.Predicate, after_test, after_result, page_size)o
	return nil, errors.Reason("Not yet implemented").Err()
}

func getTopLevelInvocationsForRequest(ctx context.Context, in *pb.GetTestResultHistoryRequest) ([]*pb.Invocation, bool, pb.GetTestResultHistoryResponse_Order, error) {
	cpRange := in.GetCpRange()
	timeRange := in.GetTimeRange()
	var invocationsByCp, invocationsByTimestamp []*pb.invocation
	var moreByCp, moreByTimestamp bool
	// TODO(): Add predicate filter to query.
	if cpRange != nil || timeRange == nil {
		invocationsByCp, moreByCp, err := invocations.InOrdinalRange(ctx, in.Realm, cpRange.earliest, cpRange.latest)
		if err != nil {
			return nil, 0, err
		}
	}
	if timeRange != nil || cpRange == nil {
		invocationsByTimestamp, moreByTimestamp, err := invocations.InTimestampRange(ctx, in.Realm, timeRange.earliest, timeRange.latest)
		if err != nil {
			return nil, 0, err
		}
	}
	if invocationsByTimestamp == nil || len(invocationsByTimestamp) == 0{
		return invocationsByCp, pb.GetTestResultHistoryResponse_COMMIT_POSITION, nil
	}
	if invocationsByCp == nil || len(invocationsByCp) == 0{
		return invocationsByTimestamp, pb.GetTestResultHistoryResponse_INVOCATION_TIMESTAMP, nil
	}
	if invocationsByCp[0].CreateTime > invocationsByTimestamp[0].CreateTime {
		return invocationsByCp, pb.GetTestResultHistoryResponse_COMMIT_POSITION, nil
	}
	return invocationsByTimestamp, pb.GetTestResultHistoryResponse_INVOCATION_TIMESTAMP, nil
}
