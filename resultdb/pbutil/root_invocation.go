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

package pbutil

import (
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	rootInvocationIDPattern = `[a-z][a-z0-9_\-.]*`
	rootInvocationMaxLength = 100
)

var rootInvocationIDRe = regexpf("^%s$", rootInvocationIDPattern)
var rootInvocationNameRe = regexpf("^rootInvocations/(%s)$", rootInvocationIDPattern)

// ValidateRootInvocationID returns a non-nil error if the given root invocation ID is invalid.
func ValidateRootInvocationID(id string) error {
	if len(id) > rootInvocationMaxLength {
		return fmt.Errorf("root invocation ID must be at most %v bytes long", rootInvocationMaxLength)
	}
	return validate.SpecifiedWithRe(rootInvocationIDRe, id)
}

// ValidateRootInvocationName returns a non-nil error if the given root invocation name is invalid.
func ValidateRootInvocationName(name string) error {
	_, err := ParseRootInvocationName(name)
	return err
}

// ParseRootInvocationName extracts the root invocation id from a root invocation resource name.
//
// This function returns an error if parsing fails.
// Note: Error construction has an non-negligible CPU cost due to stack walk.
// Use this when detailed error information is needed.
// For a boolean check without error overhead, use TryParseInvocationName.
func ParseRootInvocationName(name string) (id string, err error) {
	if name == "" {
		return "", validate.Unspecified()
	}

	m := rootInvocationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", validate.DoesNotMatchReErr(rootInvocationNameRe)
	}
	id = m[1]
	if len(id) > rootInvocationMaxLength {
		return "", fmt.Errorf("root invocation ID must be at most %v bytes long", rootInvocationMaxLength)
	}
	return id, nil
}

// TryParseRootInvocationName attempts to extract the root invocation id from a name.
// It returns the id and true if the name is valid, otherwise it returns
// an empty string and false.
func TryParseRootInvocationName(name string) (id string, ok bool) {
	m := rootInvocationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	id = m[1]
	if len(id) > rootInvocationMaxLength {
		return "", false
	}
	return id, true
}

// RootInvocationName constructs a root invocation resource name from an id.
// Does not validate id; use ValidateInvocationID to do so.
func RootInvocationName(id string) string {
	return "rootInvocations/" + id
}

// ValidateStreamingExportState validates the streaming export state is a valid
// value.
func ValidateStreamingExportState(state pb.RootInvocation_StreamingExportState) error {
	if state == pb.RootInvocation_STREAMING_EXPORT_STATE_UNSPECIFIED {
		return errors.New("unspecified")
	}
	if _, ok := pb.RootInvocation_State_name[int32(state)]; !ok {
		return errors.Fmt("unknown state %v", state)
	}
	return nil
}

// IsFinalRootInvocationState returns if the given root invocation state is a final state.
func IsFinalRootInvocationState(state pb.RootInvocation_State) bool {
	return state&pb.RootInvocation_FINAL_STATE_MASK != 0
}

// WorkUnitToRootInvocationState converts a work unit state to a root invocation state.
func WorkUnitToRootInvocationState(state pb.WorkUnit_State) pb.RootInvocation_State {
	switch state {
	case pb.WorkUnit_PENDING:
		return pb.RootInvocation_PENDING
	case pb.WorkUnit_RUNNING:
		return pb.RootInvocation_RUNNING
	case pb.WorkUnit_SUCCEEDED:
		return pb.RootInvocation_SUCCEEDED
	case pb.WorkUnit_SKIPPED:
		return pb.RootInvocation_SKIPPED
	case pb.WorkUnit_FAILED:
		return pb.RootInvocation_FAILED
	case pb.WorkUnit_CANCELLED:
		return pb.RootInvocation_CANCELLED
	case pb.WorkUnit_FINAL_STATE_MASK:
		// This state should not be passed to this function
		// as it is not an actual state.
		panic(fmt.Sprintf("not an actual work unit state %s", state))
	default:
		// All other states are not supported.
		panic(fmt.Sprintf("unknown work unit state %s", state))
	}
}
