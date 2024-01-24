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

package wheels

import (
	"fmt"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cipd/client/cipd/template"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/vpython/api/vpython"
)

// pep425MacPlatform is a parsed PEP425 Mac platform string.
//
// The string is formatted:
// macosx_<maj>_<min>_<cpu-arch>
//
// For example:
//   - macosx_10_6_intel
//   - macosx_10_0_fat
//   - macosx_10_2_x86_64
type pep425MacPlatform struct {
	major int
	minor int
	arch  string
}

// parsePEP425MacPlatform parses a pep425MacPlatform from the supplied
// platform string. If the string does not contain a recognizable Mac
// platform, this function returns nil.
func parsePEP425MacPlatform(v string) *pep425MacPlatform {
	parts := strings.SplitN(v, "_", 4)
	if len(parts) != 4 {
		return nil
	}
	if parts[0] != "macosx" {
		return nil
	}

	var ma pep425MacPlatform
	var err error
	if ma.major, err = strconv.Atoi(parts[1]); err != nil {
		return nil
	}
	if ma.minor, err = strconv.Atoi(parts[2]); err != nil {
		return nil
	}

	ma.arch = parts[3]
	return &ma
}

// less returns true if "ma" represents a Mac version before "other".
func (ma *pep425MacPlatform) less(other *pep425MacPlatform) bool {
	switch {
	case ma.major < other.major:
		return true
	case ma.major > other.major:
		return false
	case ma.minor < other.minor:
		return true
	default:
		return false
	}
}

// pep425IsBetterMacPlatform processes two PEP425 platform strings and
// returns true if "candidate" is a superior PEP425 tag candidate than "cur".
//
// This function favors, in order:
//   - Mac platforms over non-Mac platforms,
//   - arm64 > intel > others
//   - Older Mac versions over newer ones
func pep425IsBetterMacPlatform(cur, candidate string) bool {
	// Parse a Mac platform string
	curPlatform := parsePEP425MacPlatform(cur)
	candidatePlatform := parsePEP425MacPlatform(candidate)

	archScore := func(c *pep425MacPlatform) int {
		// Smaller is better
		switch c.arch {
		case "arm64":
			return 0
		case "intel":
			return 1
		default:
			return 2
		}
	}

	switch {
	case curPlatform == nil:
		return candidatePlatform != nil
	case candidatePlatform == nil:
		return false
	case archScore(candidatePlatform) != archScore(curPlatform):
		return archScore(candidatePlatform) < archScore(curPlatform)
	case candidatePlatform.less(curPlatform):
		// We prefer the lowest Mac architecture available.
		return true
	default:
		return false
	}
}

// Determies if the specified platform is a Linux platform and, if so, if it
// is a "manylinux1_" Linux platform.
func isLinuxPlatform(plat string) (is bool, many bool) {
	switch {
	case strings.HasPrefix(plat, "linux_"):
		is = true
	case strings.HasPrefix(plat, "manylinux1_"):
		is, many = true, true
	}
	return
}

// pep425IsBetterLinuxPlatform processes two PEP425 platform strings and
// returns true if "candidate" is a superior PEP425 tag candidate than "cur".
//
// This function favors, in order:
//   - Linux platforms over non-Linux platforms.
//   - "manylinux1_" over non-"manylinux1_".
//
// Examples of expected Linux platform strings are:
//   - linux1_x86_64
//   - linux1_i686
//   - manylinux1_i686
func pep425IsBetterLinuxPlatform(cur, candidate string) bool {
	// We prefer "manylinux1_" platforms over "linux_" platforms.
	curIs, curMany := isLinuxPlatform(cur)
	candidateIs, candidateMany := isLinuxPlatform(candidate)
	switch {
	case !curIs:
		return candidateIs
	case !candidateIs:
		return false
	case curMany:
		return false
	default:
		return candidateMany
	}
}

