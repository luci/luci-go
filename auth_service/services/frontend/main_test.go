// Copyright 2026 The LUCI Authors.
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

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/impl/model"
)

func TestAuthorizeAPIAccess(t *testing.T) {
	t.Parallel()

	ftt.Run("authorizeAPIAccess with UnfilteredAccessGroup", t, func(t *ftt.Test) {
		mw := authorizeAPIAccess([]string{model.UnfilteredAccessGroup})

		t.Run("Anonymous user is denied", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			called := false
			mw(rctx, func(c *router.Context) {
				called = true
			})
			assert.Loosely(t, called, should.BeFalse)
			assert.Loosely(t, rw.Code, should.Equal(http.StatusForbidden))
			assert.Loosely(t, rw.Body.String(), should.ContainSubstring("anonymous identity"))
		})

		t.Run("User not in unfiltered access group is denied", func(t *ftt.Test) {
			ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{"some-other-group"},
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			called := false
			mw(rctx, func(c *router.Context) {
				called = true
			})
			assert.Loosely(t, called, should.BeFalse)
			assert.Loosely(t, rw.Code, should.Equal(http.StatusForbidden))
			assert.Loosely(t, rw.Body.String(), should.ContainSubstring("access denied"))
		})

		t.Run("User in unfiltered access group is allowed", func(t *ftt.Test) {
			ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{model.UnfilteredAccessGroup},
			})
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Writer:  rw,
			}
			called := false
			mw(rctx, func(c *router.Context) {
				called = true
			})
			assert.Loosely(t, called, should.BeTrue)
			assert.Loosely(t, rw.Code, should.Equal(http.StatusOK))
		})
	})
}
