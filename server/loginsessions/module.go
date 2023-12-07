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

// Package loginsessions implements Login Sessions backend that is used to
// perform interactive logins in LUCI CLI tools.
package loginsessions

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"
	"unicode"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/loginsessionspb"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/loginsessions/internal"
	"go.chromium.org/luci/server/loginsessions/internal/assets"
	"go.chromium.org/luci/server/loginsessions/internal/statepb"
	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/templates"
)

// ModuleName can be used to refer to this module when declaring dependencies.
var ModuleName = module.RegisterName("go.chromium.org/luci/server/loginsessions")

// ModuleOptions contain configuration of the login sessions server module.
type ModuleOptions struct {
	// RootURL is the root URL of the login session server to use in links.
	//
	// E.g. "https://<publicly routable domain name>".
	//
	// Required for production mode.
	RootURL string
}

// Register registers the command line flags.
func (o *ModuleOptions) Register(f *flag.FlagSet) {
	f.StringVar(
		&o.RootURL,
		"login-sessions-root-url",
		o.RootURL,
		"The root URL of the login session server to use in links e.g. "+
			"`https://<publicly routable domain name>`. Required.",
	)
}

// NewModule returns a server module that implements login sessions backend.
func NewModule(opts *ModuleOptions) module.Module {
	if opts == nil {
		opts = &ModuleOptions{}
	}
	return &loginSessionsModule{opts: opts}
}

// NewModuleFromFlags is a variant of NewModule that initializes options through
// command line flags.
//
// Calling this function registers flags in flag.CommandLine. They are usually
// parsed in server.Main(...).
func NewModuleFromFlags() module.Module {
	opts := &ModuleOptions{}
	opts.Register(flag.CommandLine)
	return NewModule(opts)
}

// loginSessionsModule implements module.Module.
type loginSessionsModule struct {
	opts           *ModuleOptions
	srv            *loginSessionsServer
	tmpl           *templates.Bundle // if nil render template args as JSON (for tests)
	insecureCookie bool              // if true allow non-HTTPS cookie (for tests)
}

// Name is part of module.Module interface.
func (*loginSessionsModule) Name() module.Name {
	return ModuleName
}

// Dependencies is part of module.Module interface.
func (*loginSessionsModule) Dependencies() []module.Dependency {
	return []module.Dependency{
		// For encryption of OpenIDState.
		module.RequiredDependency(secrets.ModuleName),
		// For prod session store based on Datastore.
		module.RequiredDependency(gaeemulation.ModuleName),
		// For cleaning up old sessions.
		module.RequiredDependency(cron.ModuleName),
	}
}

// Initialize is part of module.Module interface.
func (m *loginSessionsModule) Initialize(ctx context.Context, host module.Host, opts module.HostOptions) (context.Context, error) {
	var store internal.SessionStore
	var provider internal.OAuthClientProvider

	if opts.Prod {
		if m.opts.RootURL == "" {
			return nil, errors.Reason("-login-sessions-root-url is required").Err()
		}
		m.opts.RootURL = strings.TrimSuffix(m.opts.RootURL, "/")
		if !strings.HasPrefix(m.opts.RootURL, "https://") {
			return nil, errors.Reason("-login-sessions-root-url should start with https://, got %q", m.opts.RootURL).Err()
		}
		store = &internal.DatastoreSessionStore{}
		provider = internal.AuthDBClientProvider
	} else {
		// Fakes for local development mode.
		m.opts.RootURL = fmt.Sprintf("http://%s", host.HTTPAddr())
		store = &internal.MemorySessionStore{}
		provider = func(context.Context, string) (*internal.OAuthClient, error) {
			return &internal.OAuthClient{
				ProviderName:          "Google Accounts",
				AuthorizationEndpoint: internal.GoogleAuthorizationEndpoint,
			}, nil
		}
	}

	// Install the RPC server called by the CLI tools.
	m.srv = &loginSessionsServer{
		opts:     m.opts,
		store:    store,
		provider: provider,
	}
	loginsessionspb.RegisterLoginSessionsServer(host, m.srv)

	// Load templates for the browser flow.
	m.tmpl = &templates.Bundle{
		Loader:          templates.AssetsLoader(assets.Assets()),
		DebugMode:       func(context.Context) bool { return !opts.Prod },
		DefaultTemplate: "base",
		FuncMap: template.FuncMap{
			"includeCSS": func(name string) template.CSS { return template.CSS(assets.GetAsset(name)) },
		},
	}
	if err := m.tmpl.EnsureLoaded(ctx); err != nil {
		return nil, err
	}

	// Install web routes for the browser flow.
	m.installRoutes(host.Routes())

	// Install the cron handler that cleans old sessions.
	cron.RegisterHandler("loginsessions-cleanup", func(ctx context.Context) error {
		return m.srv.store.Cleanup(ctx)
	})

	return ctx, nil
}

