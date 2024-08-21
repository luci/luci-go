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

package upload

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
)

func TestIDs(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = secrets.Use(ctx, &testsecrets.Store{})
		ctx, clk := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		t.Run("NewOpID works", func(t *ftt.Test) {
			op1, err := NewOpID(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op1, should.Equal(1))

			op2, err := NewOpID(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, op2, should.Equal(2))
		})

		t.Run("Wrap/unwrap works", func(t *ftt.Test) {
			caller := identity.Identity("user:caller@example.com")
			wrapped, err := WrapOpID(ctx, 1234, caller)
			assert.Loosely(t, err, should.BeNil)

			// Ok.
			unwrapped, err := UnwrapOpID(ctx, wrapped, caller)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, unwrapped, should.Equal(1234))

			// Wrong caller ID.
			_, err = UnwrapOpID(ctx, wrapped, "user:another@example.com")
			assert.Loosely(t, err, should.ErrLike("failed to validate"))

			// Expired.
			clk.Add(5*time.Hour + time.Minute)
			_, err = UnwrapOpID(ctx, wrapped, caller)
			assert.Loosely(t, err, should.ErrLike("failed to validate"))
		})
	})
}
