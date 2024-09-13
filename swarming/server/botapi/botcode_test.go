// Copyright 2024 The LUCI Authors.
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

package botapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"

	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/model"
)

func TestBotCode(t *testing.T) {
	t.Parallel()

	const (
		botBootstrapGroup = "test-bot-bootstrap"
		bootstrapperIdent = "user:good-bootstrap@example.com"
		anonymous         = "anonymous:anonymous"
		goodBot           = "good-bot"
		allowedIP         = "192.0.2.1"
		unknownIP         = "192.0.2.2"
	)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = caching.WithEmptyProcessCache(ctx) // for the bootstrap token

		err := datastore.Put(ctx, &model.LegacyBootstrapSecret{
			Key:    model.LegacyBootstrapSecretKey(ctx),
			Values: [][]byte{{0, 1, 2, 3, 4}},
		})
		assert.Loosely(t, err, should.BeNil)

		authDB := authtest.NewFakeDB(
			authtest.MockMembership(bootstrapperIdent, botBootstrapGroup),
			authtest.MockIPAllowlist(allowedIP, "test-project-bots"),
		)

		mockedCfg := cfgtest.NewMockedConfigs()
		mockedCfg.Settings.Auth.BotBootstrapGroup = botBootstrapGroup
		mockedCfg.MockBot(goodBot, "unimportant-pool")
		mockedCfg.MockBotPackage("stable", map[string]string{"version": "stable"})
		mockedCfg.MockBotPackage("canary", map[string]string{"version": "canary"})
		cfg := cfgtest.MockConfigs(ctx, mockedCfg)

		stableDigest := cfg.Cached(ctx).VersionInfo.StableBot.Digest
		canaryDigest := cfg.Cached(ctx).VersionInfo.CanaryBot.Digest

		srv := NewBotAPIServer(cfg, "test-project")
		srv.authorizeBot = func(ctx context.Context, botID string, methods []*configpb.BotAuth) error {
			if botID == goodBot {
				return nil
			}
			return errors.Reason("denied").Err()
		}

		call := func(callerID identity.Identity, peerIP, version string, q url.Values, botIDHeader string) *httptest.ResponseRecorder {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       callerID,
				PeerIPOverride: net.ParseIP(peerIP),
				FakeDB:         authDB,
			})

			uri := "/bot_code"
			if len(q) != 0 {
				uri += "?" + q.Encode()
			}
			req := httptest.NewRequest("GET", uri, nil).WithContext(ctx)
			if botIDHeader != "" {
				req.Header.Set("X-Luci-Swarming-Bot-ID", botIDHeader)
			}

			var params []httprouter.Param
			if version != "" {
				params = append(params, httprouter.Param{
					Key:   "Version",
					Value: version,
				})
			}

			rr := httptest.NewRecorder()
			srv.BotCode(&router.Context{
				Writer:  rr,
				Request: req,
				Params:  params,
			})
			return rr
		}

		t.Run("Happy stable via redirect", func(t *ftt.Test) {
			// Hitting /bot_code redirects to a stable version.
			rr := call(bootstrapperIdent, unknownIP, "", nil, "")
			assert.That(t, rr.Code, should.Equal(http.StatusFound))
			assert.That(t, rr.Result().Header.Get("Location"), should.Equal(
				"/swarming/api/v1/bot/bot_code/"+stableDigest,
			))

			// Hitting the stable version produces the blob.
			rr = call(anonymous, unknownIP, stableDigest, nil, "")
			assert.That(t, rr.Code, should.Equal(http.StatusOK))
			assert.That(t, rr.Result().Header.Get("Cache-Control"), should.Equal("public, max-age=3600"))
			assert.That(t, rr.Body.Len(), should.BeGreaterThan(0))
		})

		t.Run("Hitting canary directly", func(t *ftt.Test) {
			// Hitting the canary version produces the blob.
			rr := call(anonymous, unknownIP, canaryDigest, nil, "")
			assert.That(t, rr.Code, should.Equal(http.StatusOK))
			assert.That(t, rr.Result().Header.Get("Cache-Control"), should.Equal("public, max-age=3600"))
			assert.That(t, rr.Body.Len(), should.BeGreaterThan(0))
		})

		t.Run("Redirect using token", func(t *ftt.Test) {
			tok, err := model.GenerateBootstrapToken(ctx, bootstrapperIdent)
			assert.Loosely(t, err, should.ErrLike(nil))

			// Good.
			rr := call(anonymous, unknownIP, "", url.Values{"tok": {tok}}, "")
			assert.That(t, rr.Code, should.Equal(http.StatusFound))

			// Bad.
			rr = call(anonymous, unknownIP, "", url.Values{"tok": {tok + "zzz"}}, "")
			assert.That(t, rr.Code, should.Equal(http.StatusForbidden))
		})

		t.Run("Redirect using bot auth", func(t *ftt.Test) {
			// Good, using bot_id.
			rr := call(anonymous, unknownIP, "", url.Values{"bot_id": {goodBot}}, "")
			assert.That(t, rr.Code, should.Equal(http.StatusFound))

			// Good, using the header.
			rr = call(anonymous, unknownIP, "", nil, goodBot)
			assert.That(t, rr.Code, should.Equal(http.StatusFound))

			// Bad.
			rr = call(anonymous, unknownIP, "", nil, "unknown")
			assert.That(t, rr.Code, should.Equal(http.StatusForbidden))
		})

		t.Run("Redirect using IP allowlist", func(t *ftt.Test) {
			// Good, using allowed IP.
			rr := call(anonymous, allowedIP, "", nil, "ignored-bot-id")
			assert.That(t, rr.Code, should.Equal(http.StatusFound))

			// Bad, using unknown IP.
			rr = call(anonymous, unknownIP, "", nil, "ignored-bot-id")
			assert.That(t, rr.Code, should.Equal(http.StatusForbidden))
		})

		t.Run("Hitting unknown version", func(t *ftt.Test) {
			rr := call(bootstrapperIdent, unknownIP, "unknown-version", nil, "")
			assert.That(t, rr.Code, should.Equal(http.StatusFound))
			assert.That(t, rr.Result().Header.Get("Location"), should.Equal(
				"/swarming/api/v1/bot/bot_code/"+stableDigest,
			))
		})

		t.Run("Query string stripping", func(t *ftt.Test) {
			rr := call(bootstrapperIdent, unknownIP, stableDigest, url.Values{"bot_id": {"ignored"}}, "")
			assert.That(t, rr.Code, should.Equal(http.StatusFound))
			assert.That(t, rr.Result().Header.Get("Location"), should.Equal(
				"/swarming/api/v1/bot/bot_code/"+stableDigest,
			))
		})
	})
}