// installRoutes installs web routes into the router.
//
// Extracted into a separate function to be called from tests.
func (m *loginSessionsModule) installRoutes(r *router.Router) {
	r.GET("/cli/login/:SessionID", nil, m.loginSessionPage)
	r.POST("/cli/cancel", nil, m.loginCancelPage)
	r.GET("/cli/confirm", nil, m.loginConfirmPageGET)
	r.POST("/cli/confirm", nil, m.loginConfirmPagePOST)
}

// loginCookieName is a login cookie name matching the given session ID.
func (m *loginSessionsModule) loginCookieName(sessionID string) string {
	return "_LUCI_CLI_LOGIN_" + sessionID
}

// redirectURI is a OAuth redirect URI, it must match the client configuration.
func (m *loginSessionsModule) redirectURI() string {
	return fmt.Sprintf("%s/cli/confirm", m.opts.RootURL)
}

// loginSessionPage renders the starting login page and sets the login cookie.
func (m *loginSessionsModule) loginSessionPage(ctx *router.Context) {
	// Check the session exists and is still in PENDING state.
	session := m.pendingSessionOrRenderErr(ctx, ctx.Params.ByName("SessionID"))
	if session == nil {
		return
	}

	// Load OAuth client details to show on the page. Bail if this client is no
	// longer known (this should be rare).
	oauthClient, err := m.srv.provider(ctx.Request.Context(), session.OauthClientId)
	switch {
	case err != nil:
		m.renderInternalError(ctx, "error fetching OAuth client %s: %s", session.OauthClientId, err)
		return
	case oauthClient == nil:
		m.renderBadRequestError(ctx, "OAuth client %s is no longer allowed", session.OauthClientId)
		return
	}

	// Generate a random cookie that we'll double check in the OAuth redirect to
	// make sure the user actually visited our page with the scary phishing
	// warnings before launching the OAuth flow through the authorization server.
	loginCookieValue := internal.RandomAlphaNum(20)

	// Generate `state` blob that would come back to us in the redirect URL from
	// the authorization endpoint.
	state, err := internal.EncryptState(ctx.Request.Context(), &statepb.OpenIDState{
		LoginSessionId:   session.Id,
		LoginCookieValue: loginCookieValue,
	})
	if err != nil {
		m.renderInternalError(ctx, "error preparing encrypted state: %s", err)
		return
	}

	// Set the login cookie and render the initial page.
	http.SetCookie(ctx.Writer, &http.Cookie{
		Name:     m.loginCookieName(session.Id),
		Value:    loginCookieValue,
		Path:     "/cli/",
		MaxAge:   int(session.Expiry.AsTime().Sub(clock.Now(ctx.Request.Context())).Seconds()),
		Secure:   !m.insecureCookie,
		HttpOnly: true,
	})
	m.renderTemplate(ctx, http.StatusOK, "pages/start.html", templates.Args{
		"Session":     session,
		"OAuthClient": oauthClient,
		"OAuthState":  state,
		"OAuthRedirectParams": map[string]string{
			"response_type":         "code",
			"scope":                 strings.Join(session.OauthScopes, " "),
			"access_type":           "offline", // want a refresh token
			"prompt":                "consent", // want a NEW refresh token
			"client_id":             session.OauthClientId,
			"redirect_uri":          m.redirectURI(),
			"nonce":                 session.Id,                     // per Login Sessions protocol
			"code_challenge":        session.OauthS256CodeChallenge, // PKCE
			"code_challenge_method": "S256",                         // PKCE
			"state":                 state,
		},
	})
}

