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

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/aip"
	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/reclustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	"go.chromium.org/luci/analysis/internal/perms"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// MaxClusterRequestSize is the maximum number of test results to cluster in
// one call to Cluster(...).
const MaxClusterRequestSize = 1000

// MaxBatchGetClustersRequestSize is the maximum number of clusters to obtain
// impact for in one call to BatchGetClusters().
const MaxBatchGetClustersRequestSize = 1000

type AnalysisClient interface {
	ReadClusters(ctx context.Context, luciProject string, clusterIDs []clustering.ClusterID) ([]*analysis.Cluster, error)
	ReadClusterFailures(ctx context.Context, options analysis.ReadClusterFailuresOptions) (cfs []*analysis.ClusterFailure, err error)
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
	if !config.ProjectRe.MatchString(req.Project) {
		return nil, invalidArgumentError(errors.Reason("project").Err())
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

func (c *clustersServer) BatchGet(ctx context.Context, req *pb.BatchGetClustersRequest) (*pb.BatchGetClustersResponse, error) {
	project, err := parseProjectName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}
	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetCluster, perms.PermExpensiveClusterQueries); err != nil {
		return nil, err
	}

	if len(req.Names) > MaxBatchGetClustersRequestSize {
		return nil, invalidArgumentError(fmt.Errorf(
			"too many names: at most %v clusters can be retrieved in one request", MaxBatchGetClustersRequestSize))
	}
	if len(req.Names) == 0 {
		// Return INVALID_ARGUMENT if no names specified, as per google.aip.dev/231.
		return nil, invalidArgumentError(errors.New("names must be specified"))
	}

	cfg, err := readProjectConfig(ctx, project)
	if err != nil {
		return nil, err
	}

	// The cluster ID requested in each request item.
	clusterIDs := make([]clustering.ClusterID, 0, len(req.Names))

	for i, name := range req.Names {
		clusterProject, clusterID, err := parseClusterName(name)
		if err != nil {
			return nil, invalidArgumentError(errors.Annotate(err, "name %v", i).Err())
		}
		if clusterProject != project {
			return nil, invalidArgumentError(fmt.Errorf("name %v: project must match parent project (%q)", i, project))
		}
		clusterIDs = append(clusterIDs, clusterID)
	}

	clusters, err := c.analysisClient.ReadClusters(ctx, project, clusterIDs)
	if err != nil {
		if err == analysis.ProjectNotExistsErr {
			return nil, appstatus.Error(codes.NotFound,
				"LUCI Analysis BigQuery dataset not provisioned for project or cluster analysis is not yet available")
		}
		return nil, err
	}

	readClusterByID := make(map[clustering.ClusterID]*analysis.Cluster)
	for _, c := range clusters {
		readClusterByID[c.ClusterID] = c
	}

	readableRealms, err := perms.QueryRealms(ctx, project, nil, rdbperms.PermListTestResults)
	if err != nil {
		return nil, err
	}
	readableRealmsSet := stringset.NewFromSlice(readableRealms...)

	// As per google.aip.dev/231, the order of responses must be the
	// same as the names in the request.
	results := make([]*pb.Cluster, 0, len(clusterIDs))
	for i, clusterID := range clusterIDs {
		c, ok := readClusterByID[clusterID]
		if !ok {
			c = &analysis.Cluster{
				ClusterID: clusterID,
				// No impact available for cluster (e.g. because no examples
				// in BigQuery). Use suitable default values (all zeros
				// for impact).
			}
		}

		result := &pb.Cluster{
			Name:       req.Names[i],
			HasExample: ok,
			UserClsFailedPresubmit: &pb.Cluster_MetricValues{
				OneDay:   newCounts(c.PresubmitRejects1d),
				ThreeDay: newCounts(c.PresubmitRejects3d),
				SevenDay: newCounts(c.PresubmitRejects7d),
			},
			CriticalFailuresExonerated: &pb.Cluster_MetricValues{
				OneDay:   newCounts(c.CriticalFailuresExonerated1d),
				ThreeDay: newCounts(c.CriticalFailuresExonerated3d),
				SevenDay: newCounts(c.CriticalFailuresExonerated7d),
			},
			Failures: &pb.Cluster_MetricValues{
				OneDay:   newCounts(c.Failures1d),
				ThreeDay: newCounts(c.Failures3d),
				SevenDay: newCounts(c.Failures7d),
			},
		}

		if !clusterID.IsBugCluster() && ok {
			example := &clustering.Failure{
				TestID: c.ExampleTestID(),
				Reason: &pb.FailureReason{
					PrimaryErrorMessage: c.ExampleFailureReason.StringVal,
				},
			}

			// Whether the user has access to at least one test result in the cluster.
			canSeeAtLeastOneExample := false
			for _, r := range c.Realms {
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
				result.Title = suggestedClusterTitle(c.ClusterID, example, hasAccessToGivenExample, cfg)
				result.EquivalentFailureAssociationRule = failureAssociationRule(c.ClusterID, example, cfg)
			}
		}
		results = append(results, result)
	}
	return &pb.BatchGetClustersResponse{
		Clusters: results,
	}, nil
}

