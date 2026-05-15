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

package ensure

import (
	"bytes"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/template"
)

var badEnsureFiles = []struct {
	name string
	file string
	opts *ResolveOptions
	err  string
}{
	{
		"too many tokens",
		"this has too many tokens",
		nil,
		"bad version",
	},

	{
		"no version",
		"just/a/package",
		nil,
		"bad version",
	},

	{
		"empty directive",
		"@ foobar",
		nil,
		`unknown @directive: "@"`,
	},

	{
		"unknown directive",
		"@nerbs foobar",
		nil,
		`unknown @directive: "@nerbs"`,
	},

	{
		"windows subdir",
		"@subdir folder\\thing",
		nil,
		`bad subdir "folder\\thing": backslashes are not allowed`,
	},

	{
		"messy subdir",
		"@subdir folder/../something",
		nil,
		`bad subdir "folder/../something": should be simplified to "something"`,
	},

	{
		"relative subdir",
		"@subdir ../../something",
		nil,
		`bad subdir "../../something": contains disallowed dot-path prefix`,
	},

	{
		"absolute subdir",
		"@subdir /etc",
		nil,
		`bad subdir "/etc": absolute paths are not allowed`,
	},

	{
		"extra slashes",
		"@subdir //foo/bar/baz",
		nil,
		`bad subdir`,
	},

	{
		"windows style",
		"@subdir c:/foo/bar/baz",
		nil,
		`bad subdir`,
	},

	{
		"invalid expansion",
		"@subdir $${os=linux}",
		nil,
		`bad subdir "$${os=linux}"`,
	},

	{
		"empty setting",
		"$ something",
		nil,
		`unknown $setting: "$"`,
	},

	{
		"bad url",
		"$serviceurl ://sad.url",
		nil,
		"url is invalid",
	},

	{
		"bad paranoid mode",
		"$ParanoidMode ZZZ",
		nil,
		`unrecognized paranoid mode`,
	},

	{
		"bad override mode",
		"$overrideinstallmode not_athing",
		nil,
		"invalid install mode",
	},

	{
		"symlink override mode",
		"$overrideinstallmode symlink",
		nil,
		"only copy mode is allowed",
	},

	{
		"too many urls",
		f(
			"$serviceurl https://something.example.com",
			"$serviceurl https://something.else.example.com",
		),
		nil,
		"$ServiceURL may only be set once per file",
	},

	{
		"bad setting",
		"$nurbs thingy",
		nil,
		`unknown $setting: "$nurbs"`,
	},

	{
		"bad template",
		"foo/bar/${not_good} version",
		nil,
		`failed to expand package template (line 1): unknown variable "${not_good}"`,
	},

	{
		"bad template (2)",
		"foo/bar/$not_good version",
		nil,
		"unable to process some variables",
	},

	{
		"duplicate package (literal)",
		f(
			"some/package/something version",
			"some/package/something latest",
		),
		nil,
		`duplicate package in subdir "": "some/package/something": defined on line 1 and 2`,
	},

	{
		"duplicate package (literal with explicit options)",
		f(
			"some/package/something version",
			"some/package/something latest",
		),
		&ResolveOptions{AllowDuplicates: false},
		`duplicate package in subdir "": "some/package/something": defined on line 1 and 2`,
	},

	{
		"duplicate package (template)",
		f(
			"some/package/${arch} version",
			"some/other/package canary",
			"some/package/test_arch latest",
		),
		nil,
		`duplicate package in subdir "": "some/package/test_arch": defined on line 1 and 3`,
	},

	{
		"bad version resolution",
		f(
			"some/package/something error_version",
		),
		nil,
		`failed to resolve some/package/something@error_version (line 1): testResolver returned error`,
	},

	{
		"bad version resolution in multiple packages",
		f(
			"some/package/something1 error_version",
			"some/package/something2 error_version",
		),
		nil,
		// errors are sorted by line number
		"err[0]: failed to resolve some/package/something1@error_version (line 1): testResolver returned error\n" +
			"err[1]: failed to resolve some/package/something2@error_version (line 2): testResolver returned error",
	},

	{
		"late setting (pkg)",
		f(
			"some/package version",
			"",
			"$ServiceURL https://something.example.com",
		),
		nil,
		`$setting found after non-$setting statements`,
	},

	{
		"late setting (directive)",
		f(
			"@Subdir some/path",
			"",
			"$ServiceURL https://something.example.com",
		),
		nil,
		`$setting found after non-$setting statements`,
	},

	{
		"verify bad platform",
		f(
			"$VerifiedPlatform foo-bar baz",
		),
		nil,
		`invalid platform entry #2: platform must be <os>-<arch>, got "baz"`,
	},
}

func TestBadEnsureFiles(t *testing.T) {
	t.Parallel()

	ftt.Run("bad ensure files", t, func(t *ftt.Test) {
		for _, tc := range badEnsureFiles {
			t.Run(tc.name, func(t *ftt.Test) {
				buf := bytes.NewBufferString(tc.file)
				f, err := ParseFile(buf)
				if err != nil {
					assert.Loosely(t, f, should.BeNil)
					assert.Loosely(t, err, should.ErrLike(tc.err))
				} else {
					assert.Loosely(t, f, should.NotBeNil)
					rf, err := f.Resolve(testResolver,
						tc.opts, template.Expander{
							"os":       "test_os",
							"arch":     "test_arch",
							"platform": "test_os-test_arch",
						})
					assert.Loosely(t, rf, should.BeNil)
					assert.Loosely(t, err, should.ErrLike(tc.err))
				}
			})
		}
	})
}
