// Copyright 2022 The LUCI Authors.
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

package botsrv

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/tokenserver/auth/machine"

	configpb "go.chromium.org/luci/swarming/proto/config"
	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsession"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/pyproxy"
)

type testRequest struct {
	Session []byte
}

func (r *testRequest) ExtractSession() []byte   { return r.Session }
func (r *testRequest) ExtractDebugRequest() any { return r }

func TestBotHandler(t *testing.T) {
	t.Parallel()

	ftt.Run("With server", t, func(t *ftt.Test) {
		now := time.Date(2044, time.April, 4, 4, 4, 4, 4, time.UTC)
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, now)

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "bot:ignored",
			UserExtra: &machine.MachineTokenInfo{
				FQDN: "good-bot.fqdn",
			},
		})

		goodDims := []string{
			"id:good-bot",
			"pool:some-pool",
			"os:Linux",
		}

		srv := &Server{
			router: router.New(),
			hmacSecret: hmactoken.NewStaticSecret(secrets.Secret{
				Active:  []byte("secret"),
				Passive: [][]byte{[]byte("also-secret")},
			}),
			cfg: cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs()),
			knownBots: func(ctx context.Context, botID string) (*KnownBotInfo, error) {
				if botID == "good-bot" {
					return &KnownBotInfo{
						SessionID:  "good-session-id",
						Dimensions: goodDims,
					}, nil
				}
				return nil, nil
			},
		}

		goodBotAuth := &configpb.BotAuth{
			RequireLuciMachineToken: true,
		}

		var lastBody *testRequest
		var lastRequest *Request
		var nextResponse Response
		var nextError error
		JSON(srv, "/with-session", func(_ context.Context, body *testRequest, r *Request) (Response, error) {
			lastBody = body
			lastRequest = r
			return nextResponse, nextError
		})
		NoSessionJSON(srv, "/no-session", func(_ context.Context, body *testRequest, r *Request) (Response, error) {
			lastBody = body
			lastRequest = r
			return nextResponse, nextError
		})

		callRaw := func(uri string, body []byte, ct string, mockedResp Response, mockedErr error) (b *testRequest, req *Request, status int, resp string) {
			lastRequest = nil
			nextResponse = mockedResp
			nextError = mockedErr
			rq := httptest.NewRequest("POST", uri, bytes.NewReader(body)).WithContext(ctx)
			if ct != "" {
				rq.Header.Set("Content-Type", ct)
			}
			rw := httptest.NewRecorder()
			srv.router.ServeHTTP(rw, rq)
			res := rw.Result()
			if res.StatusCode == http.StatusOK {
				assert.Loosely(t, res.Header.Get("Content-Type"), should.Equal("application/json; charset=utf-8"))
			}
			respBody, _ := io.ReadAll(res.Body)
			return lastBody, lastRequest, res.StatusCode, string(respBody)
		}

		call := func(uri string, body testRequest, mockedResp Response, mockedErr error) (b *testRequest, req *Request, status int, resp string) {
			blob, err := json.Marshal(&body)
			assert.NoErr(t, err)
			return callRaw(uri, blob, "application/json; charset=utf-8", mockedResp, mockedErr)
		}

		makeSession := func(botID, sessionID string, ba *configpb.BotAuth) *internalspb.Session {
			return &internalspb.Session{
				BotId:     botID,
				SessionId: sessionID,
				Expiry:    timestamppb.New(now.Add(5 * time.Minute)),
				BotConfig: &internalspb.BotConfig{
					Expiry:  timestamppb.New(now.Add(5 * time.Minute)),
					BotAuth: []*configpb.BotAuth{ba},
				},
				LastSeenConfig: timestamppb.New(now),
			}
		}

		genSessionToken := func(s *internalspb.Session) []byte {
			tok, err := botsession.Marshal(s, srv.hmacSecret)
			if err != nil {
				panic(err)
			}
			return tok
		}

		t.Run("Happy path", func(t *ftt.Test) {
			t.Run("With session", func(t *ftt.Test) {
				session := makeSession("good-bot", "sid", goodBotAuth)
				req := testRequest{
					Session: genSessionToken(session),
				}

				body, seenReq, status, resp := call("/with-session", req, "some-response", nil)
				assert.Loosely(t, status, should.Equal(http.StatusOK))
				assert.Loosely(t, resp, should.Equal("\"some-response\"\n"))
				assert.Loosely(t, body, should.Match(&req))
				assert.Loosely(t, seenReq.Session, should.Match(session))
				assert.Loosely(t, seenReq.Dimensions, should.Match(goodDims))
			})

			t.Run("No session", func(t *ftt.Test) {
				req := testRequest{
					Session: []byte("ignored"),
				}
				body, seenReq, status, resp := call("/no-session", req, "some-response", nil)
				assert.Loosely(t, status, should.Equal(http.StatusOK))
				assert.Loosely(t, resp, should.Equal("\"some-response\"\n"))
				assert.Loosely(t, body, should.Match(&req))
				assert.Loosely(t, seenReq.Session, should.BeNil)
				assert.Loosely(t, seenReq.Dimensions, should.BeNil)
			})
		})

		t.Run("Bad Content-Type", func(t *ftt.Test) {
			for _, uri := range []string{"/with-session", "/no-session"} {
				_, seenReq, status, resp := callRaw(uri, []byte("ignored"), "application/x-www-form-urlencoded", nil, nil)
				assert.Loosely(t, seenReq, should.BeNil)
				assert.Loosely(t, status, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, resp, should.ContainSubstring("bad content type"))
			}
		})

		t.Run("Not JSON", func(t *ftt.Test) {
			for _, uri := range []string{"/with-session", "/no-session"} {
				_, seenReq, status, resp := callRaw(uri, []byte("what is this"), "application/json; charset=utf-8", nil, nil)
				assert.Loosely(t, seenReq, should.BeNil)
				assert.Loosely(t, status, should.Equal(http.StatusBadRequest))
				assert.Loosely(t, resp, should.ContainSubstring("failed to deserialized"))
			}
		})

		t.Run("No session token", func(t *ftt.Test) {
			req := testRequest{}

			_, seenReq, status, resp := call("/with-session", req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("no session token"))
		})

		t.Run("Malformed session token", func(t *ftt.Test) {
			req := testRequest{
				Session: []byte("not-a-token"),
			}

			_, seenReq, status, resp := call("/with-session", req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("failed to verify or deserialize session token"))
		})

		t.Run("Broken session proto", func(t *ftt.Test) {
			session := makeSession("good-bot", "sid", goodBotAuth)
			session.BotConfig = nil

			req := testRequest{
				Session: genSessionToken(session),
			}

			_, seenReq, status, resp := call("/with-session", req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusInternalServerError))
			assert.Loosely(t, resp, should.ContainSubstring("session proto is broken: no bot_config"))
		})

		t.Run("Expired session token", func(t *ftt.Test) {
			session := makeSession("good-bot", "sid", goodBotAuth)
			session.Expiry = timestamppb.New(now.Add(-5 * time.Minute))

			req := testRequest{
				Session: genSessionToken(session),
			}

			_, seenReq, status, resp := call("/with-session", req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("session token has expired 5m0s ago"))
		})

		t.Run("Unauthorized bot", func(t *ftt.Test) {
			session := makeSession("good-bot", "sid", &configpb.BotAuth{
				RequireServiceAccount: []string{"something-else@example.com"},
			})

			req := testRequest{
				Session: genSessionToken(session),
			}

			_, seenReq, status, resp := call("/with-session", req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusUnauthorized))
			assert.Loosely(t, resp, should.ContainSubstring("bad bot credentials"))
		})

		t.Run("Unknown bot", func(t *ftt.Test) {
			session := makeSession("good-bot", "sid", goodBotAuth)
			srv.knownBots = func(ctx context.Context, botID string) (*KnownBotInfo, error) {
				return nil, nil // all bots are unknown
			}

			req := testRequest{
				Session: genSessionToken(session),
			}

			_, seenReq, status, resp := call("/with-session", req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusForbidden))
			assert.Loosely(t, resp, should.ContainSubstring("is not a registered bot"))
		})

		t.Run("Bad bot dimensions in datastore", func(t *ftt.Test) {
			session := makeSession("good-bot", "sid", goodBotAuth)
			srv.knownBots = func(ctx context.Context, botID string) (*KnownBotInfo, error) {
				return &KnownBotInfo{
					SessionID:  "sid",
					Dimensions: []string{"id:wrong-id"},
				}, nil
			}

			req := testRequest{
				Session: genSessionToken(session),
			}

			_, seenReq, status, resp := call("/with-session", req, "some-response", nil)
			assert.Loosely(t, seenReq, should.BeNil)
			assert.Loosely(t, status, should.Equal(http.StatusInternalServerError))
			assert.Loosely(t, resp, should.ContainSubstring("wrong stored \"id\" dimension"))
		})
	})
}

