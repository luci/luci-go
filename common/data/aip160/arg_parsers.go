// Copyright 2022 The LUCI Authors.
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

package aip160

import (
	"fmt"
)

// coerceComparableToImplicitFilter attempts to return the implicit filter
// string from an arg.
func coerceComparableToImplicitFilter(cmp *Comparable) (string, error) {
	if len(cmp.Member.Fields) > 0 {
		return "", fmt.Errorf("fields are not allowed without an operator, try wrapping %s in double quotes: %q", cmp.Member.Input(), cmp.Member.Input())
	}
	// Implicit filters can be quoted or unquoted.
	return cmp.Member.Value.Value, nil
}

func coerceArgToComparable(arg *Arg) (*Comparable, error) {
	if arg.Composite != nil {
		return nil, fmt.Errorf("composite expressions in arguments not supported yet")
	}
	if arg.Comparable == nil {
		return nil, fmt.Errorf("missing comparable in argument")
	}
	return arg.Comparable, nil
}

// CoerceArgToBoolConstant attempts to extract a boolean constant from
// an Arg AST node. The AST node must be the unquoted `true` or `false`
// literal.
func CoerceArgToBoolConstant(arg *Arg) (bool, error) {
	cmp, err := coerceArgToComparable(arg)
	if err != nil {
		return false, err
	}
	if cmp.Member == nil {
		return false, fmt.Errorf("invalid comparable")
	}
	// As per go/ccfe-aip-160#literals, we expect booleans to be unquoted.
	if cmp.Member.Value.Quoted {
		return false, fmt.Errorf("expected the unquoted literal 'true' or 'false' but found double-quoted string %q", cmp.Member.Value.Value)
	}
	if len(cmp.Member.Fields) > 0 {
		return false, fmt.Errorf("field navigation (using '.') is not supported")
	}
	// Literals are case-sensitive.
	if cmp.Member.Value.Value == "true" {
		return true, nil
	} else if cmp.Member.Value.Value == "false" {
		return false, nil
	}
	return false, fmt.Errorf("expected the unquoted literal 'true' or 'false' (case-sensitive) but found %q", cmp.Member.Value.Value)
}

// CoerceArgToStringConstant attempts to extract a string constant from
// an Arg AST node. The AST node must be the double-quoted string value.
func CoerceArgToStringConstant(arg *Arg) (unsafe string, err error) {
	cmp, err := coerceArgToComparable(arg)
	if err != nil {
		return "", err
	}
	if cmp.Member == nil {
		return "", fmt.Errorf("invalid comparable")
	}
	// As per go/ccfe-aip-160#literals, we expect strings to be double-quoted.
	if !cmp.Member.Value.Quoted {
		return "", fmt.Errorf("expected a quoted (\"\") string literal but got possible field reference %q, did you mean to wrap the value in quotes?", cmp.Member.Input())
	}
	if len(cmp.Member.Fields) > 0 {
		return "", fmt.Errorf("field navigation (using '.') is not supported")
	}
	return cmp.Member.Value.Value, nil
}
