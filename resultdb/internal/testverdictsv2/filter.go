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

package testverdictsv2

import (
	"go.chromium.org/luci/common/data/aip160"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

var verdictStatusEnumDef = aip160.NewEnumDefinition("pb.TestVerdict.Status", pb.TestVerdict_Status_value, int32(pb.TestVerdict_STATUS_UNSPECIFIED))
var verdictStatusOverrideEnumDef = aip160.NewEnumDefinition("pb.TestVerdict.StatusOverride", pb.TestVerdict_StatusOverride_value, int32(pb.TestVerdict_STATUS_OVERRIDE_UNSPECIFIED))

// Defines the TestVerdict message fields that are filterable. This set is fairly
// limited owing to the fact that other fields (e.g. test ID, test metadata) can be
// filtered using the test result filter support.
var filterSchema = aip160.NewDatabaseTable().WithFields(
	aip160.NewField().WithFieldPath("status").WithBackend(aip160.NewEnumColumn("Status").WithDefinition(verdictStatusEnumDef).Build()).Filterable().Build(),
	aip160.NewField().WithFieldPath("status_override").WithBackend(aip160.NewEnumColumn("StatusOverride").WithDefinition(verdictStatusOverrideEnumDef).Build()).Filterable().Build(),
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
