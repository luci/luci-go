// Copyright 2022 The LUCI Authors.
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
	"encoding/hex"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/resultdb/rdbperms"

	"go.chromium.org/luci/analysis/internal/aip"
	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/reclustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// MaxClusterRequestSize is the maximum number of test results to cluster in
// one call to Cluster(...).
const MaxClusterRequestSize = 1000

// MaxBatchGetClustersRequestSize is the maximum number of clusters to obtain
// impact for in one call to BatchGetClusters().
const MaxBatchGetClustersRequestSize = 1000

type AnalysisClient interface {
	ReadCluster(ctx context.Context, luciProject string, clusterID clustering.ClusterID) (*analysis.Cluster, error)
	ReadClusterFailures(ctx context.Context, options analysis.ReadClusterFailuresOptions) (cfs []*analysis.ClusterFailure, err error)
	ReadClusterExoneratedTestVariants(ctx context.Context, options analysis.ReadClusterExoneratedTestVariantsOptions) (tvs []*analysis.ExoneratedTestVariant, err error)
	ReadClusterExoneratedTestVariantBranches(ctx context.Context, options analysis.ReadClusterExoneratedTestVariantBranchesOptions) (tvbs []*analysis.ExoneratedTestVariantBranch, err error)
	ReadClusterHistory(ctx context.Context, options analysis.ReadClusterHistoryOptions) (ret []*analysis.ReadClusterHistoryDay, err error)
	QueryClusterSummaries(ctx context.Context, luciProject string, options *analysis.QueryClusterSummariesOptions) ([]*analysis.ClusterSummary, error)
}

type clustersServer struct {
	analysisClient AnalysisClient
}

func NewClustersServer(analysisClient AnalysisClient) *pb.DecoratedClusters {
	return &pb.DecoratedClusters{
		Prelude:  checkAllowedPrelude,
		Service:  &clustersServer{analysisClient: analysisClient},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// Cluster clusters a list of test failures. See proto definition for more.
func (*clustersServer) Cluster(ctx context.Context, req *pb.ClusterRequest) (*pb.ClusterResponse, error) {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "project").Err())
	}
	// We could make an implementation that gracefully degrades if
	// perms.PermGetRule is not available (i.e. by not returning the
	// bug associated with a rule cluster), but there is currently no point.
	// All LUCI Analysis roles currently always grants both permissions
	// together.
	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermGetClustersByFailure, perms.PermGetRule); err != nil {
		return nil, err
	}

	if len(req.TestResults) > MaxClusterRequestSize {
		return nil, invalidArgumentError(fmt.Errorf(
			"too many test results: at most %v test results can be clustered in one request", MaxClusterRequestSize))
	}

	failures := make([]*clustering.Failure, 0, len(req.TestResults))
	for i, tr := range req.TestResults {
		if err := validateTestResult(i, tr); err != nil {
			return nil, err
		}
		failures = append(failures, &clustering.Failure{
			TestID: tr.TestId,
			Reason: tr.FailureReason,
		})
	}

	// Fetch a recent project configuration.
	// (May be a recent value that was cached.)
	cfg, err := readProjectConfig(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	// Fetch a recent ruleset.
	ruleset, err := reclustering.Ruleset(ctx, req.Project, cache.StrongRead)
	if err != nil {
		return nil, err
	}

	// Perform clustering from scratch. (Incremental clustering does not make
	// sense for this RPC.)
	existing := algorithms.NewEmptyClusterResults(len(req.TestResults))

	results := algorithms.Cluster(cfg, ruleset, existing, failures)

	// Construct the response proto.
	clusteredTRs := make([]*pb.ClusterResponse_ClusteredTestResult, 0, len(results.Clusters))
	for i, r := range results.Clusters {
		request := req.TestResults[i]

		entries := make([]*pb.ClusterResponse_ClusteredTestResult_ClusterEntry, 0, len(r))
		for _, clusterID := range r {
			entry := &pb.ClusterResponse_ClusteredTestResult_ClusterEntry{
				ClusterId: createClusterIdPB(clusterID),
			}
			if clusterID.IsBugCluster() {
				// For bug clusters, the ID of the cluster is also the ID of
				// the rule that defines it. Use this property to lookup the
				// associated rule.
				ruleID := clusterID.ID
				rule := ruleset.ActiveRulesByID[ruleID]
				entry.Bug = createAssociatedBugPB(rule.Rule.BugID, cfg.Config)
			}
			entries = append(entries, entry)
		}
		clusteredTR := &pb.ClusterResponse_ClusteredTestResult{
			RequestTag: request.RequestTag,
			Clusters:   entries,
		}
		clusteredTRs = append(clusteredTRs, clusteredTR)
	}

	version := &pb.ClusteringVersion{
		AlgorithmsVersion: int32(results.AlgorithmsVersion),
		RulesVersion:      timestamppb.New(results.RulesVersion),
		ConfigVersion:     timestamppb.New(results.ConfigVersion),
	}

	return &pb.ClusterResponse{
		ClusteredTestResults: clusteredTRs,
		ClusteringVersion:    version,
	}, nil
}

