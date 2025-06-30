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
