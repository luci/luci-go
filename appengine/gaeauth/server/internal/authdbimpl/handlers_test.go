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

package authdbimpl

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/service"
	"go.chromium.org/luci/server/router"
)

func TestPubSubHandlers(t *testing.T) {
	ftt.Run("pubSubPush skips when not configured", t, func(t *ftt.Test) {
		c := gaetesting.TestingContext()
		rec := httptest.NewRecorder()
		pubSubPush(&router.Context{
			Writer:  rec,
			Request: makePostRequest(c),
		})
		assert.Loosely(t, rec.Code, should.Equal(200))
		assert.Loosely(t, rec.Body.String(), should.Equal("Auth Service URL is not configured, skipping the message"))
	})

	ftt.Run("pubSubPush works when no notification", t, func(t *ftt.Test) {
		c, _ := setupCtx(t)
		rec := httptest.NewRecorder()
		pubSubPush(&router.Context{
			Writer:  rec,
			Request: makePostRequest(c),
		})
		assert.Loosely(t, rec.Code, should.Equal(200))
		assert.Loosely(t, rec.Body.String(), should.Equal("No new valid AuthDB change notifications"))
	})

	ftt.Run("pubSubPush old notification", t, func(t *ftt.Test) {
		c, srv := setupCtx(t)
		srv.Notification = &service.Notification{Revision: 122} // older than 123
		rec := httptest.NewRecorder()
		pubSubPush(&router.Context{
			Writer:  rec,
			Request: makePostRequest(c),
		})
		assert.Loosely(t, rec.Code, should.Equal(200))
		assert.Loosely(t, rec.Body.String(), should.Equal("Processed PubSub notification for rev 122: 123 -> 123"))
	})

	ftt.Run("pubSubPush fresh notification", t, func(t *ftt.Test) {
		c, srv := setupCtx(t)
		srv.LatestRev = 130
		srv.Notification = &service.Notification{Revision: 124}
		rec := httptest.NewRecorder()
		pubSubPush(&router.Context{
			Writer:  rec,
			Request: makePostRequest(c),
		})
		assert.Loosely(t, rec.Code, should.Equal(200))
		assert.Loosely(t, rec.Body.String(), should.Equal("Processed PubSub notification for rev 124: 123 -> 130"))
	})
}

func setupCtx(t testing.TB) (context.Context, *fakeAuthService) {
	srv := &fakeAuthService{LatestRev: 123}
	c := setAuthService(gaetesting.TestingContext(), srv)
	assert.Loosely(t, ConfigureAuthService(c, "http://base_url", "http://auth-service"), should.BeNil)
	return c, srv
}

func makePostRequest(ctx context.Context) *http.Request {
	req, _ := http.NewRequestWithContext(ctx, "POST", "/doesntmatter", bytes.NewReader(nil))
	return req
}