// sessionFromState decrypts OpenIDState, checks the login cookie, and loads and
// checks the login session is still in PENDING state.
//
// It renders errors directly into the response. It returns the LoginSession on
// success or nil on errors.
func (m *loginSessionsModule) sessionFromState(ctx *router.Context, stateB64 string) *statepb.LoginSession {
	// Decrypt the state to get the session ID.
	state, err := internal.DecryptState(ctx.Request.Context(), stateB64)
	if err != nil {
		m.renderInternalError(ctx, "failed to decrypt state: %s", err)
		return nil
	}

	// Verify the state passed to us matches the login cookie. This ensures that
	// the user saw our login page (with phishing warnings) before going through
	// the authorization server redirect flow.
	cookie, err := ctx.Request.Cookie(m.loginCookieName(state.LoginSessionId))
	if err != nil || subtle.ConstantTimeCompare([]byte(state.LoginCookieValue), []byte(cookie.Value)) == 0 {
		m.renderExpiredError(ctx, "login cookie is missing or invalid")
		return nil
	}

	// Check if the session is still in PENDING state.
	return m.pendingSessionOrRenderErr(ctx, state.LoginSessionId)
}

// pendingSessionOrRenderErr fetches a session and checks it is still pending.
//
// It renders errors directly into the response. It returns the LoginSession on
// success or nil on errors.
func (m *loginSessionsModule) pendingSessionOrRenderErr(ctx *router.Context, sessionID string) *statepb.LoginSession {
	switch session, err := m.srv.store.Get(ctx.Request.Context(), sessionID); {
	case err == internal.ErrNoSession:
		m.renderExpiredError(ctx, "no such session")
		return nil
	case err != nil:
		m.renderInternalError(ctx, "error fetching session: %s", err)
		return nil
	case session.State != loginsessionspb.LoginSession_PENDING:
		m.renderExpiredError(ctx, "the session is not pending, it is %s", session.State)
		return nil
	case clock.Now(ctx.Request.Context()).After(session.Expiry.AsTime()):
		m.renderExpiredError(ctx, "the session is expired")
		return nil
	default:
		return session
	}
}

// handleRedirectURL is called by loginConfirmPageGET and loginConfirmPagePOST
// to handle parameters passed from the authorization server via the redirect
// URL.
//
// It decrypts OpenIDState, checks the login cookie, loads and checks the
// login session is still in PENDING state, and check the OAuth error.
//
// It renders errors directly into the response. It returns the pending
// LoginSession and the authorization code on success or (nil, "") on errors.
func (m *loginSessionsModule) handleRedirectURL(ctx *router.Context) (*statepb.LoginSession, string) {
	q := ctx.Request.URL.Query()

	// We must have either `code` or `error` by now (but not both).
	oauthCode := q.Get("code")
	oauthError := q.Get("error")
	switch {
	case oauthCode == "" && oauthError == "":
		oauthError = "unknown"
	case oauthError != "":
		oauthCode = ""
	}

	// If state is not available, we can at most show the error page. We don't
	// know an associated session that should be updated. We can't use a login
	// cookie for discovering the session since there may be multiple login
	// cookies for different sessions (if there are concurrent flows or some
	// cookies from previous workflows didn't expire yet).
	oauthState := q.Get("state")
	if oauthState == "" {
		m.renderBadRequestError(ctx, "the authorization provider returned error code: %s", oauthError)
		return nil, ""
	}

	// Load the session if it is still pending.
	session := m.sessionFromState(ctx, oauthState)
	if session == nil {
		return nil, ""
	}

	// If the login failed, flip the session into FAILED state right away. There's
	// no security risk in skipping checking the confirmation code in this case.
	// In fact, it would be weird to ask for a confirmation code just to fail
	// right after it is checked.
	if oauthError != "" {
		m.finalizeSession(ctx, session.Id, func(session *statepb.LoginSession) {
			session.State = loginsessionspb.LoginSession_FAILED
			session.OauthError = oauthError
		})
		return nil, ""
	}

	return session, oauthCode
}

// loginCancelPage cancels the login session and renders the corresponding page.
func (m *loginSessionsModule) loginCancelPage(ctx *router.Context) {
	session := m.sessionFromState(ctx, ctx.Request.PostFormValue("state"))
	if session != nil {
		m.finalizeSession(ctx, session.Id, func(session *statepb.LoginSession) {
			session.State = loginsessionspb.LoginSession_CANCELED
		})
	}
}

