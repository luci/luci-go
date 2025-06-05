// Copyright 2020 The LUCI Authors.
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

// Package cqdepend parses CQ-Depend directives in CL description.
//
// CQ-Depend directives are special footer lines in CL descriptions.
// Footers are roughly "Key: arbitrary value" lines in the last paragraph of CL
// description, see more at:
// https://commondatastorage.googleapis.com/chrome-infra-docs/flat/depot_tools/docs/html/git-footers.html
//
// CQ-Depend footer starts with "CQ-Depend:" prefix and contains comma
// separated values:
//
//	CQ-Depend: VALUE[,VALUE]*
//
// where each VALUE is:
//
//	[SUBDOMAIN:]NUMBER
//
// where if SUBDOMAIN is specified, it is a subdomain w/o "-review" suffix of a
// Gerrit host, e.g., just "chrome-internal" for
// https://chrome-internal-review.googlesource.com Gerrit host.
//
// For example, if a CL on chromium-review.googlesource.com has the following
// description:
//
//	Example CL with explicit dependencies.
//
//	This: is footer, because it is in the last paragraph.
//	  It may have non "key: value" lines,
//	  the next line says CL depends on chromium-review.googlesource.com/123.
//	CQ-Depend: 123
//	  and multiple CQ-Depend lines are allowed, such as next line saying
//	  this CL also depends on pdfium-review.googlesource.com/987.
//	CQ-Depend: pdfium:987
//	  Prefix is just "pdfium" and "-review" is implicitly inferred.
//	  For practicality, the following is not valid:
//	CQ-Depend: pdfium-review:456
//	  It's OK to specify multiple comma separated CQ-Depend values:
//	CQ-Depend: 123,pdfium:987
//
// Error handling:
//
//   - Valid CQ-Depend lines are ignored entirely if not in the last paragraph
//     of CL's description (because they aren't Git/Gerrit footers).
//     For example, in this description CQ-Depend is ignored:
//
//     Title.
//
//     CQ-Depend: valid:123
//     But this isn't last paragraph.
//
//     Change-Id: Ideadbeef
//     This-is: the last paragraph.
//
//   - Each CQ-Depend line is parsed independently; any invalid ones are
//     silently ignored. For example,
//     CQ-Depend: not valid, and so it is ignored.
//     CQ-Depend: valid:123
//
//   - In a single CQ-Depend line, each value is processed independently,
//     and invalid values are ignored. For example:
//     CQ-Depend: x:123, bad, 456
//     will result in just 2 dependencies: "x:123" and "456".
//
// See also Lint method, which reports all such misuses of CQ-Depend.
package cqdepend

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/git/footer"
)

// cqDependKey is normalized key as used by git footer library.
const cqDependKey = "Cq-Depend"

// Dep is a Gerrit Change representing a dependency.
type Dep struct {
	// Subdomain is set if change is on a different Gerrit host.
	Subdomain string
	Change    int64
}

func (l *Dep) cmp(r Dep) int {
	switch {
	case l.Subdomain < r.Subdomain:
		return 1
	case l.Subdomain > r.Subdomain:
		return -1
	case l.Change < r.Change:
		return 1
	case l.Change > r.Change:
		return -1
	default:
		return 0
	}
}

// Parse returns all valid dependencies without duplicates.
func Parse(description string) (deps []Dep) {
	for _, footerValue := range footer.ParseMessage(description)[cqDependKey] {
		for _, v := range strings.Split(footerValue, ",") {
			if dep, err := parseSingleDep(v); err == nil {
				deps = append(deps, dep)
			}
		}
	}
	if len(deps) <= 1 {
		return deps
	}
	sort.Slice(deps, func(i, j int) bool { return deps[i].cmp(deps[j]) == 1 })
	// Remove duplicates. We don't use the map in the first place, because
	// duplicates are highly unlikely in practice and sorting is nice for
	// determinism.
	l := 0
	for i := 1; i < len(deps); i++ {
		if d := deps[i]; d.cmp(deps[l]) != 0 {
			l += 1
			deps[l] = d
		}
	}
	return deps[:l+1]
}

var valueRegexp = regexp.MustCompile(`^\s*(\w[-\w]*:)?(\d+)\s*$`)

func parseSingleDep(v string) (Dep, error) {
	const errPrefix = "invalid format of CQ-Depend value %q: "
	dep := Dep{}
	switch res := valueRegexp.FindStringSubmatch(v); {
	case len(res) != 3:
		return dep, errors.Fmt(errPrefix+"must match %q", v, valueRegexp.String())
	case strings.HasSuffix(res[1], "-review:"):
		return dep, errors.Fmt(errPrefix+"must not include '-review'", v)
	case strings.HasSuffix(res[1], ":"):
		dep.Subdomain = strings.ToLower(res[1][:len(res[1])-1])
		fallthrough
	default:
		c, err := strconv.ParseInt(res[2], 10, 64)
		if err != nil {
			return dep, errors.Fmt(errPrefix+"change number too large", v)
		}
		dep.Change = c
		return dep, nil
	}
}

// Lint reports warnings or errors regarding CQ-Depend use.
func Lint(description string) error {
	return errors.New("Not implemented")
}
