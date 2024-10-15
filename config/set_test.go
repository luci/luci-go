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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestConfigSet(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing config set utility methods`, t, func(t *ftt.Test) {
		ss, err := ServiceSet("my-service")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ss.Validate(), should.BeNil)
		assert.Loosely(t, ss, should.Equal(Set("services/my-service")))
		ps, err := ProjectSet("my-project")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ps.Validate(), should.BeNil)
		assert.Loosely(t, ps, should.Equal(Set("projects/my-project")))

		s := MustServiceSet("foo")
		assert.Loosely(t, s.Service(), should.Equal("foo"))
		assert.Loosely(t, s.Project(), should.BeEmpty)
		assert.Loosely(t, s.Domain(), should.Equal(ServiceDomain))

		s = MustProjectSet("foo")
		assert.Loosely(t, s.Service(), should.BeEmpty)
		assert.Loosely(t, s.Project(), should.Equal("foo"))
		assert.Loosely(t, s.Domain(), should.Equal(ProjectDomain))

		s = Set("malformed/set/abc/def")
		assert.Loosely(t, s.Validate(), should.ErrLike("unknown domain \"malformed\" for config set \"malformed/set/abc/def\"; currently supported domains [projects, services]"))
		assert.Loosely(t, s.Service(), should.BeEmpty)
		assert.Loosely(t, s.Project(), should.BeEmpty)

		s, err = ServiceSet("malformed/service/set/abc")
		errText := "invalid service name \"malformed/service/set/abc\", expected to match"
		assert.Loosely(t, err, should.ErrLike(errText))
		assert.Loosely(t, s, should.BeEmpty)
		assert.Loosely(t, Set("services/malformed/service/set/abc").Validate(), should.ErrLike(errText))

		s, err = ProjectSet("malformed/project/set/abc")
		errText = "invalid project name: invalid character"
		assert.Loosely(t, err, should.ErrLike(errText))
		assert.Loosely(t, s, should.BeEmpty)
		assert.Loosely(t, Set("projects/malformed/service/set/abc").Validate(), should.ErrLike(errText))

		assert.Loosely(t, Set("/abc").Validate(), should.ErrLike("can not extract domain from config set \"/abc\". expected syntax \"domain/target\""))
		assert.Loosely(t, Set("unknown/abc").Validate(), should.ErrLike("unknown domain \"unknown\" for config set \"unknown/abc\""))
	})
}
