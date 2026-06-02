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

// Package legacy provides utilities to translate old, protobuf-based Vpython
// specifications into standard PEP 508 and PEP 440 requirements.
package legacy

import (
	"fmt"
	"strings"
	"unicode"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/standard"
	"go.chromium.org/luci/vpython/standard/legacy/pipname"
)

// TranslateLegacySpec converts a legacy vpython Spec to standard ProjectSpec.
func TranslateLegacySpec(s *vpython.Spec) (*standard.ProjectSpec, error) {
	if s == nil {
		return nil, nil
	}

	var reqs []string
	seen := make(map[string]struct{})

	for _, pkg := range s.Wheel {
		if pkg == nil || pkg.Name == "" {
			continue
		}

		name := pipname.PipNameFromPackageName(pkg.Name)
		version := pipname.PipVersionFromPackageVersion(pkg.Version)

		marker := buildPEP508Marker(pkg.MatchTag, pkg.NotMatchTag)

		line := name
		if version != "" {
			line += "==" + version
		}
		if marker != "" {
			line += "; " + marker
		}

		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		reqs = append(reqs, line)
	}

	reqPython := translatePythonVersion(s.PythonVersion)

	return &standard.ProjectSpec{
		RequiresPython: reqPython,
		Dependencies:   reqs,
	}, nil
}

func translatePythonVersion(v string) string {
	ver, err := standard.ParseVersion(v)
	if err != nil {
		return ""
	}
	switch ver.Segments {
	case 1:
		return fmt.Sprintf(">=%d.0,<%d.0", ver.Major, ver.Major+1)
	case 2:
		return fmt.Sprintf(">=%d.%d,<%d.%d", ver.Major, ver.Minor, ver.Major, ver.Minor+1)
	default:
		return fmt.Sprintf(">=%d.%d.%d,<%d.%d", ver.Major, ver.Minor, ver.Patch, ver.Major, ver.Minor+1)
	}
}

// translatePEP425Tag converts a legacy PEP 425 platform tag to standard PEP 508 marker conditions.
//
// NOTE: Standard PEP 508 environment markers grammar does NOT support the boolean prefix "not"
// operator (e.g. "not (sys_platform == 'win32')" is syntactically invalid and will raise a
// MarkerSyntaxError inside standard Python packaging tools like pip). To support platform exclusions
// safely, we pass the 'invert' boolean parameter to perform high-fidelity logical negation on-the-fly
// via De Morgan's laws, dynamically flipping comparison operators (== to !=) and logical junctions (and to or / or to and).
func translatePEP425Tag(tag *vpython.PEP425Tag, invert bool) string {
	var conds []string

	eq := func(variable, value string) string {
		if invert {
			return variable + " != '" + value + "'"
		}
		return variable + " == '" + value + "'"
	}

	andOr := func(isAnd bool) string {
		if invert {
			if isAnd {
				return " or "
			}
			return " and "
		}
		if isAnd {
			return " and "
		}
		return " or "
	}

	if tag.Python != "" {
		if strings.HasPrefix(tag.Python, "cp") && len(tag.Python) >= 4 {
			major := tag.Python[2:3]
			minor := tag.Python[3:]
			isNumeric := true
			for _, r := range minor {
				if !unicode.IsDigit(r) {
					isNumeric = false
					break
				}
			}
			if isNumeric {
				conds = append(conds, eq("python_version", major+"."+minor))
			}
		}
	}

	if tag.Platform != "" && tag.Platform != "any" {
		switch {
		case strings.HasPrefix(tag.Platform, "linux") || strings.HasPrefix(tag.Platform, "manylinux"):
			switch {
			case strings.HasSuffix(tag.Platform, "_x86_64"):
				conds = append(conds, eq("sys_platform", "linux")+andOr(true)+eq("platform_machine", "x86_64"))
			case strings.HasSuffix(tag.Platform, "_aarch64") || strings.HasSuffix(tag.Platform, "_arm64"):
				conds = append(conds, eq("sys_platform", "linux")+andOr(true)+eq("platform_machine", "aarch64"))
			case strings.HasSuffix(tag.Platform, "_i686") || strings.HasSuffix(tag.Platform, "_i386"):
				sub := "(" + eq("platform_machine", "i386") + andOr(false) + eq("platform_machine", "i686") + ")"
				conds = append(conds, eq("sys_platform", "linux")+andOr(true)+sub)
			default:
				if strings.Contains(tag.Platform, "arm") {
					conds = append(conds, eq("sys_platform", "linux")+andOr(true)+eq("platform_machine", "armv7l"))
				} else if strings.HasSuffix(tag.Platform, "_riscv64") {
					conds = append(conds, eq("sys_platform", "linux")+andOr(true)+eq("platform_machine", "riscv64"))
				} else {
					conds = append(conds, eq("sys_platform", "linux"))
				}
			}
		case strings.HasPrefix(tag.Platform, "macosx"):
			switch {
			case strings.HasSuffix(tag.Platform, "_x86_64") || strings.HasSuffix(tag.Platform, "_intel") || strings.HasSuffix(tag.Platform, "_fat64") || strings.HasSuffix(tag.Platform, "_universal"):
				conds = append(conds, eq("sys_platform", "darwin")+andOr(true)+eq("platform_machine", "x86_64"))
			case strings.HasSuffix(tag.Platform, "_arm64"):
				conds = append(conds, eq("sys_platform", "darwin")+andOr(true)+eq("platform_machine", "arm64"))
			default:
				conds = append(conds, eq("sys_platform", "darwin"))
			}
		case tag.Platform == "win32":
			conds = append(conds, eq("sys_platform", "win32")+andOr(true)+eq("platform_machine", "x86"))
		case strings.HasPrefix(tag.Platform, "win"):
			switch {
			case strings.HasSuffix(tag.Platform, "_amd64"):
				sub := "(" + eq("platform_machine", "AMD64") + andOr(false) + eq("platform_machine", "x86_64") + ")"
				conds = append(conds, eq("sys_platform", "win32")+andOr(true)+sub)
			case strings.HasSuffix(tag.Platform, "_arm64"):
				conds = append(conds, eq("sys_platform", "win32")+andOr(true)+eq("platform_machine", "ARM64"))
			default:
				conds = append(conds, eq("sys_platform", "win32"))
			}
		}
	}

	if len(conds) == 0 {
		return ""
	}
	return strings.Join(conds, andOr(true))
}

func translateTagList(tags []*vpython.PEP425Tag, invert bool) string {
	var subConds []string
	for _, tag := range tags {
		if tag == nil {
			continue
		}
		cond := translatePEP425Tag(tag, invert)
		if cond != "" {
			subConds = append(subConds, cond)
		}
	}
	if len(subConds) == 0 {
		return ""
	}
	if len(subConds) == 1 {
		return subConds[0]
	}
	joiner := " or "
	if invert {
		joiner = " and "
	}
	var parenthesized []string
	for _, sc := range subConds {
		parenthesized = append(parenthesized, "("+sc+")")
	}
	return strings.Join(parenthesized, joiner)
}

func buildPEP508Marker(matchTags, notMatchTags []*vpython.PEP425Tag) string {
	matchCond := translateTagList(matchTags, false)
	notMatchCond := translateTagList(notMatchTags, true)

	switch {
	case matchCond == "" && notMatchCond == "":
		return ""
	case matchCond != "" && notMatchCond == "":
		return matchCond
	case matchCond == "" && notMatchCond != "":
		return notMatchCond
	default:
		return "(" + matchCond + ") and (" + notMatchCond + ")"
	}
}