// loginConfirmPageGET renders the form that asks for the confirmation code.
//
// This page is the target of the redirect from the authorization server and
// it receives the OAuth authorization code as an URL parameter.
func (m *loginSessionsModule) loginConfirmPageGET(ctx *router.Context) {
	// Verify the state and the cookie and check the session is in PENDING state.
	if session, authorizationCode := m.handleRedirectURL(ctx); authorizationCode != "" {
		// The session is still good and we got the authorization code. Ask the
		// user to provide the up-to-date confirmation code before storing the
		// authorization code in the session.
		m.renderTemplate(ctx, http.StatusOK, "pages/confirm.html", templates.Args{
			"Session":    session,
			"OAuthState": ctx.Request.URL.Query().Get("state"),
			"BadCode":    false,
		})
	}
}

// loginConfirmPagePOST handles the confirmation code entered by the user.
//
// All OAuth state received from the authorization server is still in URL
// parameters of this page.
func (m *loginSessionsModule) loginConfirmPagePOST(ctx *router.Context) {
	// Verify the state and the cookie and check the session is in PENDING state.
	session, authorizationCode := m.handleRedirectURL(ctx)
	if session == nil {
		return
	}

	// Check the provided confirmation code is a known non-expired code.
	confirmationCode := ctx.Request.PostFormValue("confirmation_code")
	now := clock.Now(ctx.Request.Context())
	good := false
	for _, code := range session.ConfirmationCodes {
		if code.Expiry.AsTime().After(now) &&
			subtle.ConstantTimeCompare([]byte(confirmationCode), []byte(code.Code)) == 1 {
			good = true
			break
		}
	}
	if !good {
		// Ask the user to enter another code.
		m.renderTemplate(ctx, http.StatusOK, "pages/confirm.html", templates.Args{
			"Session":    session,
			"OAuthState": ctx.Request.URL.Query().Get("state"),
			"BadCode":    true,
		})
		return
	}

	// The confirmation code is correct! Flip the session into SUCCEEDED state and
	// store the authorization code in it. The polling native program will pick it
	// up and finish the OAuth flow.
	m.finalizeSession(ctx, session.Id, func(session *statepb.LoginSession) {
		session.State = loginsessionspb.LoginSession_SUCCEEDED
		session.OauthRedirectUrl = m.redirectURI()
		session.OauthAuthorizationCode = authorizationCode
	})
}

// finalizeSession flips the session into a final state and renders the result.
func (m *loginSessionsModule) finalizeSession(ctx *router.Context, sessionID string, cb func(*statepb.LoginSession)) {
	session, err := m.srv.store.Update(ctx.Request.Context(), sessionID, func(session *statepb.LoginSession) {
		if session.State == loginsessionspb.LoginSession_PENDING {
			updateExpiry(ctx.Request.Context(), session)
			if session.State == loginsessionspb.LoginSession_PENDING {
				cb(session)
				if session.State == loginsessionspb.LoginSession_PENDING {
					panic("the callback didn't change the state")
				}
				session.Completed = timestamppb.New(clock.Now(ctx.Request.Context()))
			}
		}
	})
	if err != nil {
		m.renderInternalError(ctx, "failed to update the session: %s", err)
		return
	}
	switch session.State {
	case loginsessionspb.LoginSession_SUCCEEDED:
		m.renderTemplate(ctx, http.StatusOK, "pages/success.html", templates.Args{"Session": session})
	case loginsessionspb.LoginSession_CANCELED:
		m.renderTemplate(ctx, http.StatusOK, "pages/canceled.html", templates.Args{"Session": session})
	case loginsessionspb.LoginSession_FAILED:
		m.renderTemplate(ctx, http.StatusOK, "pages/error.html", templates.Args{
			"Error": fmt.Sprintf("The authorization provider returned error code: %s.", session.OauthError),
		})
	case loginsessionspb.LoginSession_EXPIRED:
		m.renderExpiredError(ctx, "the session is in EXPIRED state")
	default:
		m.renderInternalError(ctx, "unexpected session state: %s", session.State)
	}
}

// renderTemplate renders an HTML template into the response.
//
// `args` will be mutated by adding `Template` key to it.
func (m *loginSessionsModule) renderTemplate(ctx *router.Context, status int, name string, args templates.Args) {
	args["Template"] = name
	if m.tmpl != nil {
		// This code path is used when running for real.
		ctx.Writer.Header().Add("Content-Type", "text/html; charset=utf-8")
		ctx.Writer.WriteHeader(status)
		m.tmpl.MustRender(ctx.Request.Context(), nil, ctx.Writer, name, args)
	} else {
		// This code path is used in tests.
		ctx.Writer.Header().Add("Content-Type", "application/json; charset=utf-8")
		ctx.Writer.WriteHeader(status)
		blob, err := json.Marshal(args)
		if err != nil {
			panic(err)
		}
		_, err = ctx.Writer.Write(blob)
		if err != nil {
			panic(err)
		}
	}
}