func validateTestResult(i int, tr *pb.ClusterRequest_TestResult) error {
	if tr.TestId == "" {
		return invalidArgumentError(fmt.Errorf("test result %v: test ID must not be empty", i))
	}
	return nil
}

func (c *clustersServer) Get(ctx context.Context, req *pb.GetClusterRequest) (*pb.Cluster, error) {
	project, clusterID, err := parseClusterName(req.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "name").Err())
	}

	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetCluster); err != nil {
		return nil, err
	}

	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	cluster, err := c.analysisClient.ReadCluster(ctx, project, clusterID)
	if err != nil {
		return nil, err
	}

	readableRealms, err := perms.QueryRealms(ctx, project, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}
	readableRealmsSet := stringset.NewFromSlice(readableRealms...)

	exists := len(cluster.Realms) > 0
	result := &pb.Cluster{
		Name:       req.Name,
		HasExample: exists,
		Metrics:    make(map[string]*pb.Cluster_TimewiseCounts),
	}
	for metricID, metricValue := range cluster.MetricValues {
		result.Metrics[string(metricID)] = createTimewiseCountsPB(metricValue)
	}

	if !clusterID.IsBugCluster() && exists {
		example := &clustering.Failure{
			TestID: cluster.ExampleTestID(),
			Reason: &pb.FailureReason{
				PrimaryErrorMessage: cluster.ExampleFailureReason.StringVal,
			},
		}

		// Whether the user has access to at least one test result in the cluster.
		canSeeAtLeastOneExample := false
		for _, r := range cluster.Realms {
			if readableRealmsSet.Has(r) {
				canSeeAtLeastOneExample = true
				break
			}
		}
		if canSeeAtLeastOneExample {
			// While the user has access to at least one test result in the cluster,
			// they may not have access to the randomly selected example we retrieved
			// from the cluster_summaries table. Therefore, we must be careful not
			// to disclose any aspect of this example other than the
			// clustering key it has in common with all other examples
			// in the cluster.
			hasAccessToGivenExample := false
			result.Title = suggestedClusterTitle(cluster.ClusterID, example, hasAccessToGivenExample, cfg)
			result.EquivalentFailureAssociationRule = failureAssociationRule(cluster.ClusterID, example, cfg)
		}
	}

	return result, nil
}

func createTimewiseCountsPB(counts metrics.TimewiseCounts) *pb.Cluster_TimewiseCounts {
	return &pb.Cluster_TimewiseCounts{
		OneDay:   createCountsPB(counts.OneDay),
		ThreeDay: createCountsPB(counts.ThreeDay),
		SevenDay: createCountsPB(counts.SevenDay),
	}
}

func createCountsPB(counts metrics.Counts) *pb.Cluster_Counts {
	return &pb.Cluster_Counts{Nominal: counts.Nominal}
}

