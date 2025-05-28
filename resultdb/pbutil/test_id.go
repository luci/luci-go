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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// schemeRE defines the syntax of valid test scheme IDs. Does not capture length limit.
var schemeRE = regexp.MustCompile(`^[a-z][a-z0-9]*$`)

// FixtureCaseName is a special value, that when used as the case name in a test identifier,
// denotes that the test covers the result of common setup/teardown logic for a fine name.
const FixtureCaseName = "*fixture"

// LegacySchemeID identifies the scheme used for tests that are not
// natively structured. This allows such tests to still be represented
// in the structured test ID space.
const LegacySchemeID = "legacy"

// LegacyModuleName identifies the module name used for tests that are not
// natively structured. This allows such tests to still be represented
// in the structured test ID space.
const LegacyModuleName = "legacy"

// ValidateTestID returns a non-nil error if testID is invalid.
func ValidateTestID(testID string) error {
	_, err := ParseAndValidateTestID(testID)
	return err
}

// BaseTestIdentifier represents a structured test identifier, without
// variant information.
type BaseTestIdentifier struct {
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
func ParseAndValidateTestID(testID string) (BaseTestIdentifier, error) {
	// Perform some basic validation before we try to parse the ID.
	if testID == "" {
		return BaseTestIdentifier{}, validate.Unspecified()
	}
	if err := ValidateUTF8Printable(testID, 512, ValidationModeLoose); err != nil {
		return BaseTestIdentifier{}, err
	}

	id, err := parseTestID(testID)
	if err != nil {
		return BaseTestIdentifier{}, err
	}
	err = ValidateBaseTestIdentifier(id)
	return id, err
}

// parseTestID parses a test identifier from its flat-form string
// representation into its structured representation.
//
// Except for legacy test IDs, the test identifier has the structure:
// :module!scheme:coarse:fine#case
//
// Where case may be a simple string or have multiple components,
// e.g. `part1:part2:part3`.
func parseTestID(testID string) (BaseTestIdentifier, error) {
	if !IsStructuredTestID(testID) {
		// This is a legacy test ID.
		return BaseTestIdentifier{
			ModuleName:   LegacyModuleName,
			ModuleScheme: LegacySchemeID,
			CaseName:     testID,
		}, nil
	}

	// This is a structured test ID.
	// We expect the next character to be the module name.
	// We will read until the next '!' character.
	moduleName, nextIndex, err := readEscapedComponent(testID, 1, componentParseOptions{Terminator: '!'})
	if err != nil {
		return BaseTestIdentifier{}, err
	}
	if moduleName == LegacyModuleName {
		// To ensure our flat test IDs roundtrip back to the same value,
		// we expect legacy test IDs to be serialized directly as the case
		// name, not as :legacy...
		return BaseTestIdentifier{}, errors.Fmt("module %q may not be used within a structured test ID encoding", LegacySchemeID)
	}
	moduleScheme, nextIndex, err := readEscapedComponent(testID, nextIndex+1, componentParseOptions{Terminator: ':'})
	if err != nil {
		return BaseTestIdentifier{}, err
	}
	coarseName, nextIndex, err := readEscapedComponent(testID, nextIndex+1, componentParseOptions{Terminator: ':'})
	if err != nil {
		return BaseTestIdentifier{}, err
	}
	fineName, nextIndex, err := readEscapedComponent(testID, nextIndex+1, componentParseOptions{Terminator: '#'})
	if err != nil {
		return BaseTestIdentifier{}, err
	}
	caseName, _, err := readEscapedComponent(testID, nextIndex+1, componentParseOptions{IsCaseName: true})
	if err != nil {
		return BaseTestIdentifier{}, err
	}
	return BaseTestIdentifier{
		ModuleName:   moduleName,
		ModuleScheme: moduleScheme,
		CoarseName:   coarseName,
		FineName:     fineName,
		CaseName:     caseName,
	}, nil
}

// IsStructuredTestID returns whether the given test ID is a structured
// test ID (starts with the prefix ":").
func IsStructuredTestID(testID string) bool {
	return strings.HasPrefix(testID, ":")
}

// componentParseOptions are options for parsing a test component.
//
// A test component is one of (module, scheme, coarse, fine, case) in:
// :module!scheme:coarse:fine#case
type componentParseOptions struct {
	// The terminator of the component. This should be one of ':', '#', '!'.
	// This must be set to zero if IsCaseName is set (denoting end of string
	// as the terminator).
	Terminator rune

	// IsCaseName enables special processing for the test case name.
	//
	// The case name consists of one or more parts separated by
	// a ':'. For example, "someTest" or
	// "topLevelTest:withSettingsPage:doesThat"
	//
	// For convenience of most clients which do not care about
	// this substructure, the case name is read as one string
	// of the form:
	//    test_case_part, {':', test_case_part} ;
	// (ENBF grammar.)
	//
	// where each test_case_part has characters : and \ escaped
	// with backslashes.
	//
	// In practice, the implementation achieves this by reading from the
	// input string until end of string, decoding the escape sequences
	// "\#" and "\!" to "#" and "!", but leaving '\:' and '\\' in place.
	IsCaseName bool
}

// readEscapedComponent reads a test ID component from the provided
// `testID` input starting at `startIndex`. The terminator it reads
// up to and string handling is controlled by `opts`.
//
// The read component (excluding terminator) is returned, and the
// advanced index (up to the position of the terminator) is returned
// as nextIndex.
//
// When reading, it decodes the escape sequences \: \# \! and \\.
func readEscapedComponent(testID string, startIndex int, opts componentParseOptions) (component string, nextIndex int, err error) {
	if !(opts.Terminator == ':' || opts.Terminator == '#' || opts.Terminator == '!' || (opts.Terminator == 0 && opts.IsCaseName)) {
		panic("bad parse options")
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
			return "", -1, errors.Fmt(`invalid UTF-8 rune at byte %v`, i)
		}
		if inEscapeSequence {
			if r != ':' && r != '\\' && r != '#' && r != '!' {
				// Invalid escape sequence.
				return "", -1, errors.Fmt(`got unexpected character %+q at byte %v, while processing escape sequence (\); only the characters :!#\ may be escaped`, r, i)
			}
			if opts.IsCaseName && (r == '\\' || r == ':') {
				// Keep the sequences \\ and \: escaped in the parsed case name.
			} else {
				// Decode the escape sequence by not copying the
				// prior \ into the output.

				// Write out whatever we have up until now except for the last slash (1 byte).
				builder.WriteString(testID[copyFromIndex : i-1])

				// Resume copying from the character after the '\'.
				copyFromIndex = i
			}

			// Exit the escape sequence.
			inEscapeSequence = false
			continue
		}
		switch r {
		case '\\':
			// Begin escape sequence.
			inEscapeSequence = true
		case ':', '#', '!':
			if r == ':' && opts.IsCaseName {
				// Read over this delimiter, it is part of the case name.
				continue
			}

			// We found a terminator (other than end of string).
			if r == opts.Terminator {
				// It is the terminator we expected.
				// Read up to the terminator, but do not consume it.
				builder.WriteString(testID[copyFromIndex:i])
				return builder.String(), i, nil
			} else {
				// Other special character that is not escaped and not our terminator.
				// Return an error message.
				if opts.IsCaseName {
					return "", -1, errors.Fmt("got delimiter character %+q at byte %v; expected normal character, escape sequence or end of string (test ID pattern is :module!scheme:coarse:fine#case)", r, i)
				}
				return "", -1, errors.Fmt("got delimiter character %+q at byte %v; expected normal character, escape sequence or delimiter %+q (test ID pattern is :module!scheme:coarse:fine#case)", r, i, opts.Terminator)
			}
		default:
			// Normal non-terminal character.
		}
	}
	// We reached the end of the string.
	if inEscapeSequence {
		// Reached end of string while still in an escape sequence. This is invalid.
		return "", -1, errors.Fmt(`unfinished escape sequence at byte %v, got end of string; expected one of :!#\`, i)
	}
	if !opts.IsCaseName {
		// We did not expect to reach end of string, we were looking for another terminator.
		return "", -1, errors.Fmt("unexpected end of string at byte %v, expected delimiter %+q (test ID pattern is :module!scheme:coarse:fine#case)", i, opts.Terminator)
	}
	builder.WriteString(testID[copyFromIndex:i])
	return builder.String(), len(testID), nil
}

// parseTestCaseName extracts the parts embedded in a test case name.
//
// A test case name looks like one of the following:
// - "myTest" (one component, most common)
// - "topLevelTest:with_settings_page:does_something" (multiple components)
// Where "myTest", "topLevelTest", "with_settings_page", "does_something"
// are parts.
//
// The test case name is one of components of the Test ID, see parseTestID
// for details.
func parseTestCaseName(caseName string) ([]string, error) {
	var result []string
	startIndex := 0
	for {
		component, nextIndex, err := readCaseNamePart(caseName, startIndex)
		if err != nil {
			return nil, err
		}
		result = append(result, component)
		if nextIndex >= len(caseName) {
			// Done
			break
		}
		// Skip over the separator and continue parsing.
		startIndex = nextIndex + 1
	}
	return result, nil
}

// readCaseNamePart reads a case name component from the provided
// case name starting at `startIndex`. It reads until
// the next unescaped ':' separator is found, or end of string.
// The escape sequences \\ and \: are decoded.
func readCaseNamePart(caseName string, startIndex int) (component string, nextIndex int, err error) {
	var builder strings.Builder
	inEscapeSequence := false

	var r rune
	var width int

	copyFromIndex := startIndex
	i := startIndex
	for ; i < len(caseName); i += width {
		r, width = utf8.DecodeRuneInString(caseName[i:])
		if r == utf8.RuneError {
			return "", -1, errors.Fmt(`invalid UTF-8 rune at byte %v`, i)
		}
		if inEscapeSequence {
			if r != '\\' && r != ':' {
				// Invalid escape sequence.
				return "", -1, errors.Fmt(`got unexpected character %+q at byte %v, while processing escape sequence (\); only the characters \ and : may be escaped`, r, i)
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
			builder.WriteString(caseName[copyFromIndex:i])
			// The character following the '\' is included verbatim unless it
			// is invalid.
			// Resume copying from the character after this '\'.
			copyFromIndex = i + width
		case ':':
			// We found a separator.
			// Read up to the separator, but do not consume it.
			builder.WriteString(caseName[copyFromIndex:i])
			return builder.String(), i, nil
		default:
			// Normal non-terminal character.
		}
	}
	// We reached the end of the string.
	if inEscapeSequence {
		// Reached end of string while still in an escape sequence. This is invalid.
		return "", -1, errors.Fmt(`unfinished escape sequence at byte %v, got end of string; expected one of \ or :`, i)
	}
	builder.WriteString(caseName[copyFromIndex:i])
	return builder.String(), len(caseName), nil
}

func TestIDFromStructuredTestIdentifier(id *pb.TestIdentifier) string {
	return EncodeTestID(ExtractBaseTestIdentifier(id))
}

// VariantFromStructuredTestIdentifier retrieves the equivalent variant for a structured test ID.
// Only suitable test identifiers validated with ValidateStructuredTestIdentifierForStorage.
func VariantFromStructuredTestIdentifier(id *pb.TestIdentifier) *pb.Variant {
	return proto.Clone(id.ModuleVariant).(*pb.Variant)
}

// VariantFromStructuredTestIdentifier retrieves the equivalent variant hash for a structured test ID.
func VariantHashFromStructuredTestIdentifier(id *pb.TestIdentifier) string {
	if id.ModuleVariantHash != "" {
		return id.ModuleVariantHash
	}
	return VariantHash(id.ModuleVariant)
}

// ParseStructuredTestIdentifierForInput constructs a test identifier from the
// given flat test ID and variant. OUTPUT_ONLY fields are NOT set.
func ParseStructuredTestIdentifierForInput(testID string, variant *pb.Variant) (*pb.TestIdentifier, error) {
	testIdentifier, err := ParseAndValidateTestID(testID)
	if err != nil {
		return nil, err
	}

	return &pb.TestIdentifier{
		ModuleName:    testIdentifier.ModuleName,
		ModuleScheme:  testIdentifier.ModuleScheme,
		ModuleVariant: variant,
		CoarseName:    testIdentifier.CoarseName,
		FineName:      testIdentifier.FineName,
		CaseName:      testIdentifier.CaseName,
	}, nil
}

// ParseStructuredTestIdentifierForOutput constructs a test identifier from the
// given flat test ID and variant. OUTPUT_ONLY fields are set.
func ParseStructuredTestIdentifierForOutput(testID string, variant *pb.Variant) (*pb.TestIdentifier, error) {
	result, err := ParseStructuredTestIdentifierForInput(testID, variant)
	if err != nil {
		return nil, err
	}
	// Set OUTPUT_ONLY fields.
	PopulateStructuredTestIdentifierHashes(result)
	return result, nil
}

// PopulateStructuredTestIdentifierHashes computes the variant hash field on TestVariantIdentifier.
// It should only be called on TestIdentifiers retrieved from storage and that were validated
// by ValidateStructuredTestIdentifierForUpload, i.e. have the variant set.
func PopulateStructuredTestIdentifierHashes(id *pb.TestIdentifier) {
	id.ModuleVariantHash = VariantHash(id.ModuleVariant)
}

// ValidateStructuredTestIdentifierForStorage validates a structured test identifier
// that is being uploaded for storage.
//
// N.B. This does not validate the test ID against the configured schemes; this
// should also be applied at upload time. (And must not be applied at other times
// to ensure old tests uploaded under old schemes continue to be ingestable and
// queryable.)
func ValidateStructuredTestIdentifierForStorage(id *pb.TestIdentifier) error {
	if id == nil {
		return validate.Unspecified()
	}
	if err := ValidateBaseTestIdentifier(ExtractBaseTestIdentifier(id)); err != nil {
		return err
	}

	// Module variant
	if err := ValidateVariant(id.ModuleVariant); err != nil {
		return errors.Fmt("module_variant: %w", err)
	}
	if id.ModuleVariantHash != "" {
		// If clients set both the hash and the variant, they should be consistent.
		// Clients may set both in upload contexts if they are passing back a
		// structured test ID retrieved via another query (e.g. creating exonerations
		// after querying failed results).
		//
		// Some methods VariantHashFromStructuredTestIdentifier) expect the variant hash,
		// if set, to be valid.
		expectedVariantHash := VariantHash(id.ModuleVariant)
		if id.ModuleVariantHash != expectedVariantHash {
			return errors.Fmt("module_variant_hash: expected %s (to match module_variant) or for value to be unset", expectedVariantHash)
		}
	}
	return nil
}

// ValidateStructuredTestIdentifierForQuery validates a structured test identifier
// is suitable as an input to a query RPC.
//
// Unlike ValidateStructuredTestIdentifierForStorage, this method allows either
// the ModuleVariant or ModuleVariantHash to be specified.
func ValidateStructuredTestIdentifierForQuery(id *pb.TestIdentifier) error {
	if id == nil {
		return validate.Unspecified()
	}
	if err := ValidateBaseTestIdentifier(ExtractBaseTestIdentifier(id)); err != nil {
		return err
	}

	// Module variant.
	if id.ModuleVariant == nil && id.ModuleVariantHash == "" {
		return errors.New("at least one of module_variant and module_variant_hash must be set")
	}
	if id.ModuleVariant != nil {
		if err := ValidateVariant(id.ModuleVariant); err != nil {
			return errors.Fmt("module_variant: %w", err)
		}
	}
	if id.ModuleVariantHash != "" {
		if err := ValidateVariantHash(id.ModuleVariantHash); err != nil {
			return errors.Fmt("module_variant_hash: %w", err)
		}
		if id.ModuleVariant != nil {
			// If clients set both the hash and the variant, they should be consistent.
			// Clients may commonly set both if they are passing back a structured test ID
			// retrieved via another query.
			expectedVariantHash := VariantHash(id.ModuleVariant)
			if id.ModuleVariantHash != expectedVariantHash {
				return errors.Fmt("module_variant_hash: expected %s (to match module_variant) or for value to be unset", expectedVariantHash)
			}
		}
	}
	return nil
}

// ValidateBaseTestIdentifier validates a structured base test identifier.
//
// Errors are annotated using field names in snake_case.
func ValidateBaseTestIdentifier(id BaseTestIdentifier) error {
	// Module name
	if err := ValidateModuleName(id.ModuleName); err != nil {
		return errors.Fmt("module_name: %w", err)
	}

	// Module scheme
	isLegacyModule := id.ModuleName == LegacyModuleName
	if err := ValidateModuleScheme(id.ModuleScheme, isLegacyModule); err != nil {
		return errors.Fmt("module_scheme: %w", err)
	}

	// Coarse name and fine name
	if err := ValidateUTF8PrintableStrict(id.CoarseName, 300); err != nil {
		return errors.Fmt("coarse_name: %w", err)
	}
	if err := validateCoarseOrFineNameLeadingCharacter(id.CoarseName); err != nil {
		return errors.Fmt("coarse_name: %w", err)
	}
	if err := ValidateUTF8PrintableStrict(id.FineName, 300); err != nil {
		return errors.Fmt("fine_name: %w", err)
	}
	if err := validateCoarseOrFineNameLeadingCharacter(id.FineName); err != nil {
		return errors.Fmt("fine_name: %w", err)
	}
	// If the scheme does not allow a fine name, it also does not allow a coarse
	// name (fine name is always used before coarse name).
	if id.FineName == "" && id.CoarseName != "" {
		return errors.New("fine_name: unspecified when coarse_name is specified")
	}

	// Case name
	if id.CaseName == "" {
		return errors.New("case_name: unspecified")
	}

	// Additional validation for legacy test identifiers.
	if isLegacyModule {
		// Legacy test identifier represented in structured form.
		if id.CoarseName != "" {
			return errors.Fmt("coarse_name: must be empty for tests in the %q module", LegacyModuleName)
		}
		if id.FineName != "" {
			return errors.Fmt("fine_name: must be empty for tests in the %q module", LegacyModuleName)
		}
		if strings.HasPrefix(id.CaseName, ":") {
			return errors.Fmt("case_name: must not start with ':' for tests in the %q module", LegacyModuleName)
		}
		if err := ValidateUTF8Printable(id.CaseName, 512, ValidationModeLoose); err != nil {
			return errors.Fmt("case_name: %w", err)
		}
		// This is a lightweight version of validateCaseNameNotReserved for legacy tests
		// that still backtests on already uploaded test results.
		if strings.HasPrefix(id.CaseName, "*") {
			return errors.Fmt("case_name: must not start with '*' for tests in the %q module", LegacyModuleName)
		}
	} else {
		// Additional validation for natively structured test identifiers.
		if err := ValidateUTF8PrintableStrict(id.CaseName, 512); err != nil {
			return errors.Fmt("case_name: %w", err)
		}
		if err := validateCaseNameNonLegacy(id.CaseName); err != nil {
			return errors.Fmt("case_name: %w", err)
		}
	}

	// Ensure that when we encode the structured test ID to a flat ID,
	// it is less than 512 bytes.
	if sizeEscapedTestID(id) > 512 {
		return errors.New("test ID exceeds 512 bytes in encoded form")
	}
	return nil
}

// ValidateModuleName validates a module name is syntactically valid.
func ValidateModuleName(name string) error {
	if name == "" {
		return errors.New("unspecified")
	}
	if err := ValidateUTF8PrintableStrict(name, 300); err != nil {
		return err
	}
	return nil
}

// ValidateModuleScheme validates a scheme name is syntactically valid.
// Set isLegacyModule to true if the module name is LegacyModuleName,
// and false otherwise.
func ValidateModuleScheme(scheme string, isLegacyModule bool) error {
	if scheme == "" {
		return errors.New("unspecified")
	}
	if err := ValidateUTF8PrintableStrict(scheme, 20); err != nil {
		return err
	}
	if !schemeRE.MatchString(scheme) {
		return errors.Fmt("does not match %q", schemeRE)
	}
	if isLegacyModule {
		if scheme != LegacySchemeID {
			return errors.Fmt("must be set to %q for tests in the %q module", LegacySchemeID, LegacyModuleName)
		}
	} else {
		if scheme == LegacySchemeID {
			return errors.Fmt("must not be set to %q except for tests in the %q module", LegacySchemeID, LegacyModuleName)
		}
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
		return errors.New("leading character is not a valid rune")
	}
	if r <= ',' {
		return errors.Fmt("character %+q may not be used as a leading character of a coarse or fine name", r)
	}
	return nil
}

// validateCaseNameNonLegacy performs additional validation on a case name
// used outside the legacy module.
//
// The additional requirements that are validated are:
//   - it does not use one of the reserved leading characters, unless it is for
//     the reserved value *fixture.
//   - the case name follows the format defined for TestResult.case_name, meaning the only
//     allowed escape sequences are \\ \:. And when broken up by colons ':' (excluding
//     escaped colons), each component is not empty.
func validateCaseNameNonLegacy(name string) error {
	r, _ := utf8.DecodeRuneInString(name)
	if r == utf8.RuneError {
		return errors.New("leading character is not a valid rune")
	}
	if r <= ',' {
		if name == FixtureCaseName {
			return nil
		}
		if r == '*' {
			return errors.Fmt("character * may not be used as a leading character of a case name, unless the case name is '%s'", FixtureCaseName)
		}
		return errors.Fmt("character %+q may not be used as a leading character of a case name", r)
	}
	parts, err := parseTestCaseName(name)
	if err != nil {
		return err
	}
	for i, component := range parts {
		if len(component) == 0 {
			return errors.Fmt("component %v is empty, each component of the case name must be non-empty", i+1)
		}
	}
	return nil
}

// ExtractBaseTestIdentifier extracts the structured Test ID from a structured test idnetifier.
func ExtractBaseTestIdentifier(id *pb.TestIdentifier) BaseTestIdentifier {
	return BaseTestIdentifier{
		ModuleName:   id.ModuleName,
		ModuleScheme: id.ModuleScheme,
		CoarseName:   id.CoarseName,
		FineName:     id.FineName,
		CaseName:     id.CaseName,
	}
}

// EncodeTestID encodes a structured base test identifier into a flat-form test ID.
func EncodeTestID(id BaseTestIdentifier) string {
	if id.ModuleName == "legacy" && id.ModuleScheme == LegacySchemeID && id.CoarseName == "" && id.FineName == "" {
		return id.CaseName
	}
	// For module name, coarse name and fine name, the characters : ! # and \ are escaped.
	// For module scheme, no escaping is required as it uses a safe alphabet.
	// For case name, only ! and # are escaped. ':' is a valid delimiter between components
	// of the case name and should lift to the top-level encoding. Moreover, '\' and occurances of
	// ':' that are not intended to be delimieters should already be escaped for valid case names.

	var builder strings.Builder
	builder.Grow(sizeEscapedTestID(id))
	builder.WriteString(":")
	writeEscapedTestIDComponent(&builder, id.ModuleName, false)
	builder.WriteString("!")
	builder.WriteString(id.ModuleScheme)
	builder.WriteString(":")
	writeEscapedTestIDComponent(&builder, id.CoarseName, false)
	builder.WriteString(":")
	writeEscapedTestIDComponent(&builder, id.FineName, false)
	builder.WriteString("#")
	writeEscapedTestIDComponent(&builder, id.CaseName, true /* isCaseName */)
	return builder.String()
}

// writeEscapedTestIDComponent writes the string s to builder, escaping
// any occurrences of the characters ':', '\', '#' and '!'.
func writeEscapedTestIDComponent(builder *strings.Builder, s string, isCaseName bool) {
	startIndex := 0
	for i, r := range s {
		// Escape the characters #, !, :, \.
		// For case names, do not escape : and \ as any occurrences in the case name parts
		// have already been escaped when encoded into the case name and we want to avoid
		// double-escaping.
		needsEscape := ((r == ':' || r == '\\') && !isCaseName) || r == '#' || r == '!'
		if needsEscape {
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
func sizeEscapedTestID(id BaseTestIdentifier) int {
	if id.ModuleName == "legacy" && id.ModuleScheme == LegacySchemeID && id.CoarseName == "" && id.FineName == "" {
		// Legacy test ID roundtrips back to flat-form.
		return len(id.CaseName)
	}
	// Structured Test ID.
	finalSize := len(":!::#") +
		sizeEscapedTestIDComponent(id.ModuleName, false) +
		// Scheme does not need escaping as it uses a safe alphabet.
		len(id.ModuleScheme) +
		sizeEscapedTestIDComponent(id.CoarseName, false) +
		sizeEscapedTestIDComponent(id.FineName, false) +
		sizeEscapedTestIDComponent(id.CaseName, true /* isCaseName */)
	return finalSize
}

// sizeEscapedTestIDComponent returns the size of a test ID component
// after escaping the characters ':', '\', '#' and '!'.
func sizeEscapedTestIDComponent(s string, isCaseName bool) int {
	escapesRequired := 0
	for _, r := range s {
		needsEscape := ((r == ':' || r == '\\') && !isCaseName) || r == '#' || r == '!'
		if needsEscape {
			escapesRequired++
		}
	}
	return len(s) + escapesRequired
}

type validationMode int

const (
	// ValidationModeStrict does not allow the unicode replacement character U+FFFD.
	ValidationModeStrict validationMode = iota
	// ValidationModeLoose allows the unicode replacement character U+FFFD. This
	// should be used for legacy-form test IDs.
	ValidationModeLoose
)

// validateUTF8Printable validates that a string is valid UTF-8, in Normal form C,
// consists only for printable runes, and does not contain the unicode replacement
// character (U+FFFD).
func ValidateUTF8PrintableStrict(text string, maxLength int) error {
	return ValidateUTF8Printable(text, maxLength, ValidationModeStrict)
}

// ValidateUTF8Printable validates that a string is valid UTF-8, in Normal form C,
// and consists only for printable runes.
func ValidateUTF8Printable(text string, maxLength int, mode validationMode) error {
	if len(text) > maxLength {
		return errors.Fmt("longer than %v bytes", maxLength)
	}
	if !utf8.ValidString(text) {
		return errors.New("not a valid utf8 string")
	}
	if !norm.NFC.IsNormalString(text) {
		return errors.New("not in unicode normalized form C")
	}
	for i, rune := range text {
		if !unicode.IsPrint(rune) {
			return fmt.Errorf("non-printable rune %+q at byte index %d", rune, i)
		}
		if mode == ValidationModeStrict && rune == utf8.RuneError {
			return fmt.Errorf("unicode replacement character (U+FFFD) at byte index %d", i)
		}
	}
	return nil
}

// EncodeCaseName encodes a set of case name parts into a test case name.
func EncodeCaseName(components ...string) string {
	if len(components) <= 0 {
		panic("at least one component must be specified")
	}

	var result strings.Builder
	sizeRequired := 0
	for i := 0; i < len(components); i++ {
		sizeRequired += sizeEscapedCaseNameComponent(components[i])
	}
	// For N-1 separating forward slashes.
	sizeRequired += len(components) - 1
	result.Grow(sizeRequired)

	for i := 0; i < len(components); i++ {
		if i > 0 {
			result.WriteRune(':')
		}
		writeEscapedCaseNameComponent(&result, components[i])
	}
	return result.String()
}

// sizeEscapedTestIDComponent returns the size of a test ID component
// after escaping the characters ':' and '\'.
func sizeEscapedCaseNameComponent(s string) int {
	escapesRequired := 0
	for _, r := range s {
		if r == ':' || r == '\\' {
			escapesRequired++
		}
	}
	return len(s) + escapesRequired
}

// writeEscapedCaseNameComponent writes the string s to builder, escaping
// any occurrences of the characters ':', '\'.
func writeEscapedCaseNameComponent(builder *strings.Builder, s string) {
	// The start of the range to copy.
	startIndex := 0

	for i, r := range s {
		if r == ':' || r == '\\' {
			// Copy the range up to now.
			builder.WriteString(s[startIndex:i])

			// Insert the escape sequence.
			builder.WriteByte('\\')
			builder.WriteRune(r)

			// Continue copying from the next rune onwards.
			startIndex = i + 1
		}
	}
	builder.WriteString(s[startIndex:])
}
