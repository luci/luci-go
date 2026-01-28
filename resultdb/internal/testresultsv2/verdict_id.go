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

	"go.chromium.org/luci/common/errors"

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
// in the given ordering.
func (id VerdictID) Compare(other VerdictID, order Ordering) int {
	if order == OrderingByPrimaryKey {
		if id.RootInvocationShardID != other.RootInvocationShardID {
			if id.RootInvocationShardID.Before(other.RootInvocationShardID) {
				return -1
			}
			return 1
		}
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

// The maximum number of nominated verdict IDs.
//
// This comes from two places:
//   - If the IDs are ever used in an IN clause, we have
//     https://cloud.google.com/spanner/quotas#query-limits ("Values in an IN operator")
//     limiting us to at most 10,000 items.
//   - Any Spanner parameters will count towards the request size limit of 10 MiB. As each
//     test ID is up to around 512 characters, with the addition of the RootInvocation and
//     VariantHash, the total size could be some 6-7 MiB. See:
//     https://docs.cloud.google.com/spanner/quotas#request-limits
const MaxNominatedVerdicts = 10_000

// SpannerVerdictID is the struct used to represent a test verdict ID in Spanner.
// It matches the schema of the TestResultsV2 and TestExonerationsV2 table without additional
// conversion.
type SpannerVerdictID struct {
	// The root invocation shard.
	RootInvocationShardID string
	// Test identifier components.
	ModuleName        string
	ModuleScheme      string
	ModuleVariantHash string
	T1CoarseName      string
	T2FineName        string
	T3CaseName        string
}

// SpannerVerdictIDs returns the given verdict IDs in a form that can be passed to Spanner.
func SpannerVerdictIDs(verdictIDs []VerdictID) ([]SpannerVerdictID, error) {
	if len(verdictIDs) > MaxNominatedVerdicts {
		return nil, errors.Fmt("got %d nominated verdicts, but only %d are allowed", len(verdictIDs), MaxNominatedVerdicts)
	}
	spannerIDs := make([]SpannerVerdictID, 0, len(verdictIDs))
	for _, id := range verdictIDs {
		spannerIDs = append(spannerIDs, SpannerVerdictID{
			RootInvocationShardID: id.RootInvocationShardID.RowID(),
			ModuleName:            id.ModuleName,
			ModuleScheme:          id.ModuleScheme,
			ModuleVariantHash:     id.ModuleVariantHash,
			T1CoarseName:          id.CoarseName,
			T2FineName:            id.FineName,
			T3CaseName:            id.CaseName,
		})
	}
	return spannerIDs, nil
}
