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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/auth/signing/signingtest"
)

func TestGetRPCTransport(t *testing.T) {
	t.Parallel()

	const ownServiceAccountName = "service-own-sa@example.com"

	ftt.Run("GetRPCTransport works", t, func(t *ftt.Test) {
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

		t.Run("in NoAuth mode", func(t *ftt.Test) {
			transp, err := GetRPCTransport(ctx, NoAuth)
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, len(mock.calls), should.BeZero)
			assert.Loosely(t, len(mock.reqs[0].Header), should.BeZero)
		})

		t.Run("in AsSelf mode", func(t *ftt.Test) {
			transp, err := GetRPCTransport(ctx, AsSelf, WithScopes("A", "B"))
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, mock.calls[0], should.Resemble([]string{"A", "B"}))
			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization": {"Bearer as-self-token:A,B"},
			}))
		})

		t.Run("in AsSelf mode with default scopes", func(t *ftt.Test) {
			transp, err := GetRPCTransport(ctx, AsSelf)
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, mock.calls[0], should.Resemble([]string{"https://www.googleapis.com/auth/userinfo.email"}))
			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization": {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
			}))
		})

		t.Run("in AsSelf mode with ID token, static aud", func(t *ftt.Test) {
			mocks := &rpcMocks{
				MintIDTokenForServiceAccount: func(ic context.Context, p MintIDTokenParams) (*Token, error) {
					assert.Loosely(t, p, should.Resemble(MintIDTokenParams{
						ServiceAccount: ownServiceAccountName,
						Audience:       "https://example.com/aud",
						MinTTL:         2 * time.Minute,
					}))
					return &Token{
						Token:  "id-token",
						Expiry: clock.Now(ic).Add(time.Hour),
					}, nil
				},
			}

			transp, err := GetRPCTransport(ctx, AsSelf, WithIDTokenAudience("https://example.com/aud"), mocks)
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://another.example.com"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization": {"Bearer id-token"},
			}))
		})

		t.Run("in AsSelf mode with ID token, pattern aud", func(t *ftt.Test) {
			mocks := &rpcMocks{
				MintIDTokenForServiceAccount: func(ic context.Context, p MintIDTokenParams) (*Token, error) {
					assert.Loosely(t, p, should.Resemble(MintIDTokenParams{
						ServiceAccount: ownServiceAccountName,
						Audience:       "https://another.example.com:443/aud",
						MinTTL:         2 * time.Minute,
					}))
					return &Token{
						Token:  "id-token",
						Expiry: clock.Now(ic).Add(time.Hour),
					}, nil
				},
			}

			transp, err := GetRPCTransport(ctx, AsSelf, WithIDTokenAudience("https://${host}/aud"), mocks)
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://another.example.com:443"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization": {"Bearer id-token"},
			}))
		})

		t.Run("in AsUser mode, authenticated", func(t *ftt.Test) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: "user:abc@example.com"},
			})

			transp, err := GetRPCTransport(ctx, AsUser, WithDelegationTags("a:b", "c:d"), &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*Token, error) {
					assert.Loosely(t, p, should.Resemble(DelegationTokenParams{
						TargetHost: "example.com",
						Tags:       []string{"a:b", "c:d"},
						MinTTL:     10 * time.Minute,
					}))
					return &Token{Token: "deleg_tok"}, nil
				},
			})
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com/some-path/sd"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, mock.calls[0], should.Resemble([]string{"https://www.googleapis.com/auth/userinfo.email"}))
			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization":         {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
				"X-Delegation-Token-V1": {"deleg_tok"},
			}))
		})

		t.Run("in AsProject mode", func(t *ftt.Test) {
			callExampleCom := func(ctx context.Context) {
				transp, err := GetRPCTransport(ctx, AsProject, WithProject("infra"), &rpcMocks{
					MintProjectToken: func(ic context.Context, p ProjectTokenParams) (*Token, error) {
						assert.Loosely(t, p, should.Resemble(ProjectTokenParams{
							MinTTL:      2 * time.Minute,
							LuciProject: "infra",
							OAuthScopes: defaultOAuthScopes,
						}))
						return &Token{
							Token:  "scoped tok",
							Expiry: clock.Now(ctx).Add(time.Hour),
						}, nil
					},
				})
				assert.Loosely(t, err, should.BeNil)
				_, err = transp.RoundTrip(makeReq("https://example.com/some-path/sd"))
				assert.Loosely(t, err, should.BeNil)
			}

			t.Run("external service", func(t *ftt.Test) {
				callExampleCom(WithState(ctx, &state{
					db: &fakeDB{internalService: "not-example.com"},
				}))
				assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
					"Authorization": {"Bearer scoped tok"},
				}))
			})

			t.Run("internal service", func(t *ftt.Test) {
				callExampleCom(WithState(ctx, &state{
					db: &fakeDB{internalService: "example.com"},
				}))
				assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
					"Authorization":  {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
					"X-Luci-Project": {"infra"},
				}))
			})
		})

		t.Run("in AsUser mode, anonymous", func(t *ftt.Test) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: identity.AnonymousIdentity},
			})

			transp, err := GetRPCTransport(ctx, AsUser, &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*Token, error) {
					panic("must not be called")
				},
			})
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{}))
		})

		t.Run("in AsUser mode, with existing token", func(t *ftt.Test) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: identity.AnonymousIdentity},
			})

			transp, err := GetRPCTransport(ctx, AsUser, WithDelegationToken("deleg_tok"), &rpcMocks{
				MintDelegationToken: func(ic context.Context, p DelegationTokenParams) (*Token, error) {
					panic("must not be called")
				},
			})
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, mock.calls[0], should.Resemble([]string{"https://www.googleapis.com/auth/userinfo.email"}))
			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization":         {"Bearer as-self-token:https://www.googleapis.com/auth/userinfo.email"},
				"X-Delegation-Token-V1": {"deleg_tok"},
			}))
		})

		t.Run("in AsUser mode with both delegation tags and token", func(t *ftt.Test) {
			_, err := GetRPCTransport(
				ctx, AsUser, WithDelegationToken("deleg_tok"), WithDelegationTags("a:b"))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in NoAuth mode with delegation tags, should error", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, NoAuth, WithDelegationTags("a:b"))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in NoAuth mode with scopes, should error", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, NoAuth, WithScopes("A"))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in NoAuth mode with ID token, should error", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, NoAuth, WithIDTokenAudience("aud"))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in AsSelf mode with ID token and scopes, should error", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, AsSelf, WithScopes("A"), WithIDTokenAudience("aud"))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in AsSelf mode with bad aud pattern, should error", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, AsSelf, WithIDTokenAudience("${huh}"))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in AsCredentialsForwarder mode, anonymous", func(t *ftt.Test) {
			ctx := WithState(ctx, &state{
				user:       &User{Identity: identity.AnonymousIdentity},
				endUserErr: ErrNoForwardableCreds,
			})

			transp, err := GetRPCTransport(ctx, AsCredentialsForwarder)
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)

			// No credentials passed.
			assert.Loosely(t, mock.reqs[0].Header, should.HaveLength(0))
		})

		t.Run("in AsCredentialsForwarder mode, non-anonymous", func(t *ftt.Test) {
			ctx := WithState(ctx, &state{
				user: &User{Identity: "user:a@example.com"},
				endUserTok: &oauth2.Token{
					TokenType:   "Bearer",
					AccessToken: "abc.def",
				},
				endUserExtraHeaders: map[string]string{"X-Extra": "val"},
			})

			transp, err := GetRPCTransport(ctx, AsCredentialsForwarder)
			assert.Loosely(t, err, should.BeNil)
			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)

			// Passed the token and the extra header.
			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization": {"Bearer abc.def"},
				"X-Extra":       {"val"},
			}))
		})

		t.Run("in AsCredentialsForwarder mode, non-forwardable", func(t *ftt.Test) {
			ctx := WithState(ctx, &state{
				user:       &User{Identity: "user:a@example.com"},
				endUserErr: ErrNoForwardableCreds,
			})

			_, err := GetRPCTransport(ctx, AsCredentialsForwarder)
			assert.Loosely(t, err, should.Equal(ErrNoForwardableCreds))
		})

		t.Run("in AsActor mode with account", func(t *ftt.Test) {
			mocks := &rpcMocks{
				MintAccessTokenForServiceAccount: func(ic context.Context, p MintAccessTokenParams) (*Token, error) {
					assert.Loosely(t, p, should.Resemble(MintAccessTokenParams{
						ServiceAccount: "abc@example.com",
						Scopes:         []string{auth.OAuthScopeEmail},
						MinTTL:         2 * time.Minute,
					}))
					return &Token{
						Token:  "blah-blah",
						Expiry: clock.Now(ic).Add(time.Hour),
					}, nil
				},
			}

			transp, err := GetRPCTransport(ctx, AsActor, WithServiceAccount("abc@example.com"), mocks)
			assert.Loosely(t, err, should.BeNil)

			_, err = transp.RoundTrip(makeReq("https://example.com"))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
				"Authorization": {"Bearer blah-blah"},
			}))
		})

		t.Run("in AsActor mode without account, error", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, AsActor)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in AsProject mode without project, error", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, AsProject)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("in AsSessionUser mode without session", func(t *ftt.Test) {
			_, err := GetRPCTransport(ctx, AsSessionUser)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("in AsSessionUser mode", func(t *ftt.Test) {
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

			t.Run("OAuth2 token", func(t *ftt.Test) {
				transp, err := GetRPCTransport(ctx, AsSessionUser)
				assert.Loosely(t, err, should.BeNil)
				_, err = transp.RoundTrip(makeReq("https://example.com"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
					"Authorization": {"Bearer access_token"},
				}))
			})

			t.Run("ID token", func(t *ftt.Test) {
				transp, err := GetRPCTransport(ctx, AsSessionUser, WithIDToken())
				assert.Loosely(t, err, should.BeNil)
				_, err = transp.RoundTrip(makeReq("https://example.com"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, mock.reqs[0].Header, should.Resemble(http.Header{
					"Authorization": {"Bearer id_token"},
				}))
			})

			t.Run("Trying to override scopes", func(t *ftt.Test) {
				_, err := GetRPCTransport(ctx, AsSessionUser, WithScopes("a"))
				assert.Loosely(t, err, should.NotBeNil)
			})

			t.Run("Trying to override aud", func(t *ftt.Test) {
				_, err := GetRPCTransport(ctx, AsSessionUser, WithIDTokenAudience("aud"))
				assert.Loosely(t, err, should.NotBeNil)
			})
		})

		t.Run("when headers are needed, Request context is used", func(t *ftt.Test) {
			// Contexts with different auth state.
			const (
				anon  = "anonymous:anonymous"
				user1 = "user:1@example.com"
				user2 = "user:2@example.com"
				fail  = "fail"
			)
			bg := ctx
			ctx1 := WithState(ctx, &state{user: &User{Identity: user1}})
			ctx2 := WithState(ctx, &state{user: &User{Identity: user2}})

			// Use a mode which actually uses transport context to compute headers.
			run := func(t testing.TB, reqCtx, transCtx context.Context) (usedUser string) {
				mocks := &rpcMocks{
					MintAccessTokenForServiceAccount: func(ic context.Context, _ MintAccessTokenParams) (*Token, error) {
						if st := GetState(ic); st != nil {
							usedUser = string(st.User().Identity)
						} else {
							usedUser = "???"
						}
						return &Token{
							Token:  "blah",
							Expiry: clock.Now(ic).Add(time.Hour),
						}, nil
					},
				}
				transp, err := GetRPCTransport(transCtx, AsActor, WithServiceAccount("abc@example.com"), mocks)
				assert.Loosely(t, err, should.BeNil)
				req := makeReq("https://example.com")
				if reqCtx != nil {
					req = req.WithContext(reqCtx)
				}
				_, err = transp.RoundTrip(req)
				if err != nil {
					usedUser = fail
				}
				return
			}

			t.Run("Transport is using background context", func(t *ftt.Test) {
				t.Run("no request context", func(t *ftt.Test) {
					assert.Loosely(t, run(t, nil, bg), should.Equal(anon))
				})
				t.Run("background request context", func(t *ftt.Test) {
					assert.Loosely(t, run(t, bg, bg), should.Equal(anon))
				})
				t.Run("user request context: overrides background", func(t *ftt.Test) {
					assert.Loosely(t, run(t, ctx1, bg), should.Equal(user1))
				})
			})

			t.Run("Transport is using a user context", func(t *ftt.Test) {
				t.Run("no request context", func(t *ftt.Test) {
					assert.Loosely(t, run(t, nil, ctx1), should.Equal(user1))
				})
				t.Run("background request context", func(t *ftt.Test) {
					// Note: this is potentially bad behavior but it is not trivial to
					// prevent it. This test exist to document it happens.
					assert.Loosely(t, run(t, bg, ctx1), should.Equal(user1))
				})
				t.Run("same user request context", func(t *ftt.Test) {
					assert.Loosely(t, run(t, ctx1, ctx1), should.Equal(user1))
				})
				t.Run("different user request context: forbidden", func(t *ftt.Test) {
					assert.Loosely(t, run(t, ctx2, ctx1), should.Equal(fail))
				})
			})
		})
	})
}

