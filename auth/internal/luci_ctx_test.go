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
	baseCtx, err := lucictx.Set(context.Background(), "local_auth", nil)
	if err != nil {
		t.Fatal(err) // this should never happen
	}

	Convey("Requires local_auth", t, func() {
		_, err := NewLUCIContextTokenProvider(baseCtx, []string{"A"}, http.DefaultTransport)
		So(err, ShouldErrLike, `no "local_auth" in LUCI_CONTEXT`)
	})

	Convey("Requires default_account_id", t, func() {
		ctx := lucictx.SetLocalAuth(baseCtx, &lucictx.LocalAuth{
			Accounts: []lucictx.LocalAuthAccount{{ID: "zzz"}},
		})
		_, err := NewLUCIContextTokenProvider(ctx, []string{"A"}, http.DefaultTransport)
		So(err, ShouldErrLike, `no "default_account_id"`)
	})

	Convey("With mock server", t, func(c C) {
		requests := make(chan rpcs.GetOAuthTokenRequest, 10000)
		responses := make(chan interface{}, 1)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c.So(r.Method, ShouldEqual, "POST")
			c.So(r.RequestURI, ShouldEqual, "/rpc/LuciLocalAuthService.GetOAuthToken")
			c.So(r.Header.Get("Content-Type"), ShouldEqual, "application/json")

			req := rpcs.GetOAuthTokenRequest{}
			c.So(json.NewDecoder(r.Body).Decode(&req), ShouldBeNil)

			requests <- req

			var resp interface{}
			select {
			case resp = <-responses:
			default:
				panic("Unexpected request")
			}

			switch resp := resp.(type) {
			case rpcs.GetOAuthTokenResponse:
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
			RPCPort: uint32(ts.Listener.Addr().(*net.TCPAddr).Port),
			Secret:  []byte("zekret"),
			Accounts: []lucictx.LocalAuthAccount{
				{ID: "acc_id", Email: "some-acc-email@example.com"},
			},
			DefaultAccountID: "acc_id",
		})

		p, err := NewLUCIContextTokenProvider(ctx, []string{"B", "A"}, http.DefaultTransport)
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
				Email: "some-acc-email@example.com",
			})

			So(<-requests, ShouldResemble, rpcs.GetOAuthTokenRequest{
				Scopes:    []string{"B", "A"},
				Secret:    []byte("zekret"),
				AccountID: "acc_id",
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
}
