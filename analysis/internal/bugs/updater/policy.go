// Copyright 2023 The LUCI Authors.
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

package updater

import (
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/bugs"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

// PolicyActivationThresholds returns the set of thresholds that result
// in a policy activating. The returned thresholds should be treated as
// an 'OR', i.e. any of the given metric thresholds can result in a policy
// activating.
// As multiple policies can use the same metric, the returned set of
// thresholds may contain duplicates.
func PolicyActivationThresholds(policies []*configpb.BugManagementPolicy) []*configpb.ImpactMetricThreshold {
	var results []*configpb.ImpactMetricThreshold
	for _, policy := range policies {
		for _, metric := range policy.Metrics {
			results = append(results, &configpb.ImpactMetricThreshold{
				MetricId:  metric.MetricId,
				Threshold: metric.ActivationThreshold,
			})
		}
	}
	return results
}

// updatePolicyActivations updates policy activations for a failure
// association rule, given its current state, configured policies
// and cluster metrics.
//
// If any updates need to be made, a new *bugspb.BugManagementState
// is returned. Otherwise, this method returns nil.
func updatePolicyActivations(state *bugspb.BugManagementState, policies []*configpb.BugManagementPolicy, clusterMetrics *bugs.ClusterMetrics, now time.Time) (updatedState *bugspb.BugManagementState, changed bool) {
	evaluations := evaluatePolicyActivations(policies, clusterMetrics)

	// Proto3 serializes nil and empty maps to exactly the same bytes.
	// For the implementation here, we prefer to deal with the empty
	// maps, so we coerce them, but it does not represent a semantic
	// change to the proto.
	policyState := state.PolicyState
	if policyState == nil {
		policyState = make(map[string]*bugspb.BugManagementState_PolicyState)
	}

	newPolicyState := make(map[string]*bugspb.BugManagementState_PolicyState)

	changed = false
	for policyID, evaluation := range evaluations {
		state, ok := policyState[string(policyID)]
		if !ok {
			// Create a policy state entry for the new policy.
			state = &bugspb.BugManagementState_PolicyState{}
			changed = true
		}
		if !state.IsActive && evaluation == policyEvaluationActivate {
			// Transition the policy to active.
			// Make updates to a copied proto so that the side-effects
			// do not propogate to the passed proto.
			state = proto.Clone(state).(*bugspb.BugManagementState_PolicyState)
			state.IsActive = true
			state.LastActivationTime = timestamppb.New(now)
			changed = true
		}
		if state.IsActive && evaluation == policyEvaluationDeactivate {
			// Transition the policy to inactive.
			// Make updates to a copied proto so that the side-effects
			// do not propogate to the passed proto.
			state = proto.Clone(state).(*bugspb.BugManagementState_PolicyState)
			state.IsActive = false
			state.LastDeactivationTime = timestamppb.New(now)
			changed = true
		}
		newPolicyState[string(policyID)] = state
	}
	for policyID := range policyState {
		if _, ok := newPolicyState[policyID]; !ok {
			// We are removing a policies which is no longer configured.
			changed = true
		}
	}

	if changed {
		return &bugspb.BugManagementState{
			RuleAssociationNotified: state.RuleAssociationNotified,
			PolicyState:             newPolicyState,
		}, true
	}
	return state, false
}

type PolicyID string

type policyEvaluation int

const (
	// Neither the activation or deactivation criteria was met. The policy
	// activation should remain unchanged.
	policyEvaluationUnchanged policyEvaluation = iota
	// The policy deactivation criteria was met.
	policyEvaluationDeactivate
	// The policy activation criteria was met.
	policyEvaluationActivate
)

func evaluatePolicyActivations(policies []*configpb.BugManagementPolicy, clusterMetrics *bugs.ClusterMetrics) map[PolicyID]policyEvaluation {
	result := make(map[PolicyID]policyEvaluation)
	for _, policy := range policies {
		result[PolicyID(policy.Id)] = evaluatePolicy(policy, clusterMetrics)
	}
	return result
}

func evaluatePolicy(policy *configpb.BugManagementPolicy, clusterMetrics *bugs.ClusterMetrics) policyEvaluation {
	isDeactivationCriteriaMet := true
	for _, metric := range policy.Metrics {
		if clusterMetrics.MeetsThreshold(metrics.ID(metric.MetricId), metric.ActivationThreshold) {
			// The activation criteria is met on one of the metrics.
			// The policy should activate.
			return policyEvaluationActivate
		}
		if clusterMetrics.MeetsThreshold(metrics.ID(metric.MetricId), metric.DeactivationThreshold) {
			// If the deactivation threshold is met or exceeds on
			// any metric, deactivation is inhibited.
			isDeactivationCriteriaMet = false
		}
	}
	if isDeactivationCriteriaMet {
		return policyEvaluationDeactivate
	} else {
		// Hysteresis band: keep active policies active, and inactive policies inactive.
		return policyEvaluationUnchanged
	}
}
