// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package spec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/luci/luci-go/vpython/api/vpython"

	. "github.com/smartystreets/goconvey/convey"
)

func mkTag(version, abi, arch string) *vpython.Pep425Tag {
	return &vpython.Pep425Tag{
		Version: version,
		Abi:     abi,
		Arch:    arch,
	}
}

func tagString(tags []*vpython.Pep425Tag) string {
	parts := make([]string, len(tags))
	for i, tag := range tags {
		parts[i] = tag.TagString()
	}
	return strings.Join(parts, ", ")
}

func TestPEP425Matches(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		tags       []*vpython.Pep425Tag
		matches    []*vpython.Pep425Tag
		notMatches []*vpython.Pep425Tag
	}{
		{
			tags: nil,
			notMatches: []*vpython.Pep425Tag{
				mkTag("", "", ""),
				mkTag("cp27", "cp27mu", "manylinux1_x86_64"),
			},
		},
		{
			tags: []*vpython.Pep425Tag{
				mkTag("cp27", "cp27mu", "manylinux1_x86_64"),
				mkTag("py2", "cp27m", "macosx_10_9_universal"),
			},
			matches: []*vpython.Pep425Tag{
				mkTag("cp27", "", ""),
				mkTag("", "cp27mu", ""),
				mkTag("", "", "manylinux1_x86_64"),
				mkTag("py2", "", ""),
				mkTag("", "cp27m", ""),
				mkTag("", "", "macosx_10_9_universal"),
				mkTag("", "cp27mu", "manylinux1_x86_64"),
			},
			notMatches: []*vpython.Pep425Tag{
				mkTag("", "", ""),
				mkTag("cp27", "cp27mu", "win_amd64"),
				mkTag("cp27", "cp27mu", "macosx_10_9_universal"),
			},
		},
		{
			tags: []*vpython.Pep425Tag{
				mkTag("cp27", "cp27mu", ""),
			},
			matches: []*vpython.Pep425Tag{
				mkTag("cp27", "cp27mu", ""),
			},
			notMatches: []*vpython.Pep425Tag{
				mkTag("", "", ""),
				mkTag("cp27", "cp27mu", "otherArch"),
			},
		},
	}

	Convey(`Test cases for PEP425 tag matching`, t, func() {
		for _, tc := range testCases {
			Convey(fmt.Sprintf(`With system tags: %s`, tagString(tc.tags)), func() {
				for _, m := range tc.matches {
					Convey(fmt.Sprintf(`Tag matches: %s`, m.TagString()), func() {
						So(PEP425Matches(m, tc.tags), ShouldBeTrue)
					})
				}

				for _, m := range tc.notMatches {
					Convey(fmt.Sprintf(`Tag doesn't match: %s`, m.TagString()), func() {
						So(PEP425Matches(m, tc.tags), ShouldBeFalse)
					})
				}
			})
		}
	})
}

func TestPackageMatches(t *testing.T) {
	t.Parallel()

	mkPkg := func(name string, tags ...*vpython.Pep425Tag) *vpython.Spec_Package {
		return &vpython.Spec_Package{
			Name:     name,
			MatchTag: tags,
		}
	}

	testCases := []struct {
		tags         []*vpython.Pep425Tag
		matchPkgs    []*vpython.Spec_Package
		notMatchPkgs []*vpython.Spec_Package
	}{
		{
			tags: nil,
			matchPkgs: []*vpython.Spec_Package{
				mkPkg("NoTags"),
			},
			notMatchPkgs: []*vpython.Spec_Package{
				mkPkg("EmptyMatch", mkTag("", "", "")),
				mkPkg("MissingMatch", mkTag("cp27", "cp27mu", "manylinux1_x86_64")),
			},
		},
		{
			tags: []*vpython.Pep425Tag{
				mkTag("cp27", "cp27mu", "manylinux1_x86_64"),
				mkTag("py2", "cp27m", "macosx_10_9_universal"),
			},
			matchPkgs: []*vpython.Spec_Package{
				mkPkg("NoTags"),
				mkPkg("OneMatchingTag", mkTag("cp27", "", "")),
				mkPkg("MultipleMatchingTag", mkTag("cp27", "", ""), mkTag("", "cp27m", "")),
			},
			notMatchPkgs: []*vpython.Spec_Package{
				mkPkg("EmptyMatch", mkTag("", "", "")),
				mkPkg("MissingMatch", mkTag("none", "none", "none")),
			},
		},
	}

	Convey(`Test cases for package tag matching`, t, func() {
		for _, tc := range testCases {
			Convey(fmt.Sprintf(`With system tags: %s`, tagString(tc.tags)), func() {
				for _, m := range tc.matchPkgs {
					Convey(fmt.Sprintf(`Package %q matches: %s`, m.Name, tagString(m.MatchTag)), func() {
						So(PackageMatches(m, tc.tags), ShouldBeTrue)
					})
				}

				for _, m := range tc.notMatchPkgs {
					Convey(fmt.Sprintf(`Package %q doesn't match: %s`, m.Name, tagString(m.MatchTag)), func() {
						So(PackageMatches(m, tc.tags), ShouldBeFalse)
					})
				}
			})
		}
	})
}
