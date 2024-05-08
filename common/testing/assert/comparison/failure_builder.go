// Copyright 2024 The LUCI Authors.
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

package comparison

import (
	"fmt"
	"strings"
)

// A FailureBuilder builds a Failure and has a fluent interface.
type FailureBuilder struct {
	*Failure
}

// SetFailureComparison sets the Comparison field of `Failure`, formating
// `typeArgs` into strings with `fmt.Sprintf("%T")`.
//
// Example:
//
//	actual := 123
//	SetFailureComparison(f, "should.Equal", &actual)
//
// Will FailureBuilder for a comparison `should.Equal[*int]`.
func SetFailureComparison(f *Failure, comparisonName string, typeArgs ...any) {
	var typeArgsS []string
	if len(typeArgs) > 0 {
		typeArgsS = make([]string, len(typeArgs))
		for i, arg := range typeArgs {
			typeArgsS[i] = fmt.Sprintf("%T", arg)
		}
	}

	f.Comparison = &Failure_ComparisonFunc{
		Name:          comparisonName,
		TypeArguments: typeArgsS,
	}
}

// NewFailureBuilder makes a new FailureBuilder, filling in the  for the given
// comparisonName and exemplar type arguments.
//
// For example:
//
//	actual := 123
//	NewFailureBuilder("should.Equal", &actual)
//
// Will make a new FailureBuilder for a comparison `should.Equal[*int]`.
func NewFailureBuilder(comparisonName string, typeArgs ...any) *FailureBuilder {
	ret := &FailureBuilder{&Failure{}}
	SetFailureComparison(ret.Failure, comparisonName, typeArgs...)
	return ret
}

// AddFindingf adds a new single-line Finding to this Failure with the
// given `name`.
//
// If `args` is empty, then `format` will be used as the Finding value verbatim.
// Otherwise the value will be formatted as `fmt.Sprintf(format, args...)`.
//
// The finding will have the type "FindingTypeHint_Text".
func (fb *FailureBuilder) AddFindingf(name, format string, args ...any) *FailureBuilder {
	fb.fixNilFailure()

	value := format
	if len(args) > 0 {
		value = fmt.Sprintf(format, args...)
	}
	fb.Findings = append(fb.Findings, &Failure_Finding{
		Name:  name,
		Value: strings.Split(value, "\n"),
		Type:  FindingTypeHint_Text,
	})
	return fb
}

// fixNilFailure will populate fb.Failure with an empty Failure iff it is nil.
func (fb *FailureBuilder) fixNilFailure() {
	if fb.Failure == nil {
		fb.Failure = &Failure{}
	}
}

// Because adds a new finding "Because" to the Failure with AddFormattedFinding.
func (fb *FailureBuilder) Because(format string, args ...interface{}) *FailureBuilder {
	return fb.AddFindingf("Because", format, args...)
}

// Actual adds a new finding "Actual" to the Failure.
//
// `actual` will be rendered with fmt.Sprintf("%#v").
func (fb *FailureBuilder) Actual(actual any) *FailureBuilder {
	return fb.AddFindingf("Actual", "%#v", actual)
}

// Expected adds a new finding "Expected" to the Failure.
//
// `Expected` will be rendered with fmt.Sprintf("%#v").
func (fb *FailureBuilder) Expected(Expected any) *FailureBuilder {
	return fb.AddFindingf("Expected", "%#v", Expected)
}

// Marks the previously-added Finding with Level 'Warn' if it has a long value.
//
// A long value is defined as:
//   - More than one line OR
//   - A line exceeding 30 characters in length.
//
// No-op if there are no findings in the failure yet.
func (fb *FailureBuilder) WarnIfLong() *FailureBuilder {
	const lengthThreshold = 30

	fb.fixNilFailure()
	if len(fb.Findings) > 0 {
		f := fb.Findings[len(fb.Findings)-1]
		if len(f.Value) > 1 || len(f.Value[0]) > lengthThreshold {
			f.Level = FindingLogLevel_Warn
		}
	}
	return fb
}