// preferredPlatformFuncForTagSet examines a tag set and returns a function
// that compares two "platform" tags.
//
// The comparison function is chosen based on the operating system represented
// by the tag set. This choice is made with the assumption that the tag set
// represents a realistic platform (e.g., no mixed Mac and Linux tags).
func preferredPlatformFuncForTagSet(tags []*vpython.PEP425Tag) func(cur, candidate string) bool {
	// Identify the operating system from the tag set. Iterate through tags until
	// we see an indicator.
	for _, tag := range tags {
		// Linux?
		if is, _ := isLinuxPlatform(tag.Platform); is {
			return pep425IsBetterLinuxPlatform
		}

		// Mac
		if plat := parsePEP425MacPlatform(tag.Platform); plat != nil {
			return pep425IsBetterMacPlatform
		}
	}

	// No opinion.
	return func(cur, candidate string) bool { return false }
}

// isNewerPy3Abi returns true if the candidate string identifies a new, unstable
// ABI that should be preferred over the long-term stable "abi3", which we don't
// build wheels against.
func isNewerPy3Abi(cur, candidate string) bool {
	// We don't bother finding the latest ABI (e.g. preferring "cp39" over
	// "cp38"). Each release only has one supported unstable ABI, so we should
	// never encounter more than one anyway.
	return (cur == "abi3" || cur == "none") && strings.HasPrefix(candidate, "cp3")
}

// Prefer specific Python (e.g., cp27) over generic (e.g., py27).
func isSpecificImplAbi(python string) bool {
	return !strings.HasPrefix(python, "py")
}

// pep425TagSelector chooses the "best" PEP425 tag from a set of potential tags.
// This "best" tag will be used to resolve our CIPD templates and allow for
// Python implementation-specific CIPD template parameters.
func pep425TagSelector(tags []*vpython.PEP425Tag) *vpython.PEP425Tag {
	var best *vpython.PEP425Tag

	// isPreferredOSPlatform is an OS-specific platform preference function.
	isPreferredOSPlatform := preferredPlatformFuncForTagSet(tags)

	isBetter := func(t *vpython.PEP425Tag) bool {
		switch {
		case best == nil:
			return true
		case t.Count() > best.Count():
			// More populated fields is more specificity.
			return true
		case best.AnyPlatform() && !t.AnyPlatform():
			// More specific platform is preferred.
			return true
		case !best.HasABI() && t.HasABI():
			// More specific ABI is preferred.
			return true
		case isNewerPy3Abi(best.Abi, t.Abi):
			// Prefer the newest supported ABI tag. In theory this can break if
			// we have wheels built against a long-term stable ABI like abi3, as
			// we'll only look for packages built against the newest, unstable
			// ABI. But in practice that doesn't happen, as dockerbuild
			// produces packages tagged with the unstable ABIs.
			return true
		case isPreferredOSPlatform(best.Platform, t.Platform) && (isSpecificImplAbi(t.Python) || !isSpecificImplAbi(best.Python)):
			// Prefer a better platform, but not if it means moving
			// to a less-specific ABI.
			return true
		case isSpecificImplAbi(t.Python) && !isSpecificImplAbi(best.Python):
			return true

		default:
			return false
		}
	}

	for _, t := range tags {
		tag := proto.Clone(t).(*vpython.PEP425Tag)
		if isBetter(tag) {
			best = tag
		}
	}
	return best
}

