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

package testaggregations

import (
	"go.chromium.org/luci/common/data/aip160"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var moduleStatusEnumDef = aip160.NewEnumDefinition("pb.TestAggregation.ModuleStatus", pb.TestAggregation_ModuleStatus_value, int32(pb.TestAggregation_MODULE_STATUS_UNSPECIFIED))

var filterSchema = aip160.NewDatabaseTable().WithFields(
	aip160.NewField().WithFieldPath("verdict_counts", "failed").WithBackend(aip160.NewIntegerColumn("TestsFailed")).Filterable().Build(),
	aip160.NewField().WithFieldPath("verdict_counts", "flaky").WithBackend(aip160.NewIntegerColumn("TestsFlaky")).Filterable().Build(),
	aip160.NewField().WithFieldPath("verdict_counts", "passed").WithBackend(aip160.NewIntegerColumn("TestsPassed")).Filterable().Build(),
	aip160.NewField().WithFieldPath("verdict_counts", "skipped").WithBackend(aip160.NewIntegerColumn("TestsSkipped")).Filterable().Build(),
	aip160.NewField().WithFieldPath("verdict_counts", "execution_errored").WithBackend(aip160.NewIntegerColumn("TestsExecutionErrored")).Filterable().Build(),
	aip160.NewField().WithFieldPath("verdict_counts", "precluded").WithBackend(aip160.NewIntegerColumn("TestsPrecluded")).Filterable().Build(),
	aip160.NewField().WithFieldPath("verdict_counts", "exonerated").WithBackend(aip160.NewIntegerColumn("TestsExonerated")).Filterable().Build(),
	aip160.NewField().WithFieldPath("module_status").WithBackend(aip160.NewEnumColumn("ModuleStatus").WithDefinition(moduleStatusEnumDef).Build()).Filterable().Build(),
).Build()

// ValidateFilter validates an AIP-160 filter string.
func ValidateFilter(filter string) error {
	f, err := aip160.ParseFilter(filter)
	if err != nil {
		return err
	}
	// Trial generate the filter SQL. This will pick up any errors not found
	// at parse time.
	_, _, err = filterSchema.WhereClause(f, "", "f_")
	if err != nil {
		return err
	}
	return nil
}

// whereClause generates a WHERE clause for the given AIP-160 filter string.
func whereClause(filter, tableAlias, parameterPrefix string) (string, []aip160.SqlQueryParameter, error) {
	f, err := aip160.ParseFilter(filter)
	if err != nil {
		return "", nil, err
	}
	return filterSchema.WhereClause(f, tableAlias, parameterPrefix)
}
