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
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// NominatedVerdictsClause returns a WHERE clause that matches the given test IDs.
func NominatedVerdictsClause(nominatedIDs []VerdictID, params map[string]any) (predicate string, err error) {
	// At most 10,000 IDs can be nominated (see "Values in an IN operator"):
	// https://docs.cloud.google.com/spanner/quotas#query-limits
	const maxNominatedIDs = 10_000
	if len(nominatedIDs) > maxNominatedIDs {
		return "", fmt.Errorf("number of nominated IDs (%d) exceeds the limit of %d", len(nominatedIDs), maxNominatedIDs)
	}

	type spannerTestID struct {
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

	spannerIDs := make([]spannerTestID, 0, len(nominatedIDs))
	for _, id := range nominatedIDs {
		spannerIDs = append(spannerIDs, spannerTestID{
			RootInvocationShardID: id.RootInvocationShardID.RowID(),
			ModuleName:            id.ModuleName,
			ModuleScheme:          id.ModuleScheme,
			ModuleVariantHash:     id.ModuleVariantHash,
			T1CoarseName:          id.CoarseName,
			T2FineName:            id.FineName,
			T3CaseName:            id.CaseName,
		})
	}

	params["nominatedIDs"] = spannerIDs
	return `STRUCT(RootInvocationShardId, ModuleName, ModuleScheme, ModuleVariantHash, T1CoarseName, T2FineName, T3CaseName) IN UNNEST(@nominatedIDs)`, nil
}

// PrefixWhereClause returns a WHERE clause that matches the given test ID prefix.
// The WHERE clause will be used to filter the TestResultsV2 and TestExonerationsV2 tables.
func PrefixWhereClause(prefix *pb.TestIdentifierPrefix, params map[string]any) (predicate string, err error) {
	// This should have been validated by the user of the Query type,
	// but verify again for robustness.
	if err := pbutil.ValidateTestIdentifierPrefixForQuery(prefix); err != nil {
		return "", err
	}
	if prefix.Level == pb.AggregationLevel_INVOCATION {
		// No prefix filter or prefix captures all test results in the invocation.
		return "TRUE", nil
	}

	var predicateBuilder strings.Builder
	predicateBuilder.WriteString("ModuleName = @prefixModuleName AND ModuleScheme = @prefixModuleScheme AND ModuleVariantHash = @prefixModuleVariantHash")
	var moduleVariantHash string
	if prefix.Id.ModuleVariant != nil {
		// Module variant was specified as a variant proto.
		moduleVariantHash = pbutil.VariantHash(prefix.Id.ModuleVariant)
	} else if prefix.Id.ModuleVariantHash != "" {
		// Module variant was specified as a hash.
		moduleVariantHash = prefix.Id.ModuleVariantHash
	} else {
		return "", errors.Fmt("prefix filter must specify Variant or VariantHash for a level of MODULE and below")
	}
	params["prefixModuleName"] = prefix.Id.ModuleName
	params["prefixModuleScheme"] = prefix.Id.ModuleScheme
	params["prefixModuleVariantHash"] = moduleVariantHash

	if prefix.Level == pb.AggregationLevel_MODULE {
		return predicateBuilder.String(), nil
	}

	predicateBuilder.WriteString(" AND T1CoarseName = @prefixCoarseName")
	params["prefixCoarseName"] = prefix.Id.CoarseName
	if prefix.Level == pb.AggregationLevel_COARSE {
		return predicateBuilder.String(), nil
	}

	predicateBuilder.WriteString(" AND T2FineName = @prefixFineName")
	params["prefixFineName"] = prefix.Id.FineName
	if prefix.Level == pb.AggregationLevel_FINE {
		return predicateBuilder.String(), nil
	}

	predicateBuilder.WriteString(" AND T3CaseName = @prefixCaseName")
	params["prefixCaseName"] = prefix.Id.CaseName
	return predicateBuilder.String(), nil
}
