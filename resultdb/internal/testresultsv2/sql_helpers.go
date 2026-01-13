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
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

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
