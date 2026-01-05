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
	"strings"
	"unicode/utf8"

	"go.chromium.org/luci/common/data/aip160"

	"go.chromium.org/luci/resultdb/pbutil"
)

// regexpMetacharacters is the set of characters that have meaning (beyond
// the literal value) to the RE2 regular expression engine.
var regexpMetacharacters map[rune]struct{}

func init() {
	regexpMetacharacters = make(map[rune]struct{})
	for _, r := range `\.+*?()|[]{}^$` {
		regexpMetacharacters[r] = struct{}{}
	}
}

type StructuredTestIDColumnNames struct {
	// The database column name that stores the Module Name.
	ModuleName string
	// The database column name that stores the Module Scheme.
	ModuleScheme string
	// The database column name that stores the Coarse Name.
	CoarseName string
	// The database column name that stores the Fine Name.
	FineName string
	// The database column name that stores the Case Name.
	CaseName string
}

// TestIDFieldBackend implements FieldBackend for the flat test ID field, where the database
// stores the test ID in structured form (e.g. ModuleName, ModuleScheme, T1CoarseName, T2FineName,
// T3CaseName).
type TestIDFieldBackend struct {
	columns StructuredTestIDColumnNames
}

// RestrictionQuery implements FieldBackend.
func (s *TestIDFieldBackend) RestrictionQuery(restriction aip160.RestrictionContext, g aip160.Generator) (string, error) {
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
		result := fmt.Sprintf("(%s = %s AND %s = %s AND %s = %s AND %s = %s AND %s = %s)",
			g.ColumnReference(s.columns.ModuleName), g.BindString(testID.ModuleName),
			g.ColumnReference(s.columns.ModuleScheme), g.BindString(testID.ModuleScheme),
			g.ColumnReference(s.columns.CoarseName), g.BindString(testID.CoarseName),
			g.ColumnReference(s.columns.FineName), g.BindString(testID.FineName),
			g.ColumnReference(s.columns.CaseName), g.BindString(testID.CaseName))
		if negated {
			result = fmt.Sprintf("(NOT %s)", result)
		}
		return result, nil
	case ":":
		// Partial match.
		cond, err := testIDPartialMatchQuery(argValueUnsafe, s.columns, g)
		if err != nil {
			return "", fmt.Errorf("argument for field %q: %w", restriction.FieldPath.String(), err)
		}
		return cond, nil
	default:
		return "", aip160.OperatorNotImplementedError(restriction.Comparator, restriction.FieldPath, "TEST_ID")
	}
}

// ImplicitRestrictionQuery implements FieldBackend.
func (s *TestIDFieldBackend) ImplicitRestrictionQuery(ir aip160.ImplicitRestrictionContext, g aip160.Generator) (string, error) {
	return "", aip160.ImplicitRestrictionUnsupportedError()
}

func (s *TestIDFieldBackend) OrderBy(desc bool) (string, error) {
	return "", fmt.Errorf("flat test ID field do not support being ordered by")
}

func NewFlatTestIDFieldBackend(columnNames StructuredTestIDColumnNames) *TestIDFieldBackend {
	return &TestIDFieldBackend{
		columns: columnNames,
	}
}

// partialTestIDSegment represents a segment of a (partial) structured-form flat test ID.
//
// For example, `:module!myscheme:package:class#method:submethod1:somespecial\#\!\:\\text`
// will be parsed into the following series of segments:
// { Separator: `:`, Value: "module"}
// { Separator: `!`, Value: "myscheme"}
// { Separator: `:`, Value: "package"}
// { Separator: `:`, Value: "class"}
// { Separator: `#`, Value: "method"}
// { Separator: `:`, Value: "submethod1"}
// { Separator: `:`, Value: `somespecial#!:\text`} (note the escape sequences were decoded)
//
// Similarly, the partial test ID `sometext:` will be parsed as:
// { Separator: 0, Value: "sometext" }
// { Separator: `:`, Value: "" }
//
// Generally, one segment corresponds to one component of a structured test ID, although
// as illustrated by the second example, it will not always be clear which component it
// will match ("sometext:" could match the scheme, package, or part of the case name).
//
// The exception is the case name, which can correspond to multiple segments due to
// support for extended test ID hierarchy (multi-part case names).
// In the database and our protos, multi-part case names are stored in a flattened-form,
// For example, the parts [`method`, `submethod1`, `somespecial#!:\text`] are encoded as
// `method:submethod1:somepsecial#!\:\\text` in the case name field in the database.
// This requires special handling when matching. See pbutil.EncodeCaseName for details.
type partialTestIDSegment struct {
	// Leading separator of the segment.
	// This will be one of '!', ':', '#', or 0 (if segment starts with no separator).
	// 0 is only possible for the first segment in a sequence.
	Separator rune
	// The content of the segment.
	// Escaped '#', '!', ':', and '\' in the original string will have been decoded.
	Value string
	// Whether the segment ends with an unfinished escape sequence (unmatched trailing `\`).
	// This means that the character after `Value` will be one of '!', ':', '#', or '\',
	// but we don't know which.
	// This is only possible for the last segment in a sequence.
	UnfinishedEscapeSequence bool
}

