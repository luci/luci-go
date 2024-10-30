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

package loginsessions

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/loginsessionspb"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	"go.chromium.org/luci/server/loginsessions/internal"
	"go.chromium.org/luci/server/loginsessions/internal/statepb"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

const mockedAuthorizationEndpoint = "http://localhost/authorization"

func TestModule(t *testing.T) {
	t.Parallel()

	ftt.Run("With module", t, func(t *ftt.Test) {
		var now = testclock.TestRecentTimeUTC.Round(time.Millisecond)

		timestampFromNow := func(d time.Duration) *timestamppb.Timestamp {
			return timestamppb.New(now.Add(d))
		}

		ctx, tc := testclock.UseTime(context.Background(), now)
		ctx = gologger.StdConfig.Use(ctx)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		opts := &ModuleOptions{
			RootURL: "", // set below after we get httptest server
		}
		mod := &loginSessionsModule{
			opts: opts,
			srv: &loginSessionsServer{
				opts:  opts,
				store: &internal.MemorySessionStore{},
				provider: func(_ context.Context, id string) (*internal.OAuthClient, error) {
					if id == "allowed-client-id" {
						return &internal.OAuthClient{
							AuthorizationEndpoint: mockedAuthorizationEndpoint,
						}, nil
					}
					return nil, nil
				},
			},
			insecureCookie: true,
		}

		handler := router.New()
		handler.Use(router.MiddlewareChain{
			func(rc *router.Context, next router.Handler) {
				rc.Request = rc.Request.WithContext(ctx)
				next(rc)
			},
		})
		mod.installRoutes(handler)
		srv := httptest.NewServer(handler)
		defer srv.Close()

		opts.RootURL = srv.URL

		jar, err := cookiejar.New(nil)
		assert.Loosely(t, err, should.BeNil)
		web := &http.Client{Jar: jar}

		// Union of all template args ever passed to templates.
		type templateArgs struct {
			Template            string
			Session             *statepb.LoginSession
			OAuthClient         *internal.OAuthClient
			OAuthState          string
			OAuthRedirectParams map[string]string
			BadCode             bool
			Error               string
		}

		parseWebResponse := func(resp *http.Response, err error) templateArgs {
			assert.Loosely(t, err, should.BeNil)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			assert.Loosely(t, err, should.BeNil)
			var args templateArgs
			assert.Loosely(t, json.Unmarshal(body, &args), should.BeNil)
			return args
		}

		webGET := func(url string) templateArgs {
			return parseWebResponse(web.Get(url))
		}

		webPOST := func(url string, vals url.Values) templateArgs {
			return parseWebResponse(web.PostForm(url, vals))
		}

		createSessionReq := func() *loginsessionspb.CreateLoginSessionRequest {
			return &loginsessionspb.CreateLoginSessionRequest{
				OauthClientId:          "allowed-client-id",
				OauthScopes:            []string{"scope-0", "scope-1"},
				OauthS256CodeChallenge: "code-challenge",
				ExecutableName:         "executable",
				ClientHostname:         "hostname",
			}
		}

		t.Run("CreateLoginSession + GetLoginSession", func(t *ftt.Test) {
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, session, should.Resemble(&loginsessionspb.LoginSession{
				Id:                      session.Id,
				Password:                session.Password,
				State:                   loginsessionspb.LoginSession_PENDING,
				Created:                 timestampFromNow(0),
				Expiry:                  timestampFromNow(sessionExpiry),
				LoginFlowUrl:            fmt.Sprintf("%s/cli/login/%s", srv.URL, session.Id),
				PollInterval:            durationpb.New(time.Second),
				ConfirmationCode:        session.ConfirmationCode,
				ConfirmationCodeExpiry:  durationpb.New(confirmationCodeExpiryMax),
				ConfirmationCodeRefresh: durationpb.New(confirmationCodeExpiryRefresh),
			}))

			pwd := session.Password
			session.Password = nil

			got, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got, should.Resemble(session))

			// Later the confirmation code gets stale and a new one is generated.
			tc.Set(now.Add(confirmationCodeExpiryRefresh + time.Second))
			got1, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got1.ConfirmationCode, should.NotEqual(session.ConfirmationCode))

			// Have two codes in the store right now.
			stored, err := mod.srv.store.Get(ctx, session.Id)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, stored.ConfirmationCodes, should.HaveLength(2))

			// Later the expired code is kicked out of the storage.
			tc.Set(now.Add(confirmationCodeExpiryMax + time.Second))
			got2, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, got2.ConfirmationCode, should.Equal(got1.ConfirmationCode))

			// Have only one code now.
			stored, err = mod.srv.store.Get(ctx, session.Id)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, stored.ConfirmationCodes, should.HaveLength(1))

			// Later the session itself expires.
			tc.Set(now.Add(sessionExpiry + time.Second))
			exp, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, exp, should.Resemble(&loginsessionspb.LoginSession{
				Id:           session.Id,
				State:        loginsessionspb.LoginSession_EXPIRED,
				Created:      timestampFromNow(0),
				Expiry:       timestampFromNow(sessionExpiry),
				Completed:    timestampFromNow(sessionExpiry + time.Second),
				LoginFlowUrl: fmt.Sprintf("%s/cli/login/%s", srv.URL, session.Id),
			}))

			// And this is the final stage.
			tc.Set(now.Add(sessionExpiry + time.Hour))
			exp2, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, exp2, should.Resemble(exp))
		})

		t.Run("CreateLoginSession validation", func(t *ftt.Test) {
			t.Run("Browser headers", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs("sec-fetch-site", "none"))
				_, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})
			t.Run("Missing OAuth client ID", func(t *ftt.Test) {
				req := createSessionReq()
				req.OauthClientId = ""
				_, err := mod.srv.CreateLoginSession(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
			t.Run("Missing OAuth scopes", func(t *ftt.Test) {
				req := createSessionReq()
				req.OauthScopes = nil
				_, err := mod.srv.CreateLoginSession(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
			t.Run("Missing OAuth challenge", func(t *ftt.Test) {
				req := createSessionReq()
				req.OauthS256CodeChallenge = ""
				_, err := mod.srv.CreateLoginSession(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
			t.Run("Unknown OAuth client", func(t *ftt.Test) {
				req := createSessionReq()
				req.OauthClientId = "unknown"
				_, err := mod.srv.CreateLoginSession(ctx, req)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})
		})

		t.Run("GetLoginSession validation", func(t *ftt.Test) {
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			assert.Loosely(t, err, should.BeNil)

			t.Run("Browser headers", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs("sec-fetch-site", "none"))
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       session.Id,
					LoginSessionPassword: session.Password,
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			})
			t.Run("Missing ID", func(t *ftt.Test) {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionPassword: session.Password,
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
			t.Run("Missing password", func(t *ftt.Test) {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId: session.Id,
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
			t.Run("Missing session", func(t *ftt.Test) {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       "missing",
					LoginSessionPassword: session.Password,
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})
			t.Run("Wrong password", func(t *ftt.Test) {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       session.Id,
					LoginSessionPassword: []byte("wrong"),
				})
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})
		})

		t.Run("Full successful flow", func(t *ftt.Test) {
			// The CLI tool starts a new login session.
			sessionReq := createSessionReq()
			session, err := mod.srv.CreateLoginSession(ctx, sessionReq)
			assert.Loosely(t, err, should.BeNil)

			// The user opens the web page with the session and gets the cookie and
			// the authorization endpoint redirect parameters.
			tmpl := webGET(session.LoginFlowUrl)
			assert.Loosely(t, tmpl.Error, should.BeEmpty)
			assert.Loosely(t, tmpl.Template, should.Equal("pages/start.html"))
			assert.Loosely(t, tmpl.OAuthState, should.NotBeEmpty)
			assert.Loosely(t, tmpl.OAuthRedirectParams, should.Resemble(map[string]string{
				"access_type":           "offline",
				"client_id":             sessionReq.OauthClientId,
				"code_challenge":        sessionReq.OauthS256CodeChallenge,
				"code_challenge_method": "S256",
				"nonce":                 session.Id,
				"prompt":                "consent",
				"redirect_uri":          srv.URL + "/cli/confirm",
				"response_type":         "code",
				"scope":                 strings.Join(sessionReq.OauthScopes, " "),
				"state":                 tmpl.OAuthState,
			}))

			// The user goes through the login flow and ends up back with a code.
			// This renders a page asking for the confirmation code.
			confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
				"code":  {"authorization-code"},
				"state": {tmpl.OAuthState},
			}).Encode()
			tmpl = webGET(confirmURL)
			assert.Loosely(t, tmpl.Error, should.BeEmpty)
			assert.Loosely(t, tmpl.Template, should.Equal("pages/confirm.html"))
			assert.Loosely(t, tmpl.OAuthState, should.NotBeEmpty)

			// A correct confirmation code is entered and accepted.
			tmpl = webPOST(confirmURL, url.Values{
				"confirmation_code": {session.ConfirmationCode},
			})
			assert.Loosely(t, tmpl.Error, should.BeEmpty)
			assert.Loosely(t, tmpl.Template, should.Equal("pages/success.html"))

			// The session is completed and the code is returned to the CLI.
			session, err = mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: session.Password,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, session, should.Resemble(&loginsessionspb.LoginSession{
				Id:                     session.Id,
				State:                  loginsessionspb.LoginSession_SUCCEEDED,
				Created:                timestampFromNow(0),
				Expiry:                 timestampFromNow(sessionExpiry),
				Completed:              timestampFromNow(0),
				LoginFlowUrl:           fmt.Sprintf("%s/cli/login/%s", srv.URL, session.Id),
				OauthAuthorizationCode: "authorization-code",
				OauthRedirectUrl:       srv.URL + "/cli/confirm",
			}))

			// Visiting the session page again shows it is gone.
			tmpl = webGET(session.LoginFlowUrl)
			assert.Loosely(t, tmpl.Error, should.ContainSubstring("No such login session"))
		})

		t.Run("Session page errors", func(t *ftt.Test) {
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			assert.Loosely(t, err, should.BeNil)

			t.Run("Wrong session ID", func(t *ftt.Test) {
				// Opening a non-existent session page is an error.
				tmpl := webGET(session.LoginFlowUrl + "extra")
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("No such login session"))
			})

			t.Run("Expired session", func(t *ftt.Test) {
				// Opening an old session is an error.
				tc.Add(sessionExpiry + time.Second)
				tmpl := webGET(session.LoginFlowUrl)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("No such login session"))
			})
		})

		t.Run("Redirect page errors", func(t *ftt.Test) {
			// Create the session and get the session cookie.
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			assert.Loosely(t, err, should.BeNil)
			tmpl := webGET(session.LoginFlowUrl)
			assert.Loosely(t, tmpl.OAuthState, should.NotBeEmpty)

			checkSessionState := func(state loginsessionspb.LoginSession_State, msg string) {
				session, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       session.Id,
					LoginSessionPassword: session.Password,
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, session.State, should.Equal(state))
				assert.Loosely(t, session.OauthError, should.Equal(msg))
			}

			t.Run("OK", func(t *ftt.Test) {
				// Just double check the test setup is correct.
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()
				webPOST(confirmURL, url.Values{
					"confirmation_code": {session.ConfirmationCode},
				})
				checkSessionState(loginsessionspb.LoginSession_SUCCEEDED, "")
			})

			t.Run("No OAuth code or error", func(t *ftt.Test) {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
				}).Encode()
				tmpl = webGET(confirmURL)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("The authorization provider returned error code: unknown"))
				checkSessionState(loginsessionspb.LoginSession_FAILED, "unknown")
			})

			t.Run("OAuth error", func(t *ftt.Test) {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"error": {"boom"},
				}).Encode()
				tmpl = webGET(confirmURL)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("The authorization provider returned error code: boom"))
				checkSessionState(loginsessionspb.LoginSession_FAILED, "boom")
			})

			t.Run("No state", func(t *ftt.Test) {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"error": {"boom"},
				}).Encode()
				tmpl = webGET(confirmURL)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("The authorization provider returned error code: boom"))
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			t.Run("Bad state", func(t *ftt.Test) {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState[:20]},
					"code":  {"authorization-code"},
				}).Encode()
				tmpl = webGET(confirmURL)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("Internal server error"))
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			t.Run("Expired session", func(t *ftt.Test) {
				tc.Add(sessionExpiry + time.Second)
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()
				tmpl = webGET(confirmURL)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("finished or expired"))
				checkSessionState(loginsessionspb.LoginSession_EXPIRED, "")
			})

			t.Run("Missing login cookie", func(t *ftt.Test) {
				emptyJar, err := cookiejar.New(nil)
				assert.Loosely(t, err, should.BeNil)
				web.Jar = emptyJar
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()
				tmpl = webGET(confirmURL)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("finished or expired"))
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			t.Run("Wrong login cookie", func(t *ftt.Test) {
				u, err := url.Parse(srv.URL + "/cli/session")
				assert.Loosely(t, err, should.BeNil)
				jar.SetCookies(u, []*http.Cookie{
					{
						Name:     mod.loginCookieName(session.Id),
						Value:    base64.RawURLEncoding.EncodeToString([]byte("wrong")),
						Path:     "/cli/",
						MaxAge:   100000,
						HttpOnly: true,
					},
				})
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()
				tmpl = webGET(confirmURL)
				assert.Loosely(t, tmpl.Error, should.ContainSubstring("finished or expired"))
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			t.Run("Wrong confirmation code", func(t *ftt.Test) {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()

				t.Run("Empty", func(t *ftt.Test) {
					tmpl = webPOST(confirmURL, url.Values{
						"confirmation_code": {""},
					})
					assert.Loosely(t, tmpl.BadCode, should.BeTrue)
					checkSessionState(loginsessionspb.LoginSession_PENDING, "")
				})

				t.Run("Wrong", func(t *ftt.Test) {
					tmpl = webPOST(confirmURL, url.Values{
						"confirmation_code": {"wrong"},
					})
					assert.Loosely(t, tmpl.BadCode, should.BeTrue)
					checkSessionState(loginsessionspb.LoginSession_PENDING, "")
				})

				t.Run("Stale, but still valid", func(t *ftt.Test) {
					tc.Add(confirmationCodeExpiryRefresh + time.Second)
					webPOST(confirmURL, url.Values{
						"confirmation_code": {session.ConfirmationCode},
					})
					checkSessionState(loginsessionspb.LoginSession_SUCCEEDED, "")
				})

				t.Run("Expired", func(t *ftt.Test) {
					tc.Add(confirmationCodeExpiryMax + time.Second)
					tmpl = webPOST(confirmURL, url.Values{
						"confirmation_code": {session.ConfirmationCode},
					})
					assert.Loosely(t, tmpl.BadCode, should.BeTrue)
					checkSessionState(loginsessionspb.LoginSession_PENDING, "")
				})
			})
		})

		t.Run("Session cancellation", func(t *ftt.Test) {
			// Create the session and get the session cookie.
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			assert.Loosely(t, err, should.BeNil)
			tmpl := webGET(session.LoginFlowUrl)
			assert.Loosely(t, tmpl.OAuthState, should.NotBeEmpty)

			// Cancel it.
			tmpl = webPOST(srv.URL+"/cli/cancel", url.Values{
				"state": {tmpl.OAuthState},
			})
			assert.Loosely(t, tmpl.Template, should.Equal("pages/canceled.html"))

			// Verify it is indeed canceled.
			session, err = mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: session.Password,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, session.State, should.Equal(loginsessionspb.LoginSession_CANCELED))
		})
	})
}