// renderInternalError logs the error and renders generic "Internal error" page.
func (m *loginSessionsModule) renderInternalError(ctx *router.Context, msg string, args ...any) {
	logging.Errorf(ctx.Request.Context(), "Internal error: "+msg, args...)
	m.renderTemplate(ctx, http.StatusInternalServerError, "pages/error.html", templates.Args{
		"Error": "Internal server error.",
	})
}

// renderExpiredError logs the error and renders generic "Session expired" page.
func (m *loginSessionsModule) renderExpiredError(ctx *router.Context, msg string, args ...any) {
	logging.Warningf(ctx.Request.Context(), "Expiry error: "+msg, args...)
	m.renderTemplate(ctx, http.StatusNotFound, "pages/error.html", templates.Args{
		"Error": "No such login session or it has finished or expired. Please restart the login flow from scratch.",
	})
}

// renderBadRequestError logs and renders "bad argument" error page.
func (m *loginSessionsModule) renderBadRequestError(ctx *router.Context, msg string, args ...any) {
	logging.Warningf(ctx.Request.Context(), "Bad request: "+msg, args...)

	// Make it title case, add final '.'.
	pretty := []rune(fmt.Sprintf(msg, args...))
	if len(pretty) == 0 || pretty[len(pretty)-1] != '.' {
		pretty = append(pretty, '.')
	}
	pretty[0] = unicode.ToTitle(pretty[0])

	m.renderTemplate(ctx, http.StatusBadRequest, "pages/error.html", templates.Args{
		"Error": string(pretty),
	})
}

////////////////////////////////////////////////////////////////////////////////

const (
	// Overall limit on lifetime of a session.
	sessionExpiry = 5 * time.Minute
	// Lifetime of a new confirmation code.
	confirmationCodeExpiryMax = 30 * time.Second
	// Minimal confirmation code expiry returned by the API.
	confirmationCodeExpiryMin = 5 * time.Second
	// If all codes are older than this, make a new code.
	confirmationCodeExpiryRefresh = 20 * time.Second
)

type loginSessionsServer struct {
	loginsessionspb.UnimplementedLoginSessionsServer

	opts     *ModuleOptions
	store    internal.SessionStore
	provider internal.OAuthClientProvider
}

func (srv *loginSessionsServer) CreateLoginSession(ctx context.Context, req *loginsessionspb.CreateLoginSessionRequest) (resp *loginsessionspb.LoginSession, err error) {
	// Rejects attempts to use the API from a browser.
	if err := checkBrowserHeaders(ctx); err != nil {
		return nil, err
	}

	// Do some basic validation. No need to be super thorough, the login flow will
	// fail anyway if some parameters are not recognized by the authorization
	// server.
	if req.OauthClientId == "" {
		return nil, status.Error(codes.InvalidArgument, "OAuth client ID is required")
	}
	if len(req.OauthScopes) == 0 {
		return nil, status.Error(codes.InvalidArgument, "OAuth scopes are required")
	}
	if req.OauthS256CodeChallenge == "" {
		return nil, status.Errorf(codes.InvalidArgument, "OAuth code challenge is required")
	}

	// Check if this OAuth client is known to us.
	switch oauthClient, err := srv.provider(ctx, req.OauthClientId); {
	case err != nil:
		logging.Errorf(ctx, "Internal error fetching OAuth client %s: %s", req.OauthClientId, err)
		return nil, status.Errorf(codes.Internal, "internal error fetching OAuth client")
	case oauthClient == nil:
		return nil, status.Errorf(codes.PermissionDenied, "OAuth client %s is not allowed", req.OauthClientId)
	}

	now := clock.Now(ctx)

	// Create the session in PENDING state.
	session := &statepb.LoginSession{
		Id:                     internal.RandomAlphaNum(40),
		Password:               internal.RandomBlob(40),
		State:                  loginsessionspb.LoginSession_PENDING,
		Created:                timestamppb.New(now),
		Expiry:                 timestamppb.New(now.Add(sessionExpiry)),
		OauthClientId:          req.OauthClientId,
		OauthScopes:            req.OauthScopes,
		OauthS256CodeChallenge: req.OauthS256CodeChallenge,
		ExecutableName:         req.ExecutableName,
		ClientHostname:         req.ClientHostname,
		ConfirmationCodes: []*statepb.LoginSession_ConfirmationCode{
			{
				Code:    internal.RandomAlphaNum(40),
				Expiry:  timestamppb.New(now.Add(confirmationCodeExpiryMax)),
				Refresh: timestamppb.New(now.Add(confirmationCodeExpiryRefresh)),
			},
		},
	}
	if err := srv.store.Create(ctx, session); err != nil {
		logging.Errorf(ctx, "Internal error creating the session: %s", err)
		return nil, status.Errorf(codes.Internal, "internal error creating the session")
	}

	// Return the session with the password.
	return srv.sessionResponse(ctx, session, session.Password)
}

