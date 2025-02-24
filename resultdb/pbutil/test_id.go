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

package pbutil

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// schemeRE defines the syntax of valid test scheme IDs. Does not capture length limit.
var schemeRE = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// FixtureCaseName is a special value, that when used as the case name in a test identifier,
// denotes that the test covers the result of common setup/teardown logic for a fine name.
const FixtureCaseName = "*fixture"

// ValidateTestID returns a non-nil error if testID is invalid.
func ValidateTestID(testID string) error {
	_, err := ParseAndValidateTestID(testID)
	return err
}

// TestIdentifier represents a structured test identifier (without
// variant information).
type TestIdentifier struct {
	ModuleName   string
	ModuleScheme string
	CoarseName   string
	FineName     string
	CaseName     string
}

// ParseAndValidateTestID parses the given flat-form test ID into a
// structured test ID and ensures it is valid.
//
// If the testID starts with a colon, it is assumed to be a structured test
// identifier, and is parsed as follows:
//
//	:module_name!module_scheme:coarse_name:fine_name#case_name
//
// Otherwise it is treated as a legacy test identifier.
//
// See TestVariantIdentifier in common.proto for more details about
// structured test identifiers.
func ParseAndValidateTestID(testID string) (TestIdentifier, error) {
	// Perform some basic validation before we try to parse the ID.
	if testID == "" {
		return TestIdentifier{}, validate.Unspecified()
	}
	if err := validateUTF8Printable(testID, 512); err != nil {
		return TestIdentifier{}, err
	}

	id, err := parseTestIdentifier(testID)
	if err != nil {
		return TestIdentifier{}, err
	}
	err = validateTestIdentifier(id)
	return id, err
}

// parseTestIdentifier parses a test identifier from its flat-form string
// representation into its structured representation.
func parseTestIdentifier(testID string) (TestIdentifier, error) {
	if !strings.HasPrefix(testID, ":") {
		// This is a legacy test ID.
		return TestIdentifier{
			ModuleName:   "legacy",
			ModuleScheme: "legacy",
			CaseName:     testID,
		}, nil
	}

	// This is a structured test ID.
	// We expect the next character to be the module name.
	// We will read until the next '!' character.
	moduleName, nextIndex, err := readEscapedComponent(testID, 1, '!')
	if err != nil {
		return TestIdentifier{}, err
	}
	moduleScheme, nextIndex, err := readEscapedComponent(testID, nextIndex+1, ':')
	if err != nil {
		return TestIdentifier{}, err
	}
	coarseName, nextIndex, err := readEscapedComponent(testID, nextIndex+1, ':')
	if err != nil {
		return TestIdentifier{}, err
	}
	fineName, nextIndex, err := readEscapedComponent(testID, nextIndex+1, '#')
	if err != nil {
		return TestIdentifier{}, err
	}
	caseName, _, err := readEscapedComponent(testID, nextIndex+1, 0)
	if err != nil {
		return TestIdentifier{}, err
	}
	return TestIdentifier{
		ModuleName:   moduleName,
		ModuleScheme: moduleScheme,
		CoarseName:   coarseName,
		FineName:     fineName,
		CaseName:     caseName,
	}, nil
}

