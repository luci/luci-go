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
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var refHashRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

type ChangePointClient interface {
	// ReadChangepoints read changepoints for a certain week. The week is specified by any time at that week in UTC.
	ReadChangepoints(ctx context.Context, project string, week time.Time) ([]*changepoints.ChangepointRow, error)
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
	rows, err := c.changePointClient.ReadChangepoints(ctx, request.Project, time.Now().UTC())
	if err != nil {
		return nil, errors.Annotate(err, "read BigQuery changepoints").Err()
	}
	groups := changepoints.GroupChangepoints(rows)
	groupSummaries := make([]*pb.ChangepointGroupSummary, 0, len(groups))
	for _, g := range groups {
		filtered := filterAndSortChangepointsWithPredicate(g, request.Predicate)
		if len(filtered) > 0 {
			groupSummaries = append(groupSummaries, toChangepointGroupSummary(filtered))
		}
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

// QueryChangepointsInGroup finds and returns changepoints in a particular group.
func (c *changepointsServer) QueryChangepointsInGroup(ctx context.Context, req *pb.QueryChangepointsInGroupRequest) (*pb.QueryChangepointsInGroupResponse, error) {
	// Currently, we only allow Googlers to use this API.
	// TODO: implement proper ACL check with realms.
	if err := checkAllowed(ctx, googlerOnlyGroup); err != nil {
		return nil, err
	}
	if err := validateQueryChangepointsInGroupRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}
	rows, err := c.changePointClient.ReadChangepoints(ctx, req.Project, req.GroupKey.StartHour.AsTime())
	if err != nil {
		return nil, errors.Annotate(err, "read BigQuery changepoints").Err()
	}
	groups := changepoints.GroupChangepoints(rows)
	group, found := changepointGroupWithGroupKey(groups, req.GroupKey)
	if !found {
		return nil, appstatus.Error(codes.NotFound, "changepoint group not found")
	}
	filteredCps := filterAndSortChangepointsWithPredicate(group, req.Predicate)
	changepointsToReturn := make([]*pb.Changepoint, 0, len(filteredCps))
	for _, t := range filteredCps {
		changepointsToReturn = append(changepointsToReturn, toPBChangepoint(t))
	}
	if len(changepointsToReturn) > 1000 {
		// Truncate to 1000 changepoints to avoid overloading the RPC.
		// TODO: remove this and implement pagination.
		changepointsToReturn = changepointsToReturn[:1000]
	}
	return &pb.QueryChangepointsInGroupResponse{
		Changepoints: changepointsToReturn,
	}, nil
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

// filterAndSortChangepointsWithPredicate filters the changepoints.
// Changepoints will be sorted in by test id, variant hash, ref hash, nominal start position.
func filterAndSortChangepointsWithPredicate(cps []*changepoints.ChangepointRow, predicate *pb.ChangepointPredicate) []*changepoints.ChangepointRow {
	filtered := []*changepoints.ChangepointRow{}
	for _, cp := range cps {
		if predicate == nil {
			filtered = append(filtered, cp)
			continue
		}
		if !strings.HasPrefix(cp.TestID, predicate.TestIdPrefix) {
			continue
		}
		rateChange := cp.UnexpectedVerdictRateAfter - cp.UnexpectedVerdictRateBefore
		if predicate.UnexpectedVerdictRateChangeRange != nil &&
			(rateChange < float64(predicate.UnexpectedVerdictRateChangeRange.LowerBound) ||
				rateChange > float64(predicate.UnexpectedVerdictRateChangeRange.UpperBound)) {
			continue
		}
		filtered = append(filtered, cp)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return changepoints.CompareTestVariantBranchChangepoint(filtered[i], filtered[j])
	})
	return filtered
}

// changepointGroupWithGroupKey finds the group that matches the group_key,
// returns the matched group and a bool to indicate whether a matching group is found.
// We consider the changepoint group matches the group_key if there is a changepoint in the group such that:
//   - the changepoint is of the same test variant branch as group_key (same test_id, variant_hash, ref_hash), AND
//   - the changepoint's 99% confidence interval includes the group_key's nominal_start_position, AND
//   - out of all changepoints that satisfy the above 2 rules, the changepoints has the shortest nominal start position distance with group_key.
func changepointGroupWithGroupKey(groups [][]*changepoints.ChangepointRow, groupKey *pb.QueryChangepointsInGroupRequest_ChangepointIdentifier) ([]*changepoints.ChangepointRow, bool) {
	var matchingGroup []*changepoints.ChangepointRow
	var matchingChangepoint *changepoints.ChangepointRow
	distanceFromGroupKey := func(cp *changepoints.ChangepointRow) int64 {
		return int64(math.Abs(float64(groupKey.NominalStartPosition) - float64(cp.NominalStartPosition)))
	}
	for _, group := range groups {
		for _, cp := range group {
			if groupKey.TestId == cp.TestID &&
				groupKey.VariantHash == cp.VariantHash &&
				groupKey.RefHash == cp.RefHash &&
				groupKey.NominalStartPosition <= cp.UpperBound99th &&
				groupKey.NominalStartPosition >= cp.LowerBound99th &&
				(matchingChangepoint == nil || distanceFromGroupKey(cp) < distanceFromGroupKey(matchingChangepoint)) {
				matchingGroup = group
				matchingChangepoint = cp
			}
		}
	}
	if matchingGroup == nil {
		return nil, false
	}
	return matchingGroup, true
}

func toChangepointGroupSummary(group []*changepoints.ChangepointRow) *pb.ChangepointGroupSummary {
	// Set the mimimum changepoint as the canonical changepoint to represent this group.
	// Note, this canonical changepoint is different from the canonical changepoint used to create the group.
	canonical := group[0]
	return &pb.ChangepointGroupSummary{
		CanonicalChangepoint: toPBChangepoint(canonical),
		Statistics: &pb.ChangepointGroupStatistics{
			Count:                        int32(len(group)),
			UnexpectedVerdictRateBefore:  aggregateRate(group, func(tvr *changepoints.ChangepointRow) float64 { return tvr.UnexpectedVerdictRateBefore }),
			UnexpectedVerdictRateAfter:   aggregateRate(group, func(tvr *changepoints.ChangepointRow) float64 { return tvr.UnexpectedVerdictRateAfter }),
			UnexpectedVerdictRateCurrent: aggregateRate(group, func(tvr *changepoints.ChangepointRow) float64 { return tvr.UnexpectedVerdictRateCurrent }),
			UnexpectedVerdictRateChange:  aggregateRateChange(group),
		},
	}
}

func toPBChangepoint(cp *changepoints.ChangepointRow) *pb.Changepoint {
	return &pb.Changepoint{
		Project:     cp.Project,
		TestId:      cp.TestID,
		VariantHash: cp.VariantHash,
		RefHash:     cp.RefHash,
		Ref: &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    cp.Ref.Gitiles.Host.String(),
					Project: cp.Ref.Gitiles.Project.String(),
					Ref:     cp.Ref.Gitiles.Ref.String(),
				},
			},
		},
		StartHour:                         timestamppb.New(cp.StartHour),
		StartPositionLowerBound_99Th:      cp.LowerBound99th,
		StartPositionUpperBound_99Th:      cp.UpperBound99th,
		NominalStartPosition:              cp.NominalStartPosition,
		PreviousSegmentNominalEndPosition: cp.PreviousNominalEndPosition,
	}
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

