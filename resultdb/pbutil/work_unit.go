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

	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	workUnitIDPattern   = `[a-z][a-z0-9_\-.]*(?::[a-z][a-z0-9_\-.]*)?`
	workUnitIDMaxLength = 100
)

var workUnitIDRe = regexpf("^%s$", workUnitIDPattern)
var workUnitNameRe = regexpf("^rootInvocations/(%s)/workUnits/(%s)$", rootInvocationIDPattern, workUnitIDPattern)

// ValidateWorkUnitID returns a non-nil error if the given work unit ID is invalid.
func ValidateWorkUnitID(id string) error {
	if len(id) > workUnitIDMaxLength {
		return fmt.Errorf("work unit ID must be at most %v bytes long", workUnitIDMaxLength)
	}
	return validate.SpecifiedWithRe(workUnitIDRe, id)
}

// ValidateWorkUnitName returns a non-nil error if the given work unit name is invalid.
func ValidateWorkUnitName(name string) error {
	_, _, err := ParseWorkUnitName(name)
	return err
}

// ParseWorkUnitName extracts the root invocation and work unit id from a work unit resource name.
//
// This function returns an error if parsing fails.
// Note: Error construction has an non-negligible CPU cost due to stack walk.
// Use this when detailed error information is needed.
// For a boolean check without error overhead, use TryParseWorkUnitName.
func ParseWorkUnitName(name string) (rootInvocationID string, workUnitID string, err error) {
	if name == "" {
		return "", "", validate.Unspecified()
	}

	m := workUnitNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", validate.DoesNotMatchReErr(workUnitNameRe)
	}
	rootInvocationID = m[1]
	workUnitID = m[2]
	if len(rootInvocationID) > rootInvocationMaxLength {
		return "", "", fmt.Errorf("root invocation ID must be at most %v bytes long", rootInvocationMaxLength)
	}
	if len(workUnitID) > workUnitIDMaxLength {
		return "", "", fmt.Errorf("work unit ID must be at most %v bytes long", workUnitIDMaxLength)
	}
	return rootInvocationID, workUnitID, nil
}

// TryParseWorkUnitName attempts to extract the work unit id from a name.
// It returns the id and true if the name is valid, otherwise it returns
// an empty string and false.
func TryParseWorkUnitName(name string) (rootInvocationID string, workUnitID string, ok bool) {
	m := workUnitNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", "", false
	}
	rootInvocationID = m[1]
	workUnitID = m[2]
	if len(rootInvocationID) > rootInvocationMaxLength {
		return "", "", false
	}
	if len(workUnitID) > workUnitIDMaxLength {
		return "", "", false
	}
	return rootInvocationID, workUnitID, true
}

// WorkUnitName constructs a work unit resource name from a root invocation
// ID and a work unit ID.
// Does not validate the specified IDs; use ValidateRootInvocationID and
// ValidateWorkUnitID to do so.
func WorkUnitName(rootInvocationID, workUnitID string) string {
	return "rootInvocations/" + rootInvocationID + "/workUnits/" + workUnitID
}

// ValidateWorkUnitView validates the work unit view is valid.
func ValidateWorkUnitView(view pb.WorkUnitView) error {
	switch view {
	case pb.WorkUnitView_WORK_UNIT_VIEW_UNSPECIFIED, pb.WorkUnitView_WORK_UNIT_VIEW_FULL, pb.WorkUnitView_WORK_UNIT_VIEW_BASIC:
		return nil
	default:
		return fmt.Errorf("unrecognized view %q", view)
	}
}