// readEscapedComponent reads a test ID component from the provided
// `testID` input starting at `startIndex`. It reads until the given
// terminator is found, which must be one of ':', '#', '!' or U+0000
// (which denotes end of string).
//
// The read component (excluding terminator) is returned, and the
// advanced index (up to the position of the terminator) is returned
// as nextIndex.
//
// When reading, it decodes the escape sequences \: \# \! and \\.
func readEscapedComponent(testID string, startIndex int, terminator rune) (component string, nextIndex int, err error) {
	if terminator != ':' && terminator != '#' && terminator != '!' && terminator != 0 {
		panic("unsupported terminator")
	}

	var builder strings.Builder
	inEscapeSequence := false

	var r rune
	var width int

	copyFromIndex := startIndex
	i := startIndex
	for ; i < len(testID); i += width {
		r, width = utf8.DecodeRuneInString(testID[i:])
		if r == utf8.RuneError {
			return "", -1, errors.Reason(`invalid UTF-8 rune at byte %v`, i).Err()
		}
		if inEscapeSequence {
			if r != ':' && r != '\\' && r != '#' && r != '!' {
				// Invalid escape sequence.
				return "", -1, errors.Reason(`got unexpected character %+q at byte %v, while processing escape sequence (\); only the characters :!#\ may be escaped`, r, i).Err()
			}
			// Exit the escape sequence.
			inEscapeSequence = false
			continue
		}
		switch r {
		case '\\':
			// Begin escape sequence.
			inEscapeSequence = true
			// Write out whatever we have up until now.
			builder.WriteString(testID[copyFromIndex:i])
			// The character following the '\' is included verbatim unless it
			// is invalid.
			// Resume copying from the character after this '\'.
			copyFromIndex = i + width
		case ':', '#', '!':
			// We found a terminator (other than end of string).
			if r == terminator {
				// It is the terminator we expected.
				// Read up to the terminator, but do not consume it.
				builder.WriteString(testID[copyFromIndex:i])
				return builder.String(), i, nil
			} else {
				// Other special character that is not escaped and not our terminator.
				// Return an error message.
				if terminator != 0 {
					return "", -1, errors.Reason("got delimeter character %+q at byte %v; expected normal character, escape sequence or delimeter %+q (test ID pattern is :module!scheme:coarse:fine#case)", r, i, terminator).Err()
				}
				return "", -1, errors.Reason("got delimeter character %+q at byte %v; expected normal character, escape sequence or end of string (test ID pattern is :module!scheme:coarse:fine#case)", r, i).Err()
			}
		default:
			// Normal non-terminal character.
		}
	}
	// We reached the end of the string.
	if inEscapeSequence {
		// Reached end of string while still in an escape sequence. This is invalid.
		return "", -1, errors.Reason(`unfinished escape sequence at byte %v, got end of string; expected one of :!#\`, i).Err()
	}
	if terminator != 0 {
		// We did not expect to reach end of string, we were looking for another terminator.
		return "", -1, errors.Reason("unexpected end of string at byte %v, expected delimeter %+q (test ID pattern is :module!scheme:coarse:fine#case)", i, terminator).Err()
	}
	builder.WriteString(testID[copyFromIndex:i])
	return builder.String(), len(testID), nil
}

// ValidateTestVariantIdentifier validates a structured test variant identifier.
//
// N.B. This does not validate the test ID against the configured schemes, but
// this validation must only be applied at upload time. (And must not be applied
// at other times to ensure old tests uploaded under old schemes continue to be
// ingestable and queryable.)
func ValidateTestVariantIdentifier(id *pb.TestVariantIdentifier) error {
	if id == nil {
		return validate.Unspecified()
	}
	testID := TestIdentifier{
		ModuleName:   id.ModuleName,
		ModuleScheme: id.ModuleScheme,
		CoarseName:   id.CoarseName,
		FineName:     id.FineName,
		CaseName:     id.CaseName,
	}
	if err := validateTestIdentifier(testID); err != nil {
		return err
	}

	// Module variant
	if err := ValidateVariant(id.ModuleVariant); err != nil {
		return errors.Annotate(err, "module_variant").Err()
	}
	if id.ModuleVariantHash != "" {
		// Currently this is an output-only field. At some point, we might want to
		// allow clients to set it. To make this possible without breaking compatibility,
		// we should already start validating the values they set to make sure they are valid.
		expectedVariantHash := VariantHash(id.ModuleVariant)
		if id.ModuleVariantHash != expectedVariantHash {
			return errors.Reason("module_variant_hash: expected %s to match specified variant", expectedVariantHash).Err()
		}
	}

	return nil
}

// validateTestIdentifier validates a structured test identifier.
//
// Errors are annotated using field names in snake_case.
func validateTestIdentifier(id TestIdentifier) error {
	// Module name
	if id.ModuleName == "" {
		return errors.Reason("module_name: unspecified").Err()
	}
	if err := validateUTF8Printable(id.ModuleName, 300); err != nil {
		return errors.Annotate(err, "module_name").Err()
	}

	// Module scheme
	if id.ModuleScheme == "" {
		return errors.Reason("module_scheme: unspecified").Err()
	}
	if err := validateUTF8Printable(id.ModuleScheme, 20); err != nil {
		return errors.Annotate(err, "module_scheme").Err()
	}
	if !schemeRE.MatchString(id.ModuleScheme) {
		return errors.Reason("module_scheme: does not match %q", schemeRE).Err()
	}

	// Coarse name and fine name
	if err := validateUTF8Printable(id.CoarseName, 300); err != nil {
		return errors.Annotate(err, "coarse_name").Err()
	}
	if err := validateCoarseOrFineNameLeadingCharacter(id.CoarseName); err != nil {
		return errors.Annotate(err, "coarse_name").Err()
	}
	if err := validateUTF8Printable(id.FineName, 300); err != nil {
		return errors.Annotate(err, "fine_name").Err()
	}
	if err := validateCoarseOrFineNameLeadingCharacter(id.FineName); err != nil {
		return errors.Annotate(err, "fine_name").Err()
	}
	// If the scheme does not allow a fine name, it also does not allow a coarse
	// name (fine name is always used before coarse name).
	if id.FineName == "" && id.CoarseName != "" {
		return errors.Reason("fine_name: unspecified when coarse_name is specified").Err()
	}

	// Case name
	if id.CaseName == "" {
		return errors.Reason("case_name: unspecified").Err()
	}
	if err := validateUTF8Printable(id.CaseName, 512); err != nil {
		return errors.Annotate(err, "case_name").Err()
	}

	// Additional validation for legacy test identifiers.
	if id.ModuleName == "legacy" {
		// Legacy test identifier represented in structured form.
		if id.ModuleScheme != "legacy" {
			return errors.Reason("module_scheme: must be set to 'legacy' for tests in the 'legacy' module").Err()
		}
		if id.CoarseName != "" {
			return errors.Reason("coarse_name: must be empty for tests in the 'legacy' module").Err()
		}
		if id.FineName != "" {
			return errors.Reason("fine_name: must be empty for tests in the 'legacy' module").Err()
		}
		if strings.HasPrefix(id.CaseName, ":") {
			return errors.Reason("case_name: must not start with ':' for tests in the 'legacy' module").Err()
		}
		// This is a lightweight version of validateCaseNameNotReserved for legacy tests
		// that still backtests on already uploaded test results.
		if strings.HasPrefix(id.CaseName, "*") {
			return errors.Reason("case_name: must not start with '*' for tests in the 'legacy' module").Err()
		}
	} else {
		// Additional validation for natively structured test identifiers.
		if id.ModuleScheme == "legacy" {
			return errors.Reason("module_scheme: must not be set to 'legacy' except for tests in the 'legacy' module").Err()
		}
		if err := validateCaseNameLeadingCharacter(id.CaseName); err != nil {
			return errors.Annotate(err, "case_name").Err()
		}
	}

	// Ensure that when we encode the structured test ID to a flat ID,
	// it is less than 512 bytes.
	if sizeEscapedTestID(id) > 512 {
		return errors.Reason("test ID exceeds 512 bytes in encoded form").Err()
	}
	return nil
}

// validateCoarseOrFineNameLeadingCharacter validates a coarse or fine name
// does not start with one of the reserved leading characters.
func validateCoarseOrFineNameLeadingCharacter(name string) error {
	// These may be left unspecified for some schemes.
	if name == "" {
		return nil
	}
	// We reserve all leading characters equal to or less than ',' (in practice, this
	// is the set !”#$%&’()*+, as all the non-printables are ruled out anyway) so that we
	// have a set of names we can use for special purposes down the track. These names
	// will have the useful property of being sorted before all other names in an
	// alphebetical sorting.
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return errors.Reason("leading character is not a valid rune").Err()
	}
	if r <= ',' {
		return errors.Reason("character %+q may not be used as a leading character of a coarse or fine name", r).Err()
	}
	return nil
}

// validateCaseNameLeadingCharacter performs additional validation on a case name to
// ensure it does not use one of the reserved leading characters, unless it is for
// one of the allowed reserved values.
//
// This validation should only be applied to non-legacy test identifiers as some
// legacy test IDs may use such reserved leading characters.
func validateCaseNameLeadingCharacter(name string) error {
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return errors.Reason("leading character is not a valid rune").Err()
	}
	if r <= ',' {
		if name == FixtureCaseName {
			return nil
		}
		if r == '*' {
			return errors.Reason("character * may not be used as a leading character of a case name, unless the case name is '%s'", FixtureCaseName).Err()
		}
		return errors.Reason("character %+q may not be used as a leading character of a case name", r).Err()
	}
	return nil
}

// EncodeTestID encodes a structured test ID into a flat-form test ID.
func EncodeTestID(id TestIdentifier) string {
	if id.ModuleName == "legacy" && id.ModuleScheme == "legacy" && id.CoarseName == "" && id.FineName == "" {
		return id.CaseName
	}
	var builder strings.Builder
	builder.Grow(sizeEscapedTestID(id))
	builder.WriteString(":")
	writeEscapedTestIDComponent(&builder, id.ModuleName)
	builder.WriteString("!")
	builder.WriteString(id.ModuleScheme)
	builder.WriteString(":")
	writeEscapedTestIDComponent(&builder, id.CoarseName)
	builder.WriteString(":")
	writeEscapedTestIDComponent(&builder, id.FineName)
	builder.WriteString("#")
	writeEscapedTestIDComponent(&builder, id.CaseName)
	return builder.String()
}

// writeEscapedTestIDComponent writes the string s to builder, escaping
// any occurrences of the characters ':', '\', '#' and '!'.
func writeEscapedTestIDComponent(builder *strings.Builder, s string) {
	startIndex := 0
	for i, r := range s {
		if r == ':' || r == '\\' || r == '#' || r == '!' {
			builder.WriteString(s[startIndex:i])
			builder.WriteByte('\\')
			builder.WriteRune(r)
			startIndex = i + 1
		}
	}
	builder.WriteString(s[startIndex:])
}

// sizeEscapedTestID returns the size of the test ID when encoded to flat-form.
// This is useful for validation and allocating string buffers.
func sizeEscapedTestID(id TestIdentifier) int {
	if id.ModuleName == "legacy" && id.ModuleScheme == "legacy" && id.CoarseName == "" && id.FineName == "" {
		// Legacy test ID roundtrips back to flat-form.
		return len(id.CaseName)
	}
	// Structured Test ID.
	finalSize := len(":!::#") +
		sizeEscapedTestIDComponent(id.ModuleName) +
		len(id.ModuleScheme) +
		sizeEscapedTestIDComponent(id.CoarseName) +
		sizeEscapedTestIDComponent(id.FineName) +
		sizeEscapedTestIDComponent(id.CaseName)
	return finalSize
}

// sizeEscapedTestIDComponent returns the size of a test ID component
// after escaping the characters ':', '\', '#' and '!'.
func sizeEscapedTestIDComponent(s string) int {
	escapesRequired := 0
	for _, r := range s {
		if r == ':' || r == '\\' || r == '#' || r == '!' {
			escapesRequired++
		}
	}
	return len(s) + escapesRequired
}

// validateUTF8Printable validates that a string is valid UTF-8, in Normal form C,
// and consists only for printable runes.
func validateUTF8Printable(text string, maxLength int) error {
	if len(text) > maxLength {
		return errors.Reason("longer than %v bytes", maxLength).Err()
	}
	if !utf8.ValidString(text) {
		return errors.Reason("not a valid utf8 string").Err()
	}
	if !norm.NFC.IsNormalString(text) {
		return errors.Reason("not in unicode normalized form C").Err()
	}
	for i, rune := range text {
		if !unicode.IsPrint(rune) {
			return fmt.Errorf("non-printable rune %+q at byte index %d", rune, i)
		}
	}
	return nil
}
