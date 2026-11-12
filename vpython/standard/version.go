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
	"slices"
	"strconv"
	"strings"
)

// DefaultPythonVersion is the default CPython major.minor version targeted when
// a spec's requires-python constraint is empty or satisfied by the default interpreter.
const DefaultPythonVersion = "3.11"

// DefaultUVVersion is the pinned CIPD version tag for on-demand uv
// ("infra/3pp/tools/uv/${platform}") when uv is unbundled from vpython3.
// Built from https://chromium.googlesource.com/infra/infra/+/HEAD/3pp/uv/3pp.pb.
const DefaultUVVersion = "version:3@0.10.9.prebuilt"

// Default pinned CIPD version tags for on-demand CPython 3.8, 3.11, and 3.14
// ("infra/3pp/tools/cpython3/${platform}") when unbundled from vpython3.
// Built from https://chromium.googlesource.com/infra/infra/+/HEAD/3pp/cpython3/3pp.pb.
const (
	DefaultCPython38Version  = "version:3@3.8.10.chromium.35"
	DefaultCPython311Version = "version:3@3.11.9.chromium.39"
	DefaultCPython314Version = "version:3@3.14.10.chromium.4"
)

// pep440Regex is a prefix-only PEP 440 regular expression.
// By matching only the leading release and optional pre-release blocks without a trailing anchor ($),
// we naturally skip/ignore any trailing post-releases, dev-releases, or Google's custom LTS/build suffixes
// (e.g. ".chromium.31" or "-rc"), keeping the parser simple and backward-compatible.
var pep440Regex = regexp.MustCompile(`^([1-9]\d*!)?(0|[1-9]\d*)(\.(0|[1-9]\d*))*((a|b|rc)(0|[1-9]\d*))?`)

// baseReleaseRegex extracts the leading major.minor.patch release digits blocks sequence.
var baseReleaseRegex = regexp.MustCompile(`^v?(\d+(?:\.\d+){0,2})`)

// chromiumBuildRegex matches an exact 3pp CPython build constraint or version
// string for the supported major.minor tracks (3.8, 3.11, 3.14), such as
// "3.14.8.chromium.1", "==3.14.8.chromium.1", or "2@3.11.8.chromium.35".
var chromiumBuildRegex = regexp.MustCompile(`^\s*(?:===|==)?\s*v?(?:([1-9]\d*)[@!])?((3\.(?:8|11|14))\.\d+\.chromium\.\d+)\s*$`)

// ParseChromiumBuild extracts the major.minor track, normalized build version,
// and CIPD version tag from an exact 3pp CPython build string or equality
// constraint (e.g. "==3.14.8.chromium.1" or "3.11.9.chromium.35"). Only the
// supported 3.8, 3.11, and 3.14 tracks are accepted.
func ParseChromiumBuild(s string) (track, build, cipdVersion string, ok bool) {
	m := chromiumBuildRegex.FindStringSubmatch(s)
	if m == nil {
		return "", "", "", false
	}
	epoch, rawBuild, track := m[1], m[2], m[3]
	if epoch == "" {
		epoch = "3"
	}
	build = rawBuild
	if epoch != "3" {
		build = epoch + "@" + rawBuild
	}
	return track, build, fmt.Sprintf("version:%s@%s", epoch, rawBuild), true
}

// Version represents a MAJOR.MINOR.PATCH version segment sequence.
type Version struct {
	Major    int
	Minor    int
	Patch    int
	Segments int    // Number of explicitly parsed segments (1, 2, or 3)
	Raw      string // The normalized raw string representation (without "v" prefix)
}

// ParseVersion extracts and parses the MAJOR.MINOR.PATCH integers from a version string.
func ParseVersion(v string) (Version, error) {
	v = strings.TrimSpace(v)
	if strings.Contains(v, "..") {
		return Version{}, fmt.Errorf("invalid version format: contains double dots %q", v)
	}

	// Remove leading "v" prefix before matching the PEP 440 template layout
	cleanStr := strings.TrimPrefix(v, "v")

	// 1. Validate strict compliance using our safe prefix PEP 440 regular expression!
	if !pep440Regex.MatchString(cleanStr) {
		return Version{}, fmt.Errorf("invalid version format (non-PEP440 compliant): %q", v)
	}

	// 2. Extract Major, Minor, and Patch by manual string splitting on the base release digits part!
	releaseMatch := baseReleaseRegex.FindStringSubmatch(cleanStr)
	if releaseMatch == nil {
		return Version{}, fmt.Errorf("failed to parse release segments: %q", v)
	}
	releaseStr := releaseMatch[1]
	parts := strings.Split(releaseStr, ".")

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("failed to parse major version %q: %w", parts[0], err)
	}

	minor := 0
	if len(parts) > 1 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse minor version %q: %w", parts[1], err)
		}
	}

	patch := 0
	if len(parts) > 2 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("failed to parse patch version %q: %w", parts[2], err)
		}
	}

	return Version{
		Major:    major,
		Minor:    minor,
		Patch:    patch,
		Segments: len(parts),
		Raw:      cleanStr,
	}, nil
}

