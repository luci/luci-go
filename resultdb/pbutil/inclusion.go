// Copyright 2019 The LUCI Authors.
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

package pbutil

import (
	"fmt"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var inclusionNameRe = regexpf("^invocations/(%s)/inclusions/(%s)$", invocationIDPattern, invocationIDPattern)

// ParseInclusionName retrieves invocation ids.
func ParseInclusionName(name string) (includingInvocationID, includedInvocationID string, err error) {
	m := inclusionNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", doesNotMatch(inclusionNameRe)
	}
	return m[1], m[2], nil
}

// InclusionName formats an inclusion name.
// Does not validate invocation ids, use ValidateInvocationID.
func InclusionName(includingInvocationID, includedInvocationID string) string {
	return fmt.Sprintf("invocations/%s/inclusions/%s", includingInvocationID, includedInvocationID)
}

// ValidateCreateInclusionRequest returns a non-nil error if req is invalid.
func ValidateCreateInclusionRequest(req *pb.CreateInclusionRequest) error {
	if _, err := ParseInvocationName(req.IncludingInvocation); err != nil {
		return errors.Annotate(err, "including_invocation").Err()
	}
	if _, err := ParseInvocationName(req.GetInclusion().GetIncludedInvocation()); err != nil {
		return errors.Annotate(err, "inclusion.included_invocation").Err()
	}

	if req.Inclusion.IncludedInvocation == req.IncludingInvocation {
		return errors.Reason("cannot include itself").Err()
	}
	return nil
}

// ValidateOverrideInclusionRequest returns a non-nil error if req is invalid.
func ValidateOverrideInclusionRequest(req *pb.OverrideInclusionRequest) error {
	if _, err := ParseInvocationName(req.IncludingInvocation); err != nil {
		return errors.Annotate(err, "including_invocation").Err()
	}
	if _, err := ParseInvocationName(req.OverriddenIncludedInvocation); err != nil {
		return errors.Annotate(err, "overridden_included_invocation").Err()
	}
	if _, err := ParseInvocationName(req.OverridingIncludedInvocation); err != nil {
		return errors.Annotate(err, "overriding_included_invocation").Err()
	}

	if req.OverridingIncludedInvocation == req.IncludingInvocation || req.OverriddenIncludedInvocation == req.IncludingInvocation {
		return errors.Reason("cannot include itself").Err()
	}

	if req.OverridingIncludedInvocation == req.OverriddenIncludedInvocation {
		return errors.Reason("cannot override itself").Err()
	}

	return nil
}
