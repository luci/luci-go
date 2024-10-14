// Copyright 2015 The LUCI Authors.
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

package deprecated

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth"
)

func TestSession(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx, _ := testclock.UseTime(context.Background(), time.Unix(1442540000, 0))
		s := MemorySessionStore{}

		ss, err := s.GetSession(ctx, "missing")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ss, should.BeNil)

		sid, err := s.OpenSession(ctx, "uid", &auth.User{Name: "dude"}, clock.Now(ctx).Add(1*time.Hour))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sid, should.Equal("uid/1"))

		ss, err = s.GetSession(ctx, "uid/1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ss, should.Resemble(&Session{
			SessionID: "uid/1",
			UserID:    "uid",
			User:      auth.User{Name: "dude"},
			Exp:       clock.Now(ctx).Add(1 * time.Hour),
		}))

		assert.Loosely(t, s.CloseSession(ctx, "uid/1"), should.BeNil)

		ss, err = s.GetSession(ctx, "missing")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ss, should.BeNil)
	})
}
