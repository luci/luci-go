// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
func PackageMatches(pkg *vpython.Spec_Package, tags []*vpython.Pep425Tag) bool {
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
func PEP425Matches(match *vpython.Pep425Tag, tags []*vpython.Pep425Tag) bool {
	// Special case: empty match matches nothing.
	if match.IsZero() {
		return false
	}

	for _, tag := range tags {
		if v := match.Version; v != "" && tag.Version != v {
			continue
		}
		if v := match.Abi; v != "" && tag.Abi != v {
			continue
		}
		if v := match.Arch; v != "" && tag.Arch != v {
			continue
		}
		return true
	}

	return false
}
