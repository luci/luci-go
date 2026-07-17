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

package backend

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/gae/service/taskqueue"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/gce/appengine/model"
)

type nonDevInfo struct {
	info.RawInterface
}

func (n *nonDevInfo) IsDevAppServer() bool {
	return false
}

func TestCronSecurity(t *testing.T) {
	t.Parallel()

	ftt.Run("Cron Security", t, func(t *ftt.Test) {
		baseCtx := memory.Use(context.Background())
		baseCtx = authtest.MockAuthConfig(baseCtx)
		baseCtx = auth.WithState(baseCtx, &authtest.FakeState{
			Identity: identity.AnonymousIdentity,
		})
		datastore.GetTestable(baseCtx).Consistent(true)

		// Force IsDevAppServer to return false to enable RequireCron check.
		baseCtx = info.AddFilters(baseCtx, func(ctx context.Context, parent info.RawInterface) info.RawInterface {
			return &nonDevInfo{parent}
		})

		tqTestable := taskqueue.GetTestable(baseCtx)
		for _, q := range []string{
			countVMsQueue, createInstanceQueue, createVMQueue, deleteBotQueue,
			destroyInstanceQueue, expandConfigQueue, manageBotQueue, reportQuotaQueue,
			terminateBotQueue, auditInstancesQueue, drainVMQueue, inspectSwarmingQueue,
			deleteStaleSwarmingBotsQueue,
		} {
			tqTestable.CreateQueue(q)
		}

		if err := datastore.Put(baseCtx, &model.Config{ID: "victim-config"}); err != nil {
			t.Fatalf("seed datastore: %v", err)
		}

		mw := router.NewMiddlewareChain(func(c *router.Context, next router.Handler) {
			c.Request = c.Request.WithContext(baseCtx)
			next(c)
		})

		r := router.New()
		InstallHandlers(r, mw)

		t.Run("Unauthenticated request fails with 403", func(t *ftt.Test) {
			req := httptest.NewRequest(http.MethodGet, "/internal/cron/expand-configs", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			assert.Loosely(t, rec.Code, should.Equal(http.StatusForbidden))
			assert.Loosely(t, tqTestable.GetScheduledTasks()[expandConfigQueue], should.BeEmpty)
		})

		t.Run("Request with X-Appengine-Cron header succeeds", func(t *ftt.Test) {
			req := httptest.NewRequest(http.MethodGet, "/internal/cron/expand-configs", nil)
			req.Header.Set("X-Appengine-Cron", "true")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			assert.Loosely(t, rec.Code, should.Equal(http.StatusOK))
			assert.Loosely(t, tqTestable.GetScheduledTasks()[expandConfigQueue], should.HaveLength(1))
		})
	})
}
