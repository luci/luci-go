// Copyright 2025 The LUCI Authors.
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

package bugs

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestUpdatePolicyActivations(t *testing.T) {
	lastActivationTime := time.Date(2100, 1, 1, 1, 1, 1, 1, time.UTC)
	testTime := time.Date(2100, 1, 2, 1, 1, 1, 1, time.UTC)
	testCases := []struct {
		name                       string
		bugManagementState         *bugspb.BugManagementState
		clusterMetrics             ClusterMetrics
		testTime                   time.Time
		expectedBugManagementState *bugspb.BugManagementState
		expectedChanged            bool
		expectedThresholdsMet      ThresholdsMetPerTimeInterval
	}{
		{
			name:               "activate an inactive policy",
			bugManagementState: &bugspb.BugManagementState{},
			clusterMetrics: ClusterMetrics{
				metrics.CriticalFailuresExonerated.ID: MetricValues{OneDay: 100},
			},
			testTime: testTime,
			expectedBugManagementState: &bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"exoneration-policy": {
						IsActive:           true,
						ActivationNotified: true,
					},
				},
			},
			expectedChanged:       true,
			expectedThresholdsMet: ThresholdsMetPerTimeInterval{OneDay: true, ThreeDay: false, SevenDay: false},
		},
		{
			name: "deactivate an active policy",
			bugManagementState: &bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"exoneration-policy": {
						IsActive:           true,
						LastActivationTime: timestamppb.New(lastActivationTime),
						ActivationNotified: true,
					},
				},
			},
			clusterMetrics: ClusterMetrics{
				metrics.CriticalFailuresExonerated.ID: MetricValues{OneDay: 5},
			},
			testTime: testTime,
			expectedBugManagementState: &bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"exoneration-policy": {
						IsActive:           false,
						ActivationNotified: true,
					},
				},
			},
			expectedChanged:       true,
			expectedThresholdsMet: ThresholdsMetPerTimeInterval{OneDay: false, ThreeDay: false, SevenDay: false},
		},
		{
			name: "do nothing if threshold is between deactivation threshold and activation threshold",
			bugManagementState: &bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"exoneration-policy": {
						IsActive:           true,
						LastActivationTime: timestamppb.New(lastActivationTime),
						ActivationNotified: true,
					},
				},
			},
			clusterMetrics: ClusterMetrics{
				metrics.CriticalFailuresExonerated.ID: MetricValues{OneDay: 50},
			},
			testTime: testTime,
			expectedBugManagementState: &bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"exoneration-policy": {
						IsActive:           true,
						ActivationNotified: true,
					},
				},
			},
			expectedChanged:       false,
			expectedThresholdsMet: ThresholdsMetPerTimeInterval{OneDay: false, ThreeDay: false, SevenDay: false},
		},
		{
			name: "return threshold information to falsify closed bugs",
			bugManagementState: &bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"exoneration-policy": {
						IsActive:           true,
						LastActivationTime: timestamppb.New(lastActivationTime),
						ActivationNotified: true,
					},
				},
			},
			clusterMetrics: ClusterMetrics{
				metrics.CriticalFailuresExonerated.ID: MetricValues{OneDay: 110},
			},
			testTime: testTime,
			expectedBugManagementState: &bugspb.BugManagementState{
				RuleAssociationNotified: true,
				PolicyState: map[string]*bugspb.BugManagementState_PolicyState{
					"exoneration-policy": {
						IsActive:           true,
						ActivationNotified: true,
					},
				},
			},
			expectedChanged:       false,
			expectedThresholdsMet: ThresholdsMetPerTimeInterval{OneDay: true, ThreeDay: false, SevenDay: false},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			updatedBugManagementState, changed := UpdatePolicyActivations(tc.bugManagementState, bugManagementPolicies(t), tc.clusterMetrics, tc.testTime)
			isActive := updatedBugManagementState.PolicyState["exoneration-policy"].IsActive
			expectedIsActive := false
			if tc.expectedBugManagementState.PolicyState != nil {
				expectedIsActive = tc.expectedBugManagementState.PolicyState["exoneration-policy"].IsActive
			}
			if isActive != expectedIsActive {
				t.Errorf("the PolicyState's IsActive field %v on the updated BugManagementState does not match the expected IsActive field %v", isActive, expectedIsActive)
			}
			if tc.clusterMetrics == nil && updatedBugManagementState.PolicyState != nil {
				t.Errorf("the PolicyState on the updated BugManagementState was expected to be nil")
			}
			if changed != tc.expectedChanged {
				t.Errorf("changed %v from the result does not match expected changed %v", changed, tc.expectedChanged)
			}
		})
	}
}

