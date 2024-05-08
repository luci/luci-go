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

	"go.chromium.org/luci/resultdb/internal/invocations/invocationspb"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	invocationIDPattern                     = `[a-z][a-z0-9_\-:.]{0,99}`
	invocationExtendedPropertyKeyPattern    = `[a-z]([a-z0-9_]{0,61}[a-z0-9])?`
	invocationExtendedPropertySchemaPattern = `[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+`
	MaxSizeInvocationExtendedPropertyValue  = 20 * 1024  // 20 KB
	MaxSizeInvocationExtendedProperties     = 100 * 1024 // 100 KB

)

var invocationIDRe = regexpf("^%s$", invocationIDPattern)
var invocationNameRe = regexpf("^invocations/(%s)$", invocationIDPattern)
var invocationExtendedPropertyKeyRe = regexpf("^%s$", invocationExtendedPropertyKeyPattern)
var invocationExtendedPropertySchemaRe = regexpf("^%s$", invocationExtendedPropertySchemaPattern)

// ValidateInvocationID returns a non-nil error if id is invalid.
func ValidateInvocationID(id string) error {
	return validateWithRe(invocationIDRe, id)
}

// ValidateInvocationName returns a non-nil error if name is invalid.
func ValidateInvocationName(name string) error {
	_, err := ParseInvocationName(name)
	return err
}

// ParseInvocationName extracts the invocation id.
func ParseInvocationName(name string) (id string, err error) {
	if name == "" {
		return "", unspecified()
	}

	m := invocationNameRe.FindStringSubmatch(name)
	if m == nil {
		return "", doesNotMatch(invocationNameRe)
	}
	return m[1], nil
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

// ValidateSourceSpec validates a source specification.
func ValidateSourceSpec(sourceSpec *pb.SourceSpec) error {
	// Treat nil sourceSpec message as empty message.
	if sourceSpec.GetInherit() && sourceSpec.GetSources() != nil {
		return errors.Reason("only one of inherit and sources may be set").Err()
	}
	if sourceSpec.GetSources() != nil {
		if err := ValidateSources(sourceSpec.Sources); err != nil {
			return errors.Annotate(err, "sources").Err()
		}
	}
	return nil
}

// ValidateSources validates a set of sources.
func ValidateSources(sources *pb.Sources) error {
	if sources == nil {
		return errors.Reason("unspecified").Err()
	}
	if err := ValidateGitilesCommit(sources.GetGitilesCommit()); err != nil {
		return errors.Annotate(err, "gitiles_commit").Err()
	}

	if len(sources.Changelists) > 10 {
		return errors.Reason("changelists: exceeds maximum of 10 changelists").Err()
	}
	type distinctChangelist struct {
		host   string
		change int64
	}
	clToIndex := make(map[distinctChangelist]int)

	for i, cl := range sources.Changelists {
		if err := ValidateGerritChange(cl); err != nil {
			return errors.Annotate(err, "changelists[%v]", i).Err()
		}
		cl := distinctChangelist{
			host:   cl.Host,
			change: cl.Change,
		}
		if duplicateIndex, ok := clToIndex[cl]; ok {
			return errors.Reason("changelists[%v]: duplicate change modulo patchset number; same change at changelists[%v]", i, duplicateIndex).Err()
		}
		clToIndex[cl] = i
	}
	return nil
}

// ValidateInvocationExtendedPropertyKey returns a non-nil error if key is invalid.
func ValidateInvocationExtendedPropertyKey(key string) error {
	return validateWithRe(invocationExtendedPropertyKeyRe, key)
}

// ValidateInvocationExtendedProperties returns a non-nil error if extendedProperties is invalid.
func ValidateInvocationExtendedProperties(extendedProperties map[string]*structpb.Struct) error {
	for key, value := range extendedProperties {
		if err := ValidateInvocationExtendedPropertyKey(key); err != nil {
			return errors.Annotate(err, "key %q", key).Err()
		}
		if err := validateProperties(value, MaxSizeInvocationExtendedPropertyValue); err != nil {
			return errors.Annotate(err, "[%q]", key).Err()
		}
		schemaVal, schemaExist := value.Fields["@type"]
		if !schemaExist {
			return errors.Reason("[%q] should have a key \"@type\"", key).Err()
		}
		if err := validateWithRe(invocationExtendedPropertySchemaRe, schemaVal.GetStringValue()); err != nil {
			return errors.Annotate(err, "[%q][\"@type\"]", key).Err()
		}
	}
	internalExtendedProperties := &invocationspb.ExtendedProperties{
		ExtendedProperties: extendedProperties,
	}
	if proto.Size(internalExtendedProperties) > MaxSizeInvocationExtendedProperties {
		return errors.Reason("exceeds the maximum size of %d bytes", MaxSizeInvocationExtendedProperties).Err()
	}
	return nil
}
