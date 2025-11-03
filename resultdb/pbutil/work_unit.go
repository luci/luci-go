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
	"regexp"
	"unicode/utf8"

	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	workUnitIDPattern        = `[a-z][a-z0-9_\-.]*(?::[a-z][a-z0-9_\-.]*)?`
	workUnitIDMaxLength      = 100
	workUnitKindLength       = 50
	summaryMarkdownMaxLength = 4096 // bytes
	moduleShardKeyMaxLength  = 50   // bytes
)

var workUnitIDRe = regexpf("^%s$", workUnitIDPattern)
var workUnitNameRe = regexpf("^rootInvocations/(%s)/workUnits/(%s)$", rootInvocationIDPattern, workUnitIDPattern)
var moduleShardKeyRe = regexp.MustCompile("^[a-z0-9_\\-]+$")
var workUnitKindRe = regexp.MustCompile("^[A-Z0-9]+(_[A-Z0-9]+)+$")

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
	if err := ValidateRootInvocationID(rootInvocationID); err != nil {
		return "", "", fmt.Errorf("root invocation ID: %w", err)
	}
	workUnitID = m[2]
	if err := ValidateWorkUnitID(workUnitID); err != nil {
		return "", "", fmt.Errorf("work unit ID: %w", err)
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

// ExtendedPropertiesEqual compares two map[string]*structpb.Struct for equality.
func ExtendedPropertiesEqual(a, b map[string]*structpb.Struct) bool {
	if len(a) != len(b) {
		return false
	}

	for k, vA := range a {
		vB, ok := b[k]
		if !ok {
			return false
		}
		if !proto.Equal(vA, vB) {
			return false
		}
	}
	return true
}

// ValidateSummaryMarkdown validates a work unit's summary markdown.
// enforceLength controls whether length limits are to be enforced.
// If they are not enforced on the API surface, TruncateSummaryMarkdown
// should be used.
func ValidateSummaryMarkdown(summaryMarkdown string, enforceLength bool) error {
	if enforceLength && len(summaryMarkdown) > summaryMarkdownMaxLength {
		return fmt.Errorf("must be at most %v bytes long (was %v bytes)", summaryMarkdownMaxLength, len(summaryMarkdown))
	}
	if !utf8.ValidString(summaryMarkdown) {
		return errors.New("not valid UTF-8")
	}
	return nil
}

// TruncateSummaryMarkdown truncates the summary markdown to
// fit within its legal limit.
func TruncateSummaryMarkdown(summaryMarkdown string) string {
	return TruncateString(summaryMarkdown, summaryMarkdownMaxLength)
}

// ValidateWorkUnitState validates the work unit state is a valid value.
func ValidateWorkUnitState(state pb.WorkUnit_State) error {
	if state == pb.WorkUnit_STATE_UNSPECIFIED {
		return validate.Unspecified()
	}
	if _, ok := pb.WorkUnit_State_name[int32(state)]; !ok {
		return errors.Fmt("unknown state %v", state)
	}
	if state == pb.WorkUnit_FINAL_STATE_MASK {
		return errors.New("FINAL_STATE_MASK is not a valid state")
	}
	return nil
}

// IsFinalWorkUnitState returns if the given work unit state is a final state.
func IsFinalWorkUnitState(state pb.WorkUnit_State) bool {
	return state&pb.WorkUnit_FINAL_STATE_MASK != 0
}

// ValidateModuleShardKey returns an error if moduleShardKey is not a valid
// module shard key.
func ValidateModuleShardKey(moduleShardKey string) error {
	if len(moduleShardKey) > moduleShardKeyMaxLength {
		return fmt.Errorf("must be at most %v bytes long (was %v bytes)", moduleShardKeyMaxLength, len(moduleShardKey))
	}
	return validate.SpecifiedWithRe(moduleShardKeyRe, moduleShardKey)
}

// ValidateWorkUnitKind validates a work unit kind.
func ValidateWorkUnitKind(kind string) error {
	if kind == "" {
		return validate.Unspecified()
	}
	if len(kind) > workUnitKindLength {
		return fmt.Errorf("must be at most %v bytes long (was %v bytes)", workUnitKindLength, len(kind))
	}
	if !workUnitKindRe.MatchString(kind) {
		return fmt.Errorf("must match %q", workUnitKindRe.String())
	}
	return nil
}
