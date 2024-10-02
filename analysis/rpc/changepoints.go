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
	"go.chromium.org/luci/analysis/internal/pagination"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

var refHashRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

type ChangePointClient interface {
	// ReadChangepoints read changepoints for a certain week. The week is specified by any time at that week in UTC.
	ReadChangepoints(ctx context.Context, project string, week time.Time) ([]*changepoints.ChangepointDetailRow, error)
	ReadChangepointGroupSummaries(ctx context.Context, opts changepoints.ReadChangepointGroupSummariesOptions) ([]*changepoints.GroupSummary, string, error)
	ReadChangepointsInGroup(ctx context.Context, opts changepoints.ReadChangepointsInGroupOptions) (changepoints []*changepoints.ChangepointRow, err error)
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
func (c *changepointsServer) QueryChangepointGroupSummaries(ctx context.Context, request *pb.QueryChangepointGroupSummariesRequestLegacy) (*pb.QueryChangepointGroupSummariesResponseLegacy, error) {
	if err := pbutil.ValidateProject(request.GetProject()); err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "project").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, request.Project, perms.PermListChangepointGroups); err != nil {
		return nil, err
	}
	if err := validateQueryChangepointGroupSummariesRequestLegacy(request); err != nil {
		return nil, invalidArgumentError(err)
	}
	week := time.Now().UTC()
	if request.BeginOfWeek != nil {
		week = request.BeginOfWeek.AsTime()
	}
	rows, err := c.changePointClient.ReadChangepoints(ctx, request.Project, week)
	if err != nil {
		return nil, errors.Annotate(err, "read BigQuery changepoints").Err()
	}
	groups := changepoints.GroupChangepoints(ctx, rows)
	groupSummaries := make([]*pb.ChangepointGroupSummary, 0, len(groups))
	for _, g := range groups {
		filtered := filterAndSortChangepointsWithPredicate(g, request.Predicate)
		if len(filtered) > 0 {
			groupSummary, err := aggregateChangepoints(filtered)
			if err != nil {
				return nil, errors.Annotate(err, "construct changepoint group summary proto").Err()
			}
			groupSummaries = append(groupSummaries, groupSummary)
		}
	}
	// Sort the groups in start_hour DESC order. This sort is deterministic given the uniqueness of
	// (test_id, variant_hash, ref_hash, nominal_start_position) for all changepoints.
	sort.Slice(groupSummaries, func(i, j int) bool {
		ti, tj := groupSummaries[i].CanonicalChangepoint, groupSummaries[j].CanonicalChangepoint
		switch {
		case !ti.StartHour.AsTime().Equal(tj.StartHour.AsTime()):
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
	return &pb.QueryChangepointGroupSummariesResponseLegacy{GroupSummaries: groupSummaries}, nil
}

// QueryChangepointGroupSummaries groups changepoints in a LUCI project and returns a summary of each group.
func (c *changepointsServer) QueryGroupSummaries(ctx context.Context, request *pb.QueryChangepointGroupSummariesRequest) (*pb.QueryChangepointGroupSummariesResponse, error) {
	if err := pbutil.ValidateProject(request.GetProject()); err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "project").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, request.Project, perms.PermListChangepointGroups); err != nil {
		return nil, err
	}
	if err := validateQueryChangepointGroupSummariesRequest(request); err != nil {
		return nil, invalidArgumentError(err)
	}
	pageSize := int(pageSizeLimiter.Adjust(request.PageSize))
	opts := changepoints.ReadChangepointGroupSummariesOptions{
		Project:       request.Project,
		TestIDContain: request.Predicate.GetTestIdContain(),
		PageSize:      pageSize,
		PageToken:     request.PageToken,
	}
	rows, nextPageToken, err := c.changePointClient.ReadChangepointGroupSummaries(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "read BigQuery changepoint groups").Err()
	}
	results := make([]*pb.ChangepointGroupSummary, 0, len(rows))
	for _, row := range rows {
		result, err := toChangepointGroupSummary(row)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return &pb.QueryChangepointGroupSummariesResponse{GroupSummaries: results, NextPageToken: nextPageToken}, nil
}

