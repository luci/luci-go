// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rules

import (
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ToExternalBugManagementStatePB converts the internal *bugspb.BugManagementState
// to its external (filtered) representation.
func ToExternalBugManagementStatePB(r *bugspb.BugManagementState) *pb.BugManagementState {
	policyState := make(map[string]*pb.BugManagementState_PolicyState, len(r.PolicyState))
	for policy, state := range r.PolicyState {
		policyState[policy] = toExternalPolicyStatePB(state)
	}
	return &pb.BugManagementState{
		PolicyState: policyState,
	}
}

func toExternalPolicyStatePB(s *bugspb.BugManagementState_PolicyState) *pb.BugManagementState_PolicyState {
	return &pb.BugManagementState_PolicyState{
		IsActive:             s.IsActive,
		LastActivationTime:   s.LastActivationTime,
		LastDeactivationTime: s.LastDeactivationTime,
	}
}
