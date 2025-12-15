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
	"regexp"
	"slices"
	"strings"
	"time"
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

// CoerceArgToDurationConstant attempts to extract a duration constant from
// an Arg AST node. The AST node must be the unquoted duration value, like
// `1.2s`.
func CoerceArgToDurationConstant(arg *Arg) (time.Duration, error) {
	comparable, err := coerceArgToComparable(arg)
	if err != nil {
		return 0, err
	}
	if comparable.Member == nil {
		return 0, fmt.Errorf("invalid comparable")
	}
	// As per go/ccfe-aip-160#literals, durations should be unquoted.
	if comparable.Member.Value.Quoted {
		return 0, fmt.Errorf(`durations must be an unquoted number with 's' suffix like 1.2s but got a quoted string %q`, comparable.Member.Value.Value)
	}
	input := comparable.Member.Value.Value
	if len(comparable.Member.Fields) > 0 {
		if len(comparable.Member.Fields) > 1 || comparable.Member.Fields[0].Quoted {
			return 0, fmt.Errorf(`expected a duration like 1.2s, but got %q`, comparable.Member.Input())
		}
		input += "." + comparable.Member.Fields[0].Value
	}
	return parseDuration(input)
}

var durationRE = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,9})?s$`)

func parseDuration(value string) (time.Duration, error) {
	// Parse duration.
	if !durationRE.MatchString(value) {
		return 0, fmt.Errorf("%q is not a valid duration, expected a number followed by 's' (e.g. \"20.1s\"), allowed pattern %q", value, durationRE)
	}
	// Golang durations support a superset of the allowed AIP-160 duration syntax.
	// Moreover, Golang durations are represented as INT64 nanoseconds which gives
	// the required nanosecond precision of go/ccfe-aip-160#literals.
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%q is not a valid duration", value)
	}
	if duration < 0 {
		return 0, fmt.Errorf("%q is not a valid duration, duration cannot be negative", value)
	}
	return duration, nil
}

type EnumDefinition struct {
	// The name of the enum type represented by this column.
	typeName string

	// The enum values. This is usually the enum's _name map.
	values map[string]int32

	// The values from `values` that are disallowed. E.g. the unspecified value.
	// Use of disallowed values results in slightly different error than using
	// a value that is not in `values`.
	disallowedValues map[int32]struct{}
}

// NewEnumDefinition defines a new enumeration.
//
// The `values` map is usually the enum's _name map.
// `disallowedValues` often includes the enum's UNSPECIFIED value and any
// other values that are not allowed on the underlying record.
func NewEnumDefinition(typeName string, values map[string]int32, disallowedValues ...int32) *EnumDefinition {
	def := &EnumDefinition{
		typeName:         typeName,
		values:           values,
		disallowedValues: map[int32]struct{}{},
	}
	for _, v := range disallowedValues {
		def.disallowedValues[v] = struct{}{}
	}
	return def
}

// allowedValues returns a string representation of the allowed values for this
// enum definition.
func (e *EnumDefinition) allowedValues() string {
	var values []string
	for k, v := range e.values {
		if _, ok := e.disallowedValues[v]; ok {
			continue
		}
		values = append(values, k)
	}
	slices.SortFunc(values, func(a, b string) int {
		if e.values[a] == e.values[b] {
			// Normally enums do not have two values which map to the same value, but
			// if they do, compare by name.
			return strings.Compare(a, b)
		}
		if e.values[a] > e.values[b] {
			return 1
		}
		return -1
	})
	return strings.Join(values, ", ")
}

// CoarceArgToEnumConstant attempts to extract an enum constant from
// an Arg AST node. The AST node must be the unquoted enum value.
func CoarceArgToEnumConstant(arg *Arg, def *EnumDefinition) (int32, error) {
	cmp, err := coerceArgToComparable(arg)
	if err != nil {
		return 0, err
	}
	if cmp.Member == nil {
		return 0, fmt.Errorf("invalid comparable")
	}
	// As per go/ccfe-aip-160#literals, we expect emum values to be unquoted.
	if cmp.Member.Value.Quoted {
		return 0, fmt.Errorf("expected an unquoted enum value but found double-quoted string %q", cmp.Member.Value.Value)
	}
	if len(cmp.Member.Fields) > 0 {
		return 0, fmt.Errorf("field navigation (using '.') is not supported")
	}
	valueToParse := cmp.Member.Value.Value
	argValue, ok := def.values[valueToParse]
	if !ok {
		return 0, fmt.Errorf("%q is not one of the valid enum values, expected one of [%s]", valueToParse, def.allowedValues())
	}
	if _, ok := def.disallowedValues[argValue]; ok {
		return 0, fmt.Errorf("%q is not allowed for this enum, expected one of [%s]", valueToParse, def.allowedValues())
	}
	return argValue, nil
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
