// Copyright 2026 The LUCI Authors.
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

package testresultsv2

import (
	"strings"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
)

// VerdictID represents the identifier of a test verdict (without work unit or result ID).
// It identifies a specific test variant within a root invocation.
type VerdictID struct {
	// The root invocation shard.
	// Note: it is a system invariant that all results for a given test ID will be
	// in exactly one shard. Which shard is determined by the sharding algorithm
	// used by the invocation.
	RootInvocationShardID rootinvocations.ShardID
	// Test identifier components.
	ModuleName        string
	ModuleScheme      string
	ModuleVariantHash string
	CoarseName        string
	FineName          string
	CaseName          string
}

// Compare returns -1 iff id < other, 0 iff id == other and 1 iff id > other
// in TestResultV2 / TestExonerationV2 table order.
func (id VerdictID) Compare(other VerdictID) int {
	if id.RootInvocationShardID != other.RootInvocationShardID {
		if id.RootInvocationShardID.Before(other.RootInvocationShardID) {
			return -1
		}
		return 1
	}
	if id.ModuleName != other.ModuleName {
		return strings.Compare(id.ModuleName, other.ModuleName)
	}
	if id.ModuleScheme != other.ModuleScheme {
		return strings.Compare(id.ModuleScheme, other.ModuleScheme)
	}
	if id.ModuleVariantHash != other.ModuleVariantHash {
		return strings.Compare(id.ModuleVariantHash, other.ModuleVariantHash)
	}
	if id.CoarseName != other.CoarseName {
		return strings.Compare(id.CoarseName, other.CoarseName)
	}
	if id.FineName != other.FineName {
		return strings.Compare(id.FineName, other.FineName)
	}
	if id.CaseName != other.CaseName {
		return strings.Compare(id.CaseName, other.CaseName)
	}
	return 0
}
