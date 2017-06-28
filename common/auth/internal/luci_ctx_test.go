// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

	"github.com/luci/luci-go/common/auth/localauth/rpcs"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/lucictx"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestLUCIContextProvider(t *testing.T) {
	t.Parallel()

	Convey("Requires local_auth", t, func() {
		_, err := NewLUCIContextTokenProvider(context.Background(), []string{"A"}, http.DefaultTransport)
		So(err, ShouldErrLike, `no "local_auth" in LUCI_CONTEXT`)
	})

	Convey("Requires default_account_id", t, func() {
		ctx := context.Background()
		ctx = lucictx.SetLocalAuth(ctx, &lucictx.LocalAuth{
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

		ctx := context.Background()
		ctx = lucictx.SetLocalAuth(ctx, &lucictx.LocalAuth{
			RPCPort:          uint32(ts.Listener.Addr().(*net.TCPAddr).Port),
			Secret:           []byte("zekret"),
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
			So(tok, ShouldResemble, &oauth2.Token{
				AccessToken: "zzz",
				TokenType:   "Bearer",
				Expiry:      time.Unix(1487456796, 0).UTC(),
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