// QueryChangepointsInGroup finds and returns changepoints in a particular group.
func (c *changepointsServer) QueryChangepointsInGroup(ctx context.Context, req *pb.QueryChangepointsInGroupRequest) (*pb.QueryChangepointsInGroupResponse, error) {
	if err := pbutil.ValidateProject(req.GetProject()); err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "project").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermGetChangepointGroup); err != nil {
		return nil, err
	}
	if err := validateQueryChangepointsInGroupRequest(req); err != nil {
		return nil, invalidArgumentError(err)
	}
	opts := changepoints.ReadChangepointsInGroupOptions{
		Project:       req.Project,
		TestID:        req.GroupKey.TestId,
		VariantHash:   req.GroupKey.VariantHash,
		RefHash:       req.GroupKey.RefHash,
		StartPosition: req.GroupKey.NominalStartPosition,
		TestIDContain: req.Predicate.GetTestIdContain(),
	}
	rows, err := c.changePointClient.ReadChangepointsInGroup(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "read BigQuery changepoints").Err()
	}
	if len(rows) == 0 {
		return nil, appstatus.Error(codes.NotFound, "changepoint group not found")
	}
	cps := make([]*pb.Changepoint, 0, len(rows))
	for _, row := range rows {
		cp, err := toPBChangepoint(row)
		if err != nil {
			return nil, err
		}
		cps = append(cps, cp)
	}
	return &pb.QueryChangepointsInGroupResponse{
		Changepoints: cps,
	}, nil
}

type rateFunc func(*changepoints.ChangepointDetailRow) float64

