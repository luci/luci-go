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
)

// Defines the fields that modules can be filtered on, in the context of test aggregations.
var moduleFilterSchema = aip160.NewDatabaseTable().WithFields(
	aip160.NewField().WithFieldPath("test_id_structured", "module_name").WithBackend(aip160.NewStringColumn("ModuleName")).FilterableImplicitly().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "module_scheme").WithBackend(aip160.NewStringColumn("ModuleScheme")).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "module_variant").WithBackend(aip160.NewKeyValueColumn("ModuleVariantMasked").WithStringArray().Build()).Filterable().Build(),
	aip160.NewField().WithFieldPath("test_id_structured", "module_variant_hash").WithBackend(aip160.NewStringColumn("ModuleVariantHash")).Filterable().Build(),
).Build()

// moduleWhereClause generates a WHERE clause for the given module AIP-160 filter string.
func moduleWhereClause(filter, tableAlias, parameterPrefix string) (string, []aip160.SqlQueryParameter, error) {
	f, err := aip160.ParseFilter(filter)
	if err != nil {
		return "", nil, err
	}
	return moduleFilterSchema.WhereClause(f, tableAlias, parameterPrefix)
}
