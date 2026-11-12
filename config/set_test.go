// Copyright 2016 The LUCI Authors.
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

package config

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestServiceSet(t *testing.T) {
	t.Parallel()

	ss, err := ServiceSet("my-service")
	check.NoErr(t, err)
	check.NoErr(t, ss.Validate())
	check.Loosely(t, ss, should.Equal("services/my-service"))

	s := MustServiceSet("foo")
	check.That(t, s.Service(), should.Equal("foo"))
	check.That(t, s.Project(), should.Equal(""))
	check.That(t, s.Domain(), should.Equal(ServiceDomain))
}

func TestProjectSet(t *testing.T) {
	t.Parallel()

	ps, err := ProjectSet("my-project")
	check.NoErr(t, err)
	check.NoErr(t, ps.Validate())
	check.Loosely(t, ps, should.Equal("projects/my-project"))

	ps = MustProjectSet("foo")
	check.That(t, ps.Service(), should.Equal(""))
	check.That(t, ps.Project(), should.Equal("foo"))
	check.That(t, ps.Domain(), should.Equal(ProjectDomain))
}

func TestValidateErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		cs      Set
		wantErr string
	}{
		{
			"malformed/set/abc/def",
			`unknown domain "malformed" for config set "malformed/set/abc/def"; currently supported domains [projects, services]`,
		},
		{
			"services/malformed/service/set/abc",
			`invalid service name "malformed/service/set/abc", expected to match`,
		},
		{
			"projects/malformed/project/set/abc",
			`invalid project name: invalid character`,
		},
		{
			"/abc",
			`can not extract domain from config set "/abc". expected syntax "domain/target"`,
		},
		{
			"unknown/abc",
			`unknown domain "unknown" for config set "unknown/abc"`,
		},
		{
			Set(strings.Repeat("a", MaxConfigSetLength+1)),
			"too long",
		},
	}

	for _, tc := range cases {
		t.Run(string(tc.cs), func(t *testing.T) {
			t.Parallel()

			assert.ErrIsLike(t, tc.cs.Validate(), tc.wantErr)
		})
	}
}