// testIDPartialMatchQuery generates SQL that matches rows
// where `input` is a substring of the flat-form test ID.
//
// It considers that the flat-form test ID may be in one of two forms:
// structured form: :ModuleName!ModuleScheme:CoarseName:FineName#CaseName
// legacy form:     arbitrary-string-not-starting-with-colon
func testIDPartialMatchQuery(input string, fields StructuredTestIDColumnNames, g aip160.Generator) (string, error) {
	// Consider structured form.

	// Because we are implementing a substring match, we need to handle all ways
	// that a structured-form flat test ID of the form
	// :some_module!scheme:coarse_name:fine_name#case_name
	// could have been truncated to produce the `input` substring we need to match.
	//
	// The most tricky case is if the input filter has partial escape sequences
	// from the original test ID, as these do not map to a whole structured
	// test ID character. Fortunately, the only characters escaped when
	// formatting a structured test ID are :, \, ! and # (as \:, \\, \! and \#).
	//
	// We handle these as follows:
	//
	// Missing leading '\':
	// If a leading "\" was truncated, it can drastically affect the parsing,
	// as `\:` (escaped ':') differs significantly in interpretation
	// from `\\:` (escaped '\' followed by ':'). We consider both cases: missing
	// leading '\', and not. Often, only one form will parse as a valid, e.g.
	// "something" is valid where as "\something" is not as \s is not a valid
	// escape sequence.
	// If both forms are valid, we will create a predicate for both forms and
	// combine them with an OR.
	//
	// Incomplete trailing escape sequence (e.g. `something\`):
	// We could handle this case by asking the query layer to handle partial characters,
	// i.e. if a trailing escape sequence is found, look for [:\!#] in its place at
	// the structured test ID layer.
	// As it is not clear it is worth the complexity, we simply error out in this case
	// and ask the user to change the filter.

	var queryVariants []string

	segments, ok, err := parsePartialTestID(input)
	if err != nil {
		// The input is invalid.
		return "", err
	}
	if ok {
		// Input looks like a partial structured ID. Try to match.
		queryVariants = append(queryVariants, structuredTestIDPartialMatchQuery(segments, fields, g))
	}

	segments, ok, err = parsePartialTestID(`\` + input)
	if err != nil {
		// The input is invalid.
		return "", err
	}
	if ok {
		// Input looks like a partial structured ID. Try to match.
		queryVariants = append(queryVariants, structuredTestIDPartialMatchQuery(segments, fields, g))
	}

	// Consider the case where the argument is a substring of a legacy test ID.
	legacyCond := legacyTestIDPartialMatchQuery(input, fields, g)
	queryVariants = append(queryVariants, legacyCond)

	return fmt.Sprintf("(%s)", strings.Join(queryVariants, " OR ")), nil
}

// parsePartialTestID parses a partial test ID string into segments on the
// assumption it is a substring of a structured-form flat test ID, and parsing
// starts on a character boundary of the original structured test ID (i.e. not
// in the middle of an escape sequence).
//
// If this assumption is invalidated, returns ok = false. If the
// parsing fails because the test ID is never valid, returns an error.
func parsePartialTestID(input string) (segments []partialTestIDSegment, ok bool, err error) {
	var result []partialTestIDSegment
	currentSegment := partialTestIDSegment{}

	runes := []rune(input)
	n := len(runes)
	i := 0

	// If the string starts with a separator, the first segment has that separator.
	// If not, the first segment has Separator 0.
	if n > 0 {
		switch runes[0] {
		case ':', '!', '#':
			currentSegment.Separator = runes[0]
			i++
		}
	}

	var builder []rune

loop:
	for i < n {
		r := runes[i]
		if r == utf8.RuneError {
			return nil, false, fmt.Errorf("invalid UTF-8 rune at byte %v", i)
		}
		switch r {
		case '\\':
			// Escape sequence
			if i+1 >= n {
				// We have run out of input before the escape sequence finished.
				// Indicate the segment has an unfinished escape sequence. This means
				// the next character must be one of `:`, `!`, `#` or `\`.
				currentSegment.UnfinishedEscapeSequence = true
				break loop
			}
			next := runes[i+1]
			if next == ':' || next == '!' || next == '#' || next == '\\' {
				builder = append(builder, next)
				i += 2
			} else {
				// This is an invalid escape sequence for an encoded a structured test ID,
				// but the input could still be valid:
				// - as a legacy test ID.
				// - if we missed the leading '\' leading to us interpreting the
				//   escape sequences in the wrong way. E.g. input was `\a` but
				//   this appears in the full test ID as `\\a`.
				return nil, false, nil
			}
		case ':', '!', '#':
			// New segment
			currentSegment.Value = string(builder)
			result = append(result, currentSegment)
			currentSegment = partialTestIDSegment{Separator: r}
			builder = nil
			i++
		default:
			builder = append(builder, r)
			i++
		}
	}
	currentSegment.Value = string(builder)
	result = append(result, currentSegment)
	return result, true, nil
}

