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

package testresultsv2

import (
	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MaxFilterLengthBytes is the maximum length, in bytes, of an AIP-160 filter on the test results.
// This is intended to avoid timeouts related to overly complex filters and limit the size of
// the generated SQL.
const MaxFilterLengthBytes = 16 * 1024

var statusEnumDef = aip160.NewEnumDefinition("pb.TestResult.Status", pb.TestResult_Status_value, int32(pb.TestResult_STATUS_UNSPECIFIED))
var testIDColumnDefs = StructuredTestIDColumnNames{
	ModuleName:   "ModuleName",
	ModuleScheme: "ModuleScheme",
	CoarseName:   "T1CoarseName",
	FineName:     "T2FineName",
	CaseName:     "T3CaseName",
}

// Security: filters on a column that a user does not have access to can leak information. With
// certain filter types, this can be done rather quickly.
//
// E.g. if "module_variant.key:s" matches, they could try "module_variant.key:sa", "module_variant.key:sb",
// until they find a filter that matches, say "module_variant.key:se". With repetition, this allows shifting out
// the full value, "module_variant.key:secret". This attack works in ~N*A queries, where N is the length of the
// value and A is the size of the alphabet.
//
// To prevent this, we required implementers to define a custom columns on the TestResultsV2 table that masks
// out columns if the user does not have access to it. The columns required are:
// - ModuleVariantMasked
// - TagsMasked
// - TestMetadataNameMasked
// - TestMetadataLocationRepoMasked
// - TestMetadataLocationFileNameMasked
//
// The columns are named differently to the original table columns to avoid
// accidental security bugs if this comment is not followed, as the filters
// will simply not work.
var filterSchema = aip160.NewDatabaseTable().WithFields(
	aip160.NewField().WithFieldPath("test_id").WithBackend(NewFlatTestIDFieldBackend(testIDColumnDefs)).Filterable().Build(),
	aip160.NewField().WithFieldPath("variant").WithBackend(aip160.NewKeyValueColumn("ModuleVariantMasked").WithStringArray().Build()).Filterable().Build(),
	aip160.NewField().WithFieldPath("variant_hash").WithBackend(aip160.NewStringColumn("ModuleVariantHash")).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "module_name").WithBackend(aip160.NewStringColumn("ModuleName")).FilterableImplicitly().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "module_scheme").WithBackend(aip160.NewStringColumn("ModuleScheme")).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "module_variant").WithBackend(aip160.NewKeyValueColumn("ModuleVariantMasked").WithStringArray().Build()).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "module_variant_hash").WithBackend(aip160.NewStringColumn("ModuleVariantHash")).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "coarse_name").WithBackend(aip160.NewStringColumn("T1CoarseName")).FilterableImplicitly().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "fine_name").WithBackend(aip160.NewStringColumn("T2FineName")).FilterableImplicitly().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "case_name").WithBackend(aip160.NewStringColumn("T3CaseName")).FilterableImplicitly().Build(),
	aip160.NewField().WithFieldPath("tags").WithBackend(aip160.NewKeyValueColumn("TagsMasked").WithStringArray().Build()).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_metadata", "name").WithBackend(aip160.NewStringColumn("TestMetadataNameMasked")).FilterableImplicitly().Build(),
	aip160.NewField().WithFieldPath("test_metadata", "location", "repo").WithBackend(aip160.NewStringColumn("TestMetadataLocationRepoMasked")).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_metadata", "location", "file_name").WithBackend(aip160.NewStringColumn("TestMetadataLocationFileNameMasked")).FilterableImplicitly().Build(),
	aip160.NewField().WithFieldPath("duration").WithBackend(aip160.NewDurationColumn("RunDurationNanos").Build()).Filterable().Build(),
	aip160.NewField().WithFieldPath("status").WithBackend(aip160.NewEnumColumn("StatusV2").WithDefinition(statusEnumDef).Build()).Filterable().Build(),
	// Future extensions: in the old Test Results tab in MILO, we also supported (test result) status and duration.
	// As at writing, this requires extending the aip160 library to support enums and durations (stored as INT64 nanoseconds).
).Build()

// ValidateFilter validates an AIP-160 filter string.
func ValidateFilter(filter string) error {
	if len(filter) > MaxFilterLengthBytes {
		return errors.Fmt("filter is too long (got %v bytes, want at most %v bytes)", len(filter), MaxFilterLengthBytes)
	}
	f, err := aip160.ParseFilter(filter)
	if err != nil {
		return err
	}
	// Trial generate the filter SQL. This will pick up any errors not found
	// at parse time.
	_, _, err = filterSchema.WhereClause(f, "", "trf_")
	if err != nil {
		return err
	}
	return nil
}

// WhereClause generates a WHERE clause for the given AIP-160 filter string.
func WhereClause(filter, tableAlias, parameterPrefix string) (string, []aip160.SqlQueryParameter, error) {
	f, err := aip160.ParseFilter(filter)
	if err != nil {
		return "", nil, err
	}
	return filterSchema.WhereClause(f, tableAlias, parameterPrefix)
}