func aggregateRate(group []*changepoints.ChangepointDetailRow, f rateFunc) *pb.ChangepointGroupStatistics_RateDistribution {
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

func aggregateRateChange(group []*changepoints.ChangepointDetailRow) *pb.ChangepointGroupStatistics_RateChangeBuckets {
	bucket := pb.ChangepointGroupStatistics_RateChangeBuckets{}
	for _, g := range group {
		change := g.UnexpectedSourceVerdictRateAfter - g.UnexpectedSourceVerdictRateBefore
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
func filterAndSortChangepointsWithPredicate(cps []*changepoints.ChangepointDetailRow, predicate *pb.ChangepointPredicateLegacy) []*changepoints.ChangepointDetailRow {
	filtered := []*changepoints.ChangepointDetailRow{}
	for _, cp := range cps {
		if predicate == nil {
			filtered = append(filtered, cp)
			continue
		}
		if !strings.HasPrefix(cp.TestID, predicate.TestIdPrefix) {
			continue
		}
		rateChange := cp.UnexpectedSourceVerdictRateAfter - cp.UnexpectedSourceVerdictRateBefore
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

func aggregateChangepoints(group []*changepoints.ChangepointDetailRow) (summary *pb.ChangepointGroupSummary, err error) {
	// Set the mimimum changepoint as the canonical changepoint to represent this group.
	// Note, this canonical changepoint is different from the canonical changepoint used to create the group.
	canonical := group[0]
	canonicalpb, err := toPBChangepointLegacy(canonical)
	if err != nil {
		return nil, errors.Annotate(err, "construct changepoint proto").Err()
	}
	return &pb.ChangepointGroupSummary{
		CanonicalChangepoint: canonicalpb,
		Statistics: &pb.ChangepointGroupStatistics{
			Count:                        int32(len(group)),
			UnexpectedVerdictRateBefore:  aggregateRate(group, func(tvr *changepoints.ChangepointDetailRow) float64 { return tvr.UnexpectedSourceVerdictRateBefore }),
			UnexpectedVerdictRateAfter:   aggregateRate(group, func(tvr *changepoints.ChangepointDetailRow) float64 { return tvr.UnexpectedSourceVerdictRateAfter }),
			UnexpectedVerdictRateCurrent: aggregateRate(group, func(tvr *changepoints.ChangepointDetailRow) float64 { return tvr.UnexpectedSourceVerdictRateCurrent }),
			UnexpectedVerdictRateChange:  aggregateRateChange(group),
		},
	}, nil
}

func toChangepointGroupSummary(group *changepoints.GroupSummary) (summary *pb.ChangepointGroupSummary, err error) {
	canonicalpb, err := toPBChangepoint(&group.CanonicalChangepoint)
	if err != nil {
		return nil, errors.Annotate(err, "construct changepoint proto").Err()
	}
	return &pb.ChangepointGroupSummary{
		CanonicalChangepoint: canonicalpb,
		Statistics: &pb.ChangepointGroupStatistics{
			Count:                        int32(group.Total),
			UnexpectedVerdictRateBefore:  toPBRateDistribution(&group.UnexpectedSourceVerdictRateBefore),
			UnexpectedVerdictRateAfter:   toPBRateDistribution(&group.UnexpectedSourceVerdictRateAfter),
			UnexpectedVerdictRateCurrent: toPBRateDistribution(&group.UnexpectedSourceVerdictRateCurrent),
			UnexpectedVerdictRateChange:  toPBRateChangeBuckets(&group.UnexpectedSourveVerdictRateChange),
		},
	}, nil
}

func toPBRateDistribution(dist *changepoints.RateDistribution) *pb.ChangepointGroupStatistics_RateDistribution {
	return &pb.ChangepointGroupStatistics_RateDistribution{
		Average: float32(dist.Mean),
		Buckets: &pb.ChangepointGroupStatistics_RateDistribution_RateBuckets{
			CountLess_5Percent:             int32(dist.Less5Percent),
			CountAbove_5LessThan_95Percent: int32(dist.Above5LessThan95Percent),
			CountAbove_95Percent:           int32(dist.Above95Percent),
		},
	}
}

func toPBRateChangeBuckets(dist *changepoints.RateChangeDistribution) *pb.ChangepointGroupStatistics_RateChangeBuckets {
	return &pb.ChangepointGroupStatistics_RateChangeBuckets{
		CountIncreased_0To_20Percent:   int32(dist.Increase0to20percent),
		CountIncreased_20To_50Percent:  int32(dist.Increase20to50percent),
		CountIncreased_50To_100Percent: int32(dist.Increase50to100percent),
	}
}

func toPBChangepointLegacy(cp *changepoints.ChangepointDetailRow) (*pb.Changepoint, error) {
	variant, err := pbutil.VariantFromJSON(cp.Variant.String())
	if err != nil {
		return nil, err
	}
	return &pb.Changepoint{
		Project:     cp.Project,
		TestId:      cp.TestID,
		VariantHash: cp.VariantHash,
		Variant:     variant,
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
	}, nil
}

func toPBChangepoint(cp *changepoints.ChangepointRow) (*pb.Changepoint, error) {
	variant, err := pbutil.VariantFromJSON(cp.Variant.String())
	if err != nil {
		return nil, err
	}
	return &pb.Changepoint{
		Project:     cp.Project,
		TestId:      cp.TestID,
		VariantHash: cp.VariantHash,
		Variant:     variant,
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
	}, nil
}

func validateQueryChangepointGroupSummariesRequestLegacy(req *pb.QueryChangepointGroupSummariesRequestLegacy) error {
	// Project already validated by caller.

	if req.Predicate != nil {
		if err := validateChangepointPredicateLegacy(req.Predicate); err != nil {
			return errors.Annotate(err, "predicate").Err()
		}
	}
	if req.BeginOfWeek != nil {
		isSunday := req.BeginOfWeek.AsTime().Weekday() == time.Sunday
		isMidnight := req.BeginOfWeek.AsTime().Truncate(24*time.Hour) == req.BeginOfWeek.AsTime()
		if !isSunday || !isMidnight {
			return errors.New("begin_of_week: must be Sunday midnight")
		}
	}

	return nil
}

func validateQueryChangepointGroupSummariesRequest(req *pb.QueryChangepointGroupSummariesRequest) error {
	// Project already validated by caller.

	if req.Predicate != nil {
		if err := validateChangepointPredicate(req.Predicate); err != nil {
			return errors.Annotate(err, "predicate").Err()
		}
	}
	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}
	return nil
}

func validateQueryChangepointsInGroupRequest(req *pb.QueryChangepointsInGroupRequest) error {
	// Project already validated by caller.

	if req.Predicate != nil {
		if err := validateChangepointPredicate(req.Predicate); err != nil {
			return errors.Annotate(err, "predicate").Err()
		}
	}
	if err := validateGroupKey(req.GroupKey); err != nil {
		return errors.Annotate(err, "group_key").Err()
	}
	return nil
}

func validateGroupKey(key *pb.QueryChangepointsInGroupRequest_ChangepointIdentifier) error {
	if key == nil {
		return errors.New("unspecified")
	}
	if err := rdbpbutil.ValidateTestID(key.TestId); err != nil {
		return errors.Annotate(err, "test_id").Err()
	}
	if err := ValidateVariantHash(key.VariantHash); err != nil {
		return errors.Annotate(err, "variant_hash").Err()
	}
	if err := ValidateRefHash(key.RefHash); err != nil {
		return errors.Annotate(err, "ref_hash").Err()
	}
	return nil
}

func validateChangepointPredicate(predicate *pb.ChangepointPredicate) error {
	if predicate == nil {
		return errors.New("unspecified")
	}
	if predicate.TestIdContain != "" {
		if err := validateTestIDPart(predicate.TestIdContain); err != nil {
			return errors.Annotate(err, "test_id_prefix").Err()
		}
	}
	return nil
}

func validateChangepointPredicateLegacy(predicate *pb.ChangepointPredicateLegacy) error {
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