func validateQueryChangepointsInGroupRequest(req *pb.QueryChangepointsInGroupRequest) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Annotate(err, "project").Err()
	}
	if req.Predicate != nil {
		if err := validateChangepointPredicate(req.Predicate); err != nil {
			return errors.Annotate(err, "predicate").Err()
		}
	}
	if req.GroupKey == nil {
		return errors.New("group_key: unspecified")
	}
	if err := rdbpbutil.ValidateTestID(req.GroupKey.TestId); err != nil {
		return errors.Annotate(err, "test_id").Err()
	}
	if err := ValidateVariantHash(req.GroupKey.VariantHash); err != nil {
		return errors.Annotate(err, "variant_hash").Err()
	}
	if err := ValidateRefHash(req.GroupKey.RefHash); err != nil {
		return errors.Annotate(err, "ref_hash").Err()
	}
	return nil
}

func validateChangepointPredicate(predicate *pb.ChangepointPredicate) error {
	if predicate == nil {
		return errors.New("unspecified")
	}
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

func ValidateVariantHash(variantHash string) error {
	if !variantHashRe.MatchString(variantHash) {
		return errors.Reason("variant hash %s must match %s", variantHash, variantHashRe).Err()
	}
	return nil
}

func ValidateRefHash(refHash string) error {
	if !refHashRe.MatchString(refHash) {
		return errors.Reason("ref hash %s must match %s", refHash, refHashRe).Err()
	}
	return nil
}
