// Copyright 2026 The LUCI Authors.
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

package standard

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// pep440Regex is a prefix-only PEP 440 regular expression.
// By matching only the leading release and optional pre-release blocks without a trailing anchor ($),
// we naturally skip/ignore any trailing post-releases, dev-releases, or Google's custom LTS/build suffixes
// (e.g. ".chromium.31" or "-rc"), keeping the parser simple and backward-compatible.
var pep440Regex = regexp.MustCompile(`^([1-9]\d*!)?(0|[1-9]\d*)(\.(0|[1-9]\d*))*((a|b|rc)(0|[1-9]\d*))?`)

// baseReleaseRegex extracts the leading major.minor.patch release digits blocks sequence.
var baseReleaseRegex = regexp.MustCompile(`^v?(\d+(?:\.\d+)*)`)

// version represents a MAJOR.MINOR.PATCH version segment sequence.
type version struct {
	Major    int
	Minor    int
	Patch    int
	Segments int    // Number of explicitly parsed segments (1, 2, or 3)
	Raw      string // The normalized raw string representation (without "v" prefix)
}

// parseVersion extracts and parses the MAJOR.MINOR.PATCH integers from a version string.
func parseVersion(v string) (version, error) {
	v = strings.TrimSpace(v)
	if strings.Contains(v, "..") {
		return version{}, fmt.Errorf("invalid version format: contains double dots %q", v)
	}

	// Remove leading "v" prefix before matching the PEP 440 template layout
	cleanStr := strings.TrimPrefix(v, "v")

	// 1. Validate strict compliance using our safe prefix PEP 440 regular expression!
	if !pep440Regex.MatchString(cleanStr) {
		return version{}, fmt.Errorf("invalid version format (non-PEP440 compliant): %q", v)
	}

	// 2. Extract Major, Minor, and Patch by manual string splitting on the base release digits part!
	releaseMatch := baseReleaseRegex.FindStringSubmatch(cleanStr)
	if releaseMatch == nil {
		return version{}, fmt.Errorf("failed to parse release segments: %q", v)
	}
	releaseStr := releaseMatch[1]
	parts := strings.Split(releaseStr, ".")

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return version{}, fmt.Errorf("failed to parse major version %q: %w", parts[0], err)
	}

	minor := 0
	if len(parts) > 1 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return version{}, fmt.Errorf("failed to parse minor version %q: %w", parts[1], err)
		}
	}

	patch := 0
	if len(parts) > 2 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return version{}, fmt.Errorf("failed to parse patch version %q: %w", parts[2], err)
		}
	}

	return version{
		Major:    major,
		Minor:    minor,
		Patch:    patch,
		Segments: len(parts),
		Raw:      cleanStr,
	}, nil
}

// lessThan returns true if this version is older than the other.
func (v version) lessThan(other version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

// equal returns true if the versions are identical at the MAJOR.MINOR (and optional PATCH) level.
func (v version) equal(other version, checkPatch bool) bool {
	if v.Major != other.Major || v.Minor != other.Minor {
		return false
	}
	if checkPatch {
		return v.Patch == other.Patch
	}
	return true
}

type constraintOp string

const (
	opEq         constraintOp = "=="
	opNe         constraintOp = "!="
	opGe         constraintOp = ">="
	opLe         constraintOp = "<="
	opGt         constraintOp = ">"
	opLt         constraintOp = "<"
	opCompatible constraintOp = "~="
	opArbitrary  constraintOp = "==="
)

type subConstraint struct {
	op  constraintOp
	ver version
}

func (sc subConstraint) isSatisfied(v version) bool {
	switch sc.op {
	case opEq:
		return v.equal(sc.ver, sc.ver.Segments == 3)
	case opArbitrary:
		return v.Raw == sc.ver.Raw
	case opNe:
		return !v.equal(sc.ver, sc.ver.Segments == 3)
	case opGe:
		return v.equal(sc.ver, true) || sc.ver.lessThan(v)
	case opLe:
		return v.equal(sc.ver, true) || v.lessThan(sc.ver)
	case opGt:
		return sc.ver.lessThan(v)
	case opLt:
		return v.lessThan(sc.ver)
	case opCompatible:
		if !(v.equal(sc.ver, true) || sc.ver.lessThan(v)) {
			return false
		}
		if sc.ver.Segments == 2 {
			return v.Major == sc.ver.Major
		}
		return v.Major == sc.ver.Major && v.Minor == sc.ver.Minor
	default:
		return false
	}
}

// constraint represents a compound, comma-separated environment constraint list (AND logic).
type constraint struct {
	subs []subConstraint
}

// parseConstraint parses standard Python constraint logic from strings.
//
// Examples of valid constraint inputs:
//   - "==3.11"       (exact constraint)
//   - ">=3.8"        (minimum boundary)
//   - "~=3.8.0"      (compatible release, e.g. >=3.8, <4.0)
//   - ">=3.8,<3.10"  (compound bounds using AND logic)
func parseConstraint(c string) (constraint, error) {
	var res constraint
	for p := range strings.SplitSeq(c, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}

		var op constraintOp
		var verStr string
		// Order from longest to shortest operators to prevent partial matches
		for _, possibleOp := range [...]constraintOp{opArbitrary, opCompatible, opGe, opLe, opEq, opNe, opGt, opLt} {
			if remaining, found := strings.CutPrefix(p, string(possibleOp)); found {
				op = possibleOp
				verStr = remaining
				break
			}
		}
		// Default to exact match operator if none is prefixed
		if op == "" {
			op = opEq
			verStr = p
		}

		ver, err := parseVersion(verStr)
		if err != nil {
			return constraint{}, err
		}
		res.subs = append(res.subs, subConstraint{op: op, ver: ver})
	}
	return res, nil
}

// isSatisfied returns true if the version satisfies all sub-constraints.
func (c constraint) isSatisfied(v version) bool {
	for _, sub := range c.subs {
		if !sub.isSatisfied(v) {
			return false
		}
	}
	return true
}

// MatchInterpreter matches a constraint string against available Chrome runtime names on disk.
// It strictly selects the HIGHEST available version that satisfies the constraint list,
// returning an error if no interpreters qualify.
func MatchInterpreter(constraintStr string, available []string) (string, error) {
	// If the list of available interpreters is empty, fail fast!
	if len(available) == 0 {
		return "", fmt.Errorf("no available python interpreters found")
	}

	c, err := parseConstraint(constraintStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse constraint %q: %w", constraintStr, err)
	}

	var bestTarget string
	var bestVer version
	for _, av := range available {
		v, err := parseVersion(av)
		if err != nil {
			continue
		}
		if c.isSatisfied(v) {
			if bestTarget == "" || bestVer.lessThan(v) {
				bestTarget = av
				bestVer = v
			}
		}
	}

	if bestTarget == "" {
		return "", fmt.Errorf("no available python interpreters satisfy constraint %q (available: %v)", constraintStr, available)
	}
	return bestTarget, nil
}