// failureAssociationRule returns the failure association rule for the
// given cluster ID, assuming the provided example is still a current
// example of the cluster.
// It is assumed the user does not have access to the specific test
// result represented by exampleFailure, but does have access to at
// least one other test result in the cluster. As such, this method
// must only return aspects of the test result which are common
// to all test results in this cluster.
func failureAssociationRule(clusterID clustering.ClusterID, exampleFailure *clustering.Failure, cfg *compiledcfg.ProjectConfig) string {
	// Ignore error, it is only returned if algorithm cannot be found.
	alg, _ := algorithms.SuggestingAlgorithm(clusterID.Algorithm)
	if alg != nil {
		// Check the example is still in the cluster. Sometimes cluster
		// examples are stale (e.g. because cluster configuration has
		// changed and re-clustering is yet to be fully complete and
		// reflected in the cluster_summaries table).
		//
		// If the example is stale, it cannot be used as the basis for
		// deriving the failure association rule to show to the user.
		// This is for two reasons:
		// 1) Functionality. The rule derived from the example
		//    would not be the correct rule for this cluster.
		// 2) Security. The example failure provided may not be from a realm
		//    the user has access to. As a result of a configuration change,
		//    it may now be in a new cluster.
		//    There is no guarantee the user has access to any test results
		//    in this new cluster, even if it contains some of the test results
		//    of the old cluster, which the user could see some examples of.
		//    The failure association rule for the new cluster is one that the
		//    user may not be allowed to see.
		exampleClusterID := hex.EncodeToString(alg.Cluster(cfg, exampleFailure))
		if exampleClusterID == clusterID.ID {
			return alg.FailureAssociationRule(cfg, exampleFailure)
		}
	}
	return ""
}

// suggestedClusterTitle returns a human-readable description of the cluster,
// using an example failure to help recover the unhashed clustering key.
// hasAccessToGivenExample indicates if the user has permission to see the specific
// example of the cluster (exampleFailure), or (if false) whether they can
// only see one example (but not necessarily exampleFailure).
// If it is false, the result of this method will not contain any aspects
// of the test result other than the aspects which are common to all other
// test results in the cluster (i.e. the clustering key).
func suggestedClusterTitle(clusterID clustering.ClusterID, exampleFailure *clustering.Failure, hasAccessToGivenExample bool, cfg *compiledcfg.ProjectConfig) string {
	// Ignore error, it is only returned if algorithm cannot be found.
	alg, _ := algorithms.SuggestingAlgorithm(clusterID.Algorithm)
	if alg != nil {
		// Check the example is still in the cluster. Sometimes cluster
		// examples are stale (e.g. because cluster configuration has
		// changed and re-clustering is yet to be fully complete and
		// reflected in the cluster_summaries table).
		//
		// If the example is stale, it cannot be used as the basis for
		// deriving the clustering key (cluster definition) to show to
		// the user. This is for two reasons:
		// 1) Functionality. The clustering key derived from the example
		//    would not be the correct clustering key for this cluster.
		// 2) Security. The example failure provided may not be from a realm
		//    the user has access to. As a result of a configuration change,
		//    it may now be in a new cluster.
		//    There is no guarantee the user has access to any test results
		//    in this new cluster, even if it contains some of the test results
		//    of the current cluster, which the user could see some examples of.
		//    The failure association rule for the new cluster is one that the
		//    user may not be allowed to see.
		exampleClusterID := hex.EncodeToString(alg.Cluster(cfg, exampleFailure))
		if exampleClusterID == clusterID.ID {
			return alg.ClusterTitle(cfg, exampleFailure)
		}
	}
	// Fallback.
	if hasAccessToGivenExample {
		// The user has access to the specific test result used as an example.
		// We are fine to disclose it; we do not have to rely on sanitising it
		// down to the common clustering key.
		if clusterID.IsTestNameCluster() {
			// Fallback for old test name clusters.
			return exampleFailure.TestID
		}
		if clusterID.IsFailureReasonCluster() {
			// Fallback for old reason-based clusters.
			return exampleFailure.Reason.PrimaryErrorMessage
		}
	}
	// Fallback for all other cases.
	return "(definition unavailable due to ongoing reclustering)"
}

func (c *clustersServer) GetReclusteringProgress(ctx context.Context, req *pb.GetReclusteringProgressRequest) (*pb.ReclusteringProgress, error) {
	project, err := parseReclusteringProgressName(req.Name)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "name").Err())
	}
	// Getting reclustering progress is considered part of getting a cluster:
	// whenever you retrieve a cluster, you should be able to tell if the
	// information you are reading is up to date.
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetCluster); err != nil {
		return nil, err
	}

	progress, err := runs.ReadReclusteringProgress(ctx, project)
	if err != nil {
		return nil, err
	}

	return &pb.ReclusteringProgress{
		Name:             req.Name,
		ProgressPerMille: int32(progress.ProgressPerMille),
		Last: &pb.ClusteringVersion{
			AlgorithmsVersion: int32(progress.Last.AlgorithmsVersion),
			RulesVersion:      timestamppb.New(progress.Last.RulesVersion),
			ConfigVersion:     timestamppb.New(progress.Last.ConfigVersion),
		},
		Next: &pb.ClusteringVersion{
			AlgorithmsVersion: int32(progress.Next.AlgorithmsVersion),
			RulesVersion:      timestamppb.New(progress.Next.RulesVersion),
			ConfigVersion:     timestamppb.New(progress.Next.ConfigVersion),
		},
	}, nil
}

