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

package spec

import (
	"github.com/luci/luci-go/vpython/api/vpython"
)

// PackageMatches returns true if any of the match qualifiers in the supplied
// package match at least one of the supplied PEP425 tags.
//
// As a special case, if the package doesn't specify any match tags, it will
// always match regardless of the supplied PEP425 tags.
//
// See PEP425Matches for more information.
func PackageMatches(pkg *vpython.Spec_Package, tags []*vpython.PEP425Tag) bool {
	if len(pkg.MatchTag) == 0 {
		return true
	}

	for _, matchTag := range pkg.MatchTag {
		if PEP425Matches(matchTag, tags) {
			return true
		}
	}
	return false
}

// PEP425Matches returns true if match matches at least one of the tags in tags.
//
// A match is determined if the non-zero fields in match equal the equivalent
// fields in a tag.
func PEP425Matches(match *vpython.PEP425Tag, tags []*vpython.PEP425Tag) bool {
	// Special case: empty match matches nothing.
	if match.IsZero() {
		return false
	}

	for _, tag := range tags {
		if v := match.Python; v != "" && tag.Python != v {
			continue
		}
		if v := match.Abi; v != "" && tag.Abi != v {
			continue
		}
		if v := match.Platform; v != "" && tag.Platform != v {
			continue
		}
		return true
	}

	return false
}
