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
	"context"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth/delegation"

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
				"Authorization": {"Bearer as-self-token:A,B"},
			})
		})

		Convey("in AsSelf mode with default scopes", func(c C) {
			t, err := GetRPCTransport(ctx, AsSelf)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			So(mock.calls[0], ShouldResemble, []string{"https://www.googleapis.com/auth/userinfo.email"})
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
			})
		})

		Convey("in AsUser mode, authenticated", func(c C) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: "user:abc@example.com"},
			})

			t, err := GetRPCTransport(ctx, AsUser, WithDelegationTags("a:b", "c:d"), &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*delegation.Token, error) {
					c.So(p, ShouldResemble, DelegationTokenParams{
						TargetHost: "example.com",
						Tags:       []string{"a:b", "c:d"},
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
				"Authorization":         {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
				"X-Delegation-Token-V1": {"deleg_tok"},
			})
		})

		Convey("in AsProject mode", func(c C) {
			callExampleCom := func(ctx context.Context) {
				t, err := GetRPCTransport(ctx, AsProject, WithProject("infra"), &rpcMocks{
					MintProjectToken: func(ic context.Context, p ProjectTokenParams) (*oauth2.Token, error) {
						c.So(p, ShouldResemble, ProjectTokenParams{
							MinTTL:      2 * time.Minute,
							LuciProject: "infra",
							OAuthScopes: defaultOAuthScopes,
						})
						return &oauth2.Token{
							AccessToken: "scoped tok",
							TokenType:   "Bearer",
						}, nil
					},
				})
				So(err, ShouldBeNil)
				_, err = t.RoundTrip(makeReq("https://example.com/some-path/sd"))
				So(err, ShouldBeNil)
			}

			Convey("external service", func() {
				callExampleCom(WithState(ctx, &state{
					db: &fakeDB{internalService: "not-example.com"},
				}))
				So(mock.reqs[0].Header, ShouldResemble, http.Header{
					"Authorization": {"Bearer scoped tok"},
				})
			})

			Convey("internal service", func() {
				callExampleCom(WithState(ctx, &state{
					db: &fakeDB{internalService: "example.com"},
				}))
				So(mock.reqs[0].Header, ShouldResemble, http.Header{
					"Authorization":  {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
					"X-Luci-Project": {"infra"},
				})
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
				"Authorization":         {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
				"X-Delegation-Token-V1": {"deleg_tok"},
			})
		})

		Convey("in AsUser mode with both delegation tags and token", func(c C) {
			_, err := GetRPCTransport(
				ctx, AsUser, WithDelegationToken("deleg_tok"), WithDelegationTags("a:b"))
			So(err, ShouldNotBeNil)
		})

		Convey("in NoAuth mode with delegation tags, should error", func(c C) {
			_, err := GetRPCTransport(ctx, NoAuth, WithDelegationTags("a:b"))
			So(err, ShouldNotBeNil)
		})

		Convey("in NoAuth mode with scopes, should error", func(c C) {
			_, err := GetRPCTransport(ctx, NoAuth, WithScopes("A"))
			So(err, ShouldNotBeNil)
		})

		Convey("in AsCredentialsForwarder mode, anonymous", func(c C) {
			ctx := WithState(ctx, &state{
				user:       &User{Identity: identity.AnonymousIdentity},
				endUserErr: ErrNoForwardableCreds,
			})

			t, err := GetRPCTransport(ctx, AsCredentialsForwarder)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			// No credentials passed.
			So(mock.reqs[0].Header, ShouldHaveLength, 0)
		})

		Convey("in AsCredentialsForwarder mode, non-anonymous", func(c C) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: "user:a@example.com"},
				endUserTok: &oauth2.Token{
					TokenType:   "Bearer",
					AccessToken: "abc.def",
				},
			})

			t, err := GetRPCTransport(ctx, AsCredentialsForwarder)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			// Passed the token.
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer abc.def"},
			})
		})

		Convey("in AsCredentialsForwarder mode, non-forwardable", func(c C) {
			ctx := WithState(ctx, &state{
				user:       &User{Identity: "user:a@example.com"},
				endUserErr: ErrNoForwardableCreds,
			})

			_, err := GetRPCTransport(ctx, AsCredentialsForwarder)
			So(err, ShouldEqual, ErrNoForwardableCreds)
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

		Convey("in AsProject mode without project, error", func(C C) {
			_, err := GetRPCTransport(ctx, AsProject)
			So(err, ShouldNotBeNil)
		})

		Convey("when headers are needed, Request context deadline is obeyed", func() {
			// Use a mode which actually uses transport context to compute headers.
			run := func(C C, reqCtx, transCtx context.Context) (usedDeadline time.Time, set bool) {
				mocks := &rpcMocks{
					MintAccessTokenForServiceAccount: func(ic context.Context, _ MintAccessTokenParams) (*oauth2.Token, error) {
						usedDeadline, set = ic.Deadline()
						return &oauth2.Token{TokenType: "Bearer", AccessToken: "blah"}, nil
					},
				}
				t, err := GetRPCTransport(transCtx, AsActor, WithServiceAccount("abc@example.com"), mocks)
				C.So(err, ShouldBeNil)
				req := makeReq("https://example.com")
				if reqCtx != nil {
					req = req.WithContext(reqCtx)
				}
				_, err = t.RoundTrip(req)
				C.So(err, ShouldBeNil)
				return
			}

			Convey("no request context", func(C C) {
				_, set := run(C, nil, ctx)
				So(set, ShouldBeFalse)
			})

			Convey("no deadlines", func(C C) {
				_, set := run(C, ctx, ctx)
				So(set, ShouldBeFalse)
			})

			Convey("must be before request deadline", func() {
				reqCtx, reqCtxCancel := context.WithTimeout(ctx, time.Minute)
				defer reqCtxCancel()
				reqDeadline, _ := reqCtx.Deadline()

				Convey("no transport deadline", func(C C) {
					usedDeadline, set := run(C, reqCtx, ctx)
					So(set, ShouldBeTrue)
					So(usedDeadline, ShouldEqual, reqDeadline)
				})
				Convey("later transport deadline", func(C C) {
					transCtx, transCtxCancel := context.WithTimeout(ctx, time.Hour)
					defer transCtxCancel()
					usedDeadline, set := run(C, reqCtx, transCtx)
					So(set, ShouldBeTrue)
					So(usedDeadline, ShouldEqual, reqDeadline)
				})
				Convey("earlier transport deadline used as is", func(C C) {
					transCtx, transCtxCancel := context.WithTimeout(ctx, time.Second)
					defer transCtxCancel()
					transDeadline, _ := transCtx.Deadline()
					usedDeadline, set := run(C, reqCtx, transCtx)
					So(set, ShouldBeTrue)
					So(usedDeadline, ShouldEqual, transDeadline)
				})
			})
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
				AccessToken: "as-self-token:https://www.googleapis.com/auth/userinfo.email",
				TokenType:   "Bearer",
			})
		})

		Convey("With a specific list of scopes", func() {
			ts, err := GetTokenSource(ctx, AsSelf, WithScopes("foo", "bar", "baz"))
			So(err, ShouldBeNil)
			tok, err := ts.Token()
			So(err, ShouldBeNil)
			So(tok, ShouldResemble, &oauth2.Token{
				AccessToken: "as-self-token:foo,bar,baz",
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
		AccessToken: "as-self-token:" + strings.Join(scopes, ","),
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
