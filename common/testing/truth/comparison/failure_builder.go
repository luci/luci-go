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

	"go.chromium.org/luci/common/testing/truth/failure"
)

// A SummaryBuilder builds a failure.Summary and has a fluent interface.
type SummaryBuilder struct {
	*failure.Summary
}

// SetComparison sets the Comparison field of `failure.Summary`, formating
// `typeArgs` into strings with `fmt.Sprintf("%T")`.
//
// Example:
//
//	actual := 123
//	SetComparison(f, "should.Equal", &actual)
//
// Will SummaryBuilder for a comparison `should.Equal[*int]`.
func SetComparison(f *failure.Summary, comparisonName string, typeArgs ...any) {
	var typeArgsS []string
	if len(typeArgs) > 0 {
		typeArgsS = make([]string, len(typeArgs))
		for i, arg := range typeArgs {
			typeArgsS[i] = fmt.Sprintf("%T", arg)
		}
	}

	f.Comparison = &failure.Comparison{
		Name:          comparisonName,
		TypeArguments: typeArgsS,
	}
}

// NewSummaryBuilder makes a new SummaryBuilder, filling in the  for the given
// comparisonName and exemplar type arguments.
//
// For example:
//
//	actual := 123
//	NewSummaryBuilder("should.Equal", &actual)
//
// Will make a new SummaryBuilder for a comparison `should.Equal[*int]`.
func NewSummaryBuilder(comparisonName string, typeArgs ...any) *SummaryBuilder {
	ret := &SummaryBuilder{&failure.Summary{}}
	SetComparison(ret.Summary, comparisonName, typeArgs...)
	return ret
}

// AddComparisonArgs adds new arguments to the failure.Summary.ComparisonFunc formatted
// with %v.
func (sb *SummaryBuilder) AddComparisonArgs(args ...any) *SummaryBuilder {
	sb.fixNilFailure()
	slc := make([]string, len(args))
	for i := range slc {
		slc[i] = fmt.Sprintf("%v", args[i])
	}
	sb.Comparison.Arguments = append(sb.Comparison.Arguments, slc...)
	return sb
}

// AddFindingf adds a new single-line Finding to this failure.Summary with the
// given `name`.
//
// If `args` is empty, then `format` will be used as the Finding value verbatim.
// Otherwise the value will be formatted as `fmt.Sprintf(format, args...)`.
//
// The finding will have the type "FindingTypeHint_Text".
func (sb *SummaryBuilder) AddFindingf(name, format string, args ...any) *SummaryBuilder {
	sb.fixNilFailure()

	value := format
	if len(args) > 0 {
		value = fmt.Sprintf(format, args...)
	}
	sb.Findings = append(sb.Findings, &failure.Finding{
		Name:  name,
		Value: strings.Split(value, "\n"),
		Type:  failure.FindingTypeHint_Text,
	})
	return sb
}

// fixNilFailure will populate sb.Summary with an empty failure.Summary iff it is nil.
func (sb *SummaryBuilder) fixNilFailure() {
	if sb.Summary == nil {
		sb.Summary = &failure.Summary{}
	}
}

// Because adds a new finding "Because" to the failure.Summary with AddFormattedFinding.
func (sb *SummaryBuilder) Because(format string, args ...any) *SummaryBuilder {
	return sb.AddFindingf("Because", format, args...)
}

// Actual adds a new finding "Actual" to the failure.Summary.
//
// `actual` will be rendered with fmt.Sprintf("%#v").
func (sb *SummaryBuilder) Actual(actual any) *SummaryBuilder {
	return sb.AddFindingf("Actual", "%#v", actual)
}

// Expected adds a new finding "Expected" to the failure.Summary.
//
// `Expected` will be rendered with fmt.Sprintf("%#v").
func (sb *SummaryBuilder) Expected(Expected any) *SummaryBuilder {
	return sb.AddFindingf("Expected", "%#v", Expected)
}

// WarnIfLong marks the previously-added Finding with Level 'Warn' if it has a long value.
//
// A long value is defined as:
//   - More than one line OR
//   - A line exceeding 30 characters in length.
//
// No-op if there are no findings in the failure yet.
func (sb *SummaryBuilder) WarnIfLong() *SummaryBuilder {
	const lengthThreshold = 30

	sb.fixNilFailure()
	if len(sb.Findings) > 0 {
		f := sb.Findings[len(sb.Findings)-1]
		if len(f.Value) > 1 || len(f.Value[0]) > lengthThreshold {
			f.Level = failure.FindingLogLevel_Warn
		}
	}
	return sb
}

// GetFailure returns sb.Summary if it contains any Findings, otherwise returns
// nil.
//
// This is useful if you build your comparison with a series of conditional
// findings.
func (sb *SummaryBuilder) GetFailure() *failure.Summary {
	if sb.Summary == nil || len(sb.Findings) == 0 {
		return nil
	}
	return sb.Summary
}

// RenameFinding finds the first Finding with the name `oldname` and renames it
// to `newname`.
//
// Does nothing if `oldname` is not one of the current Findings.
func (sb *SummaryBuilder) RenameFinding(oldname, newname string) *SummaryBuilder {
	sb.fixNilFailure()

	for _, finding := range sb.Findings {
		if finding.Name == oldname {
			finding.Name = newname
		}
	}

	return sb
}
