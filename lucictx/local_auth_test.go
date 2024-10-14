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

package lucictx

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLocalAuth(t *testing.T) {
	t.Parallel()

	ftt.Run("SwitchLocalAccount works", t, func(t *ftt.Test) {
		c := context.Background()

		t.Run("No local_auth at all", func(t *ftt.Test) {
			ctx, err := SwitchLocalAccount(c, "some")
			assert.Loosely(t, ctx, should.BeNil)
			assert.Loosely(t, err, should.Equal(ErrNoLocalAuthAccount))
		})

		t.Run("Noop change", func(t *ftt.Test) {
			c := SetLocalAuth(c, &LocalAuth{
				DefaultAccountId: "some",
				Accounts: []*LocalAuthAccount{
					{Id: "some"},
				},
			})
			ctx, err := SwitchLocalAccount(c, "some")
			assert.Loosely(t, ctx, should.Equal(c))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("Switching into existing account", func(t *ftt.Test) {
			c := SetLocalAuth(c, &LocalAuth{
				DefaultAccountId: "one",
				Accounts: []*LocalAuthAccount{
					{Id: "one"},
					{Id: "two"},
				},
			})
			ctx, err := SwitchLocalAccount(c, "two")
			assert.Loosely(t, ctx, should.NotBeNil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, GetLocalAuth(ctx).DefaultAccountId, should.Equal("two"))
		})

		t.Run("Switching into non-existing account", func(t *ftt.Test) {
			c := SetLocalAuth(c, &LocalAuth{
				DefaultAccountId: "one",
				Accounts: []*LocalAuthAccount{
					{Id: "one"},
				},
			})
			ctx, err := SwitchLocalAccount(c, "two")
			assert.Loosely(t, ctx, should.BeNil)
			assert.Loosely(t, err, should.Equal(ErrNoLocalAuthAccount))
		})

		t.Run("Clearing local auth", func(t *ftt.Test) {
			c := SetLocalAuth(c, &LocalAuth{
				DefaultAccountId: "one",
				Accounts: []*LocalAuthAccount{
					{Id: "some"},
				},
			})
			c = SetLocalAuth(c, nil)

			assert.Loosely(t, GetLocalAuth(c), should.BeNil)

			ctx, err := SwitchLocalAccount(c, "some")
			assert.Loosely(t, ctx, should.BeNil)
			assert.Loosely(t, err, should.Equal(ErrNoLocalAuthAccount))
		})
	})
}
