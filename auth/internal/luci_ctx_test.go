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
	"go.chromium.org/luci/lucictx"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLUCIContextProvider(t *testing.T) {
	t.Parallel()

	// Clear any existing LUCI_CONTEXT["local_auth"], it may be present if the
	// test runs on a LUCI bot.
	baseCtx := lucictx.Set(context.Background(), "local_auth", nil)

	Convey("Requires local_auth", t, func() {
		_, err := NewLUCIContextTokenProvider(baseCtx, []string{"A"}, "", http.DefaultTransport)
		So(err, ShouldErrLike, `no "local_auth" in LUCI_CONTEXT`)
	})

	Convey("Requires default_account_id", t, func() {
		ctx := lucictx.SetLocalAuth(baseCtx, &lucictx.LocalAuth{
			Accounts: []*lucictx.LocalAuthAccount{{Id: "zzz"}},
		})
		_, err := NewLUCIContextTokenProvider(ctx, []string{"A"}, "", http.DefaultTransport)
		So(err, ShouldErrLike, `no "default_account_id"`)
	})

	Convey("With mock server", t, func(c C) {
		requests := make(chan any, 10000)
		responses := make(chan any, 1)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Method, ShouldEqual, "POST")
			c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/json")

			switch r.RequestURI {
			case "/rpc/LuciLocalAuthService.GetOAuthToken":
				req := rpcs.GetOAuthTokenRequest{}
				c.So(json.NewDecoder(r.Body).Decode(&req), ShouldBeNil)
				requests <- req
			case "/rpc/LuciLocalAuthService.GetIDToken":
				req := rpcs.GetIDTokenRequest{}
				c.So(json.NewDecoder(r.Body).Decode(&req), ShouldBeNil)
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
				c.So(json.NewEncoder(w).Encode(resp), ShouldBeNil)
			case rpcs.GetIDTokenResponse:
				w.WriteHeader(200)
				c.So(json.NewEncoder(w).Encode(resp), ShouldBeNil)
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

		Convey("Access tokens", func() {
			p, err := NewLUCIContextTokenProvider(ctx, []string{"B", "A"}, "", http.DefaultTransport)
			So(err, ShouldBeNil)

			Convey("Happy path", func() {
				responses <- rpcs.GetOAuthTokenResponse{
					AccessToken: "zzz",
					Expiry:      1487456796,
				}

				tok, err := p.MintToken(ctx, nil)
				So(err, ShouldBeNil)
				So(tok, ShouldResemble, &Token{
					Token: oauth2.Token{
						AccessToken: "zzz",
						TokenType:   "Bearer",
						Expiry:      time.Unix(1487456796, 0).UTC(),
					},
					IDToken: NoIDToken,
					Email:   "some-acc-email@example.com",
				})

				So(<-requests, ShouldResemble, rpcs.GetOAuthTokenRequest{
					BaseRequest: rpcs.BaseRequest{
						Secret:    []byte("zekret"),
						AccountID: "acc_id",
					},
					Scopes: []string{"B", "A"},
				})
			})

			Convey("HTTP 500", func() {
				responses <- 500
				tok, err := p.MintToken(ctx, nil)
				So(tok, ShouldBeNil)
				So(err, ShouldErrLike, `local auth - HTTP 500`)
				So(transient.Tag.In(err), ShouldBeTrue)
			})

			Convey("HTTP 403", func() {
				responses <- 403
				tok, err := p.MintToken(ctx, nil)
				So(tok, ShouldBeNil)
				So(err, ShouldErrLike, `local auth - HTTP 403`)
				So(transient.Tag.In(err), ShouldBeFalse)
			})

			Convey("RPC level error", func() {
				responses <- rpcs.GetOAuthTokenResponse{
					BaseResponse: rpcs.BaseResponse{
						ErrorCode:    123,
						ErrorMessage: "omg, error",
					},
				}
				tok, err := p.MintToken(ctx, nil)
				So(tok, ShouldBeNil)
				So(err, ShouldErrLike, `local auth - RPC code 123: omg, error`)
				So(transient.Tag.In(err), ShouldBeFalse)
			})
		})

		Convey("ID tokens", func() {
			p, err := NewLUCIContextTokenProvider(ctx, []string{"audience:test-aud"}, "test-aud", http.DefaultTransport)
			So(err, ShouldBeNil)

			Convey("Happy path", func() {
				responses <- rpcs.GetIDTokenResponse{
					IDToken: "zzz",
					Expiry:  1487456796,
				}

				tok, err := p.MintToken(ctx, nil)
				So(err, ShouldBeNil)
				So(tok, ShouldResemble, &Token{
					Token: oauth2.Token{
						AccessToken: NoAccessToken,
						TokenType:   "Bearer",
						Expiry:      time.Unix(1487456796, 0).UTC(),
					},
					IDToken: "zzz",
					Email:   "some-acc-email@example.com",
				})

				So(<-requests, ShouldResemble, rpcs.GetIDTokenRequest{
					BaseRequest: rpcs.BaseRequest{
						Secret:    []byte("zekret"),
						AccountID: "acc_id",
					},
					Audience: "test-aud",
				})
			})
		})
	})
}