func (c *clustersServer) QueryClusterSummaries(ctx context.Context, req *pb.QueryClusterSummariesRequest) (*pb.QueryClusterSummariesResponse, error) {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "project").Err())
	}

	if err := pbutil.ValidateTimeRange(ctx, req.TimeRange); err != nil {
		err = errors.Annotate(err, "time_range").Err()
		return nil, invalidArgumentError(err)
	}

	// TODO(b/239768873): Provide some sort of fallback for users who do not
	// have permission to run expensive queries if no filters are applied.

	// We could make an implementation that gracefully deals with the situation
	// where the user does not have perms.PermGetRule, but there is currently
	// no point as the LUCI Analysis reader role currently always grants
	// PermGetRule with PermListClusters.
	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermListClusters, perms.PermGetRule); err != nil {
		return nil, err
	}
	canSeeRuleDefinition, err := perms.HasProjectPermission(ctx, req.Project, perms.PermGetRuleDefinition)
	if err != nil {
		return nil, err
	}

	// Fetch a recent project configuration.
	// (May be a recent value that was cached.)
	cfg, err := readProjectConfig(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	view := req.View
	if view == pb.ClusterSummaryView_CLUSTER_SUMMARY_VIEW_UNSPECIFIED {
		view = pb.ClusterSummaryView_BASIC
	}
	var includeMetricBreakdown = view == pb.ClusterSummaryView_FULL

	var ruleset *cache.Ruleset
	var clusters []*analysis.ClusterSummary
	var bqErr error
	// Parallelise call to Biquery (slow call)
	// with the datastore/spanner calls to reduce the critical path.
	err = parallel.FanOutIn(func(ch chan<- func() error) {
		ch <- func() error {
			start := time.Now()
			var err error

			// Fetch a recent ruleset.
			ruleset, err = reclustering.Ruleset(ctx, req.Project, cache.StrongRead)
			if err != nil {
				return err
			}
			logging.Infof(ctx, "QueryClusterSummaries: Ruleset part took %v", time.Since(start))
			return nil
		}
		ch <- func() error {
			start := time.Now()
			// To avoid the error returned from the service being non-deterministic
			// if both goroutines error, populate any error encountered here
			// into bqErr and return no error.
			opts := &analysis.QueryClusterSummariesOptions{
				TimeRange:              req.TimeRange,
				IncludeMetricBreakdown: includeMetricBreakdown,
			}
			var err error

			opts.FailureFilter, err = aip.ParseFilter(req.FailureFilter)
			if err != nil {
				bqErr = invalidArgumentError(errors.Annotate(err, "failure_filter").Err())
				return nil
			}
			opts.OrderBy, err = aip.ParseOrderBy(req.OrderBy)
			if err != nil {
				bqErr = invalidArgumentError(errors.Annotate(err, "order_by").Err())
				return nil
			}
			opts.Metrics, err = metricsByName(req.Project, cfg, req.Metrics)
			if err != nil {
				bqErr = invalidArgumentError(errors.Annotate(err, "metrics").Err())
				return nil
			}
			opts.Realms, err = perms.QueryRealmsNonEmpty(ctx, req.Project, nil, perms.ListTestResultsAndExonerations...)
			if err != nil {
				bqErr = err
				return nil
			}

			clusters, err = c.analysisClient.QueryClusterSummaries(ctx, req.Project, opts)
			if err != nil {
				if analysis.InvalidArgumentTag.In(err) {
					bqErr = invalidArgumentError(err)
					return nil
				}
				bqErr = errors.Annotate(err, "query clusters for failures").Err()
				return nil
			}
			logging.Infof(ctx, "QueryClusterSummaries: BigQuery part took %v", time.Since(start))
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	// To avoid the error returned from the service being non-deterministic
	// if both goroutines error, return error from bigQuery part after any other errors.
	if bqErr != nil {
		return nil, bqErr
	}

	result := []*pb.ClusterSummary{}
	for _, c := range clusters {
		cs := &pb.ClusterSummary{
			ClusterId: createClusterIdPB(c.ClusterID),
			Metrics:   make(map[string]*pb.ClusterSummary_MetricValue),
		}
		for id, metricValue := range c.MetricValues {
			cs.Metrics[string(id)] = &pb.ClusterSummary_MetricValue{
				Value:          metricValue.Value,
				DailyBreakdown: metricValue.DailyBreakdown,
			}
		}

		if c.ClusterID.IsBugCluster() {
			ruleID := c.ClusterID.ID
			rule := ruleset.ActiveRulesByID[ruleID]
			if rule != nil {
				cs.Bug = createAssociatedBugPB(rule.Rule.BugID, cfg.Config)
				if canSeeRuleDefinition {
					cs.Title = rule.Rule.RuleDefinition
				} else {
					// Because the query is limited to running over the test
					// failures the user has access to, they have permission
					// to see the example Test ID for the cluster.

					// Attempt to provide a description of the failures matched
					// by the rule from the data the user can see, without
					// revealing the content of the rule itself.
					cs.Title = fmt.Sprintf("Selected failures in %s", c.ExampleTestID)
					if c.UniqueTestIDs > 1 {
						cs.Title += fmt.Sprintf(" (and %v more)", c.UniqueTestIDs-1)
					}
				}
			} else {
				// Rule is inactive / in process of being archived.
				cs.Title = "(rule archived)"
			}
		} else {
			example := &clustering.Failure{
				TestID: c.ExampleTestID,
				Reason: &pb.FailureReason{
					PrimaryErrorMessage: c.ExampleFailureReason.StringVal,
				},
			}
			// Because QueryClusterSummaries only reads failures the user has
			// access to, the example is one the user has access to, and
			// so we can use it for the title.
			hasAccessToGivenExample := true
			cs.Title = suggestedClusterTitle(c.ClusterID, example, hasAccessToGivenExample, cfg)
		}

		result = append(result, cs)
	}
	return &pb.QueryClusterSummariesResponse{ClusterSummaries: result}, nil
}

// metricsByName retrieves the metrics with the given name from a
// given LUCI Project and configuration. If the metric is not
// from the given LUCI Project, an error will be returned.
func metricByName(project string, cfg *compiledcfg.ProjectConfig, name string) (metrics.Definition, error) {
	metricProject, id, err := parseProjectMetricName(name)
	if err != nil {
		return metrics.Definition{}, err
	}
	if metricProject != project {
		return metrics.Definition{}, errors.Reason("metric %s cannot be used as it is from a different LUCI Project", name).Err()
	}
	metric, err := metrics.ByID(id)
	if err != nil {
		return metrics.Definition{}, err
	}
	return metric.AdaptToProject(project, cfg.Config.Metrics), nil
}

// metricsByName retrieves the metrics with the given names from a
// given LUCI Project and configuration. If the metrics are not
// from the given LUCI Project, an error will be returned.
func metricsByName(project string, cfg *compiledcfg.ProjectConfig, names []string) ([]metrics.Definition, error) {
	results := make([]metrics.Definition, 0, len(names))
	for _, name := range names {
		metric, err := metricByName(project, cfg, name)
		if err != nil {
			return nil, err
		}
		results = append(results, metric)
	}
	return results, nil
}

func (c *clustersServer) QueryClusterFailures(ctx context.Context, req *pb.QueryClusterFailuresRequest) (*pb.QueryClusterFailuresResponse, error) {
	project, clusterID, err := parseClusterFailuresName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}

	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetCluster); err != nil {
		return nil, err
	}

	// Fetch a recent project configuration.
	// (May be a recent value that was cached.)
	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	opts := analysis.ReadClusterFailuresOptions{
		Project:   project,
		ClusterID: clusterID,
	}
	opts.Realms, err = perms.QueryRealmsNonEmpty(ctx, project, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		// If the user has permission in no realms, QueryRealmsNonEmpty
		// will return an appstatus error PERMISSION_DENIED.
		// Otherwise, e.g. in case AuthDB was unavailable, the error will
		// not be an appstatus error and the client will get an internal
		// server error.
		return nil, err
	}
	if req.MetricFilter != "" {
		metric, err := metricByName(project, cfg, req.MetricFilter)
		if err != nil {
			return nil, invalidArgumentError(errors.Annotate(err, "filter_metric").Err())
		}
		opts.MetricFilter = &metric
	}

	failures, err := c.analysisClient.ReadClusterFailures(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "query cluster failures").Err()
	}
	response := &pb.QueryClusterFailuresResponse{}
	for _, f := range failures {
		response.Failures = append(response.Failures, createDistinctClusterFailurePB(f))
	}

	return response, nil
}

