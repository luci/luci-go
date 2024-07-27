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

package internal

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/integration/localauth/rpcs"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/lucictx"
)

func TestLUCIContextProvider(t *testing.T) {
	t.Parallel()

	// Clear any existing LUCI_CONTEXT["local_auth"], it may be present if the
	// test runs on a LUCI bot.
	baseCtx := lucictx.Set(context.Background(), "local_auth", nil)

	ftt.Run("Requires local_auth", t, func(t *ftt.Test) {
		_, err := NewLUCIContextTokenProvider(baseCtx, []string{"A"}, "", http.DefaultTransport)
		assert.Loosely(t, err, should.ErrLike(`no "local_auth" in LUCI_CONTEXT`))
	})

	ftt.Run("Requires default_account_id", t, func(t *ftt.Test) {
		ctx := lucictx.SetLocalAuth(baseCtx, &lucictx.LocalAuth{
			Accounts: []*lucictx.LocalAuthAccount{{Id: "zzz"}},
		})
		_, err := NewLUCIContextTokenProvider(ctx, []string{"A"}, "", http.DefaultTransport)
		assert.Loosely(t, err, should.ErrLike(`no "default_account_id"`))
	})

	ftt.Run("With mock server", t, func(c *ftt.Test) {
		requests := make(chan any, 10000)
		responses := make(chan any, 1)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Loosely(c, r.Method, should.Equal("POST"))
			assert.Loosely(c, r.Header.Get("Content-Type"), should.Equal("application/json"))

			switch r.RequestURI {
			case "/rpc/LuciLocalAuthService.GetOAuthToken":
				req := rpcs.GetOAuthTokenRequest{}
				assert.Loosely(c, json.NewDecoder(r.Body).Decode(&req), should.BeNil)
				requests <- req
			case "/rpc/LuciLocalAuthService.GetIDToken":
				req := rpcs.GetIDTokenRequest{}
				assert.Loosely(c, json.NewDecoder(r.Body).Decode(&req), should.BeNil)
				requests <- req
			default:
				http.Error(w, "Unknown method", 404)
			}

			var resp any
			select {
			case resp = <-responses:
			default:
				panic("Unexpected request")
			}

			switch resp := resp.(type) {
			case rpcs.GetOAuthTokenResponse:
				w.WriteHeader(200)
				assert.Loosely(c, json.NewEncoder(w).Encode(resp), should.BeNil)
			case rpcs.GetIDTokenResponse:
				w.WriteHeader(200)
				assert.Loosely(c, json.NewEncoder(w).Encode(resp), should.BeNil)
			case int:
				http.Error(w, http.StatusText(resp), resp)
			default:
				panic("unexpected response type")
			}
		}))
		defer ts.Close()

		ctx := lucictx.SetLocalAuth(baseCtx, &lucictx.LocalAuth{
			RpcPort: uint32(ts.Listener.Addr().(*net.TCPAddr).Port),
			Secret:  []byte("zekret"),
			Accounts: []*lucictx.LocalAuthAccount{
				{Id: "acc_id", Email: "some-acc-email@example.com"},
			},
			DefaultAccountId: "acc_id",
		})

		c.Run("Access tokens", func(c *ftt.Test) {
			p, err := NewLUCIContextTokenProvider(ctx, []string{"B", "A"}, "", http.DefaultTransport)
			assert.Loosely(c, err, should.BeNil)

			c.Run("Happy path", func(c *ftt.Test) {
				responses <- rpcs.GetOAuthTokenResponse{
					AccessToken: "zzz",
					Expiry:      1487456796,
				}

				tok, err := p.MintToken(ctx, nil)
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, tok, should.Resemble(&Token{
					Token: oauth2.Token{
						AccessToken: "zzz",
						TokenType:   "Bearer",
						Expiry:      time.Unix(1487456796, 0).UTC(),
					},
					IDToken: NoIDToken,
					Email:   "some-acc-email@example.com",
				}))

				assert.Loosely(c, <-requests, should.Resemble(rpcs.GetOAuthTokenRequest{
					BaseRequest: rpcs.BaseRequest{
						Secret:    []byte("zekret"),
						AccountID: "acc_id",
					},
					Scopes: []string{"B", "A"},
				}))
			})

			c.Run("HTTP 500", func(c *ftt.Test) {
				responses <- 500
				tok, err := p.MintToken(ctx, nil)
				assert.Loosely(c, tok, should.BeNil)
				assert.Loosely(c, err, should.ErrLike(`local auth - HTTP 500`))
				assert.Loosely(c, transient.Tag.In(err), should.BeTrue)
			})

			c.Run("HTTP 403", func(c *ftt.Test) {
				responses <- 403
				tok, err := p.MintToken(ctx, nil)
				assert.Loosely(c, tok, should.BeNil)
				assert.Loosely(c, err, should.ErrLike(`local auth - HTTP 403`))
				assert.Loosely(c, transient.Tag.In(err), should.BeFalse)
			})

			c.Run("RPC level error", func(c *ftt.Test) {
				responses <- rpcs.GetOAuthTokenResponse{
					BaseResponse: rpcs.BaseResponse{
						ErrorCode:    123,
						ErrorMessage: "omg, error",
					},
				}
				tok, err := p.MintToken(ctx, nil)
				assert.Loosely(c, tok, should.BeNil)
				assert.Loosely(c, err, should.ErrLike(`local auth - RPC code 123: omg, error`))
				assert.Loosely(c, transient.Tag.In(err), should.BeFalse)
			})
		})

		c.Run("ID tokens", func(c *ftt.Test) {
			p, err := NewLUCIContextTokenProvider(ctx, []string{"audience:test-aud"}, "test-aud", http.DefaultTransport)
			assert.Loosely(c, err, should.BeNil)

			c.Run("Happy path", func(c *ftt.Test) {
				responses <- rpcs.GetIDTokenResponse{
					IDToken: "zzz",
					Expiry:  1487456796,
				}

				tok, err := p.MintToken(ctx, nil)
				assert.Loosely(c, err, should.BeNil)
				assert.Loosely(c, tok, should.Resemble(&Token{
					Token: oauth2.Token{
						AccessToken: NoAccessToken,
						TokenType:   "Bearer",
						Expiry:      time.Unix(1487456796, 0).UTC(),
					},
					IDToken: "zzz",
					Email:   "some-acc-email@example.com",
				}))

				assert.Loosely(c, <-requests, should.Resemble(rpcs.GetIDTokenRequest{
					BaseRequest: rpcs.BaseRequest{
						Secret:    []byte("zekret"),
						AccountID: "acc_id",
					},
					Audience: "test-aud",
				}))
			})
		})
	})
}