// sqlFactor represents a factor in a SQL query. Factors
// are combined with AND. Expressed here as a function
// to be lazily-evaluated when actually needed, to avoid
// binding parameters that are never used.
type sqlFactor func(g aip160.Generator) string

// sqlProduct represents a product of SQL factors. Products
// are combined with OR.
type sqlProduct struct {
	factors []sqlFactor
}

// testIDPartialMatchQuery returns a SQL query that matches the given partial
// structured-form flat test ID, which has been parsed into segments.
func structuredTestIDPartialMatchQuery(segments []partialTestIDSegment, colNames StructuredTestIDColumnNames, g aip160.Generator) string {
	if len(segments) == 0 {
		return "FALSE"
	}

	type column struct {
		ColumnName       string
		LeadingSeparator rune
	}
	columns := []column{
		{ColumnName: colNames.ModuleName, LeadingSeparator: ':'},
		{ColumnName: colNames.ModuleScheme, LeadingSeparator: '!'},
		{ColumnName: colNames.CoarseName, LeadingSeparator: ':'},
		{ColumnName: colNames.FineName, LeadingSeparator: ':'},
		{ColumnName: colNames.CaseName, LeadingSeparator: '#'},
	}

	// query variations are combined with an OR in the resulting query.
	var queryVariations []sqlProduct

	// Iterate over all the ways the partial test ID could match.
	// This loop tries to anchor the first segment to each column in turn (startColumn), and
	// then checks if the subsequent segments can validly match the subsequent columns.
	for startColumn := 0; startColumn < len(columns); startColumn++ {
		valid := true

		var factors []sqlFactor
		currentColumn := startColumn
		for segIdx := 0; segIdx < len(segments); segIdx++ {
			seg := segments[segIdx]
			fieldName := columns[currentColumn].ColumnName

			if seg.Separator == 0 && segIdx != 0 {
				panic("logic error: only the first segments may be missing a leading separator")
			}
			if seg.UnfinishedEscapeSequence && segIdx != len(segments)-1 {
				panic("logic error: unfinished escape sequence may only appear on the last segment")
			}

			var factor sqlFactor
			// Is this the case name column?
			if currentColumn == len(columns)-1 {
				// Yes. Check the grammar matches.
				// - '#': indicates a match that must start from the start of the case name
				// - ':': is only allowed for the first segment, and is possible if the test ID
				//        uses the support for extended test ID hierarchies within the case name
				//        (case name of form "sub_hierarchy:sub_hierarchy:test_name")
				// - 0: this is the first segment and it has no leading grammar
				if !(seg.Separator == '#' || (segIdx == 0 && seg.Separator == ':') || seg.Separator == 0) {
					valid = false
					break
				}

				isStartAnchored := seg.Separator == '#'
				// Case name is never end-anchored.
				// Multiple segments can match against the case name due to
				// extended depth test hierarchy support in the case name.
				var ok bool
				factor, ok = matchCaseName(isStartAnchored, fieldName, segments[segIdx:])
				if !ok {
					valid = false
					break
				}
			} else {
				// Check the grammar matches.
				expectedSep := columns[currentColumn].LeadingSeparator
				if !(seg.Separator == expectedSep || seg.Separator == 0) {
					valid = false
					break
				}

				// Anchored by grammar if we matched the separator.
				isStartAnchored := seg.Separator != 0
				// End is anchored if there are more segments to match.
				isEndAnchored := segIdx != len(segments)-1
				factor = matchPart(isStartAnchored, isEndAnchored, fieldName, seg)
			}
			// Factors are joined with AND.
			factors = append(factors, factor)

			currentColumn++
			if currentColumn >= len(columns) {
				break
			}
		}

		if valid && len(factors) > 0 {
			// We have found another way the partial test ID could match. Include
			// it in the resulting query. Query variations are combined with 'OR'.
			queryVariations = append(queryVariations, sqlProduct{factors: factors})
		}
	}

	if len(queryVariations) == 0 {
		return "FALSE"
	}
	// A native legacy test ID can never be encoded as a structured test ID, so add
	// the condition we can never match on the ModuleName of "legacy".
	return fmt.Sprintf(`(%s AND %s <> "legacy")`, generateProductsSQL(queryVariations, g), g.ColumnReference(colNames.ModuleName))
}