func createDistinctClusterFailurePB(f *analysis.ClusterFailure) *pb.DistinctClusterFailure {
	var exonerations []*pb.DistinctClusterFailure_Exoneration
	for _, ex := range f.Exonerations {
		exonerations = append(exonerations, &pb.DistinctClusterFailure_Exoneration{
			Reason: analysis.FromBQExonerationReason(ex.Reason.StringVal),
		})
	}

	var changelists []*pb.Changelist
	for _, cl := range f.Changelists {
		changelists = append(changelists, &pb.Changelist{
			Host:     cl.Host.StringVal,
			Change:   cl.Change.Int64,
			Patchset: int32(cl.Patchset.Int64),
		})
	}

	buildStatus := analysis.FromBQBuildStatus(f.BuildStatus.StringVal)

	var presubmitRun *pb.DistinctClusterFailure_PresubmitRun
	if f.PresubmitRunID != nil {
		presubmitRun = &pb.DistinctClusterFailure_PresubmitRun{
			PresubmitRunId: &pb.PresubmitRunId{
				System: f.PresubmitRunID.System.StringVal,
				Id:     f.PresubmitRunID.ID.StringVal,
			},
			Owner:  f.PresubmitRunOwner.StringVal,
			Mode:   analysis.FromBQPresubmitRunMode(f.PresubmitRunMode.StringVal),
			Status: analysis.FromBQPresubmitRunStatus(f.PresubmitRunStatus.StringVal),
		}
	}

	return &pb.DistinctClusterFailure{
		TestId:                      f.TestID.StringVal,
		Variant:                     createVariantPB(f.Variant),
		PartitionTime:               timestamppb.New(f.PartitionTime.Timestamp),
		PresubmitRun:                presubmitRun,
		IsBuildCritical:             f.IsBuildCritical.Bool,
		Exonerations:                exonerations,
		BuildStatus:                 buildStatus,
		IngestedInvocationId:        f.IngestedInvocationID.StringVal,
		IsIngestedInvocationBlocked: f.IsIngestedInvocationBlocked.Bool,
		Changelists:                 changelists,
		Count:                       f.Count,
		FailureReasonPrefix:         f.FailureReasonPrefix.StringVal,
	}
}