func TestTokenSource(t *testing.T) {
	t.Parallel()

	ftt.Run("GetTokenSource works", t, func(t *ftt.Test) {
		ctx := context.Background()
		mock := &clientRPCTransportMock{}
		ctx = ModifyConfig(ctx, func(cfg Config) Config {
			cfg.AccessTokenProvider = mock.getAccessToken
			cfg.AnonymousTransport = mock.getTransport
			return cfg
		})

		t.Run("With no scopes", func(t *ftt.Test) {
			ts, err := GetTokenSource(ctx, AsSelf)
			assert.Loosely(t, err, should.BeNil)
			tok, err := ts.Token()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
				AccessToken: "as-self-token:https://www.googleapis.com/auth/userinfo.email",
				TokenType:   "Bearer",
			}))
		})

		t.Run("With a specific list of scopes", func(t *ftt.Test) {
			ts, err := GetTokenSource(ctx, AsSelf, WithScopes("foo", "bar", "baz"))
			assert.Loosely(t, err, should.BeNil)
			tok, err := ts.Token()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
				AccessToken: "as-self-token:foo,bar,baz",
				TokenType:   "Bearer",
			}))
		})

		t.Run("With ID token, static aud", func(t *ftt.Test) {
			_, err := GetTokenSource(ctx, AsSelf, WithIDTokenAudience("https://host.example.com"))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("With ID token, pattern aud", func(t *ftt.Test) {
			_, err := GetTokenSource(ctx, AsSelf, WithIDTokenAudience("https://${host}"))
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("NoAuth is not allowed", func(t *ftt.Test) {
			ts, err := GetTokenSource(ctx, NoAuth)
			assert.Loosely(t, ts, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("AsUser is not allowed", func(t *ftt.Test) {
			ts, err := GetTokenSource(ctx, AsUser)
			assert.Loosely(t, ts, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}

func TestParseAudPattern(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		cb, err := parseAudPattern("https://${host}/zzz")
		assert.Loosely(t, err, should.BeNil)

		s, err := cb(&http.Request{
			Host: "something.example.com:443",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.Equal("https://something.example.com:443/zzz"))
	})

	ftt.Run("Static", t, func(t *ftt.Test) {
		cb, err := parseAudPattern("no-vars-here")
		assert.Loosely(t, cb, should.BeNil)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Malformed", t, func(t *ftt.Test) {
		cb, err := parseAudPattern("aaa-${host)-bbb")
		assert.Loosely(t, cb, should.BeNil)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Unknown var", t, func(t *ftt.Test) {
		cb, err := parseAudPattern("aaa-${unknown}-bbb")
		assert.Loosely(t, cb, should.BeNil)
		assert.Loosely(t, err, should.NotBeNil)
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