func TestPythonProxying(t *testing.T) {
	t.Parallel()

	ftt.Run("With server", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = mathrand.Set(ctx, rand.New(rand.NewSource(123)))

		var m sync.Mutex
		var calls []string

		recordCall := func(call string) {
			m.Lock()
			defer m.Unlock()
			calls = append(calls, call)
		}

		collectCalls := func() []string {
			m.Lock()
			defer m.Unlock()
			out := calls
			calls = nil
			return out
		}

		// A server representing Python.
		pySrv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			recordCall("py:" + req.URL.RequestURI())
		}))
		defer pySrv.Close()

		cfg := cfgtest.MockConfigs(ctx, &cfgtest.MockedConfigs{
			Settings: &configpb.SettingsCfg{
				TrafficMigration: &configpb.TrafficMigration{
					Routes: []*configpb.TrafficMigration_Route{
						// Should be ignore: Go only route.
						{Name: "/swarming/api/v1/bot/rbe/something", RouteToGoPercent: 0},
						// Always routed to Py.
						{Name: "/swarming/api/v1/bot/python", RouteToGoPercent: 0},
						// Always routed to Go.
						{Name: "/swarming/api/v1/bot/go", RouteToGoPercent: 100},
						// Random chance.
						{Name: "/swarming/api/v1/bot/rand", RouteToGoPercent: 20},
					},
				},
			},
		})

		goSrv := router.New()
		middlewares := []router.Middleware{
			pythonProxyMiddleware(pyproxy.NewProxy(cfg, pySrv.URL)),
		}

		routes := []string{
			"/swarming/api/v1/bot/rbe/something",
			"/swarming/api/v1/bot/rbe/something-else",
			"/swarming/api/v1/bot/unconfigured",
			"/swarming/api/v1/bot/python",
			"/swarming/api/v1/bot/python/:Param",
			"/swarming/api/v1/bot/go",
			"/swarming/api/v1/bot/go/:Param",
			"/swarming/api/v1/bot/rand",
			"/swarming/api/v1/bot/rand/:Param",
		}
		for _, route := range routes {
			goSrv.POST(route, middlewares, func(ctx *router.Context) {
				recordCall("go:" + ctx.Request.URL.RequestURI())
			})
		}

		post := func(uri string) {
			req := httptest.NewRequest("POST", uri, strings.NewReader("{}"))
			rr := httptest.NewRecorder()
			goSrv.ServeHTTP(rr, req.WithContext(ctx))
			assert.That(t, rr.Code, should.Equal(200))
		}

		// Routes unconditionally routed to Go.
		post("/swarming/api/v1/bot/rbe/something")
		post("/swarming/api/v1/bot/rbe/something-else")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"go:/swarming/api/v1/bot/rbe/something",
			"go:/swarming/api/v1/bot/rbe/something-else",
		}))

		// Routes not in the config are routed to Py.
		post("/swarming/api/v1/bot/unconfigured")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"py:/swarming/api/v1/bot/unconfigured",
		}))

		// Routes configured to always route to Py.
		post("/swarming/api/v1/bot/python")
		post("/swarming/api/v1/bot/python/arg")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"py:/swarming/api/v1/bot/python",
			"py:/swarming/api/v1/bot/python/arg",
		}))

		// Routes configured to always route to Go.
		post("/swarming/api/v1/bot/go")
		post("/swarming/api/v1/bot/go/arg")
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"go:/swarming/api/v1/bot/go",
			"go:/swarming/api/v1/bot/go/arg",
		}))

		// Routes with random splitting.
		for i := 0; i < 5; i++ {
			post("/swarming/api/v1/bot/rand")
			post("/swarming/api/v1/bot/rand/arg")
		}
		assert.Loosely(t, collectCalls(), should.Match([]string{
			"py:/swarming/api/v1/bot/rand",
			"py:/swarming/api/v1/bot/rand/arg",
			"go:/swarming/api/v1/bot/rand",
			"py:/swarming/api/v1/bot/rand/arg",
			"py:/swarming/api/v1/bot/rand",
			"py:/swarming/api/v1/bot/rand/arg",
			"py:/swarming/api/v1/bot/rand",
			"go:/swarming/api/v1/bot/rand/arg",
			"py:/swarming/api/v1/bot/rand",
			"go:/swarming/api/v1/bot/rand/arg",
		}))
	})
}

func TestBotDimensions(t *testing.T) {
	t.Parallel()

	dims := BotDimensions{
		"k1:v1",
		"k1:v2",
		"k2:v3",
		"broken",
	}

	assert.That(t, dims.DimensionValues("k1"), should.Match([]string{"v1", "v2"}))
	assert.That(t, dims.DimensionValues("??"), should.Match([]string(nil)))
	assert.That(t, dims.ToMap(), should.Match(map[string][]string{
		"k1": {"v1", "v2"},
		"k2": {"v3"},
	}))
}