// LessThan returns true if this version is older than the other.
func (v Version) LessThan(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

// Equal returns true if the versions are identical at the MAJOR.MINOR (and optional PATCH) level.
func (v Version) Equal(other Version, checkPatch bool) bool {
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
	ver Version
}

func (sc subConstraint) isSatisfied(v Version) bool {
	switch sc.op {
	case opEq:
		return v.Equal(sc.ver, sc.ver.Segments == 3)
	case opArbitrary:
		return v.Raw == sc.ver.Raw
	case opNe:
		return !v.Equal(sc.ver, sc.ver.Segments == 3)
	case opGe:
		return v.Equal(sc.ver, true) || sc.ver.LessThan(v)
	case opLe:
		return v.Equal(sc.ver, true) || v.LessThan(sc.ver)
	case opGt:
		return sc.ver.LessThan(v)
	case opLt:
		return v.LessThan(sc.ver)
	case opCompatible:
		if !(v.Equal(sc.ver, true) || sc.ver.LessThan(v)) {
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

		ver, err := ParseVersion(verStr)
		if err != nil {
			return constraint{}, err
		}
		res.subs = append(res.subs, subConstraint{op: op, ver: ver})
	}
	return res, nil
}

// isSatisfied returns true if the version satisfies all sub-constraints.
func (c constraint) isSatisfied(v Version) bool {
	for _, sub := range c.subs {
		if !sub.isSatisfied(v) {
			return false
		}
	}
	return true
}

// RequirementSatisfied reports whether version satisfies a comma-separated
// PEP 440 specifier list such as ">=1.0,!=1.5" or "==1.15.1+chromium.2".
func RequirementSatisfied(specifiers, version string) (bool, error) {
	c, err := parseConstraint(specifiers)
	if err != nil {
		return false, err
	}
	v, err := ParseVersion(version)
	if err != nil {
		return false, err
	}
	for _, sub := range c.subs {
		var ok bool
		switch sub.op {
		case opEq:
			ok = releaseEqual(sub.ver, v)
		case opNe:
			ok = !releaseEqual(sub.ver, v)
		default:
			ok = sub.isSatisfied(v)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func releaseEqual(spec, candidate Version) bool {
	if strings.HasSuffix(spec.Raw, ".*") {
		if spec.Segments == 1 {
			return candidate.Major == spec.Major
		}
		return candidate.Equal(spec, spec.Segments >= 3)
	}
	candRaw := candidate.Raw
	if !strings.Contains(spec.Raw, "+") {
		candRaw, _, _ = strings.Cut(candRaw, "+")
	}
	return candidate.Equal(spec, true) && strings.EqualFold(releaseTail(spec.Raw), releaseTail(candRaw))
}

func releaseTail(raw string) string {
	m := baseReleaseRegex.FindString(raw)
	return raw[len(m):]
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
	var bestVer Version
	for _, av := range available {
		v, err := ParseVersion(av)
		if err != nil {
			continue
		}
		if c.isSatisfied(v) {
			if bestTarget == "" || bestVer.LessThan(v) {
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

// ResolveTargetVersion matches a constraint string against available Python
// runtime versions, preferring defaultVer when it is present in available and
// satisfies constraintStr, and falling back to MatchInterpreter otherwise.
func ResolveTargetVersion(constraintStr, defaultVer string, available []string) (string, error) {
	if track, build, _, ok := ParseChromiumBuild(constraintStr); ok && slices.Contains(available, track) {
		return build, nil
	}
	if defaultVer != "" && slices.Contains(available, defaultVer) {
		c, err := parseConstraint(constraintStr)
		if err != nil {
			return "", fmt.Errorf("failed to parse constraint %q: %w", constraintStr, err)
		}
		defV, err := ParseVersion(defaultVer)
		if err != nil {
			return "", fmt.Errorf("failed to parse default version %q: %w", defaultVer, err)
		}
		if c.isSatisfied(defV) {
			return defaultVer, nil
		}
	}
	return MatchInterpreter(constraintStr, available)
}
