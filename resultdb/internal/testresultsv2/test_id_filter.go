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
	"fmt"

	"go.chromium.org/luci/common/data/aip160"

	"go.chromium.org/luci/resultdb/pbutil"
)

// testIDFieldBackend implements FieldBackend for the flat test ID fields stored component-wise as
// ModuleName, ModuleScheme, T1CoarseName, T2FineName, T3CaseName.
type testIDFieldBackend struct {
}

// RestrictionQuery implements FieldBackend.
func (s *testIDFieldBackend) RestrictionQuery(restriction aip160.RestrictionContext, g aip160.Generator) (string, error) {
	if len(restriction.NestedFields) > 0 {
		return "", aip160.FieldsUnsupportedError(restriction.FieldPath)
	}
	argValueUnsafe, err := aip160.CoerceArgToStringConstant(restriction.Arg)
	if err != nil {
		return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
	}
	switch restriction.Comparator {
	case "=", "!=":
		negated := restriction.Comparator == "!="
		testID, err := pbutil.ParseAndValidateTestID(argValueUnsafe)
		if err != nil {
			return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
		}
		result := fmt.Sprintf("(ModuleName = %s AND ModuleScheme = %s AND T1CoarseName = %s AND T2FineName = %s AND T3CaseName = %s)",
			g.BindString(testID.ModuleName), g.BindString(testID.ModuleScheme), g.BindString(testID.CoarseName), g.BindString(testID.FineName), g.BindString(testID.CaseName))
		if negated {
			result = fmt.Sprintf("(NOT %s)", result)
		}
		return result, nil
	default:
		return "", aip160.OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "TEST_ID")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (s *testIDFieldBackend) ImplicitRestrictionQuery(ir aip160.ImplicitRestrictionContext, g aip160.Generator) (string, error) {
	return "", aip160.ImplicitRestrictionUnsupportedError()
}

func (s *testIDFieldBackend) OrderBy(desc bool) (string, error) {
	return "", fmt.Errorf("flat test ID field do not support being ordered by")
}

func newFlatTestIDFieldBackend() *testIDFieldBackend {
	return &testIDFieldBackend{}
}