func generateProductsSQL(queryVariations []sqlProduct, g aip160.Generator) string {
	if len(queryVariations) == 0 {
		return "FALSE"
	}
	if len(queryVariations) == 1 {
		return generateProductSQL(queryVariations[0], g)
	}
	var sb strings.Builder
	sb.WriteString("(")
	for i, product := range queryVariations {
		if i > 0 {
			sb.WriteString(" OR ")
		}
		sb.WriteString(generateProductSQL(product, g))
	}
	sb.WriteString(")")
	return sb.String()
}

func generateProductSQL(product sqlProduct, g aip160.Generator) string {
	if len(product.factors) == 1 {
		return product.factors[0](g)
	}
	var sb strings.Builder
	sb.WriteString("(")
	for i, factor := range product.factors {
		if i > 0 {
			sb.WriteString(" AND ")
		}
		sb.WriteString(factor(g))
	}
	sb.WriteString(")")
	return sb.String()
}

// matchPart returns a SQL term that matches the given segment's value (and unfinished
// escape sequence, if any) in the given database column. The segment separator is not matched.
// isStartAnchored indicates whether the segment must match starting at the beginning of fieldName.
// isEndAnchorsed indicates whether the segment must match ending at the end of fieldName.
func matchPart(isStartAnchored, isEndAnchorsed bool, columnName string, segment partialTestIDSegment) sqlFactor {
	// Defer parameter binding to avoid binding parameters unless this factor
	// actually ends up used in the final query.
	return func(g aip160.Generator) string {
		if segment.UnfinishedEscapeSequence {
			if isEndAnchorsed {
				panic("logic error: unfinished escape sequence may only appear on the last segment, which are never end-anchored")
			}
			// The segment to be matched contains partial character information, use slower
			// regexp-based matching path. The partial character information is equivalent to
			// extension with one of `:`, `!`, `#` or `\`.
			re2exp := matchValueRegexp(isStartAnchored, segment.Value) + `[:!#\\]`
			return fmt.Sprintf("REGEXP_CONTAINS(%s, %s)", g.ColumnReference(columnName), g.BindString(re2exp))
		}

		if isStartAnchored {
			if isEndAnchorsed {
				// Exact match.
				return fmt.Sprintf("%s = %s", g.ColumnReference(columnName), g.BindString(segment.Value))
			} else {
				// Prefix match. Prefer STARTS_WITH as this is typically faster than LIKE.
				return fmt.Sprintf("STARTS_WITH(%s, %s)", g.ColumnReference(columnName), g.BindString(segment.Value))
			}
		} else {
			if isEndAnchorsed {
				// Suffix match.
				return fmt.Sprintf("ENDS_WITH(%s, %s)", g.ColumnReference(columnName), g.BindString(segment.Value))
			} else {
				// Contains match.
				return fmt.Sprintf("%s LIKE %s", g.ColumnReference(columnName), g.BindString("%"+aip160.QuoteLike(segment.Value)+"%"))
			}
		}
	}
}