func createVariantPB(variant []*analysis.Variant) *pb.Variant {
	def := make(map[string]string)
	for _, v := range variant {
		def[v.Key.StringVal] = v.Value.StringVal
	}
	var result *pb.Variant
	if len(def) > 0 {
		result = &pb.Variant{Def: def}
	}
	return result
}

func (c *clustersServer) QueryExoneratedTestVariants(ctx context.Context, req *pb.QueryClusterExoneratedTestVariantsRequest) (*pb.QueryClusterExoneratedTestVariantsResponse, error) {
	project, clusterID, err := parseClusterExoneratedTestVariantsName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}

	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetCluster); err != nil {
		return nil, err
	}
	opts := analysis.ReadClusterExoneratedTestVariantsOptions{
		Project:   project,
		ClusterID: clusterID,
	}
	opts.Realms, err = perms.QueryRealmsNonEmpty(ctx, project, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		// If the user has permission in no realms, QueryRealmsNonEmpty
		// will return an appstatus error PERMISSION_DENIED.
		// Otherwise, e.g. in case AuthDB was unavailable, the error will
		// not be an appstatus error and the client will get an internal
		// server error.
		return nil, err
	}

	testVariants, err := c.analysisClient.ReadClusterExoneratedTestVariants(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "query exonerated test variants").Err()
	}
	response := &pb.QueryClusterExoneratedTestVariantsResponse{}
	for _, f := range testVariants {
		response.TestVariants = append(response.TestVariants, createClusterExoneratedTestVariant(f))
	}

	return response, nil
}

func createClusterExoneratedTestVariant(tv *analysis.ExoneratedTestVariant) *pb.ClusterExoneratedTestVariant {
	return &pb.ClusterExoneratedTestVariant{
		TestId:                     tv.TestID.StringVal,
		Variant:                    createVariantPB(tv.Variant),
		CriticalFailuresExonerated: tv.CriticalFailuresExonerated,
		LastExoneration:            timestamppb.New(tv.LastExoneration.Timestamp),
	}
}

