// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package python

import (
	"fmt"
	"sort"
	"testing"

	"github.com/luci/luci-go/common/data/rand/mathrand"

	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestParseVersion(t *testing.T) {
	t.Parallel()

	successes := []struct {
		input string
		v     Version
	}{
		{"", Version{}},
		{"2", Version{2, 0, 0}},
		{"2.7", Version{2, 7, 0}},
		{"2.7.12", Version{2, 7, 12}},
		{"1000.0.1", Version{1000, 0, 1}},
	}

	failures := []struct {
		input string
		err   string
	}{
		{"a", "invalid number value"},
		{"1.2.3.4", "unsupported number of parts"},
		{"1.-2.1337", "must not be negative"},
		{"0.1.2", "version is incomplete"},
		{"1.1.X", "invalid patch value"},
		{"1.Y.1", "invalid minor value"},
		{"Y.1.1", "invalid major value"},
	}

	Convey(`Testing ParseVersion`, t, func() {
		for _, tc := range successes {
			Convey(fmt.Sprintf(`Success: %q`, tc.input), func() {
				v, err := ParseVersion(tc.input)
				So(err, ShouldBeNil)
				So(v, ShouldResemble, tc.v)
			})
		}

		for _, tc := range failures {
			Convey(fmt.Sprintf(`Failure: %q (%s)`, tc.input, tc.err), func() {
				_, err := ParseVersion(tc.input)
				So(err, ShouldErrLike, tc.err)
			})
		}
	})
}

func TestVersionSatisfied(t *testing.T) {
	t.Parallel()

	successes := []struct {
		base  Version
		other Version
	}{
		// Zero can be satisfied by anything.
		{Version{}, Version{1, 0, 0}},
		{Version{}, Version{1, 1, 0}},
		{Version{}, Version{1, 2, 3}},

		// Major version matches.
		{Version{2, 0, 0}, Version{2, 0, 0}},
		{Version{2, 0, 0}, Version{2, 7, 0}},
		{Version{2, 0, 0}, Version{2, 7, 8}},

		// Matching major, no patch, >= minor.
		{Version{2, 3, 0}, Version{2, 3, 0}},
		{Version{2, 3, 0}, Version{2, 3, 1}},
		{Version{2, 3, 0}, Version{2, 5, 0}},

		// Matching major and minor, patch, >= minor.
		{Version{2, 3, 5}, Version{2, 5, 0}},

		// Matching major and minor, >= patch.
		{Version{2, 3, 5}, Version{2, 3, 5}},
		{Version{2, 3, 5}, Version{2, 3, 6}},
	}

	failures := []struct {
		base  Version
		other Version
	}{
		// Zero cannot satisfy anything.
		{Version{}, Version{}},
		{Version{1, 2, 3}, Version{}},

		// Differing major versions.
		{Version{2, 0, 0}, Version{1, 0, 0}},
		{Version{2, 0, 0}, Version{3, 0, 0}},

		// Matching major, no patch, < minor.
		{Version{2, 3, 0}, Version{2, 2, 0}},
		{Version{2, 3, 0}, Version{2, 2, 1000}},

		// Matching major and minor, patch, < minor.
		{Version{2, 3, 5}, Version{2, 2, 1000}},

		// Matching major and minor, < patch.
		{Version{2, 3, 5}, Version{2, 3, 0}},
		{Version{2, 3, 5}, Version{2, 3, 4}},
	}

	Convey(`Testing version satisfaction`, t, func() {
		for _, tc := range successes {
			Convey(fmt.Sprintf(`%q is satisfied by %q`, tc.base, tc.other), func() {
				So(tc.base.IsSatisfiedBy(tc.other), ShouldBeTrue)
			})
		}

		for _, tc := range failures {
			Convey(fmt.Sprintf(`%q is NOT satisfied by %q`, tc.base, tc.other), func() {
				So(tc.base.IsSatisfiedBy(tc.other), ShouldBeFalse)
			})
		}
	})
}

func TestVersionLess(t *testing.T) {
	t.Parallel()

	Convey(`Testing "Less"`, t, func() {
		s := versionSlice{
			{0, 0, 0},
			{1, 0, 0},
			{1, 0, 1},
			{2, 0, 9},
			{2, 6, 4},
			{2, 7, 6},
			{3, 0, 2},
			{3, 1, 1},
			{3, 4, 0},
		}

		Convey(`Can sort.`, func() {
			cp := append(versionSlice(nil), s...)
			sort.Sort(cp)
			So(cp, ShouldResemble, s)
		})

		Convey(`Can sort reversed.`, func() {
			cp := append(versionSlice(nil), s...)
			for i := 0; i < len(cp)/2; i++ {
				j := len(cp) - i - 1
				cp[i], cp[j] = cp[j], cp[i]
			}

			sort.Sort(cp)
			So(cp, ShouldResemble, s)
		})

		Convey(`Can sort randomized.`, func() {
			mr := mathrand.Get(context.Background())
			cp := append(versionSlice(nil), s...)
			for i := range cp {
				j := mr.Intn(i + 1)
				cp[i], cp[j] = cp[j], cp[i]
			}

			sort.Sort(cp)
			So(cp, ShouldResemble, s)
		})
	})
}

type versionSlice []Version

func (s versionSlice) Len() int           { return len(s) }
func (s versionSlice) Less(i, j int) bool { return s[i].Less(&s[j]) }
func (s versionSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