func (srv *loginSessionsServer) GetLoginSession(ctx context.Context, req *loginsessionspb.GetLoginSessionRequest) (resp *loginsessionspb.LoginSession, err error) {
	// Rejects attempts to use the API from a browser.
	if err := checkBrowserHeaders(ctx); err != nil {
		return nil, err
	}

	if req.LoginSessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session ID is required")
	}
	if len(req.LoginSessionPassword) == 0 {
		return nil, status.Error(codes.InvalidArgument, "session password is required")
	}

	// Get the session or `nil` if missing.
	session, err := srv.store.Get(ctx, req.LoginSessionId)
	switch {
	case err == internal.ErrNoSession:
		session = nil
	case err != nil:
		logging.Errorf(ctx, "Internal error fetching session %s: %s", req.LoginSessionId, err)
		return nil, status.Errorf(codes.Internal, "internal error fetching session")
	}

	// Treat invalid password exactly as a missing session.
	badPassword := session != nil && subtle.ConstantTimeCompare(req.LoginSessionPassword, session.Password) == 0
	if badPassword {
		logging.Errorf(ctx, "Bad password given when fetching session %s", req.LoginSessionId)
	}
	if session == nil || badPassword {
		return nil, status.Errorf(codes.NotFound, "no such session or the password is invalid")
	}

	// Perform "lazy" session updates, like moving it to EXPIRED state or updating
	// confirmation codes. GetLoginSession RPC is the only way to "observe"
	// a session, all time-related updates can be done lazily here, no need to do
	// them proactively in crons (we still need a cron to delete old sessions, but
	// that's it).
	if needExpiry(ctx, session) || needUpdateCodes(ctx, session) {
		session, err = srv.store.Update(ctx, session.Id, func(session *statepb.LoginSession) {
			updateExpiry(ctx, session)
			updateCodes(ctx, session)
		})
		if err != nil {
			logging.Errorf(ctx, "Failed to update the session: %s", err)
			return nil, status.Errorf(codes.Internal, "internal error getting the session")
		}
	}

	// Return the session without the password.
	return srv.sessionResponse(ctx, session, nil)
}

func (srv *loginSessionsServer) sessionResponse(ctx context.Context, s *statepb.LoginSession, pwd []byte) (*loginsessionspb.LoginSession, error) {
	out := &loginsessionspb.LoginSession{
		Id:                     s.Id,
		Password:               pwd,
		State:                  s.State,
		Created:                s.Created,
		Expiry:                 s.Expiry,
		Completed:              s.Completed,
		LoginFlowUrl:           fmt.Sprintf("%s/cli/login/%s", srv.opts.RootURL, s.Id),
		OauthAuthorizationCode: s.OauthAuthorizationCode,
		OauthRedirectUrl:       s.OauthRedirectUrl,
		OauthError:             s.OauthError,
	}

	if s.State == loginsessionspb.LoginSession_PENDING {
		// If the session is "old", poll less frequently. Likely the user is away,
		// no need to hammer the server. This calculation is done on the server side
		// so we can change it without redeploying all clients.
		var pollInterval time.Duration
		if clock.Since(ctx, s.Created.AsTime()) > 2*time.Minute {
			pollInterval = 5 * time.Second
		} else {
			pollInterval = time.Second
		}
		out.PollInterval = durationpb.New(pollInterval)

		// Report only the freshest confirmation code. There's no need for the
		// client to ever use an older one if there's a newer available (but we
		// still store it until it really expires).
		var freshest *statepb.LoginSession_ConfirmationCode
		for _, code := range s.ConfirmationCodes {
			if freshest == nil || code.Refresh.AsTime().After(freshest.Refresh.AsTime()) {
				freshest = code
			}
		}
		if freshest == nil {
			panic("no confirmation codes available, should not be possible")
		}

		// It is possible (but unlikely) that our process was stuck for a while
		// after we checked the expiry and the confirmation code is already stale.
		// Return an internal error to trigger a retry. The API promises to return
		// a code with lifetime at least confirmationCodeExpiryMin. Note that we
		// use durations (instead of absolute timestamps) to avoid relying on global
		// clock synchronization.
		expiryDuration := clock.Until(ctx, freshest.Expiry.AsTime())
		if expiryDuration < confirmationCodeExpiryMin {
			logging.Errorf(ctx, "Internal error: the confirmation code expiry %s is too small", expiryDuration)
			return nil, status.Errorf(codes.Internal, "internal error generating confirmation code")
		}
		out.ConfirmationCode = freshest.Code
		out.ConfirmationCodeExpiry = durationpb.New(expiryDuration)
		out.ConfirmationCodeRefresh = durationpb.New(clock.Until(ctx, freshest.Refresh.AsTime()))
	}

	return out, nil
}

