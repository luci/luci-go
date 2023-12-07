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
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetRPCTransport(t *testing.T) {
	t.Parallel()

	const ownServiceAccountName = "service-own-sa@example.com"

	Convey("GetRPCTransport works", t, func() {
		ctx := context.Background()
		mock := &clientRPCTransportMock{}
		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.AccessTokenProvider = mock.getAccessToken
			cfg.AnonymousTransport = mock.getTransport
			cfg.Signer = signingtest.NewSigner(&signing.ServiceInfo{
				ServiceAccountName: ownServiceAccountName,
			})
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

		Convey("in AsSelf mode with ID token, static aud", func(c C) {
			mocks := &rpcMocks{
				MintIDTokenForServiceAccount: func(ic context.Context, p MintIDTokenParams) (*Token, error) {
					So(p, ShouldResemble, MintIDTokenParams{
						ServiceAccount: ownServiceAccountName,
						Audience:       "https://example.com/aud",
						MinTTL:         2 * time.Minute,
					})
					return &Token{
						Token:  "id-token",
						Expiry: clock.Now(ic).Add(time.Hour),
					}, nil
				},
			}

			t, err := GetRPCTransport(ctx, AsSelf, WithIDTokenAudience("https://example.com/aud"), mocks)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://another.example.com"))
			So(err, ShouldBeNil)

			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer id-token"},
			})
		})

		Convey("in AsSelf mode with ID token, pattern aud", func(c C) {
			mocks := &rpcMocks{
				MintIDTokenForServiceAccount: func(ic context.Context, p MintIDTokenParams) (*Token, error) {
					So(p, ShouldResemble, MintIDTokenParams{
						ServiceAccount: ownServiceAccountName,
						Audience:       "https://another.example.com:443/aud",
						MinTTL:         2 * time.Minute,
					})
					return &Token{
						Token:  "id-token",
						Expiry: clock.Now(ic).Add(time.Hour),
					}, nil
				},
			}

			t, err := GetRPCTransport(ctx, AsSelf, WithIDTokenAudience("https://${host}/aud"), mocks)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://another.example.com:443"))
			So(err, ShouldBeNil)

			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer id-token"},
			})
		})

		Convey("in AsUser mode, authenticated", func(c C) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: "user:abc@example.com"},
			})

			t, err := GetRPCTransport(ctx, AsUser, WithDelegationTags("a:b", "c:d"), &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*Token, error) {
					c.So(p, ShouldResemble, DelegationTokenParams{
						TargetHost: "example.com",
						Tags:       []string{"a:b", "c:d"},
						MinTTL:     10 * time.Minute,
					})
					return &Token{Token: "deleg_tok"}, nil
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
					MintProjectToken: func(ic context.Context, p ProjectTokenParams) (*Token, error) {
						c.So(p, ShouldResemble, ProjectTokenParams{
							MinTTL:      2 * time.Minute,
							LuciProject: "infra",
							OAuthScopes: defaultOAuthScopes,
						})
						return &Token{
							Token:  "scoped tok",
							Expiry: clock.Now(ctx).Add(time.Hour),
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
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*Token, error) {
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
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*Token, error) {
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

		Convey("in NoAuth mode with ID token, should error", func(c C) {
			_, err := GetRPCTransport(ctx, NoAuth, WithIDTokenAudience("aud"))
			So(err, ShouldNotBeNil)
		})

		Convey("in AsSelf mode with ID token and scopes, should error", func(c C) {
			_, err := GetRPCTransport(ctx, AsSelf, WithScopes("A"), WithIDTokenAudience("aud"))
			So(err, ShouldNotBeNil)
		})

		Convey("in AsSelf mode with bad aud pattern, should error", func(c C) {
			_, err := GetRPCTransport(ctx, AsSelf, WithIDTokenAudience("${huh}"))
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
				endUserExtraHeaders: map[string]string{"X-Extra": "val"},
			})

			t, err := GetRPCTransport(ctx, AsCredentialsForwarder)
			So(err, ShouldBeNil)
			_, err = t.RoundTrip(makeReq("https://example.com"))
			So(err, ShouldBeNil)

			// Passed the token and the extra header.
			So(mock.reqs[0].Header, ShouldResemble, http.Header{
				"Authorization": {"Bearer abc.def"},
				"X-Extra":       {"val"},
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

		Convey("in AsActor mode with account", func(c C) {
			mocks := &rpcMocks{
				MintAccessTokenForServiceAccount: func(ic context.Context, p MintAccessTokenParams) (*Token, error) {
					So(p, ShouldResemble, MintAccessTokenParams{
						ServiceAccount: "abc@example.com",
						Scopes:         []string{auth.OAuthScopeEmail},
						MinTTL:         2 * time.Minute,
					})
					return &Token{
						Token:  "blah-blah",
						Expiry: clock.Now(ic).Add(time.Hour),
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

		Convey("in AsActor mode without account, error", func(c C) {
			_, err := GetRPCTransport(ctx, AsActor)
			So(err, ShouldNotBeNil)
		})

		Convey("in AsProject mode without project, error", func(c C) {
			_, err := GetRPCTransport(ctx, AsProject)
			So(err, ShouldNotBeNil)
		})

		Convey("in AsSessionUser mode without session", func(c C) {
			_, err := GetRPCTransport(ctx, AsSessionUser)
			So(err, ShouldEqual, nil)
		})

		Convey("in AsSessionUser mode", func(c C) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: "user:abc@example.com"},
				session: &fakeSession{
					accessToken: &oauth2.Token{
						TokenType:   "Bearer",
						AccessToken: "access_token",
					},
					idToken: &oauth2.Token{
						TokenType:   "Bearer",
						AccessToken: "id_token",
					},
				},
			})

			Convey("OAuth2 token", func() {
				t, err := GetRPCTransport(ctx, AsSessionUser)
				So(err, ShouldBeNil)
				_, err = t.RoundTrip(makeReq("https://example.com"))
				So(err, ShouldBeNil)
				So(mock.reqs[0].Header, ShouldResemble, http.Header{
					"Authorization": {"Bearer access_token"},
				})
			})

			Convey("ID token", func() {
				t, err := GetRPCTransport(ctx, AsSessionUser, WithIDToken())
				So(err, ShouldBeNil)
				_, err = t.RoundTrip(makeReq("https://example.com"))
				So(err, ShouldBeNil)
				So(mock.reqs[0].Header, ShouldResemble, http.Header{
					"Authorization": {"Bearer id_token"},
				})
			})

			Convey("Trying to override scopes", func() {
				_, err := GetRPCTransport(ctx, AsSessionUser, WithScopes("a"))
				So(err, ShouldNotBeNil)
			})

			Convey("Trying to override aud", func() {
				_, err := GetRPCTransport(ctx, AsSessionUser, WithIDTokenAudience("aud"))
				So(err, ShouldNotBeNil)
			})
		})

		Convey("when headers are needed, Request context is used", func() {
			root := ctx

			// Contexts with different auth state.
			ctx1 := WithState(root, &state{user: &User{Identity: "user:abc@example.com"}})
			ctx2 := WithState(root, &state{user: &User{Identity: "user:abc@example.com"}})

			// Use a mode which actually uses transport context to compute headers.
			run := func(c C, reqCtx, transCtx context.Context) (usedCtx context.Context) {
				mocks := &rpcMocks{
					MintAccessTokenForServiceAccount: func(ic context.Context, _ MintAccessTokenParams) (*Token, error) {
						usedCtx = ic
						return &Token{
							Token:  "blah",
							Expiry: clock.Now(ic).Add(time.Hour),
						}, nil
					},
				}
				t, err := GetRPCTransport(transCtx, AsActor, WithServiceAccount("abc@example.com"), mocks)
				c.So(err, ShouldBeNil)
				req := makeReq("https://example.com")
				if reqCtx != nil {
					req = req.WithContext(reqCtx)
				}
				_, err = t.RoundTrip(req)
				c.So(err, ShouldBeNil)
				return
			}

			Convey("no request context", func(c C) {
				So(run(c, nil, ctx1), ShouldEqual, ctx1)
			})

			Convey("same context", func(c C) {
				So(run(c, ctx1, ctx1), ShouldEqual, ctx1)
			})

			Convey("uses request context", func(c C) {
				reqCtx, cancel := context.WithTimeout(ctx1, time.Minute)
				defer cancel()
				transCtx, cancel := context.WithTimeout(ctx1, time.Hour)
				defer cancel()
				So(run(c, reqCtx, transCtx), ShouldEqual, reqCtx)
			})

			Convey("OK on two background contexts", func(c C) {
				reqCtx, cancel := context.WithTimeout(root, time.Minute)
				defer cancel()
				transCtx, cancel := context.WithTimeout(root, time.Hour)
				defer cancel()
				So(run(c, reqCtx, transCtx), ShouldEqual, reqCtx)
			})

			Convey("request ctx is user, transport is background", func(c C) {
				reqCtx, cancel := context.WithTimeout(ctx1, time.Minute)
				reqCtxDeadline, _ := reqCtx.Deadline()
				defer cancel()
				transCtx, cancel := context.WithTimeout(root, time.Hour)
				defer cancel()
				// Used `reqCtx` for the deadline, but have background auth state.
				usedCtx := run(c, reqCtx, transCtx)
				usedDeadline, _ := usedCtx.Deadline()
				So(usedDeadline.Equal(reqCtxDeadline), ShouldBeTrue)
				So(GetState(usedCtx), ShouldResemble, GetState(transCtx))
			})

			Convey("request ctx is background, transport is user", func(c C) {
				So(func() {
					reqCtx, cancel := context.WithTimeout(root, time.Minute)
					defer cancel()
					transCtx, cancel := context.WithTimeout(ctx1, time.Hour)
					defer cancel()
					run(c, reqCtx, transCtx)
				}, ShouldPanic)
			})

			Convey("panics on contexts with different auth state", func(c C) {
				So(func() {
					reqCtx, cancel := context.WithTimeout(ctx1, time.Minute)
					defer cancel()
					transCtx, cancel := context.WithTimeout(ctx2, time.Hour)
					defer cancel()
					run(c, reqCtx, transCtx)
				}, ShouldPanic)
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

		Convey("With ID token, static aud", func() {
			_, err := GetTokenSource(ctx, AsSelf, WithIDTokenAudience("https://host.example.com"))
			So(err, ShouldBeNil)
		})

		Convey("With ID token, pattern aud", func() {
			_, err := GetTokenSource(ctx, AsSelf, WithIDTokenAudience("https://${host}"))
			So(err, ShouldNotBeNil)
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

func TestParseAudPattern(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		cb, err := parseAudPattern("https://${host}/zzz")
		So(err, ShouldBeNil)

		s, err := cb(&http.Request{
			Host: "something.example.com:443",
		})
		So(err, ShouldBeNil)
		So(s, ShouldEqual, "https://something.example.com:443/zzz")
	})

	Convey("Static", t, func() {
		cb, err := parseAudPattern("no-vars-here")
		So(cb, ShouldBeNil)
		So(err, ShouldBeNil)
	})

	Convey("Malformed", t, func() {
		cb, err := parseAudPattern("aaa-${host)-bbb")
		So(cb, ShouldBeNil)
		So(err, ShouldNotBeNil)
	})

	Convey("Unknown var", t, func() {
		cb, err := parseAudPattern("aaa-${unknown}-bbb")
		So(cb, ShouldBeNil)
		So(err, ShouldNotBeNil)
	})
}

func makeReq(url string) *http.Request {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}
	return req
}

type fakeSession struct {
	accessToken *oauth2.Token
	idToken     *oauth2.Token
}

func (s *fakeSession) AccessToken(ctx context.Context) (*oauth2.Token, error) {
	return s.accessToken, nil
}

func (s *fakeSession) IDToken(ctx context.Context) (*oauth2.Token, error) {
	return s.idToken, nil
}

type clientRPCTransportMock struct {
	calls [][]string
	reqs  []*http.Request

	cb func(req *http.Request, body string) string
}

func (m *clientRPCTransportMock) getAccessToken(ctx context.Context, scopes []string) (*oauth2.Token, error) {
	m.calls = append(m.calls, scopes)
	return &oauth2.Token{
		AccessToken: "as-self-token:" + strings.Join(scopes, ","),
		TokenType:   "Bearer",
	}, nil
}

func (m *clientRPCTransportMock) getTransport(ctx context.Context) http.RoundTripper {
	return m
}

func (m *clientRPCTransportMock) RoundTrip(req *http.Request) (*http.Response, error) {
	m.reqs = append(m.reqs, req)
	code := 500
	resp := "internal error"
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
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
		Body:       io.NopCloser(bytes.NewReader([]byte(resp))),
	}, nil
}
