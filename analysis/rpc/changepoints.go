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

package rpc

import (
	"context"
	"sort"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

type ChangePointClient interface {
	ReadChangepoints(ctx context.Context, project string) ([]*changepoints.ChangepointRow, error)
}

// NewChangepointsServer returns a new pb.ChangepointsServer.
func NewChangepointsServer(changePointClient ChangePointClient) pb.ChangepointsServer {
	return &pb.DecoratedChangepoints{
		Prelude:  checkAllowedPrelude,
		Service:  &changepointsServer{changePointClient: changePointClient},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// changepointsServer implements pb.ChangepointsServer.
type changepointsServer struct {
	changePointClient ChangePointClient
}

// QueryChangepointGroupSummaries groups changepoints in a LUCI project and returns a summary of each group.
func (c *changepointsServer) QueryChangepointGroupSummaries(ctx context.Context, request *pb.QueryChangepointGroupSummariesRequest) (*pb.QueryChangepointGroupSummariesResponse, error) {
	// Currently, we only allow Googlers to use this API.
	// TODO: implement proper ACL check with realms.
	if err := checkAllowed(ctx, googlerOnlyGroup); err != nil {
		return nil, err
	}
	if err := validateQueryChangepointGroupSummariesRequest(request); err != nil {
		return nil, invalidArgumentError(err)
	}
	rows, err := c.changePointClient.ReadChangepoints(ctx, request.Project)
	if err != nil {
		return nil, errors.Annotate(err, "read BigQuery changepoints").Err()
	}
	groups := changepoints.GroupChangepoints(rows)
	fiteredGroups := filterChangepointGroupsWithPredicate(groups, request.Predicate)

	groupSummaries := make([]*pb.ChangepointGroupSummary, 0, len(groups))
	for _, g := range fiteredGroups {
		groupSummaries = append(groupSummaries, toChangepointGroupSummary(g))
	}
	// Sort the groups in start_hour DESC order. This sort is deterministic given the uniqueness of
	// (test_id, variant_hash, ref_hash, nominal_start_position) for all changepoints.
	sort.Slice(groupSummaries, func(i, j int) bool {
		ti, tj := groupSummaries[i].CanonicalChangepoint, groupSummaries[j].CanonicalChangepoint
		switch {
		case ti.StartHour != tj.StartHour:
			return ti.StartHour.AsTime().After(tj.StartHour.AsTime())
		case ti.TestId != tj.TestId:
			return ti.TestId < tj.TestId
		case ti.VariantHash != tj.VariantHash:
			return ti.VariantHash < tj.VariantHash
		case ti.RefHash != tj.RefHash:
			return ti.RefHash < tj.RefHash
		default:
			return ti.NominalStartPosition > tj.NominalStartPosition
		}
	})
	if len(groupSummaries) > 1000 {
		// Truncate to 1000 groups to avoid overloading the RPC.
		// TODO: remove this and implement pagination.
		groupSummaries = groupSummaries[:1000]
	}
	return &pb.QueryChangepointGroupSummariesResponse{GroupSummaries: groupSummaries}, nil
}

func toChangepointGroupSummary(group []*changepoints.ChangepointRow) *pb.ChangepointGroupSummary {
	// Set the mimimum changepoint as the canonical changepoint to represent this group.
	// Note, this canonical changepoint is different from the canonical changepoint used to create the group.
	canonical := group[0]
	return &pb.ChangepointGroupSummary{
		CanonicalChangepoint: &pb.Changepoint{
			Project:     canonical.Project,
			TestId:      canonical.TestID,
			VariantHash: canonical.VariantHash,
			RefHash:     canonical.RefHash,
			Ref: &pb.SourceRef{
				System: &pb.SourceRef_Gitiles{
					Gitiles: &pb.GitilesRef{
						Host:    canonical.Ref.Gitiles.Host.String(),
						Project: canonical.Ref.Gitiles.Project.String(),
						Ref:     canonical.Ref.Gitiles.Ref.String(),
					},
				},
			},
			StartHour:                         timestamppb.New(canonical.StartHour),
			StartPositionLowerBound_99Th:      canonical.LowerBound99th,
			StartPositionUpperBound_99Th:      canonical.UpperBound99th,
			NominalStartPosition:              canonical.NominalStartPosition,
			PreviousSegmentNominalEndPosition: canonical.PreviousNominalEndPosition,
		},
		Statistics: &pb.ChangepointGroupStatistics{
			Count:                        int64(len(group)),
			UnexpectedVerdictRateBefore:  aggregateRate(group, func(tvr *changepoints.ChangepointRow) float64 { return tvr.UnexpectedVerdictRateBefore }),
			UnexpectedVerdictRateAfter:   aggregateRate(group, func(tvr *changepoints.ChangepointRow) float64 { return tvr.UnexpectedVerdictRateAfter }),
			UnexpectedVerdictRateCurrent: aggregateRate(group, func(tvr *changepoints.ChangepointRow) float64 { return tvr.UnexpectedVerdictRateCurrent }),
			UnexpectedVerdictRateChange:  aggregateRateChange(group),
		},
	}
}

type rateFunc func(*changepoints.ChangepointRow) float64

func aggregateRate(group []*changepoints.ChangepointRow, f rateFunc) *pb.ChangepointGroupStatistics_RateDistribution {
	stats := pb.ChangepointGroupStatistics_RateDistribution{
		Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{},
	}
	total := float64(0)
	for _, g := range group {
		rate := f(g)
		total += rate
		switch {
		case rate < 0.05:
			stats.Buckets.CountLess_5Percent += 1
		case rate >= 0.95:
			stats.Buckets.CountAbove_95Percent += 1
		default:
			stats.Buckets.CountAbove_5LessThan_95Percent += 1
		}
	}
	stats.Average = float32(total) / float32(len(group))
	return &stats
}

func aggregateRateChange(group []*changepoints.ChangepointRow) *pb.ChangepointGroupStatistics_RateChangeBuckets {
	bucket := pb.ChangepointGroupStatistics_RateChangeBuckets{}
	for _, g := range group {
		change := g.UnexpectedVerdictRateAfter - g.UnexpectedVerdictRateBefore
		switch {
		case change < 0.2:
			bucket.CountIncreased_0To_20Percent += 1
		case change >= 0.5:
			bucket.CountIncreased_50To_100Percent += 1
		default:
			bucket.CountIncreased_20To_50Percent += 1
		}
	}
	return &bucket
}

// filterChangepointGroupsWithPredicate filters the changepoints inside each group. A group with no changepoint after the filter will be removed.
// Changepoints in each group will be sorted in by test id, variant hash, ref hash, nominal start position.
func filterChangepointGroupsWithPredicate(groups [][]*changepoints.ChangepointRow, predicate *pb.ChangepointPredicate) [][]*changepoints.ChangepointRow {
	fiteredGroups := [][]*changepoints.ChangepointRow{}
	for _, group := range groups {
		filtered := []*changepoints.ChangepointRow{}
		for _, g := range group {
			if predicate == nil {
				filtered = append(filtered, g)
				continue
			}
			if !strings.HasPrefix(g.TestID, predicate.TestIdPrefix) {
				continue
			}
			rateChange := g.UnexpectedVerdictRateAfter - g.UnexpectedVerdictRateBefore
			if predicate.UnexpectedVerdictRateChangeRange != nil &&
				(rateChange < float64(predicate.UnexpectedVerdictRateChangeRange.LowerBound) ||
					rateChange > float64(predicate.UnexpectedVerdictRateChangeRange.UpperBound)) {
				continue
			}
			filtered = append(filtered, g)
		}
		if len(filtered) > 0 {
			sort.SliceStable(filtered, func(i, j int) bool {
				return changepoints.CompareTestVariantBranchChangepoint(filtered[i], filtered[j])
			})
			fiteredGroups = append(fiteredGroups, filtered)
		}
	}
	return fiteredGroups
}

func validateQueryChangepointGroupSummariesRequest(req *pb.QueryChangepointGroupSummariesRequest) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if req.Predicate != nil {
		if err := validateChangepointPredicate(req.Predicate); err != nil {
			return errors.Annotate(err, "predicate").Err()
		}
	}
	return nil
}

func validateChangepointPredicate(predicate *pb.ChangepointPredicate) error {
	if predicate.TestIdPrefix != "" {
		if err := validateTestIDPart(predicate.TestIdPrefix); err != nil {
			return errors.Annotate(err, "test_id_prefix").Err()
		}
	}
	changeRange := predicate.UnexpectedVerdictRateChangeRange
	if changeRange != nil {
		if changeRange.LowerBound < 0 || changeRange.LowerBound > 1 {
			return errors.Reason("unexpected_verdict_rate_change_range_range: lower_bound: should between 0 and 1").Err()
		}
		if changeRange.UpperBound < 0 || changeRange.UpperBound > 1 {
			return errors.Reason("unexpected_verdict_rate_change_range_range: upper_bound:  should between 0 and 1").Err()
		}
		if changeRange.UpperBound <= changeRange.LowerBound {
			return errors.Reason("unexpected_verdict_rate_change_range_range: upper_bound must greater or equal to lower_bound").Err()
		}
	}
	return nil
}
