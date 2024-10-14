// Copyright 2020 The LUCI Authors.
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

package realms

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestValidateProjectName(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		assert.Loosely(t, ValidateProjectName("something-blah"), should.BeNil)
		assert.Loosely(t, ValidateProjectName("@internal"), should.BeNil)

		assert.Loosely(t, ValidateProjectName("something-BLAH"), should.NotBeNil)
		assert.Loosely(t, ValidateProjectName(""), should.NotBeNil)
		assert.Loosely(t, ValidateProjectName(":"), should.NotBeNil)
		assert.Loosely(t, ValidateProjectName("@blah"), should.NotBeNil)
	})
}

func TestValidateRealmName(t *testing.T) {
	t.Parallel()

	ftt.Run("GlobalScope", t, func(t *ftt.Test) {
		call := func(r string) error {
			return ValidateRealmName(r, GlobalScope)
		}

		assert.Loosely(t, call("something-blah:realm"), should.BeNil)
		assert.Loosely(t, call("something-blah:a/b/c"), should.BeNil)
		assert.Loosely(t, call("something-blah:@root"), should.BeNil)
		assert.Loosely(t, call("something-blah:@legacy"), should.BeNil)
		assert.Loosely(t, call("something-blah:@project"), should.BeNil)

		assert.Loosely(t, call("@internal:realm"), should.BeNil)
		assert.Loosely(t, call("@internal:a/b/c"), should.BeNil)
		assert.Loosely(t, call("@internal:@root"), should.BeNil)
		assert.Loosely(t, call("@internal:@legacy"), should.BeNil)
		assert.Loosely(t, call("@internal:@project"), should.BeNil)

		assert.Loosely(t, call("realm"), should.ErrLike("should be <project>:<realm>"))
		assert.Loosely(t, call("BLAH:realm"), should.ErrLike("bad project name"))
		assert.Loosely(t, call("@blah:zzz"), should.ErrLike("bad project name"))
		assert.Loosely(t, call("blah:"), should.ErrLike("the realm name should match"))
		assert.Loosely(t, call("blah:@zzz"), should.ErrLike("the realm name should match"))
	})

	ftt.Run("ProjectScope", t, func(t *ftt.Test) {
		call := func(r string) error {
			return ValidateRealmName(r, ProjectScope)
		}

		assert.Loosely(t, call("realm"), should.BeNil)
		assert.Loosely(t, call("a/b/c"), should.BeNil)
		assert.Loosely(t, call("@root"), should.BeNil)
		assert.Loosely(t, call("@legacy"), should.BeNil)
		assert.Loosely(t, call("@project"), should.BeNil)

		assert.Loosely(t, call("blah:realm"), should.NotBeNil)
		assert.Loosely(t, call(":realm"), should.NotBeNil)
		assert.Loosely(t, call("realm:"), should.NotBeNil)
		assert.Loosely(t, call("@zzz"), should.NotBeNil)
	})
}

func TestRealmSplitJoin(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		a, b := Split("a:b")
		assert.Loosely(t, a, should.Equal("a"))
		assert.Loosely(t, b, should.Equal("b"))

		a, b = Split(":")
		assert.Loosely(t, a, should.BeEmpty)
		assert.Loosely(t, b, should.BeEmpty)

		assert.Loosely(t, func() { Split("ab") }, should.Panic)

		assert.Loosely(t, Join("a", "b"), should.Equal("a:b"))
	})
}
