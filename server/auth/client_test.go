// Copyright 2016 The LUCI Authors.
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

package auth

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/server/auth/delegation"
	"github.com/luci/luci-go/server/auth/identity"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRPCTransport(t *testing.T) {
	t.Parallel()

	Convey("GetRPCTransport works", t, func() {
		ctx := context.Background()
		mock := &clientRPCTransportMock{}
		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.AccessTokenProvider = mock.getAccessToken
			cfg.AnonymousTransport = mock.getTransport
			return cfg
		})

		Convey("in NoAuth mode", func(c C) {
			t, err := GetRPCTransport(ctx, NoAuth)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			So(len(mock.calls), ShouldEqual, 0)
			So(len(mock.reqs[0].Header), ShouldEqual, 0)
		})

		Convey("in AsSelf mode", func(c C) {
			t, err := GetRPCTransport(ctx, AsSelf, WithScopes("A", "B"))
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			So(mock.calls[0], ShouldResemble, []string{"A", "B"})
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer blah:A,B"},
			})
		})

		Convey("in AsSelf mode with default scopes", func(c C) {
			t, err := GetRPCTransport(ctx, AsSelf)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			So(mock.calls[0], ShouldResemble, []string{"https://www.googleapis.com/auth/userinfo.email"})
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer blah:https://www.googleapis.com/auth/userinfo.email"},
			})
		})

		Convey("in AsUser mode, authenticated", func(c C) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: "user:abc@example.com"},
			})

			t, err := GetRPCTransport(ctx, AsUser, &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*delegation.Token, error) {
					c.So(p, ShouldResemble, DelegationTokenParams{
						TargetHost: "example.com",
						MinTTL:     10 * time.Minute,
					})
					return &delegation.Token{Token: "deleg_tok"}, nil
				},
			})
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com/some-path/sd"))
			So(err, ShouldBeNil)

			So(mock.calls[0], ShouldResemble, []string{"https://www.googleapis.com/auth/userinfo.email"})
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization":         {"Bearer blah:https://www.googleapis.com/auth/userinfo.email"},
				"X-Delegation-Token-V1": {"deleg_tok"},
			})
		})

		Convey("in AsUser mode, anonymous", func(c C) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: identity.AnonymousIdentity},
			})

			t, err := GetRPCTransport(ctx, AsUser, &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*delegation.Token, error) {
					panic("must not be called")
				},
			})
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)
			So(mock.reqs[0].Header, ShouldResemble, http.Header{})
		})

		Convey("in AsUser mode, with existing token", func(c C) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: identity.AnonymousIdentity},
			})

			t, err := GetRPCTransport(ctx, AsUser, WithDelegationToken("deleg_tok"), &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*delegation.Token, error) {
					panic("must not be called")
				},
			})
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			So(mock.calls[0], ShouldResemble, []string{"https://www.googleapis.com/auth/userinfo.email"})
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization":         {"Bearer blah:https://www.googleapis.com/auth/userinfo.email"},
				"X-Delegation-Token-V1": {"deleg_tok"},
			})
		})

		Convey("in NoAuth mode with scopes, should error", func(c C) {
			_, err := GetRPCTransport(ctx, NoAuth, WithScopes("A"))
			So(err, ShouldNotBeNil)
		})

		Convey("in AsActor mode with account", func(C C) {
			mocks := &rpcMocks{
				MintAccessTokenForServiceAccount: func(ic context.Context, p MintAccessTokenParams) (*oauth2.Token, error) {
					So(p, ShouldResemble, MintAccessTokenParams{
						ServiceAccount: "abc@example.com",
						Scopes:         []string{auth.OAuthScopeEmail},
						MinTTL:         2 * time.Minute,
					})
					return &oauth2.Token{
						TokenType:   "Bearer",
						AccessToken: "blah-blah",
					}, nil
				},
			}

			t, err := GetRPCTransport(ctx, AsActor, WithServiceAccount("abc@example.com"), mocks)
			So(err, ShouldBeNil)

			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer blah-blah"},
			})
		})

		Convey("in AsActor mode without account, error", func(C C) {
			_, err := GetRPCTransport(ctx, AsActor)
			So(err, ShouldNotBeNil)
		})
	})
}

func TestTokenSource(t *testing.T) {
	t.Parallel()

	Convey("GetTokenSource works", t, func() {
		ctx := context.Background()
		mock := &clientRPCTransportMock{}
		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.AccessTokenProvider = mock.getAccessToken
			cfg.AnonymousTransport = mock.getTransport
			return cfg
		})

		Convey("With no scopes", func() {
			ts, err := GetTokenSource(ctx, AsSelf)
			So(err, ShouldBeNil)
			tok, err := ts.Token()
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &oauth2.Token{
				AccessToken: "blah:https://www.googleapis.com/auth/userinfo.email",
				TokenType:   "Bearer",
			})
		})

		Convey("With a specific list of scopes", func() {
			ts, err := GetTokenSource(ctx, AsSelf, WithScopes("foo", "bar", "baz"))
			So(err, ShouldBeNil)
			tok, err := ts.Token()
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &oauth2.Token{
				AccessToken: "blah:foo,bar,baz",
				TokenType:   "Bearer",
			})
		})

		Convey("NoAuth is not allowed", func() {
			ts, err := GetTokenSource(ctx, NoAuth)
			So(ts, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

		Convey("AsUser is not allowed", func() {
			ts, err := GetTokenSource(ctx, AsUser)
			So(ts, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
	})
}

func makeReq(url string) *http.Request {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	return req
}

type clientRPCTransportMock struct {
	calls [][]string
	reqs  []*http.Request

	cb func(req *http.Request, body string) string
}

func (m *clientRPCTransportMock) getAccessToken(c context.Context, scopes []string) (*oauth2.Token, error) {
	m.calls = append(m.calls, scopes)
	return &oauth2.Token{
		AccessToken: "blah:" + strings.Join(scopes, ","),
		TokenType:   "Bearer",
	}, nil
}

func (m *clientRPCTransportMock) getTransport(c context.Context) http.RoundTripper {
	return m
}

func (m *clientRPCTransportMock) RoundTrip(req *http.Request) (*http.Response, error) {
	m.reqs = append(m.reqs, req)
	code := 500
	resp := "internal error"
	if req.Body != nil {
		body, err := ioutil.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		if m.cb != nil {
			code = 200
			resp = m.cb(req, string(body))
		}
	}
	return &http.Response{
		StatusCode: code,
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(resp))),
	}, nil
}
