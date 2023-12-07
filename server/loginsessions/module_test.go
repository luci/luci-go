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

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/loginsessionspb"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/server/loginsessions/internal"
	"go.chromium.org/luci/server/loginsessions/internal/statepb"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

const mockedAuthorizationEndpoint = "http://localhost/authorization"

func TestModule(t *testing.T) {
	t.Parallel()

	Convey("With module", t, func() {
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
		So(err, ShouldBeNil)
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
			So(err, ShouldBeNil)
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			So(err, ShouldBeNil)
			var args templateArgs
			So(json.Unmarshal(body, &args), ShouldBeNil)
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

		Convey("CreateLoginSession + GetLoginSession", func() {
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			So(err, ShouldBeNil)
			So(session, ShouldResembleProto, &loginsessionspb.LoginSession{
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
			})

			pwd := session.Password
			session.Password = nil

			got, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			So(err, ShouldBeNil)
			So(got, ShouldResembleProto, session)

			// Later the confirmation code gets stale and a new one is generated.
			tc.Set(now.Add(confirmationCodeExpiryRefresh + time.Second))
			got1, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			So(err, ShouldBeNil)
			So(got1.ConfirmationCode, ShouldNotEqual, session.ConfirmationCode)

			// Have two codes in the store right now.
			stored, err := mod.srv.store.Get(ctx, session.Id)
			So(err, ShouldBeNil)
			So(stored.ConfirmationCodes, ShouldHaveLength, 2)

			// Later the expired code is kicked out of the storage.
			tc.Set(now.Add(confirmationCodeExpiryMax + time.Second))
			got2, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			So(err, ShouldBeNil)
			So(got2.ConfirmationCode, ShouldEqual, got1.ConfirmationCode)

			// Have only one code now.
			stored, err = mod.srv.store.Get(ctx, session.Id)
			So(err, ShouldBeNil)
			So(stored.ConfirmationCodes, ShouldHaveLength, 1)

			// Later the session itself expires.
			tc.Set(now.Add(sessionExpiry + time.Second))
			exp, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			So(err, ShouldBeNil)
			So(exp, ShouldResembleProto, &loginsessionspb.LoginSession{
				Id:           session.Id,
				State:        loginsessionspb.LoginSession_EXPIRED,
				Created:      timestampFromNow(0),
				Expiry:       timestampFromNow(sessionExpiry),
				Completed:    timestampFromNow(sessionExpiry + time.Second),
				LoginFlowUrl: fmt.Sprintf("%s/cli/login/%s", srv.URL, session.Id),
			})

			// And this is the final stage.
			tc.Set(now.Add(sessionExpiry + time.Hour))
			exp2, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: pwd,
			})
			So(err, ShouldBeNil)
			So(exp2, ShouldResembleProto, exp)
		})

		Convey("CreateLoginSession validation", func() {
			Convey("Browser headers", func() {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs("sec-fetch-site", "none"))
				_, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
				So(err, ShouldBeRPCPermissionDenied)
			})
			Convey("Missing OAuth client ID", func() {
				req := createSessionReq()
				req.OauthClientId = ""
				_, err := mod.srv.CreateLoginSession(ctx, req)
				So(err, ShouldBeRPCInvalidArgument)
			})
			Convey("Missing OAuth scopes", func() {
				req := createSessionReq()
				req.OauthScopes = nil
				_, err := mod.srv.CreateLoginSession(ctx, req)
				So(err, ShouldBeRPCInvalidArgument)
			})
			Convey("Missing OAuth challenge", func() {
				req := createSessionReq()
				req.OauthS256CodeChallenge = ""
				_, err := mod.srv.CreateLoginSession(ctx, req)
				So(err, ShouldBeRPCInvalidArgument)
			})
			Convey("Unknown OAuth client", func() {
				req := createSessionReq()
				req.OauthClientId = "unknown"
				_, err := mod.srv.CreateLoginSession(ctx, req)
				So(err, ShouldBeRPCPermissionDenied)
			})
		})

		Convey("GetLoginSession validation", func() {
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			So(err, ShouldBeNil)

			Convey("Browser headers", func() {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs("sec-fetch-site", "none"))
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       session.Id,
					LoginSessionPassword: session.Password,
				})
				So(err, ShouldBeRPCPermissionDenied)
			})
			Convey("Missing ID", func() {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionPassword: session.Password,
				})
				So(err, ShouldBeRPCInvalidArgument)
			})
			Convey("Missing password", func() {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId: session.Id,
				})
				So(err, ShouldBeRPCInvalidArgument)
			})
			Convey("Missing session", func() {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       "missing",
					LoginSessionPassword: session.Password,
				})
				So(err, ShouldBeRPCNotFound)
			})
			Convey("Wrong password", func() {
				_, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       session.Id,
					LoginSessionPassword: []byte("wrong"),
				})
				So(err, ShouldBeRPCNotFound)
			})
		})

		Convey("Full successful flow", func() {
			// The CLI tool starts a new login session.
			sessionReq := createSessionReq()
			session, err := mod.srv.CreateLoginSession(ctx, sessionReq)
			So(err, ShouldBeNil)

			// The user opens the web page with the session and gets the cookie and
			// the authorization endpoint redirect parameters.
			tmpl := webGET(session.LoginFlowUrl)
			So(tmpl.Error, ShouldBeEmpty)
			So(tmpl.Template, ShouldEqual, "pages/start.html")
			So(tmpl.OAuthState, ShouldNotBeEmpty)
			So(tmpl.OAuthRedirectParams, ShouldResemble, map[string]string{
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
			})

			// The user goes through the login flow and ends up back with a code.
			// This renders a page asking for the confirmation code.
			confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
				"code":  {"authorization-code"},
				"state": {tmpl.OAuthState},
			}).Encode()
			tmpl = webGET(confirmURL)
			So(tmpl.Error, ShouldBeEmpty)
			So(tmpl.Template, ShouldEqual, "pages/confirm.html")
			So(tmpl.OAuthState, ShouldNotBeEmpty)

			// A correct confirmation code is entered and accepted.
			tmpl = webPOST(confirmURL, url.Values{
				"confirmation_code": {session.ConfirmationCode},
			})
			So(tmpl.Error, ShouldBeEmpty)
			So(tmpl.Template, ShouldEqual, "pages/success.html")

			// The session is completed and the code is returned to the CLI.
			session, err = mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: session.Password,
			})
			So(err, ShouldBeNil)
			So(session, ShouldResembleProto, &loginsessionspb.LoginSession{
				Id:                     session.Id,
				State:                  loginsessionspb.LoginSession_SUCCEEDED,
				Created:                timestampFromNow(0),
				Expiry:                 timestampFromNow(sessionExpiry),
				Completed:              timestampFromNow(0),
				LoginFlowUrl:           fmt.Sprintf("%s/cli/login/%s", srv.URL, session.Id),
				OauthAuthorizationCode: "authorization-code",
				OauthRedirectUrl:       srv.URL + "/cli/confirm",
			})

			// Visiting the session page again shows it is gone.
			tmpl = webGET(session.LoginFlowUrl)
			So(tmpl.Error, ShouldContainSubstring, "No such login session")
		})

		Convey("Session page errors", func() {
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			So(err, ShouldBeNil)

			Convey("Wrong session ID", func() {
				// Opening a non-existent session page is an error.
				tmpl := webGET(session.LoginFlowUrl + "extra")
				So(tmpl.Error, ShouldContainSubstring, "No such login session")
			})

			Convey("Expired session", func() {
				// Opening an old session is an error.
				tc.Add(sessionExpiry + time.Second)
				tmpl := webGET(session.LoginFlowUrl)
				So(tmpl.Error, ShouldContainSubstring, "No such login session")
			})
		})

		Convey("Redirect page errors", func() {
			// Create the session and get the session cookie.
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			So(err, ShouldBeNil)
			tmpl := webGET(session.LoginFlowUrl)
			So(tmpl.OAuthState, ShouldNotBeEmpty)

			checkSessionState := func(state loginsessionspb.LoginSession_State, msg string) {
				session, err := mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
					LoginSessionId:       session.Id,
					LoginSessionPassword: session.Password,
				})
				So(err, ShouldBeNil)
				So(session.State, ShouldEqual, state)
				So(session.OauthError, ShouldEqual, msg)
			}

			Convey("OK", func() {
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

			Convey("No OAuth code or error", func() {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
				}).Encode()
				tmpl = webGET(confirmURL)
				So(tmpl.Error, ShouldContainSubstring, "The authorization provider returned error code: unknown")
				checkSessionState(loginsessionspb.LoginSession_FAILED, "unknown")
			})

			Convey("OAuth error", func() {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"error": {"boom"},
				}).Encode()
				tmpl = webGET(confirmURL)
				So(tmpl.Error, ShouldContainSubstring, "The authorization provider returned error code: boom")
				checkSessionState(loginsessionspb.LoginSession_FAILED, "boom")
			})

			Convey("No state", func() {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"error": {"boom"},
				}).Encode()
				tmpl = webGET(confirmURL)
				So(tmpl.Error, ShouldContainSubstring, "The authorization provider returned error code: boom")
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			Convey("Bad state", func() {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState[:20]},
					"code":  {"authorization-code"},
				}).Encode()
				tmpl = webGET(confirmURL)
				So(tmpl.Error, ShouldContainSubstring, "Internal server error")
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			Convey("Expired session", func() {
				tc.Add(sessionExpiry + time.Second)
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()
				tmpl = webGET(confirmURL)
				So(tmpl.Error, ShouldContainSubstring, "finished or expired")
				checkSessionState(loginsessionspb.LoginSession_EXPIRED, "")
			})

			Convey("Missing login cookie", func() {
				emptyJar, err := cookiejar.New(nil)
				So(err, ShouldBeNil)
				web.Jar = emptyJar
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()
				tmpl = webGET(confirmURL)
				So(tmpl.Error, ShouldContainSubstring, "finished or expired")
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			Convey("Wrong login cookie", func() {
				u, err := url.Parse(srv.URL + "/cli/session")
				So(err, ShouldBeNil)
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
				So(tmpl.Error, ShouldContainSubstring, "finished or expired")
				checkSessionState(loginsessionspb.LoginSession_PENDING, "")
			})

			Convey("Wrong confirmation code", func() {
				confirmURL := srv.URL + "/cli/confirm?" + (url.Values{
					"state": {tmpl.OAuthState},
					"code":  {"authorization-code"},
				}).Encode()

				Convey("Empty", func() {
					tmpl = webPOST(confirmURL, url.Values{
						"confirmation_code": {""},
					})
					So(tmpl.BadCode, ShouldBeTrue)
					checkSessionState(loginsessionspb.LoginSession_PENDING, "")
				})

				Convey("Wrong", func() {
					tmpl = webPOST(confirmURL, url.Values{
						"confirmation_code": {"wrong"},
					})
					So(tmpl.BadCode, ShouldBeTrue)
					checkSessionState(loginsessionspb.LoginSession_PENDING, "")
				})

				Convey("Stale, but still valid", func() {
					tc.Add(confirmationCodeExpiryRefresh + time.Second)
					webPOST(confirmURL, url.Values{
						"confirmation_code": {session.ConfirmationCode},
					})
					checkSessionState(loginsessionspb.LoginSession_SUCCEEDED, "")
				})

				Convey("Expired", func() {
					tc.Add(confirmationCodeExpiryMax + time.Second)
					tmpl = webPOST(confirmURL, url.Values{
						"confirmation_code": {session.ConfirmationCode},
					})
					So(tmpl.BadCode, ShouldBeTrue)
					checkSessionState(loginsessionspb.LoginSession_PENDING, "")
				})
			})
		})

		Convey("Session cancellation", func() {
			// Create the session and get the session cookie.
			session, err := mod.srv.CreateLoginSession(ctx, createSessionReq())
			So(err, ShouldBeNil)
			tmpl := webGET(session.LoginFlowUrl)
			So(tmpl.OAuthState, ShouldNotBeEmpty)

			// Cancel it.
			tmpl = webPOST(srv.URL+"/cli/cancel", url.Values{
				"state": {tmpl.OAuthState},
			})
			So(tmpl.Template, ShouldEqual, "pages/canceled.html")

			// Verify it is indeed canceled.
			session, err = mod.srv.GetLoginSession(ctx, &loginsessionspb.GetLoginSessionRequest{
				LoginSessionId:       session.Id,
				LoginSessionPassword: session.Password,
			})
			So(err, ShouldBeNil)
			So(session.State, ShouldEqual, loginsessionspb.LoginSession_CANCELED)
		})
	})
}