func TestInvalidationStatus(t *testing.T) {
	testCases := []struct {
		name                       string
		policyMetrics              []*configpb.BugManagementPolicy_Metric
		clusterMetrics             ClusterMetrics
		expectedInvalidationStatus BugClosureInvalidationStatus
	}{
		{
			name: "metric does not meet activation threshold",
			policyMetrics: []*configpb.BugManagementPolicy_Metric{
				{
					MetricId: metrics.CriticalFailuresExonerated.ID.String(),
					ActivationThreshold: &configpb.MetricThreshold{
						OneDay: proto.Int64(100),
					},
					DeactivationThreshold: &configpb.MetricThreshold{
						OneDay: proto.Int64(10),
					},
				},
			},
			clusterMetrics: ClusterMetrics{
				metrics.CriticalFailuresExonerated.ID: MetricValues{OneDay: 50, ThreeDay: 60, SevenDay: 70},
			},
			expectedInvalidationStatus: BugClosureInvalidationStatus{
				OneDay: BugClosureInvalidationResult{
					IsInvalidated:   false,
					ActivePolicyIDs: map[PolicyID]struct{}{},
				},
				ThreeDay: BugClosureInvalidationResult{
					IsInvalidated:   false,
					ActivePolicyIDs: map[PolicyID]struct{}{},
				},
				SevenDay: BugClosureInvalidationResult{
					IsInvalidated:   false,
					ActivePolicyIDs: map[PolicyID]struct{}{},
				},
			},
		},
		{
			name: "metric meets activation threshold",
			policyMetrics: []*configpb.BugManagementPolicy_Metric{
				{
					MetricId: metrics.CriticalFailuresExonerated.ID.String(),
					ActivationThreshold: &configpb.MetricThreshold{
						SevenDay: proto.Int64(170),
					},
					DeactivationThreshold: &configpb.MetricThreshold{
						SevenDay: proto.Int64(50),
					},
				},
			},
			clusterMetrics: ClusterMetrics{
				metrics.CriticalFailuresExonerated.ID: MetricValues{OneDay: 100, ThreeDay: 130, SevenDay: 170},
			},
			expectedInvalidationStatus: BugClosureInvalidationStatus{
				OneDay: BugClosureInvalidationResult{
					IsInvalidated:   false,
					ActivePolicyIDs: map[PolicyID]struct{}{},
				},
				ThreeDay: BugClosureInvalidationResult{
					IsInvalidated:   false,
					ActivePolicyIDs: map[PolicyID]struct{}{},
				},
				SevenDay: BugClosureInvalidationResult{
					IsInvalidated: true,
					ActivePolicyIDs: map[PolicyID]struct{}{
						PolicyID("exoneration-policy"): struct{}{},
					},
				},
			},
		},
		{
			// Because the threshold is considered satisfied if any of the individual metric
			// thresholds is met or exceeded, thresholds being met on shorter time intervals
			// implies thresholds being met on longer time intervals for bug closure invalidation.
			name: "thresholds met on a shorter time interval implies thresholds met on a longer time interval",
			policyMetrics: []*configpb.BugManagementPolicy_Metric{
				{
					MetricId: metrics.CriticalFailuresExonerated.ID.String(),
					ActivationThreshold: &configpb.MetricThreshold{
						OneDay: proto.Int64(100),
					},
					DeactivationThreshold: &configpb.MetricThreshold{
						OneDay: proto.Int64(10),
					},
				},
			},
			clusterMetrics: ClusterMetrics{
				metrics.CriticalFailuresExonerated.ID: MetricValues{OneDay: 100, ThreeDay: 130, SevenDay: 170},
			},
			expectedInvalidationStatus: BugClosureInvalidationStatus{
				OneDay: BugClosureInvalidationResult{
					IsInvalidated: true,
					ActivePolicyIDs: map[PolicyID]struct{}{
						PolicyID("exoneration-policy"): struct{}{},
					},
				},
				ThreeDay: BugClosureInvalidationResult{
					IsInvalidated: true,
					ActivePolicyIDs: map[PolicyID]struct{}{
						PolicyID("exoneration-policy"): struct{}{},
					},
				},
				SevenDay: BugClosureInvalidationResult{
					IsInvalidated: true,
					ActivePolicyIDs: map[PolicyID]struct{}{
						PolicyID("exoneration-policy"): struct{}{},
					},
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bugManagementPolicies := bugManagementPolicies(t)
			bugManagementPolicies[0].Metrics = tc.policyMetrics

			invalidationStatus := InvalidationStatus(bugManagementPolicies, tc.clusterMetrics)

			if diff := cmp.Diff(tc.expectedInvalidationStatus, invalidationStatus); diff != "" {
				t.Errorf("BaseTestResultFromTest() failed: (-want +got): \n%s", diff)
			}
		})
	}
}

func bugManagementPolicies(t *testing.T) []*configpb.BugManagementPolicy {
	t.Helper()

	return []*configpb.BugManagementPolicy{
		{
			Id:                "exoneration-policy",
			Owners:            []string{"username@google.com"},
			HumanReadableName: "test variant(s) are being exonerated in presubmit",
			Priority:          configpb.BuganizerPriority_P2,
			Metrics: []*configpb.BugManagementPolicy_Metric{
				{
					MetricId: metrics.CriticalFailuresExonerated.ID.String(),
					ActivationThreshold: &configpb.MetricThreshold{
						OneDay: proto.Int64(100),
					},
					DeactivationThreshold: &configpb.MetricThreshold{
						OneDay: proto.Int64(10),
					},
				},
			},
			Explanation: &configpb.BugManagementPolicy_Explanation{
				ProblemHtml: "problem",
				ActionHtml:  "action",
			},
			BugTemplate: &configpb.BugManagementPolicy_BugTemplate{
				CommentTemplate: `{{if .BugID.IsBuganizer }}Buganizer Bug ID: {{ .BugID.BuganizerBugID }}{{end}}` +
					`{{if .BugID.IsMonorail }}Monorail Project: {{ .BugID.MonorailProject }}; ID: {{ .BugID.MonorailBugID }}{{end}}` +
					`Rule URL: {{.RuleURL}}`,
				Buganizer: &configpb.BugManagementPolicy_BugTemplate_Buganizer{
					Hotlists: []int64{1234},
				},
			},
		},
	}
}