func (c *clustersServer) QueryExoneratedTestVariantBranches(ctx context.Context, req *pb.QueryClusterExoneratedTestVariantBranchesRequest) (*pb.QueryClusterExoneratedTestVariantBranchesResponse, error) {
	project, clusterID, err := parseClusterExoneratedTestVariantBranchesName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}

	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetCluster); err != nil {
		return nil, err
	}
	opts := analysis.ReadClusterExoneratedTestVariantBranchesOptions{
		Project:   project,
		ClusterID: clusterID,
	}
	opts.Realms, err = perms.QueryRealmsNonEmpty(ctx, project, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		// If the user has permission in no realms, QueryRealmsNonEmpty
		// will return an appstatus error PERMISSION_DENIED.
		// Otherwise, e.g. in case AuthDB was unavailable, the error will
		// not be an appstatus error and the client will get an internal
		// server error.
		return nil, err
	}

	testVariantBranches, err := c.analysisClient.ReadClusterExoneratedTestVariantBranches(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "query exonerated test variant branches").Err()
	}
	response := &pb.QueryClusterExoneratedTestVariantBranchesResponse{}
	for _, tvb := range testVariantBranches {
		response.TestVariantBranches = append(response.TestVariantBranches, createClusterExoneratedTestVariantBranch(tvb))
	}

	return response, nil
}

func createClusterExoneratedTestVariantBranch(tv *analysis.ExoneratedTestVariantBranch) *pb.ClusterExoneratedTestVariantBranch {
	return &pb.ClusterExoneratedTestVariantBranch{
		Project:                    tv.Project.StringVal,
		TestId:                     tv.TestID.StringVal,
		Variant:                    createVariantPB(tv.Variant),
		SourceRef:                  createSourceRef(tv.SourceRef),
		CriticalFailuresExonerated: tv.CriticalFailuresExonerated,
		LastExoneration:            timestamppb.New(tv.LastExoneration.Timestamp),
	}
}

func createSourceRef(sourceRef analysis.SourceRef) *pb.SourceRef {
	result := &pb.SourceRef{}
	if sourceRef.Gitiles != nil {
		result.System = &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    sourceRef.Gitiles.Host.StringVal,
				Project: sourceRef.Gitiles.Project.StringVal,
				Ref:     sourceRef.Gitiles.Ref.StringVal,
			},
		}
	}
	return result
}

// QueryHistory clusters a list of test failures. See proto definition for more.
func (c *clustersServer) QueryHistory(ctx context.Context, req *pb.QueryClusterHistoryRequest) (*pb.QueryClusterHistoryResponse, error) {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "project").Err())
	}

	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermGetConfig); err != nil {
		return nil, err
	}

	cfg, err := readProjectConfig(ctx, req.Project)
	if err != nil {
		return nil, err
	}

	opts := analysis.ReadClusterHistoryOptions{
		Project: req.Project,
		Days:    req.Days,
	}

	opts.FailureFilter, err = aip.ParseFilter(req.FailureFilter)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "failure_filter").Err())
	}

	opts.Metrics, err = metricsByName(req.Project, cfg, req.Metrics)
	if err != nil {
		return nil, invalidArgumentError(err)
	}

	realms, err := perms.QueryRealmsNonEmpty(ctx, req.Project, nil, perms.ListTestResultsAndExonerations...)
	if err != nil {
		// If the user has permission in no realms, QueryRealmsNonEmpty
		// will return an appstatus error PERMISSION_DENIED.
		// Otherwise, e.g. in case AuthDB was unavailable, the error will
		// not be an appstatus error and the client will get an internal
		// server error.
		return nil, err
	}
	opts.Realms = realms

	days, err := c.analysisClient.ReadClusterHistory(ctx, opts)
	if err != nil {
		return nil, errors.Annotate(err, "cluster history").Err()
	}

	response := &pb.QueryClusterHistoryResponse{}
	if len(days) == 0 {
		return response, nil
	}

	for _, day := range days {
		metrics := make(map[string]int32)
		for id, value := range day.MetricValues {
			metrics[id.String()] = value
		}
		response.Days = append(response.Days, &pb.ClusterHistoryDay{
			Metrics: metrics,
			Date:    day.Date.Format("2006-01-02"),
		})
	}
	return response, nil
}
