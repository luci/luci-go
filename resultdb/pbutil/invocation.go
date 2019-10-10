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
	"time"

	"github.com/golang/protobuf/ptypes"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const invocationIDPattern = `[a-z][a-z0-9_\-]*`

var invocationIDRe = regexpf("^%s$", invocationIDPattern)
var invocationNameRe = regexpf("^invocations/(%s)$", invocationIDPattern)

// ValidateInvocationID returns a non-nil error if id is invalid.
func ValidateInvocationID(id string) error {
	if !invocationIDRe.MatchString(id) {
		return doesNotMatch(invocationIDRe)
	}
	return nil
}

// ParseInvocationName retrieves the invocation id.
func ParseInvocationName(name string) (id string, err error) {
	m := invocationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", doesNotMatch(invocationNameRe)
	}
	return m[1], nil
}

// MustParseInvocationName retrieves the invocation id.
// Panics if the name is invalid. Useful for situations when name was already
// validated.
func MustParseInvocationName(name string) (id string) {
	id, err := ParseInvocationName(name)
	if err != nil {
		panic(err)
	}
	return id
}

// InvocationName produces an invocation name from an id.
// Does not validate id, use ValidateInvocationID.
func InvocationName(id string) string {
	return "invocations/" + id
}

// IsFinalized returns whether the given invocation state is final, indicating immutability.
func IsFinalized(state pb.Invocation_State) bool {
	switch state {
	case pb.Invocation_COMPLETED, pb.Invocation_INTERRUPTED:
		return true
	default:
		return false
	}
}

// NormalizeInvocation converts inv to the canonical form.
func NormalizeInvocation(inv *pb.Invocation) {
	sortStringPairs(inv.Tags)
}

// ValidateCreateInvocationRequest returns an error if req is invalid.
func ValidateCreateInvocationRequest(req *pb.CreateInvocationRequest, now time.Time) error {
	if err := ValidateInvocationID(req.InvocationId); err != nil {
		return errors.Annotate(err, "invocation_id").Err()
	}

	if err := ValidateStringPairs(req.Invocation.Tags); err != nil {
		return errors.Annotate(err, "invocation.tags").Err()
	}

	if req.Invocation.Deadline != nil {
		switch deadline, err := ptypes.Timestamp(req.Invocation.Deadline); {
		case err != nil:
			return err
		case now.Sub(deadline) > 10*time.Second:
			return errors.Reason("invocation.deadline must be at least 10 seconds in the future").Err()
		case deadline.Sub(now) > 2*24*time.Hour:
			return errors.Reason("invocation.deadline must be before 48h in the future").Err()
		}
	}

	if req.Invocation.BaseTestVariantDef != nil {
		if err := ValidateVariantDef(req.Invocation.BaseTestVariantDef); err != nil {
			return errors.Annotate(err, "invocation.base_test_variant_def").Err()
		}
	}

	return nil
}