// matchCaseName returns a SQL term that matches the given segment's value (and unfinished
// escape sequence, if any) in the given case name column.
//
// This differs from `matchPart` because the case name column can store multiple segments
// as extended hierarchy, i.e. `:part1:part2`. Within parts, `:` and `\` inside hierarchy
// parts are stored escaped with a `\`.
func matchCaseName(isStartAnchored bool, columnName string, segments []partialTestIDSegment) (result sqlFactor, ok bool) {
	// Flatten any remaining segments to a single value, checking they are separated
	// with ':' (as would be expected for extended test hierarchy).
	flattenedSegment, ok := reEncodeCaseNameParts(segments)
	if !ok {
		return nil, false
	}

	// Defer parameter binding to avoid binding parameters unless this factor
	// actually ends up used in the final query.
	return func(g aip160.Generator) string {
		if flattenedSegment.UnfinishedEscapeSequence {
			// The segment to be matched contains partial character information; use slower
			// regexp-based matching path. The partial character information is equivalent to
			// extension with one of `#`, `!`, `\\` or `\:`. Note that : and \ are escaped with
			// a `\` when they appear in the test case name as a case name segment.
			re2exp := matchValueRegexp(isStartAnchored, flattenedSegment.Value) + `([#!]|\\[:\\])`
			return fmt.Sprintf("REGEXP_CONTAINS(%s, %s)", g.ColumnReference(columnName), g.BindString(re2exp))
		}

		if isStartAnchored {
			// Prefix match. Prefer STARTS_WITH as this is typically faster than LIKE.
			return fmt.Sprintf("STARTS_WITH(%s, %s)", g.ColumnReference(columnName), g.BindString(flattenedSegment.Value))
		} else {
			// Contains match.
			return fmt.Sprintf("%s LIKE %s", g.ColumnReference(columnName), g.BindString("%"+aip160.QuoteLike(flattenedSegment.Value)+"%"))
		}
	}, true
}

// matchValueRegexp creates RE2 regular expression pattern that matches
// text which contains the given value as a substring.
//
// It is designed for use with Spanner and BigQuery's REGEXP_CONTAINS:
// - https://docs.cloud.google.com/spanner/docs/reference/standard-sql/string_functions#regexp_contains
// - https://docs.cloud.google.com/bigquery/docs/reference/standard-sql/string_functions#regexp_contains
func matchValueRegexp(isStartAnchored bool, value string) string {
	var b strings.Builder
	if isStartAnchored {
		// Match start of string.
		b.WriteString("^")
	}
	for _, r := range value {
		// Match the literal character.
		if _, ok := regexpMetacharacters[r]; ok {
			// Escape regex metacharacters with a '\'.
			b.WriteRune('\\')
			b.WriteRune(r)
		} else {
			b.WriteRune(r)
		}
	}
	// Never anchor to end of string.
	return b.String()
}

// reEncodeCaseNameParts re-encodes the given test ID segments as
// a partial case name.
// Case names can encode extended-depth test hierarchy, e.g.
// "part_1:part_2:part_3".
// Our parser will break these into multiple segments, so we need
// to reassemble them for matching.
func reEncodeCaseNameParts(segments []partialTestIDSegment) (segment partialTestIDSegment, ok bool) {
	// Check all separators from startIndex+1 are ':'
	for i := 1; i < len(segments); i++ {
		if segments[i].Separator != ':' {
			// Not a valid as a case name.
			return partialTestIDSegment{}, false
		}
	}
	var parts []string
	for i := 0; i < len(segments); i++ {
		parts = append(parts, segments[i].Value)
	}
	var result string
	// Include any leading ':' as it is part of the case name and not
	// a top-level grammar.
	if segments[0].Separator == ':' {
		result = ":"
	}
	result += pbutil.EncodeCaseName(parts...)
	return partialTestIDSegment{
		Separator:                segments[0].Separator,
		Value:                    result,
		UnfinishedEscapeSequence: segments[len(segments)-1].UnfinishedEscapeSequence,
	}, true
}

func legacyTestIDPartialMatchQuery(input string, columns StructuredTestIDColumnNames, g aip160.Generator) string {
	return fmt.Sprintf(`(%s = "legacy" AND %s = "legacy" AND %s = "" AND %s = "" AND %s LIKE %s)`,
		g.ColumnReference(columns.ModuleName),
		g.ColumnReference(columns.ModuleScheme),
		g.ColumnReference(columns.CoarseName),
		g.ColumnReference(columns.FineName),
		g.ColumnReference(columns.CaseName),
		g.BindString("%"+aip160.QuoteLike(input)+"%"))
}
