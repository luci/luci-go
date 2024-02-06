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

// Package updater contains methods to orchestrate automatic bug management,
// including automatic bug filing and automatic priority updates/auto-closure.
package updater

import (
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/rules/lang"
	"go.chromium.org/luci/analysis/internal/clustering/runs"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// testnameThresholdInflationPercent is the percentage factor by which
// the bug filing threshold is inflated when applied to test-name clusters.
// This is to bias bug-filing towards failure reason clusters, which are
// seen as generally better scoped and more actionable (because they
// focus on one reason for the test failing.)
//
// The value of 34% was selected as it is sufficient to inflate any threshold
// values which are a '3' (e.g. CV runs rejected) to a '4'. Otherwise integer
// discretization of the statistics would cancel out any intended bias.
//
// If changing this value, please also update the comment in
// project_config.proto.
const testnameThresholdInflationPercent = 34

// mergeIntoCycleErr is the error returned if a cycle is detected in a bug's
// merged-into graph when handling a bug marked as duplicate.
var mergeIntoCycleErr = errors.New("a cycle was detected in the bug merged-into graph")

// mergeIntoPermissionErr is the error returned if we get a permission error while traversing and/or
// updating duplicate bugs.
var mergeIntoPermissionErr = errors.New("permission error occured while merging duplicate bugs")

// ruleDefinitionTooLongErr is the error returned if merging two failure
// association rules results in a rule that is too long.
var ruleDefinitionTooLongErr = errors.New("the merged rule definition is too long")

// mergeIntoCycleMessage is the message posted on bugs when LUCI Analysis
// cannot deal with a bug marked as the duplicate of another because of
// a duplicate bug.
const mergeIntoCycleMessage = "LUCI Analysis cannot merge the failure" +
	" association rule for this bug into the rule for the merged-into bug," +
	" because a cycle was detected in the bug merged-into graph. Please" +
	" manually resolve the cycle, or update rules manually and archive the" +
	" rule for this bug."

const mergeIntoPermissionMessage = "LUCI Analysis cannot merge the association rule" +
	" for this bug into the rule for the merged-into bug because" +
	" it doesn't have permission to access the merged-into bug." +
	" Please make sure that LUCI Analysis has access to all the" +
	" bugs in the bug duplicate chain, " +
	" or update rules manually and archive the rule for this bug."

// ruleDefinitionTooLongMessage is the message posted on bugs when
// LUCI Analysis cannot deal with a bug marked as the duplicate of another
// because the merged rule would be too long.
const ruleDefinitionTooLongMessage = "LUCI Analysis cannot merge the failure" +
	" association rule for this bug into the rule for the merged-into bug," +
	" because the merged failure association rule would be too long. Please" +
	" manually update the rule for the merged-into bug and archive the" +
	" rule for this bug."

// BugManager implements bug creation and bug updates for a bug-tracking
// system. The BugManager determines bug content and priority given a
// cluster.
type BugManager interface {
	// Create creates a new bug for the given request, returning its ID
	// (if a bug was created) and any encountered error.
	Create(ctx context.Context, request bugs.BugCreateRequest) bugs.BugCreateResponse
	// Update updates the specified list of bugs.
	//
	// Exactly one response item is returned for each request item.
	// If an error is encountered on a specific bug, the error is recorded
	// on the bug's response item and processing continues.
	//
	// If a catastrophic error occurs, the error is returned
	// at the top-level and the responses slice should be ignored.
	Update(ctx context.Context, bugs []bugs.BugUpdateRequest) ([]bugs.BugUpdateResponse, error)
	// GetMergedInto reads the bug the given bug is merged into (if any).
	// This is to allow step-wise discovery of the canonical bug a bug
	// is merged into (if it exists and there is no cycle in the bug
	// merged-into graph).
	GetMergedInto(ctx context.Context, bug bugs.BugID) (*bugs.BugID, error)
	// UpdateDuplicateSource updates the source bug of a duplicate
	// bug relationship.
	// It normally posts a message advising the user LUCI Analysis
	// has merged the rule for the source bug to the destination
	// (merged-into) bug, and provides a new link to the failure
	// association rule.
	// If a cycle was detected, it instead posts a message that the
	// duplicate bug could not be handled and marks the bug no
	// longer a duplicate to break the cycle.
	UpdateDuplicateSource(ctx context.Context, request bugs.UpdateDuplicateSourceRequest) error
}

// BugUpdater performs updates to bugs and failure association
// rules to keep them in sync with clusters generated by analysis.
type BugUpdater struct {
	// project is the LUCI project to act on behalf of.
	project string
	// analysisClient provides access to cluster analysis.
	analysisClient AnalysisClient
	// managers stores the manager responsible for updating bugs for each
	// bug tracking system (monorail, buganizer, etc.).
	managers map[string]BugManager
	// projectCfg is the snapshot of project configuration to use for
	// the auto-bug filing run.
	projectCfg *compiledcfg.ProjectConfig
	// MaxBugsFiledPerRun is the maximum number of bugs to file each time
	// BugUpdater runs. This throttles the rate of changes to the bug system.
	MaxBugsFiledPerRun int
	// UpdateRuleBatchSize is the maximum number of rules to update in one
	// transaction, when updating rule bug management state.
	UpdateRuleBatchSize int
	// Timestamp of the cron job. Used to timestamp policy activations/deactivations
	// that happen as a result of this run.
	RunTimestamp time.Time
}

// NewBugUpdater initialises a new BugUpdater.
func NewBugUpdater(project string, mgrs map[string]BugManager, ac AnalysisClient, projectCfg *compiledcfg.ProjectConfig, runTimestamp time.Time) *BugUpdater {
	return &BugUpdater{
		project:             project,
		managers:            mgrs,
		analysisClient:      ac,
		projectCfg:          projectCfg,
		MaxBugsFiledPerRun:  1,    // Default value.
		UpdateRuleBatchSize: 1000, // Default value.
		RunTimestamp:        runTimestamp,
	}
}

// Run files/updates bugs to match high-impact clusters as
// identified by analysis. Each bug has a corresponding failure association
// rule.
// The passed progress should reflect the progress of re-clustering as captured
// in the latest analysis.
func (b *BugUpdater) Run(ctx context.Context, reclusteringProgress *runs.ReclusteringProgress) error {
	// Verify we are not currently reclustering to a new version of
	// algorithms or project configuration. If we are, we should
	// suspend bug creation, priority updates and auto-closure
	// as cluster impact is unreliable.
	metricsValid := b.verifyClusterImpactValid(ctx, reclusteringProgress)

	activeRules, err := rules.ReadActive(span.Single(ctx), b.project)
	if err != nil {
		return errors.Annotate(err, "read active failure association rules").Err()
	}

	metricsByRuleID := make(map[string]bugs.ClusterMetrics)
	if metricsValid {
		var thresholds []*configpb.ImpactMetricThreshold
		for _, p := range b.projectCfg.Config.BugManagement.GetPolicies() {
			thresholds = append(thresholds, bugs.ActivationThresholds(p)...)
		}

		// We want to read analysis for two categories of clusters:
		// - Bug Clusters: to update the priority of filed bugs.
		// - Impactful Suggested Clusters: if any suggested clusters may be
		//    near the threshold to file a new bug for, we want to
		//    read them, so we can file a bug. (Note: the thresholding applied
		//    here is weaker than the actual bug filing criteria which is
		//    implemented in this package, it exists mainly to avoid pulling
		//    back all suggested clusters).
		clusters, err := b.analysisClient.ReadImpactfulClusters(ctx, analysis.ImpactfulClusterReadOptions{
			Project:                  b.project,
			Thresholds:               thresholds,
			AlwaysIncludeBugClusters: true,
		})
		if err != nil {
			return errors.Annotate(err, "read impactful clusters").Err()
		}

		// blockedSourceClusterIDs is the set of source cluster IDs for which
		// filing new bugs should be suspended.
		blockedSourceClusterIDs := make(map[clustering.ClusterID]struct{})
		for _, r := range activeRules {
			if !reclusteringProgress.IncorporatesRulesVersion(r.CreateTime) {
				// If a bug cluster was recently filed for a source cluster, and
				// re-clustering and analysis is not yet complete (to move the
				// impact from the source cluster to the bug cluster), do not file
				// another bug for the source cluster.
				// (Of course, if a bug cluster was filed for a source cluster,
				// but the bug cluster's failure association rule was subsequently
				// modified (e.g. narrowed), it is allowed to file another bug
				// if the residual impact justifies it.)
				blockedSourceClusterIDs[r.SourceCluster] = struct{}{}
			}
		}

		if err := b.fileNewBugs(ctx, clusters, blockedSourceClusterIDs); err != nil {
			return err
		}

		for _, cluster := range clusters {
			if cluster.ClusterID.Algorithm == rulesalgorithm.AlgorithmName {
				// Use only impact from latest algorithm version.
				ruleID := cluster.ClusterID.ID
				metricsByRuleID[ruleID] = ExtractResidualMetrics(cluster)
			}
		}
	}

	var rms []ruleWithMetrics
	for _, rule := range activeRules {
		var metrics bugs.ClusterMetrics

		// Metrics are valid if re-clustering and analysis ran on the latest
		// version of this failure association rule. This avoids bugs getting
		// erroneous priority changes while metrics information is incomplete.
		ruleMetricsValid := metricsValid &&
			reclusteringProgress.IncorporatesRulesVersion(rule.PredicateLastUpdateTime)

		if ruleMetricsValid {
			var ok bool
			metrics, ok = metricsByRuleID[rule.RuleID]
			if !ok {
				// If there is no analysis, this means the cluster is
				// empty. Use empty impact.
				metrics = bugs.ClusterMetrics{}
			}
		}
		// Else leave metrics as nil. Bug-updating code takes this as an
		// indication valid metrics are not available and will not attempt
		// priority updates/auto-closure.

		rms = append(rms, ruleWithMetrics{
			RuleID:  rule.RuleID,
			Metrics: metrics,
		})
	}

	// Update bug management state (i.e. policy activations) for existing
	// rules based on current cluster metrics. Prepare the bug update requests
	// based on this state.
	bugsToUpdate, err := b.updateBugManagementState(ctx, rms)
	if err != nil {
		return errors.Annotate(err, "update bug management state").Err()
	}

	// Break bug updates down by bug system.
	bugUpdatesBySystem := make(map[string][]bugs.BugUpdateRequest)
	for _, bug := range bugsToUpdate {
		bugUpdates := bugUpdatesBySystem[bug.Bug.System]
		bugUpdates = append(bugUpdates, bug)
		bugUpdatesBySystem[bug.Bug.System] = bugUpdates
	}

	// Perform bug updates.
	var errs []error
	for system, systemBugsToUpdate := range bugUpdatesBySystem {
		err := b.updateBugsForSystem(ctx, system, systemBugsToUpdate)
		if err != nil {
			errs = append(errs, errors.Annotate(err, "updating bugs in %s", system).Err())
		}
	}
	// Returns nil if len(errs) == 0.
	return errors.Append(errs...)
}

type ruleWithMetrics struct {
	// Rule identifier.
	RuleID string
	// The bug cluster metrics. May be nil if no reliable metrics
	// are available because reclustering is in progress.
	Metrics bugs.ClusterMetrics
}

// updateBugManagementState updates policy activations for the
// specified rules using the given current metric values.
//
// BugUpdateRequests then are created based on the read rules
// and updated bug management state. The returned BugUpdateRequests
// will be in 1:1 correspondance to the specified rules.
func (b *BugUpdater) updateBugManagementState(ctx context.Context, rs []ruleWithMetrics) ([]bugs.BugUpdateRequest, error) {
	// Read and update bug management state in batches.
	// Batching is required as Spanner limits the number of mutations
	// per transaction to 40,000 (as at August 2023):
	// https://cloud.google.com/spanner/quotas#limits-for
	batches := batch(rs, b.UpdateRuleBatchSize)

	result := make([]bugs.BugUpdateRequest, 0, len(rs))
	for _, ruleBatch := range batches {
		var batchResult []bugs.BugUpdateRequest
		batchResult, err := b.updateBugManagementStateBatch(ctx, ruleBatch)
		if err != nil {
			return nil, err
		}

		result = append(result, batchResult...)
	}
	return result, nil
}

func batch[K any](items []K, batchSize int) [][]K {
	if batchSize < 1 {
		panic("batch size must be greater than 0")
	}

	batchCount := (len(items) + batchSize - 1) / batchSize
	result := make([][]K, 0, batchCount)
	for i := 0; i < batchCount; i++ {
		batchStartIndex := i * batchSize             // inclusive
		batchEndIndex := batchStartIndex + batchSize // exclusive
		if batchEndIndex > len(items) {
			batchEndIndex = len(items)
		}
		result = append(result, items[batchStartIndex:batchEndIndex])
	}
	return result
}

// updateBugManagementStateBatch updates policy activations for the
// specified rules using the given current metric values.
//
// BugUpdateRequests then are created based on the read rules
// and updated bug management state. The returned BugUpdateRequests
// will be in 1:1 correspondance to the specified rules.
func (b *BugUpdater) updateBugManagementStateBatch(ctx context.Context, rulesAndMetrics []ruleWithMetrics) ([]bugs.BugUpdateRequest, error) {
	ruleIDs := make([]string, 0, len(rulesAndMetrics))
	for _, rule := range rulesAndMetrics {
		ruleIDs = append(ruleIDs, rule.RuleID)
	}

	var result []bugs.BugUpdateRequest
	f := func(ctx context.Context) error {
		// This transaction may be retried. Reset the result each time
		// the transaction runs to avoid data from previous aborted
		// attempts leaking into subsequent attempts.
		result = make([]bugs.BugUpdateRequest, 0, len(rulesAndMetrics))

		// Read the rules in the transaction again to implement an
		// atomic Read-Update transaction, which protects against
		// update races. Subsequent bug-filing action will be based
		// only on this second read.
		// N.B.: ReadMany returns items in 1:1 correspondence to the request.
		rs, err := rules.ReadMany(ctx, b.project, ruleIDs)
		if err != nil {
			return errors.Annotate(err, "read rules").Err()
		}

		for i, r := range rs {
			// Fetches the corresponding metrics for a rule.
			clusterMetrics := rulesAndMetrics[i].Metrics

			// If metrics data is valid (e.g. no reclustering in progress).
			if clusterMetrics != nil {
				// Update which policies are active.
				updatedBugManagementState, changed := bugs.UpdatePolicyActivations(r.BugManagementState, b.projectCfg.Config.BugManagement.GetPolicies(), clusterMetrics, b.RunTimestamp)
				if changed {
					// Only update the rule if a policy has activated or
					// deactivated, to avoid unnecessary writes and rule
					// cache invalidations.
					r.BugManagementState = updatedBugManagementState

					opts := rules.UpdateOptions{}
					ms, err := rules.Update(r, opts, rules.LUCIAnalysisSystem)
					if err != nil {
						return errors.Annotate(err, "update rule").Err()
					}
					span.BufferWrite(ctx, ms)
				}
			}

			updateRequest := bugs.BugUpdateRequest{
				Bug:                              r.BugID,
				IsManagingBug:                    r.IsManagingBug,
				IsManagingBugPriority:            r.IsManagingBugPriority,
				IsManagingBugPriorityLastUpdated: r.IsManagingBugPriorityLastUpdateTime,
				RuleID:                           r.RuleID,
			}
			updateRequest.BugManagementState = r.BugManagementState
			result = append(result, updateRequest)
		}
		return nil
	}
	if _, err := span.ReadWriteTransaction(ctx, f); err != nil {
		return nil, err
	}
	return result, nil
}

func (b *BugUpdater) updateBugsForSystem(ctx context.Context, system string, bugsToUpdate []bugs.BugUpdateRequest) error {
	manager, ok := b.managers[system]
	if !ok {
		logging.Warningf(ctx, "Encountered bug(s) with an unrecognised manager: %q", system)
		return nil
	}

	// Keep a minute of time in reserve to update rules.
	// It is important that we still update the rules for bugs we did
	// successfully update as some bug behaviours rely on this as
	// part of their control loop (we will keep posting the same
	// comment on the bug until the rule is updated).
	mgrCtx, cancel := bugs.Shorten(ctx, time.Minute)
	defer cancel()

	responses, err := manager.Update(mgrCtx, bugsToUpdate)
	if err != nil {
		// Catastrophic error, exit immediately.
		return errors.Annotate(err, "update bugs").Err()
	}

	// The set of non-catastrophic errors encountered so far.
	var errs []error
	// The set of bugs marked as duplicate encountered.
	var duplicateBugs []bugs.DuplicateBugDetails
	// The updates to failure association rules required.
	var updateRuleRequests []updateRuleRequest

	for i, rsp := range responses {
		if rsp.Error != nil {
			// Capture the error, but continue processing this bug
			// and other bugs, as partial success is possible
			// and pending rule updates must be applied.
			err := errors.Annotate(rsp.Error, "updating bug (%s)", bugsToUpdate[i].Bug.String()).Err()
			errs = append(errs, err)
			logging.Errorf(ctx, "%s", err)
		}

		if rsp.IsDuplicate {
			duplicateBugs = append(duplicateBugs, bugs.DuplicateBugDetails{
				RuleID:     bugsToUpdate[i].RuleID,
				Bug:        bugsToUpdate[i].Bug,
				IsAssigned: rsp.IsDuplicateAndAssigned,
			})
			// Inhibit archiving if rules are duplicates.
			rsp.ShouldArchive = false
		}
		if rsp.ShouldArchive || rsp.DisableRulePriorityUpdates || rsp.RuleAssociationNotified || len(rsp.PolicyActivationsNotified) > 0 {
			updateRuleRequests = append(updateRuleRequests, updateRuleRequest{
				RuleID:                     bugsToUpdate[i].RuleID,
				BugID:                      bugsToUpdate[i].Bug,
				Archive:                    rsp.ShouldArchive,
				DisableRulePriorityUpdates: rsp.DisableRulePriorityUpdates,
				RuleAssociationNotified:    rsp.RuleAssociationNotified,
				PolicyActivationsNotified:  rsp.PolicyActivationsNotified,
			})
		}
	}

	if err := b.updateRules(ctx, updateRuleRequests); err != nil {
		err = errors.Annotate(err, "updating rules after updating bugs").Err()
		errs = append(errs, err)
		logging.Errorf(ctx, "%s", err)
	}

	// Handle bugs marked as duplicate.
	for _, duplicateDetails := range duplicateBugs {
		if err := b.handleDuplicateBug(ctx, duplicateDetails); err != nil {
			err = errors.Annotate(err, "handling duplicate bug (%s)", duplicateDetails.Bug.String()).Err()
			errs = append(errs, err)
			logging.Errorf(ctx, "%s", err)
		}
	}
	// Returns nil if len(errs) == 0.
	return errors.Append(errs...)
}

func (b *BugUpdater) verifyClusterImpactValid(ctx context.Context, progress *runs.ReclusteringProgress) bool {
	if progress.IsReclusteringToNewAlgorithms() {
		logging.Warningf(ctx, "Auto-bug filing paused for project %s as re-clustering to new algorithms is in progress.", b.project)
		return false
	}
	if progress.IsReclusteringToNewConfig() {
		logging.Warningf(ctx, "Auto-bug filing paused for project %s as re-clustering to new configuration is in progress.", b.project)
		return false
	}
	if algorithms.AlgorithmsVersion != progress.Next.AlgorithmsVersion {
		logging.Warningf(ctx, "Auto-bug filing paused for project %s as bug-filing is running mismatched algorithms version %v (want %v).",
			b.project, algorithms.AlgorithmsVersion, progress.Next.AlgorithmsVersion)
		return false
	}
	if !b.projectCfg.LastUpdated.Equal(progress.Next.ConfigVersion) {
		logging.Warningf(ctx, "Auto-bug filing paused for project %s as bug-filing is running mismatched config version %v (want %v).",
			b.project, b.projectCfg.LastUpdated, progress.Next.ConfigVersion)
		return false
	}
	return true
}

func (b *BugUpdater) fileNewBugs(ctx context.Context, clusters []*analysis.Cluster, blockedClusterIDs map[clustering.ClusterID]struct{}) error {
	// The set of clusters IDs to file bugs for. Used for deduplicating creation
	// requests accross policies.
	clusterIDsToCreateBugsFor := make(map[clustering.ClusterID]struct{})

	// The list of clusters to file bugs for. Uses a list instead of a set to ensure
	// the order that bugs are created is deterministic and matches the order that
	// policies are configured, which simplifies testing.
	var clustersToCreateBugsFor []*analysis.Cluster

	for _, p := range b.projectCfg.Config.BugManagement.GetPolicies() {
		sortByPolicyBugFilingPreference(clusters, p)

		for _, cluster := range clusters {
			if cluster.ClusterID.IsBugCluster() {
				// Never file another bug for a bug cluster.
				continue
			}

			// Was a bug recently filed for this suggested cluster?
			// We want to avoid race conditions whereby we file multiple bug
			// clusters for the same suggested cluster, because re-clustering and
			// re-analysis has not yet run and moved residual impact from the
			// suggested cluster to the bug cluster.
			_, ok := blockedClusterIDs[cluster.ClusterID]
			if ok {
				// Do not file a bug.
				continue
			}

			// Were the failures are confined to only automation CLs
			// and/or 1-2 user CLs? In other words, are the failures in this
			// clusters unlikely to be present in the tree?
			if cluster.DistinctUserCLsWithFailures7d.Residual < 3 &&
				cluster.PostsubmitBuildsWithFailures7d.Residual == 0 {
				// Do not file a bug.
				continue
			}

			// Only file a bug if the residual impact exceeds the threshold.
			impact := ExtractResidualMetrics(cluster)
			bugFilingThresholds := bugs.ActivationThresholds(p)
			if cluster.ClusterID.IsTestNameCluster() {
				// Use an inflated threshold for test name clusters to bias
				// bug creation towards failure reason clusters.
				bugFilingThresholds =
					bugs.InflateThreshold(bugFilingThresholds,
						testnameThresholdInflationPercent)
			}
			if !impact.MeetsAnyOfThresholds(bugFilingThresholds) {
				continue
			}

			// Create a bug for this cluster, deduplicating creation
			// requests across policies.
			if _, ok := clusterIDsToCreateBugsFor[cluster.ClusterID]; !ok {
				clustersToCreateBugsFor = append(clustersToCreateBugsFor, cluster)
				clusterIDsToCreateBugsFor[cluster.ClusterID] = struct{}{}
			}

			// The policy has picked the one cluster it wants to file a bug for.
			// If this cluster is the same as another policy, the one bug is filed
			// for both policies.
			//
			// This ensures if a top failure cluster clusters well by both reason
			// and test name, we do not file bugs for both.
			break
		}
	}

	// File new bugs.
	bugsFiled := 0
	for _, cluster := range clustersToCreateBugsFor {
		if bugsFiled >= b.MaxBugsFiledPerRun {
			break
		}
		created, err := b.createBug(ctx, cluster)
		if err != nil {
			return err
		}
		if created {
			bugsFiled++
		}
	}
	return nil
}

type updateRuleRequest struct {
	// The identity of the rule.
	RuleID string
	// The bug that was updated and/or from which the updates were sourced.
	// If the bug on the rule has changed from this value, rule updates will
	// not be applied.
	BugID bugs.BugID
	// Whether the rule should be archived.
	Archive bool
	// Whether rule priority updates should be disabled.
	DisableRulePriorityUpdates bool
	// Whether BugManagementState.RuleAssociationNotified should be set.
	RuleAssociationNotified bool
	// A map containing the IDs of policies for which
	// BugManagementState.Policies[<policyID>].ActivationNotified should
	// be set.
	PolicyActivationsNotified map[bugs.PolicyID]struct{}
}

// updateRules applies updates to failure association rules
// following a round of bug updates. This includes:
//   - archiving rules if the bug was detected in an archived state
//   - disabling automatic priority updates if it was detected that
//     the user manually set the bug priority.
//
// requests and response slices should have 1:1 correspondance, i.e.
// requests[i] corresponds to responses[i].
func (b *BugUpdater) updateRules(ctx context.Context, requests []updateRuleRequest) error {
	// Perform updates in batches to stay within mutation Spanner limits.
	requestBatches := batch(requests, b.UpdateRuleBatchSize)
	for _, batch := range requestBatches {
		err := b.updateRulesBatch(ctx, batch)
		if err != nil {
			return err
		}
	}
	return nil
}

func (b *BugUpdater) updateRulesBatch(ctx context.Context, requests []updateRuleRequest) error {
	ruleIDs := make([]string, 0, len(requests))
	for _, req := range requests {
		ruleIDs = append(ruleIDs, req.RuleID)
	}
	f := func(ctx context.Context) error {
		// Perform transactional read-update of rule to protect
		// against update races.
		rs, err := rules.ReadMany(ctx, b.project, ruleIDs)
		if err != nil {
			return errors.Annotate(err, "read rules").Err()
		}
		for i, rule := range rs {
			updateRequest := requests[i]
			if rule.RuleID != updateRequest.RuleID {
				// ReadMany's response should be in 1:1 correspondance
				// to the request.
				panic("logic error")
			}
			if rule.BugID != updateRequest.BugID {
				// A data race has occured: the rule has been modified while
				// we were updating bugs, and now the update to the rule no
				// longer makes sense. This should only occur rarely.
				logging.Warningf(ctx, "Bug associated with rule %v changed during bug-filing run, skipping updates to rule.")
				continue
			}
			updateOptions := rules.UpdateOptions{}
			if updateRequest.Archive {
				rule.IsActive = false
				updateOptions.IsAuditableUpdate = true
				updateOptions.PredicateUpdated = true
			}
			if updateRequest.DisableRulePriorityUpdates {
				rule.IsManagingBugPriority = false
				updateOptions.IsAuditableUpdate = true
				updateOptions.IsManagingBugPriorityUpdated = true
			}
			if updateRequest.RuleAssociationNotified {
				rule.BugManagementState.RuleAssociationNotified = true
			}
			for policyID := range updateRequest.PolicyActivationsNotified {
				policyState, ok := rule.BugManagementState.PolicyState[string(policyID)]
				if !ok {
					// The policy has been deleted during the bug-filing run.
					logging.Warningf(ctx, "Policy activation notified for policy %v, which is now deleted.", policyID)
					continue
				}
				policyState.ActivationNotified = true
			}
			ms, err := rules.Update(rule, updateOptions, rules.LUCIAnalysisSystem)
			if err != nil {
				// Validation error; this should never happen here.
				return errors.Annotate(err, "prepare rule update").Err()
			}
			span.BufferWrite(ctx, ms)
		}
		return nil
	}
	_, err := span.ReadWriteTransaction(ctx, f)
	if err != nil {
		return errors.Annotate(err, "update rules").Err()
	}
	return nil
}

// handleDuplicateBug handles a duplicate bug, merging its failure association
// rule with the bug it is ultimately merged into (creating the rule if it does
// not exist). In case of unhandleable errors, the source bug is kicked out of the
// duplicate state and an error message is posted on the bug.
func (b *BugUpdater) handleDuplicateBug(ctx context.Context, duplicateDetails bugs.DuplicateBugDetails) error {
	err := b.handleDuplicateBugHappyPath(ctx, duplicateDetails)
	if errors.Is(err, mergeIntoCycleErr) {
		request := bugs.UpdateDuplicateSourceRequest{
			BugDetails:   duplicateDetails,
			ErrorMessage: mergeIntoCycleMessage,
		}
		if err := b.updateDuplicateSource(ctx, request); err != nil {
			return errors.Annotate(err, "update source bug after a cycle was found").Err()
		}
	} else if errors.Is(err, ruleDefinitionTooLongErr) {
		request := bugs.UpdateDuplicateSourceRequest{
			BugDetails:   duplicateDetails,
			ErrorMessage: ruleDefinitionTooLongMessage,
		}
		if err := b.updateDuplicateSource(ctx, request); err != nil {
			return errors.Annotate(err, "update source bug after merging rule definition was found too long").Err()
		}
	} else if errors.Is(err, mergeIntoPermissionErr) {
		request := bugs.UpdateDuplicateSourceRequest{
			BugDetails:   duplicateDetails,
			ErrorMessage: mergeIntoPermissionMessage,
		}
		if err := b.updateDuplicateSource(ctx, request); err != nil {
			return errors.Annotate(err, "update source bug after merging rule definition encountered a permission error").Err()
		}
	} else if err != nil {
		return err
	}
	return nil
}

// handleDuplicateBugHappyPath handles a duplicate bug, merging its failure association
// rule with the bug it is ultimately merged into (creating the rule if it does
// not exist). The original rule is archived.
func (b *BugUpdater) handleDuplicateBugHappyPath(ctx context.Context, duplicateDetails bugs.DuplicateBugDetails) error {
	// Chase the bug merged-into graph until we find the sink of the graph.
	// (The canonical bug of the chain of duplicate bugs.)
	destBug, err := b.resolveMergedIntoBug(ctx, duplicateDetails.Bug)
	if err != nil {
		// May return mergeIntoCycleErr.
		return err
	}

	var destinationBugRuleID string

	f := func(ctx context.Context) error {
		sourceRule, _, err := readRuleForBugAndProject(ctx, duplicateDetails.Bug, b.project)
		if err != nil {
			return errors.Annotate(err, "reading rule for source bug").Err()
		}
		if !sourceRule.IsActive {
			// The source rule is no longer active. This is a race condition
			// as we only do bug updates for rules that exist at the time
			// we start bug updates.
			// An inactive rule does not match any failures so merging the
			// it into another rule should have no effect anyway.
			return nil
		}
		// Try and read the rule for the bug we are merging into.
		destinationRule, _, err :=
			readRuleForBugAndProject(ctx, destBug, b.project)
		if err != nil {
			return errors.Annotate(err, "reading rule for destination bug").Err()
		}
		if destinationRule == nil {
			// The destination bug does not have a rule in this project.
			// Simply update the source rule to point to the new bug.
			sourceRule.BugID = destBug

			// As the bug has changed, flags tracking notification of policy
			// activation must be reset.
			if sourceRule.BugManagementState.PolicyState != nil {
				for _, policyState := range sourceRule.BugManagementState.PolicyState {
					policyState.ActivationNotified = false
				}
			}

			// The destination bug is not a LUCI Analysis bug.
			// Do not automatically verify/auto-close it as we do not
			// know what problems it was for.
			sourceRule.IsManagingBug = false

			sourceRule.BugManagementState.RuleAssociationNotified = false

			ms, err := rules.Update(sourceRule, rules.UpdateOptions{
				IsAuditableUpdate: true,
			}, rules.LUCIAnalysisSystem)
			if err != nil {
				// Indicates validation error. Should never happen.
				return err
			}
			span.BufferWrite(ctx, ms)

			destinationBugRuleID = sourceRule.RuleID
			return nil
		} else {
			// The bug we are a duplicate of already has a rule.
			if destinationRule.IsActive {
				// Merge the source and destination rules with an "OR".
				mergedRule, err := lang.Merge(destinationRule.RuleDefinition, sourceRule.RuleDefinition)
				if err != nil {
					return errors.Annotate(err, "merging rules").Err()
				}
				if len(mergedRule) > rules.MaxRuleDefinitionLength {
					// The merged rule is too long to store.
					return ruleDefinitionTooLongErr
				}
				destinationRule.RuleDefinition = mergedRule
			} else {
				// Else: an inactive rule does not match any failures, so we should
				// use only the rule from the source bug.
				destinationRule.RuleDefinition = sourceRule.RuleDefinition
			}

			// Disable the source rule.
			sourceRule.IsActive = false
			ms, err := rules.Update(sourceRule, rules.UpdateOptions{
				IsAuditableUpdate: true,
				PredicateUpdated:  true,
			}, rules.LUCIAnalysisSystem)
			if err != nil {
				// Indicates validation error. Should never happen.
				return err
			}
			span.BufferWrite(ctx, ms)

			// Update the rule on the destination rule.
			destinationRule.IsActive = true
			ms, err = rules.Update(destinationRule, rules.UpdateOptions{
				IsAuditableUpdate: true,
				PredicateUpdated:  true,
			}, rules.LUCIAnalysisSystem)
			if err != nil {
				return err
			}
			span.BufferWrite(ctx, ms)

			destinationBugRuleID = destinationRule.RuleID
			return nil
		}
	}
	// Update source and destination rules in one transaction, to ensure
	// consistency.
	_, err = span.ReadWriteTransaction(ctx, f)
	if err != nil {
		return err
	}

	if !b.projectCfg.Config.BugManagement.GetDisableDuplicateBugComments() {
		// Notify that the bugs were successfully merged.
		request := bugs.UpdateDuplicateSourceRequest{
			BugDetails:        duplicateDetails,
			DestinationRuleID: destinationBugRuleID,
		}
		if err := b.updateDuplicateSource(ctx, request); err != nil {
			return errors.Annotate(err, "updating source bug").Err()
		}
	}

	return err
}

// resolveMergedIntoBug resolves the bug the given bug is ultimately merged
// into.
func (b *BugUpdater) resolveMergedIntoBug(ctx context.Context, bug bugs.BugID) (bugs.BugID, error) {
	isResolved := false
	mergedIntoBug := bug
	const maxResolutionSteps = 5
	for i := 0; i < maxResolutionSteps; i++ {
		system := mergedIntoBug.System
		manager, ok := b.managers[system]
		if !ok {
			if mergedIntoBug.System == "buganizer" {
				// Do not attempt to resolve the canoncial bug within
				// buganizer if buganizer is not registered. We hit this
				// path with buganizer not registered if a monorail bug marks
				// itself as a duplicate of a buganizer bug.
				isResolved = true
				break
			}
			return bugs.BugID{}, fmt.Errorf("encountered unknown bug system: %q", system)
		}
		mergedInto, err := manager.GetMergedInto(ctx, mergedIntoBug)
		if status.Code(err) == codes.PermissionDenied {
			// We don't have permission to view the issue
			return bugs.BugID{}, mergeIntoPermissionErr
		} else if err != nil {
			return bugs.BugID{}, err
		}
		if mergedInto == nil {
			// We have found the canoncial merged-into bug.
			isResolved = true
			break
		} else {
			mergedIntoBug = *mergedInto
		}
	}
	if !isResolved {
		// We found a cycle in the graph.
		return bugs.BugID{}, mergeIntoCycleErr
	}
	if mergedIntoBug == bug {
		// This would normally never occur, but is possible in some
		// exceptional scenarios like race conditions where a cycle
		// is broken during the graph traversal, or a bug which
		// was marked as duplicate but is no longer marked as duplicate
		// now.
		return bugs.BugID{}, fmt.Errorf("cannot deduplicate a bug into itself")
	}
	return mergedIntoBug, nil
}

// updateDuplicateSource updates the source bug of a duplicate
// bug pair (source bug, destination bug).
// It either posts a message notifying the user the rule was successfully
// merged to the destination, or notifies the user of the error and
// marks the bug no longer a duplicate (to avoid repeated attempts to
// handle the problematic duplicate bug).
func (b *BugUpdater) updateDuplicateSource(ctx context.Context, request bugs.UpdateDuplicateSourceRequest) error {
	manager, ok := b.managers[request.BugDetails.Bug.System]
	if !ok {
		return fmt.Errorf("encountered unknown bug system: %q", request.BugDetails.Bug.System)
	}
	err := manager.UpdateDuplicateSource(ctx, request)
	if err != nil {
		return err
	}
	return nil
}

// readRuleForBugAndProject reads the failure association rule for the given
// bug in the given project, if it exists. It additionally returns whether
// there is any rule in the system that manages the given bug, even if in
// a different project.
// If the rule cannot be read, it returns nil.
func readRuleForBugAndProject(ctx context.Context, bug bugs.BugID, project string) (rule *rules.Entry, anyRuleManaging bool, err error) {
	rules, err := rules.ReadByBug(ctx, bug)
	if err != nil {
		return nil, false, err
	}
	rule = nil
	anyRuleManaging = false
	for _, r := range rules {
		if r.IsManagingBug {
			anyRuleManaging = true
		}
		if r.Project == project {
			rule = r
		}
	}
	return rule, anyRuleManaging, nil
}

// sortByPolicyBugFilingPreference sorts clusters based on our preference
// to file bugs for these clusters.
func sortByPolicyBugFilingPreference(cs []*analysis.Cluster, policy *configpb.BugManagementPolicy) {
	// The current ranking approach prefers filing bugs for clusters
	// which more strongly meet the bug-filing threshold, with a bias
	// towards reason clusters.
	//
	// The order of this ranking is only important where there are
	// multiple competing clusters which meet the bug-filing threshold.
	// As bug filing runs relatively often, except in cases of contention,
	// the first bug to meet the threshold will be filed.
	sort.Slice(cs, func(i, j int) bool {
		// N.B. This does not rank clusters perfectly where the policy has
		// multiple metrics, as the first metric may only slightly the
		// threshold, but the second metric strongly exceeds.
		// Most policies have only one metric, however, so this should
		// be pretty rare.
		for _, metric := range policy.Metrics {
			if equal, less := rankByMetric(cs[i], cs[j], metrics.ID(metric.MetricId)); !equal {
				return less
			}
		}
		// If all else fails, sort by cluster ID. This is mostly to ensure
		// the code behaves deterministically when under unit testing.
		if cs[i].ClusterID.Algorithm != cs[j].ClusterID.Algorithm {
			return cs[i].ClusterID.Algorithm < cs[j].ClusterID.Algorithm
		}
		return cs[i].ClusterID.ID < cs[j].ClusterID.ID
	})
}

func rankByMetric(a, b *analysis.Cluster, metric metrics.ID) (equal bool, less bool) {
	valueA := a.MetricValues[metric].SevenDay.Residual
	valueB := b.MetricValues[metric].SevenDay.Residual
	// If one cluster we are comparing with is a test name cluster,
	// give the other cluster an impact boost in the comparison, so
	// that we bias towards filing it (instead of the test name cluster).
	if b.ClusterID.IsTestNameCluster() {
		valueA = (valueA * (100 + testnameThresholdInflationPercent)) / 100
	}
	if a.ClusterID.IsTestNameCluster() {
		valueB = (valueB * (100 + testnameThresholdInflationPercent)) / 100
	}
	equal = (valueA == valueB)
	// a less than b in the sort order is defined as a having more impact
	// than b, so that clusters are sorted in descending impact order.
	less = (valueA > valueB)
	return equal, less
}

// createBug files a new bug for the given suggested cluster,
// and stores the association from bug to failures through a new
// failure association rule.
func (b *BugUpdater) createBug(ctx context.Context, cs *analysis.Cluster) (created bool, err error) {
	alg, err := algorithms.SuggestingAlgorithm(cs.ClusterID.Algorithm)
	if err == algorithms.ErrAlgorithmNotExist {
		// The cluster is for an old algorithm that no longer exists, or
		// for a new algorithm that is not known by us yet.
		// Do not file a bug. This is not an error, it is expected during
		// algorithm version changes.
		return false, nil
	}

	summary := clusterSummaryFromAnalysis(cs)

	// Double-check the failure matches the cluster. Generating a
	// failure association rule that does not match the suggested cluster
	// could result in indefinite creation of new bugs, as the system
	// will repeatedly create new failure association rules for the
	// same suggested cluster.
	// Mismatches should usually be transient as re-clustering will fix
	// up any incorrect clustering.
	if hex.EncodeToString(alg.Cluster(b.projectCfg, &summary.Example)) != cs.ClusterID.ID {
		return false, errors.New("example failure did not match cluster ID")
	}
	rule, err := b.generateFailureAssociationRule(alg, &summary.Example)
	if err != nil {
		return false, errors.Annotate(err, "obtain failure association rule").Err()
	}

	ruleID, err := rules.GenerateID()
	if err != nil {
		return false, errors.Annotate(err, "generating rule ID").Err()
	}

	description, err := alg.ClusterDescription(b.projectCfg, summary)
	if err != nil {
		return false, errors.Annotate(err, "prepare bug description").Err()
	}

	// Set policy activations starting from a state where no policies
	// are active.
	impact := ExtractResidualMetrics(cs)
	bugManagementState, _ := bugs.UpdatePolicyActivations(&bugspb.BugManagementState{}, b.projectCfg.Config.BugManagement.GetPolicies(), impact, b.RunTimestamp)

	request := bugs.BugCreateRequest{
		RuleID:      ruleID,
		Description: description,
	}

	activePolicyIDs := make(map[bugs.PolicyID]struct{})
	for policyID, state := range bugManagementState.PolicyState {
		if state.IsActive {
			activePolicyIDs[bugs.PolicyID(policyID)] = struct{}{}
		}
	}
	request.ActivePolicyIDs = activePolicyIDs

	system, err := b.routeToBugSystem(cs)
	if err != nil {
		return false, errors.Annotate(err, "extracting bug system").Err()
	}

	if system == bugs.BuganizerSystem {
		var err error
		request.BuganizerComponent, err = extractBuganizerComponent(cs)
		if err != nil {
			return false, errors.Annotate(err, "extracting buganizer component").Err()
		}
	} else {
		request.MonorailComponents = extractMonorailComponents(cs)
	}

	manager := b.managers[system]
	response := manager.Create(ctx, request)

	if !response.Simulated && response.ID != "" {
		// We filed a bug.
		// Create a failure association rule associating the failures with a bug.

		// In filing a bug, we notified the rule association.
		bugManagementState.RuleAssociationNotified = true

		// Record which policies we notified as activating.
		for policyID := range response.PolicyActivationsNotified {
			bugManagementState.PolicyState[string(policyID)].ActivationNotified = true
		}

		newRule := &rules.Entry{
			Project:               b.project,
			RuleID:                ruleID,
			RuleDefinition:        rule,
			BugID:                 bugs.BugID{System: system, ID: response.ID},
			IsActive:              true,
			IsManagingBug:         true,
			IsManagingBugPriority: true,
			SourceCluster:         cs.ClusterID,
			BugManagementState:    bugManagementState,
		}
		create := func(ctx context.Context) error {
			user := rules.LUCIAnalysisSystem
			ms, err := rules.Create(newRule, user)
			if err != nil {
				return err
			}
			span.BufferWrite(ctx, ms)
			return nil
		}
		if _, err := span.ReadWriteTransaction(ctx, create); err != nil {
			return false, errors.Annotate(err, "create rule").Err()
		}
	}

	if response.Error != nil {
		// We encountered an error creating the bug. Note that this
		// is not mutually exclusive with having filed a bug, as
		// steps after creating the bug may have failed, and in
		// this case a failure association rule should still be created.
		return false, errors.Annotate(response.Error, "create issue in %v (created ID: %q)", system, response.ID).Err()
	}

	return true, nil
}

func (b *BugUpdater) routeToBugSystem(cs *analysis.Cluster) (string, error) {
	hasMonorail := b.projectCfg.Config.BugManagement.GetMonorail() != nil
	hasBuganizer := b.projectCfg.Config.BugManagement.GetBuganizer() != nil
	defaultSystem := b.projectCfg.Config.BugManagement.GetDefaultBugSystem()

	if !hasMonorail && !hasBuganizer {
		return "", errors.New("at least one bug filing system need to be configured")
	}
	// If only one bug system configured, pick that system.
	if !hasMonorail {
		return bugs.BuganizerSystem, nil
	}
	if !hasBuganizer {
		return bugs.MonorailSystem, nil
	}
	// When both bug systems are configured, pick the most suitable one.

	// The most impactful monorail component.
	var topMonorailComponent analysis.TopCount
	for _, tc := range cs.TopMonorailComponents {
		if tc.Value == "" {
			continue
		}
		// Any monorail component is associated for more than 30% of the
		// failures in the cluster should be checked for top impact.
		if tc.Count > ((cs.MetricValues[metrics.Failures.ID].SevenDay.Nominal * 3) / 10) {
			if tc.Count > topMonorailComponent.Count || topMonorailComponent.Value == "" {
				topMonorailComponent = tc
			}
		}
	}

	// The most impactful buganizer component.
	var topBuganizerComponent analysis.TopCount
	for _, tc := range cs.TopBuganizerComponents {
		if tc.Value == "" {
			continue
		}
		// Any buganizer component is associated for more than 30% of the
		// failures in the cluster should be checked for top impact.
		if tc.Count > ((cs.MetricValues[metrics.Failures.ID].SevenDay.Nominal * 3) / 10) {
			if tc.Count > topBuganizerComponent.Count || topBuganizerComponent.Value == "" {
				topBuganizerComponent = tc
			}
		}
	}

	if topMonorailComponent.Value == "" && topBuganizerComponent.Value == "" {
		return defaultBugSystemName(defaultSystem), nil
	} else if topMonorailComponent.Value != "" && topBuganizerComponent.Value == "" {
		return bugs.MonorailSystem, nil
	} else if topMonorailComponent.Value == "" && topBuganizerComponent.Value != "" {
		return bugs.BuganizerSystem, nil
	} else {
		// Return the system corresponding with the highest impact.
		if topMonorailComponent.Count > topBuganizerComponent.Count {
			return bugs.MonorailSystem, nil
		} else if topMonorailComponent.Count == topBuganizerComponent.Count {
			// If top components have equal impact, use the configured default system.
			return defaultBugSystemName(defaultSystem), nil
		} else {
			return bugs.BuganizerSystem, nil
		}
	}
}

func extractBuganizerComponent(cs *analysis.Cluster) (int64, error) {
	for _, tc := range cs.TopBuganizerComponents {
		// The top buganizer component that is associated for more than 30% of the
		// failures in the cluster should be on the filed bug.
		if tc.Value != "" && tc.Count > ((cs.MetricValues[metrics.Failures.ID].SevenDay.Nominal*3)/10) {
			componentID, err := strconv.ParseInt(tc.Value, 10, 64)
			if err != nil {
				return 0, errors.Annotate(err, "parse buganizer component id").Err()
			}
			return componentID, nil
		}
	}
	return 0, nil
}

func extractMonorailComponents(cs *analysis.Cluster) []string {
	var monorailComponents []string
	for _, tc := range cs.TopMonorailComponents {
		// Any monorail component is associated for more than 30% of the
		// failures in the cluster should be on the filed bug.
		if tc.Count > ((cs.MetricValues[metrics.Failures.ID].SevenDay.Nominal * 3) / 10) {
			monorailComponents = append(monorailComponents, tc.Value)
		}
	}
	return monorailComponents
}

func defaultBugSystemName(defaultSystem configpb.BugSystem) string {
	if defaultSystem == configpb.BugSystem_BUGANIZER {
		return bugs.BuganizerSystem
	} else {
		return bugs.MonorailSystem
	}
}

func clusterSummaryFromAnalysis(c *analysis.Cluster) *clustering.ClusterSummary {
	example := clustering.Failure{
		TestID: c.ExampleTestID(),
	}
	if c.ExampleFailureReason.Valid {
		example.Reason = &pb.FailureReason{PrimaryErrorMessage: c.ExampleFailureReason.StringVal}
	}
	// A list of 5 commonly occuring tests are included in bugs created
	// for failure reason clusters, to improve searchability by test name.
	var topTests []string
	for _, tt := range c.TopTestIDs {
		topTests = append(topTests, tt.Value)
	}
	return &clustering.ClusterSummary{
		Example:  example,
		TopTests: topTests,
	}
}

func (b *BugUpdater) generateFailureAssociationRule(alg algorithms.Algorithm, failure *clustering.Failure) (string, error) {
	rule := alg.FailureAssociationRule(b.projectCfg, failure)

	// Check the generated rule is valid and matches the failure.
	// An improperly generated failure association rule could result
	// in uncontrolled creation of new bugs.
	expr, err := lang.Parse(rule)
	if err != nil {
		return "", errors.Annotate(err, "rule generated by %s did not parse", alg.Name()).Err()
	}
	match := expr.Evaluate(failure)
	if !match {
		reason := ""
		if failure.Reason != nil {
			reason = failure.Reason.PrimaryErrorMessage
		}
		return "", fmt.Errorf("rule generated by %s did not match example failure (testID: %q, failureReason: %q)",
			alg.Name(), failure.TestID, reason)
	}
	return rule, nil
}
