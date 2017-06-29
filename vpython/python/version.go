// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/luci/luci-go/common/errors"
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
func ParseVersion(s string) (Version, error) {
	var v Version
	if s == "" {
		return v, nil
	}

	match := canonicalVersionRE.FindStringSubmatch(s)
	if match == nil {
		return v, errors.Reason("non-canonical Python version string: %q", s).Err()
	}
	parts := strings.Split(match[2], ".")

	parseVersion := func(value string) (int, error) {
		version, err := strconv.Atoi(value)
		if err != nil {
			return 0, errors.Annotate(err, "invalid number value: %q", value).Err()
		}
		return version, nil
	}

	// Regexp match guarantees that "parts" will have at least one component, and
	// that all components are well-formed numbers.
	var err error
	if len(parts) >= 3 {
		if v.Patch, err = parseVersion(parts[2]); err != nil {
			return v, errors.Annotate(err, "invalid patch value").Err()
		}
	}
	if len(parts) >= 2 {
		if v.Minor, err = parseVersion(parts[1]); err != nil {
			return v, errors.Annotate(err, "invalid minor value").Err()
		}
	}
	if v.Major, err = parseVersion(parts[0]); err != nil {
		return v, errors.Annotate(err, "invalid major value").Err()
	}
	if v.IsZero() {
		return v, errors.Reason("version is incomplete").Err()
	}
	return v, nil
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
