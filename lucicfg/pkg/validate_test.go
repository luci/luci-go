// Copyright 2025 The LUCI Authors.
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

package pkg

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  any
	}{
		{"@good", nil},
		{"@also-_goodaz09", nil},
		{"@also/good/blah", nil},
		{"@" + strings.Repeat("z", 299), nil},
		{"bad", "must start with @"},
		{"@BAD", "invalid character at 0 (B)"},
		{"@0bad", "must begin with a letter"},
		{"", "cannot be empty"},
		{"@/zzz", "empty path component"},
		{"@aaa//zzz", "empty path component"},
		{"@" + strings.Repeat("z", 300), "should be no longer than 300 characters"},
	}
	for _, cs := range cases {
		assert.That(t, ValidateName(cs.name), should.ErrLike(cs.err), truth.Explain("%s", cs.name))
	}
}

func TestValidateVersion(t *testing.T) {
	t.Parallel()

	ver, err := ValidateVersion("1.2.3")
	assert.NoErr(t, err)
	assert.That(t, ver.String(), should.Equal("1.2.3"))

	cases := []struct {
		ver string
		err any
	}{
		{"", "expecting <major>.<minor>.<patch>"},
		{"1.2", "expecting <major>.<minor>.<patch>"},
		{"1.2.3.4", "expecting <major>.<minor>.<patch>"},
		{"v1.2.3", "not a positive number"},
		{"-1.2.3", "not a positive number"},
	}
	for _, cs := range cases {
		_, err := ValidateVersion(cs.ver)
		assert.That(t, err, should.ErrLike(cs.err), truth.Explain("%s", cs.ver))
	}
}

func TestVersion(t *testing.T) {
	t.Parallel()

	var zero LucicfgVersion
	assert.That(t, zero.IsZero(), should.BeTrue)
	assert.That(t, zero.String(), should.Equal("0.0.0"))

	v1 := LucicfgVersion{1, 0, 0}
	assert.That(t, v1.IsZero(), should.BeFalse)
	assert.That(t, v1.String(), should.Equal("1.0.0"))

	assert.That(t, v1.Older(v1), should.BeFalse)
	assert.That(t, v1.Older(LucicfgVersion{0, 0, 9}), should.BeFalse)
	assert.That(t, v1.Older(LucicfgVersion{1, 0, 1}), should.BeTrue)
}