func newCounts(counts analysis.Counts) *pb.Cluster_MetricValues_Counts {
	return &pb.Cluster_MetricValues_Counts{Nominal: counts.Nominal}
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
			return alg.ClusterKey(cfg, exampleFailure)
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
	if !config.ProjectRe.MatchString(req.Project) {
		return nil, invalidArgumentError(errors.Reason("project").Err())
	}

	// TODO(b/239768873): Provide some sort of fallback for users who do not
	// have permission to run expensive queries if no filters are applied.

	// We could make an implementation that gracefully deals with the situation
	// where the user does not have perms.PermGetRule, but there is currently
	// no point as the LUCI Analysis reader role currently always grants
	// PermGetRule with PermListClusters.
	if err := perms.VerifyProjectPermissions(ctx, req.Project, perms.PermListClusters, perms.PermExpensiveClusterQueries, perms.PermGetRule); err != nil {
		return nil, err
	}
	canSeeRuleDefinition, err := perms.HasProjectPermission(ctx, req.Project, perms.PermGetRuleDefinition)
	if err != nil {
		return nil, err
	}

	var cfg *compiledcfg.ProjectConfig
	var ruleset *cache.Ruleset
	var clusters []*analysis.ClusterSummary
	var bqErr error
	// Parallelise call to Biquery (slow call)
	// with the datastore/spanner calls to reduce the critical path.
	err = parallel.FanOutIn(func(ch chan<- func() error) {
		ch <- func() error {
			start := time.Now()
			var err error
			// Fetch a recent project configuration.
			// (May be a recent value that was cached.)
			cfg, err = readProjectConfig(ctx, req.Project)
			if err != nil {
				return err
			}

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
			opts := &analysis.QueryClusterSummariesOptions{}
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
			opts.Realms, err = perms.QueryRealmsNonEmpty(ctx, req.Project, nil, perms.ListTestResultsAndExonerations...)
			if err != nil {
				bqErr = err
				return nil
			}

			clusters, err = c.analysisClient.QueryClusterSummaries(ctx, req.Project, opts)
			if err != nil {
				if err == analysis.ProjectNotExistsErr {
					bqErr = appstatus.Error(codes.NotFound,
						"LUCI Analysis BigQuery dataset not provisioned for project or cluster analysis is not yet available")
					return nil
				}
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
			ClusterId:                  createClusterIdPB(c.ClusterID),
			PresubmitRejects:           c.PresubmitRejects,
			CriticalFailuresExonerated: c.CriticalFailuresExonerated,
			Failures:                   c.Failures,
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

func (c *clustersServer) QueryClusterFailures(ctx context.Context, req *pb.QueryClusterFailuresRequest) (*pb.QueryClusterFailuresResponse, error) {
	project, clusterID, err := parseClusterFailuresName(req.Parent)
	if err != nil {
		return nil, invalidArgumentError(errors.Annotate(err, "parent").Err())
	}

	if err := perms.VerifyProjectPermissions(ctx, project, perms.PermGetCluster, perms.PermExpensiveClusterQueries); err != nil {
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

	failures, err := c.analysisClient.ReadClusterFailures(ctx, opts)
	if err != nil {
		if err == analysis.ProjectNotExistsErr {
			return nil, appstatus.Error(codes.NotFound,
				"LUCI Analysis BigQuery dataset not provisioned for project or clustered failures not yet available")
		}
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
			Owner: f.PresubmitRunOwner.StringVal,
			Mode:  analysis.FromBQPresubmitRunMode(f.PresubmitRunMode.StringVal),
		}
	}

	variantDef := make(map[string]string)
	for _, v := range f.Variant {
		variantDef[v.Key.StringVal] = v.Value.StringVal
	}
	var variant *pb.Variant
	if len(variantDef) > 0 {
		variant = &pb.Variant{Def: variantDef}
	}
	var tags []*pb.StringPair
	for _, t := range f.Tags {
		tags = append(tags, &pb.StringPair{Key: t.Key.String(), Value: t.Value.String()})
	}

	return &pb.DistinctClusterFailure{
		TestId:                      f.TestID.StringVal,
		Variant:                     variant,
		PartitionTime:               timestamppb.New(f.PartitionTime.Timestamp),
		PresubmitRun:                presubmitRun,
		IsBuildCritical:             f.IsBuildCritical.Bool,
		Exonerations:                exonerations,
		BuildStatus:                 buildStatus,
		IngestedInvocationId:        f.IngestedInvocationID.StringVal,
		IsIngestedInvocationBlocked: f.IsIngestedInvocationBlocked.Bool,
		Changelists:                 changelists,
		Count:                       f.Count,
		Tags:                        tags,
	}
}
