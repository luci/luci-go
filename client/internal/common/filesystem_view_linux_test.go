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

package common

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidatesBlacklist(t *testing.T) {
	type testCase struct {
		desc      string
		blacklist []string
		wantErr   bool
	}

	testCases := []testCase{
		{
			desc:      "no patterns",
			blacklist: []string{},
			wantErr:   false,
		},
		{
			desc:      "good pattern",
			blacklist: []string{"*"},
			wantErr:   false,
		},
		{
			desc:      "good patterns",
			blacklist: []string{"a*", "b*"},
			wantErr:   false,
		},
		{
			desc:      "bad pattern",
			blacklist: []string{"["},
			wantErr:   true,
		},
		{
			desc:      "bad first pattern",
			blacklist: []string{"[", "*"},
			wantErr:   true,
		},
		{
			desc:      "bad last pattern",
			blacklist: []string{"*", "["},
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		Convey(tc.desc, t, func() {
			_, err := NewFilesystemView("/root", tc.blacklist)
			if tc.wantErr {
				So(err, ShouldNotBeNil)
			} else {
				So(err, ShouldBeNil)
			}
		})
	}

}

func TestCalculatesRelativePaths(t *testing.T) {
	type testCase struct {
		desc        string
		root        string
		absPath     string
		wantRelPath string
		wantErr     bool
	}

	testCases := []testCase{
		{
			desc:        "no trailing slash",
			root:        "/a",
			absPath:     "/a/b",
			wantRelPath: "b",
			wantErr:     false,
		},
		{
			desc:        "trailing slash",
			root:        "/a",
			absPath:     "/a/b/",
			wantRelPath: "b",
			wantErr:     false,
		},
		{
			desc:        "no common root",
			root:        "/a",
			absPath:     "/x/y",
			wantRelPath: "../x/y",
			wantErr:     false,
		},
		{
			desc:        "no computable relative path",
			root:        "/a",
			absPath:     "./x/y",
			wantRelPath: "",
			wantErr:     true,
		},
		{
			desc:        "root",
			root:        "/a",
			absPath:     "/a",
			wantRelPath: ".",
			wantErr:     false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		Convey(tc.desc, t, func() {
			fsView, err := NewFilesystemView(tc.root, nil)

			So(err, ShouldBeNil)

			relPath, err := fsView.RelativePath(tc.absPath)
			So(relPath, ShouldEqual, tc.wantRelPath)
			if tc.wantErr {
				So(err, ShouldNotBeNil)
			} else {
				So(err, ShouldBeNil)
			}
		})
	}
}

func TestAppliesBlacklist(t *testing.T) {
	type testCase struct {
		desc        string
		root        string
		blacklist   []string
		absPath     string
		wantRelPath string
	}

	testCases := []testCase{
		{
			desc:        "no blacklist",
			root:        "/a",
			blacklist:   []string{},
			absPath:     "/a/x/y",
			wantRelPath: "x/y",
		},
		{
			desc:        "blacklist matches relative path",
			root:        "/a",
			blacklist:   []string{"?/z"},
			absPath:     "/a/x/z",
			wantRelPath: "",
		},
		{
			desc:        "blacklist doesn't match relative path",
			root:        "/a",
			blacklist:   []string{"?/z"},
			absPath:     "/a/x/y",
			wantRelPath: "x/y",
		},
		{
			desc:        "blacklist matches basename",
			root:        "/a",
			blacklist:   []string{"z"},
			absPath:     "/a/x/z",
			wantRelPath: "",
		},
		{
			desc:        "blacklist doesn't match basename",
			root:        "/a",
			blacklist:   []string{"z"},
			absPath:     "/a/z/y",
			wantRelPath: "z/y",
		},
		{
			desc:        "root never matches blacklist",
			root:        "/a",
			blacklist:   []string{"?"},
			absPath:     "/a",
			wantRelPath: ".",
		},
		{
			desc:        "only one blacklist need match path (1)",
			root:        "/a",
			blacklist:   []string{"z", "abc"},
			absPath:     "/a/x/z",
			wantRelPath: "",
		},
		{
			desc:        "only one blacklist need match path (2)",
			root:        "/a",
			blacklist:   []string{"abc", "z"},
			absPath:     "/a/x/z",
			wantRelPath: "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		Convey(tc.desc, t, func() {
			fsView, err := NewFilesystemView(tc.root, tc.blacklist)

			// These test cases contain only valid blacklists.
			// Invalid blacklists are tested in TestValidatesBlacklist.
			So(err, ShouldBeNil)

			relPath, err := fsView.RelativePath(tc.absPath)
			// These test cases contain only valid relative paths.
			// non-computable relative paths are tested in TestCalculatesRelativePaths.
			So(err, ShouldBeNil)
			So(relPath, ShouldEqual, tc.wantRelPath)
		})
	}
}
