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
			fsView, err := NewFilesystemView(tc.root, "")

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

func TestAppliesIgnoredPathFilter(t *testing.T) {
	type testCase struct {
		desc          string
		root          string
		ignoredPathRe string
		absPath       string
		wantRelPath   string
	}

	testCases := []testCase{
		{
			desc:        "no filter",
			root:        "/a",
			absPath:     "/a/x/y",
			wantRelPath: "x/y",
		},
		{
			desc:          "ignoredPathsRe matches relative path",
			root:          "/a",
			ignoredPathRe: "x.*",
			absPath:       "/a/x/y/z",
			wantRelPath:   "",
		},
		{
			desc:          "ignoredPathsRe matches substring",
			root:          "/a",
			ignoredPathRe: "y/",
			absPath:       "/a/x/y/z",
			wantRelPath:   "",
		},
		{
			desc:          "ignoredPathsRe matches the beginning",
			root:          "/a",
			ignoredPathRe: "^x/",
			absPath:       "/a/x/y/z",
			wantRelPath:   "",
		},
		{
			desc:          "ignoredPathsRe does not match the beginning",
			root:          "/a",
			ignoredPathRe: "^y/",
			absPath:       "/a/x/y/z",
			wantRelPath:   "x/y/z",
		},
		{
			desc:          "ignoredPathsRe matches the ending",
			root:          "/a",
			ignoredPathRe: "\\.pyc$",
			absPath:       "/a/x/y.pyc",
			wantRelPath:   "",
		},
		{
			desc:          "root never matches ignoredPathsRe",
			root:          "/a",
			ignoredPathRe: ".*",
			absPath:       "/a",
			wantRelPath:   ".",
		},
		{
			desc:          "only one ignoredPathsRe needs to match path",
			root:          "/a",
			ignoredPathRe: "(.*z)|(abc)",
			absPath:       "/a/x/z",
			wantRelPath:   "",
		},
	}

	for _, tc := range testCases {
		tc := tc
		Convey(tc.desc, t, func() {
			fsView, err := NewFilesystemView(tc.root, tc.ignoredPathRe)

			// These test cases contain only valid file path filter.
			So(err, ShouldBeNil)

			relPath, err := fsView.RelativePath(tc.absPath)
			// These test cases contain only valid relative paths.
			// non-computable relative paths are tested in TestCalculatesRelativePaths.
			So(err, ShouldBeNil)
			So(relPath, ShouldEqual, tc.wantRelPath)
		})
	}
}
