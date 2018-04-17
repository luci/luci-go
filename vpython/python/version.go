// Copyright 2017 The LUCI Authors.
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

package python

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"go.chromium.org/luci/common/errors"
)

// canonicalVersionRE is a regular expression that can match canonical Python
// versions.
//
// This has been modified from the PEP440 canonical regular expression to
// exclude parts outside of the (major.minor.patch...) section.
var canonicalVersionRE = regexp.MustCompile(
	`^([1-9]\d*!)?` +
		`((0|[1-9]\d*)(\.(0|[1-9]\d*))*)` +
		`(((a|b|rc)(0|[1-9]\d*))?(\.post(0|[1-9]\d*))?(\.dev(0|[1-9]\d*))?)` +
		`(\+.*)?$`)

// Version is a Python interpreter version.
//
// It is a simplified version of the Python interpreter version scheme defined
// in PEP 440: https://www.python.org/dev/peps/pep-0440/
//
// Notably, it extracts the major, minor, and patch values out of the version.
type Version struct {
	Major int
	Minor int
	Patch int
}

// ParseVersion parses a Python version from a version string (e.g., "1.2.3").
//
// On error, the returned Version value is not specified.
func ParseVersion(s string) (Version, error) {
	var v Version

	// Treat as PEP440.
	v, err := parsePEP440Version(s)
	if err == nil {
		return v, nil
	}

	// Try resilient parsing.
	v, err2 := resilientVersionParsing(s)
	if err2 == nil {
		return v, nil
	}

	return v, errors.Reason(
		"could not parse version from %q, failed PEP440 (%s) and resilient (%s) parsing",
		s, err, err2).Err()
}

// parsePEP440Version parses a PEP440 version string into v.
//
// Note that on error, parsePEP440Version may still modify v.
func parsePEP440Version(s string) (Version, error) {
	v := Version{}

	if s == "" {
		return v, nil
	}

	match := canonicalVersionRE.FindStringSubmatch(s)
	if match == nil {
		return v, errors.New("non-canonical Python version string")
	}
	parts := strings.Split(match[2], ".")

	// Values are expected to parse, and will panic otherwise. This is safe
	// because the value has already been determined to be canonical above.
	mustParseVersion := func(value string) int {
		version, err := strconv.Atoi(value)
		if err != nil {
			panic(fmt.Sprintf("invalid number value %q: %s", value, err))
		}
		return version
	}

	// Regexp match guarantees that "parts" will have at least one component, and
	// that all components are well-formed numbers.
	if len(parts) >= 3 {
		v.Patch = mustParseVersion(parts[2])
	}
	if len(parts) >= 2 {
		v.Minor = mustParseVersion(parts[1])
	}
	v.Major = mustParseVersion(parts[0])
	if v.IsZero() {
		return v, errors.Reason("version is incomplete").Err()
	}
	return v, nil
}

// resilientVersionParsing is a resilient version of parsing Python versions.
//
// It is used when PEP440 version parsing doesn't work, which is generally when
// the Python version string is non-compliant. Since there's still potentially
// enough information in the version string to satisfy our intent, we will try
// and parse it in a much simpler manner:
//
// 1) Strip all characters that aren't numbers or dots.
// 2) Parse left-to-right until we get 1, 2, or 3 numbers, assuming that they
//    are Major, Minor, and Patch respectively.
// 3) Require that we observe at least Major and Minor to succeed.
//
// Note that on error, resilientVersionParsing may still modify v.
func resilientVersionParsing(s string) (v Version, err error) {
	s = strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) || r == '.' {
			return r
		}

		// Delete this character.
		return -1
	}, s)

	// Accommodate empty segments (e.g., "..") by returning "false".
	parseVersion := func(value string, dest *int) error {
		if len(value) == 0 {
			return errors.New("empty version string")
		}
		version, err := strconv.Atoi(value)
		if err != nil {
			return errors.Reason("invalid number value %q: %s", value, err).Err()
		}
		*dest = version
		return nil
	}

	parts := strings.Split(s, ".")

	// Must have at least major and minor versions.
	if len(parts) < 2 {
		err = errors.Reason("reduced version %q does not have major and minor versions", s).Err()
		return
	}

	// Parse the Patch version. This is optional; if there's an error, we will
	// just swallow it.
	if len(parts) >= 3 {
		_ = parseVersion(parts[2], &v.Patch)
	}

	// Parse major and minor versions.
	if err = parseVersion(parts[1], &v.Minor); err != nil {
		err = errors.Annotate(err, "could not parse minor version").Err()
		return
	}
	if err = parseVersion(parts[0], &v.Major); err != nil {
		err = errors.Annotate(err, "could not parse major version").Err()
		return
	}
	return
}

func (v Version) String() string {
	if v.IsZero() {
		return ""
	}
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// IsZero returns true if the Version is empty. This is true if the Major field,
// which must be set, is empty.
func (v *Version) IsZero() bool { return v.Major <= 0 }

// PythonBase returns the base Python interpreter name for this version.
func (v *Version) PythonBase() string {
	switch {
	case v.IsZero():
		return "python"
	case v.Minor > 0:
		return fmt.Sprintf("python%d.%d", v.Major, v.Minor)
	default:
		return fmt.Sprintf("python%d", v.Major)
	}
}

// IsSatisfiedBy returns true if "other" is a suitable match for this version. A
// suitable match:
//
//	- MUST have a Major version.
//	- If v is zero, other is automatically suitable.
//	- If v is non-zero, other must have the same Major version as v, and a
//	  minor/patch version that is >= v's.
func (v *Version) IsSatisfiedBy(other Version) bool {
	switch {
	case other.Major <= 0:
		// "other" must have a Major version.
		return false
	case v.IsZero():
		// "v" is zero (anything), so "other" satisfies it.
		return true
	case v.Major != other.Major:
		// "other" must match "v"'s Major version precisely.
		return false
	case v.Minor > other.Minor:
		// "v" requires a Minor version that is greater than "other"'s.
		return false
	case v.Minor < other.Minor:
		// "v" requires a Minor version that is less than "other"'s.
		return true
	case v.Patch > other.Patch:
		// "v" requires a Patch version that is greater than "other"'s.
		return false
	default:
		return true
	}
}

// Less returns true if "v"'s Version semantically precedes "other".
func (v *Version) Less(other *Version) bool {
	switch {
	case v.Major < other.Major:
		return true
	case v.Major > other.Major:
		return false
	case v.Minor < other.Minor:
		return true
	case v.Minor > other.Minor:
		return false
	default:
		return (v.Patch < other.Patch)
	}
}