// getPEP425CIPDTemplates returns the set of CIPD template strings for a
// given PEP425 tag.
//
// Template parameters are derived from the most representative PEP425 tag.
// Any missing tag parameters will result in their associated template
// parameters not getting exported.
//
// The full set of exported tag parameters is:
// - py_python: The PEP425 "python" tag value (e.g., "cp27").
// - py_abi: The PEP425 Python ABI (e.g., "cp27mu").
// - py_platform: The PEP425 Python platform (e.g., "manylinux1_x86_64").
// - py_tag: The full PEP425 tag (e.g., "cp27-cp27mu-manylinux1_x86_64").
//
// This function also backports the Python platform into the CIPD "platform"
// field, ensuring that regardless of the host platform, the Python CIPD
// wheel is chosen based solely on that host's Python interpreter.
//
// Infra CIPD packages tend to use "${platform}" (generic) combined with
// "${py_abi}" and "${py_platform}" to identify its packages.
func addPEP425CIPDTemplateForTag(expander template.Expander, tag *vpython.PEP425Tag) error {
	if tag == nil {
		return errors.New("no PEP425 tag")
	}

	if tag.Python != "" {
		expander["py_python"] = tag.Python
	}
	if tag.Abi != "" {
		expander["py_abi"] = tag.Abi
	}
	if tag.Platform != "" {
		expander["py_platform"] = tag.Platform
	}
	if tag.Python != "" && tag.Abi != "" && tag.Platform != "" {
		expander["py_tag"] = tag.TagString()
	}

	// Override the CIPD "platform" based on the PEP425 tag. This allows selection
	// of Python wheels based on the platform of the Python executable rather
	// than the platform of the underlying operating system.
	//
	// For example, a 64-bit Windows version can run 32-bit Python, and we'll
	// want to use 32-bit Python wheels.
	platform := PlatformForPEP425Tag(tag)
	if platform.String() == "-" {
		return errors.Reason("failed to infer CIPD platform for tag [%s]", tag).Err()
	}
	expander["platform"] = platform.String()

	// Build the sum tag, "vpython_platform",
	// "${platform}_${py_python}_${py_abi}"
	if tag.Python != "" && tag.Abi != "" {
		expander["vpython_platform"] = fmt.Sprintf("%s_%s_%s", platform, tag.Python, tag.Abi)
	}

	return nil
}

// PlatformForPEP425Tag returns the CIPD platform inferred from a given Python
// PEP425 tag.
//
// If the platform could not be determined, an empty string will be returned.
func PlatformForPEP425Tag(t *vpython.PEP425Tag) template.Platform {
	switch platSplit := strings.SplitN(t.Platform, "_", 2); platSplit[0] {
	case "linux", "manylinux1":
		// Grab the remainder.
		//
		// Examples:
		// - linux_i686
		// - manylinux1_x86_64
		// - linux_arm64
		cpu := ""
		if len(platSplit) > 1 {
			cpu = platSplit[1]
		}
		switch cpu {
		case "i686":
			return template.Platform{OS: "linux", Arch: "386"}
		case "x86_64":
			return template.Platform{OS: "linux", Arch: "amd64"}
		case "arm64", "aarch64":
			return template.Platform{OS: "linux", Arch: "arm64"}
		case "mipsel", "mips":
			return template.Platform{OS: "linux", Arch: "mips32"}
		case "mips64":
			return template.Platform{OS: "linux", Arch: "mips64"}
		default:
			// All remaining "arm*" get the "armv6l" CIPD platform.
			if strings.HasPrefix(cpu, "arm") {
				return template.Platform{OS: "linux", Arch: "armv6l"}
			}
			return template.Platform{}
		}

	case "macosx":
		// Grab the last token.
		//
		// Examples:
		// - macosx_10_10_intel
		// - macosx_10_10_i386
		if len(platSplit) == 1 {
			return template.Platform{}
		}
		suffixSplit := strings.SplitN(platSplit[1], "_", -1)
		switch suffixSplit[len(suffixSplit)-1] {
		case "intel", "x86_64", "fat64", "universal":
			return template.Platform{OS: "mac", Arch: "amd64"}
		case "arm64":
			return template.Platform{OS: "mac", Arch: "arm64"}
		case "i386", "fat32":
			return template.Platform{OS: "mac", Arch: "386"}
		default:
			return template.Platform{}
		}

	case "win32":
		// win32
		return template.Platform{OS: "windows", Arch: "386"}
	case "win":
		// Examples:
		// - win_amd64
		if len(platSplit) == 1 {
			return template.Platform{}
		}
		switch platSplit[1] {
		case "amd64":
			return template.Platform{OS: "windows", Arch: "amd64"}
		default:
			return template.Platform{}
		}

	default:
		return template.Platform{}
	}
}
