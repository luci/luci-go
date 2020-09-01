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

package protoutil

import (
	"sort"
	"strings"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// StepNameSep is the separator between each node in step names.
const StepNameSep = "|"

// statusPrecedence is a set of step statuses that can be compared with others.
// list of step statuses, ordered from best to worst.
// See https://chromium.googlesource.com/infra/luci/luci-go/+/a38cde76483f9c6f6ad6b2dff42f8411691273b8/buildbucket/proto/step.proto#58
var StatusPrecedence = []int{
	int(pb.Status_SUCCESS), // best
	int(pb.Status_FAILURE),
	int(pb.Status_INFRA_FAILURE),
	int(pb.Status_CANCELED), // worst
}

var RelationStatuses = map[pb.Status]struct{}{
	pb.Status_SUCCESS:       {},
	pb.Status_FAILURE:       {},
	pb.Status_INFRA_FAILURE: {},
	pb.Status_CANCELED:      {},
}

// ParentStepName returns the name of the parent step.
//
// If stepName is empty or doesn't have a parent, an empty string is returned.
func ParentStepName(stepName string) string {
	i := strings.LastIndex(stepName, StepNameSep)
	if i == -1 {
		return ""
	}
	return stepName[:i]
}

// ValidateStepName validates a given step name.
func ValidateStepName(stepName string) error {
	if stepName == "" {
		return errors.Reason("required").Err()
	}

	nodes := strings.Split(stepName, StepNameSep)
	for _, node := range nodes {
		if node == "" {
			return errors.Reason("there must be at least one character before and after %q (%q)",
				StepNameSep, stepName).Err()
		}
	}
	return nil
}

// IsWorseStepStatusWithMap returns true if lhs is worse than lhs.
// If either of lhs or rhs is not one of the statuses in StatusPrecedence,
// this always returns false.
func IsWorseStepStatusWithMap(lhs, rhs pb.Status) bool {
	if _, exist := RelationStatuses[lhs]; !exist {
		return false
	}
	if _, exist := RelationStatuses[rhs]; !exist {
		return false
	}
	return lhs > rhs
}

// IsWorseStepStatusWithBS returns true if lhs is worse than lhs.
// If either of lhs or rhs is not one of the statuses in StatusPrecedence,
// this always returns false.
func IsWorseStepStatusWithBS(lhs, rhs pb.Status) bool {
	li := sort.SearchInts(StatusPrecedence, int(lhs))
	if li >= len(StatusPrecedence) || StatusPrecedence[li] != int(lhs) {
		return false
	}
	ri := sort.SearchInts(StatusPrecedence, int(rhs))
	if ri >= len(StatusPrecedence) || StatusPrecedence[ri] != int(rhs) {
		return false
	}
	return lhs > rhs
}

func IsWorseStepStatus(lhs, rhs pb.Status) bool {
	return IsWorseStepStatusWithLinear(lhs, rhs)
}

// IsWorseStepStatusWithLinear returns true if lhs is worse than lhs.
// If either of lhs or rhs is not one of the statuses in StatusPrecedence,
// this always returns false.
func IsWorseStepStatusWithLinear(lhs, rhs pb.Status) bool {
	var lfound, rfound bool
	for _, s := range StatusPrecedence {
		switch s {
		case int(lhs):
			lfound = true
		case int(rhs):
			rfound = true
		}
		if lfound && rfound {
			break
		}
	}
	if !(lfound && rfound) {
		return false
	}
	return lhs > rhs
}
