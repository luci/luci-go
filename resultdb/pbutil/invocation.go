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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	invocationIDPattern                    = `[a-z][a-z0-9_\-.]{0,99}`
	invocationExtendedPropertyKeyPattern   = `[a-z]([a-z0-9_]{0,61}[a-z0-9])?`
	MaxSizeInvocationExtendedPropertyValue = 512 * 1024      // 512 KB
	MaxSizeInvocationExtendedProperties    = 2 * 1024 * 1024 // 2 MB
)

var invocationIDRe = regexpf("^%s$", invocationIDPattern)
var invocationNameRe = regexpf("^invocations/(%s)$", invocationIDPattern)
var invocationExtendedPropertyKeyRe = regexpf("^%s$", invocationExtendedPropertyKeyPattern)

// ValidateInvocationID returns a non-nil error if id is invalid.
func ValidateInvocationID(id string) error {
	return validate.SpecifiedWithRe(invocationIDRe, id)
}

// ValidateInvocationName returns a non-nil error if name is invalid.
func ValidateInvocationName(name string) error {
	_, err := ParseInvocationName(name)
	return err
}

// ParseInvocationName extracts the invocation id.
//
// This function returns an error if parsing fails.
// Note: Error construction has an non-negligible CPU cost.
// Use this when detailed error information is needed.
// For a boolean check without error overhead, use TryParseInvocationName.
func ParseInvocationName(name string) (id string, err error) {
	if name == "" {
		return "", validate.Unspecified()
	}

	m := invocationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", validate.DoesNotMatchReErr(invocationNameRe)
	}
	return m[1], nil
}

// TryParseInvocationName attempts to extract the invocation id from a name.
// It returns the id and true if the name is valid, otherwise it returns
// an empty string and false.
func TryParseInvocationName(name string) (id string, ok bool) {
	if name == "" {
		return "", false
	}

	m := invocationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// InvocationName synthesizes an invocation name from an id.
// Does not validate id, use ValidateInvocationID.
func InvocationName(id string) string {
	return "invocations/" + id
}

// NormalizeInvocation converts inv to the canonical form.
func NormalizeInvocation(inv *pb.Invocation) {
	SortStringPairs(inv.Tags)

	changelists := inv.SourceSpec.GetSources().GetChangelists()
	SortGerritChanges(changelists)
}

// ValidateInvocationExtendedPropertyKey returns a non-nil error if key is invalid.
func ValidateInvocationExtendedPropertyKey(key string) error {
	return validate.SpecifiedWithRe(invocationExtendedPropertyKeyRe, key)
}

// ValidateInvocationExtendedProperties returns a non-nil error if extendedProperties is invalid.
func ValidateInvocationExtendedProperties(extendedProperties map[string]*structpb.Struct) error {
	for key, value := range extendedProperties {
		if err := ValidateInvocationExtendedPropertyKey(key); err != nil {
			return errors.Fmt("key %q: %w", key, err)
		}
		if value == nil {
			return errors.Fmt("[%q]: value unspecified", key)
		}
		if err := validateProperties(value, MaxSizeInvocationExtendedPropertyValue, true /* requiresType */); err != nil {
			return errors.Fmt("[%q]: %w", key, err)
		}
	}
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: extendedProperties,
	}
	if proto.Size(internalExtendedProperties) > MaxSizeInvocationExtendedProperties {
		return errors.Fmt("exceeds the maximum size of %d bytes", MaxSizeInvocationExtendedProperties)
	}
	return nil
}