// checkBrowserHeaders returns an error if there's a suspicion the pRPC request
// was made by a browser.
func checkBrowserHeaders(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	// Almost all browsers send "Sec-Fetch-Site" header, but pRPC client doesn't.
	if len(md["sec-fetch-site"]) != 0 {
		return status.Errorf(codes.PermissionDenied, "not allowed to be called from a browser")
	}
	// "Sec-Fetch-Site" is not supported at least on Safari (as of Sep 2022),
	// check the "User-Agent" instead. pRPC native client is very unlikely to use
	// Mozilla user agent (but most browsers, including Safari, do).
	for _, ua := range md["user-agent"] {
		if strings.Contains(ua, "Mozilla") {
			return status.Errorf(codes.PermissionDenied, "not allowed to be called from a browser")
		}
	}
	return nil
}

// needExpiry is true if the session should be flipped into EXPIRED state.
func needExpiry(ctx context.Context, session *statepb.LoginSession) bool {
	return session.State == loginsessionspb.LoginSession_PENDING &&
		clock.Now(ctx).After(session.Expiry.AsTime())
}

// updateExpiry flips the session into EXPIRED state if necessary.
func updateExpiry(ctx context.Context, session *statepb.LoginSession) {
	if needExpiry(ctx, session) {
		session.State = loginsessionspb.LoginSession_EXPIRED
		session.Completed = timestamppb.New(clock.Now(ctx))
	}
}

// needUpdateCodes is true if we need to expire or generate confirmation codes.
func needUpdateCodes(ctx context.Context, session *statepb.LoginSession) bool {
	if session.State != loginsessionspb.LoginSession_PENDING {
		return false
	}

	now := clock.Now(ctx)

	stale := true
	for _, code := range session.ConfirmationCodes {
		if now.After(code.Expiry.AsTime()) {
			return true // this code has expired and needs to be deleted
		}
		if now.Before(code.Refresh.AsTime()) {
			stale = false
		}
	}

	// If all codes are stale need to generate a new one.
	return stale
}

// updateCodes expires or generates confirmation codes.
func updateCodes(ctx context.Context, session *statepb.LoginSession) {
	if session.State != loginsessionspb.LoginSession_PENDING {
		return
	}

	now := clock.Now(ctx)

	// Drop expired confirmation codes.
	var codes []*statepb.LoginSession_ConfirmationCode
	for _, code := range session.ConfirmationCodes {
		if now.Before(code.Expiry.AsTime()) {
			codes = append(codes, code)
		} else {
			logging.Infof(ctx, "Expiring old confirmation code")
		}
	}

	// Add a new confirmation code if all codes are stale.
	stale := true
	for _, code := range codes {
		if now.Before(code.Refresh.AsTime()) {
			stale = false
			break
		}
	}
	if stale {
		logging.Infof(ctx, "Generating new confirmation code")
		codes = append(codes, &statepb.LoginSession_ConfirmationCode{
			Code:    internal.RandomAlphaNum(40),
			Expiry:  timestamppb.New(now.Add(confirmationCodeExpiryMax)),
			Refresh: timestamppb.New(now.Add(confirmationCodeExpiryRefresh)),
		})
	}

	session.ConfirmationCodes = codes
}
