// Copyright 2015 The LUCI Authors.
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

package deprecated

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/router"
)

// Note: this file is a part of deprecated CookieAuthMethod implementation.

// These are installed into a HTTP router by CookieAuthMethod.InstallHandlers.
const (
	loginURL    = "/auth/openid/login"
	logoutURL   = "/auth/openid/logout"
	callbackURL = "/auth/openid/callback"
)

// errBadDestinationURL is returned by normalizeURL on errors.
var errBadDestinationURL = errors.New("openid: dest URL in LoginURL or LogoutURL must be relative")

// CookieAuthMethod implements auth.Method and auth.UsersAPI and can be used as
// one of authentication method in auth.Authenticator. It is using OpenID for
// login flow, stores session ID in cookies, and session itself in supplied
// SessionStore.
//
// It requires some routes to be added to the router. Use exact same instance
// of CookieAuthMethod in auth.Authenticator and when adding routes via
// InstallHandlers.
//
// DEPRECATED. Do not use.
type CookieAuthMethod struct {
	// SessionStore keeps user sessions in some permanent storage. Must be set,
	// otherwise all methods return ErrNotConfigured.
	SessionStore SessionStore

	// Insecure is true to allow http:// URLs and non-https cookies. Useful for
	// local development.
	Insecure bool

	// IncompatibleCookies is a list of cookies to remove when setting or clearing
	// session cookie. It is useful to get rid of GAE cookies when OpenID cookies
	// are being used. Having both is very confusing.
	IncompatibleCookies []string
}

// Make sure all extra interfaces are implemented.
var _ interface {
	auth.Method
	auth.UsersAPI
	auth.Warmable
	auth.HasHandlers
} = (*CookieAuthMethod)(nil)

// InstallHandlers installs HTTP handlers used in OpenID protocol. Must be
// installed in server HTTP router for OpenID authentication flow to work.
//
// Implements auth.HasHandlers.
func (m *CookieAuthMethod) InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET(loginURL, base, m.loginHandler)
	r.GET(logoutURL, base, m.logoutHandler)
	r.GET(callbackURL, base, m.callbackHandler)
}

// Warmup prepares local caches. It's optional.
//
// Implements auth.Warmable.
func (m *CookieAuthMethod) Warmup(ctx context.Context) (err error) {
	cfg, err := FetchOpenIDSettings(ctx)
	if err != nil {
		return
	}
	if cfg.DiscoveryURL != "" {
		_, err = openid.FetchDiscoveryDoc(ctx, cfg.DiscoveryURL)
	} else {
		logging.Infof(ctx, "Skipping OpenID warmup, not configured")
	}
	return
}

// Authenticate extracts peer's identity from the incoming request. It is part
// of auth.Method interface.
func (m *CookieAuthMethod) Authenticate(ctx context.Context, r auth.RequestMetadata) (*auth.User, auth.Session, error) {
	if m.SessionStore == nil {
		return nil, nil, ErrNotConfigured
	}

	// Grab session ID from the cookie.
	sid, err := decodeSessionCookie(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	if sid == "" {
		return nil, nil, nil
	}

	// Grab session (with user information) from the store.
	session, err := m.SessionStore.GetSession(ctx, sid)
	if err != nil {
		return nil, nil, err
	}
	if session == nil {
		(logging.Fields{"sid": sid}).Warningf(ctx, "The session cookie references unknown session")
		return nil, nil, nil
	}
	(logging.Fields{
		"sid":   sid,
		"email": session.User.Email,
	}).Debugf(ctx, "Fetched the session")
	return &session.User, nil, nil
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest. It is part of
// auth.UsersAPI interface.
func (m *CookieAuthMethod) LoginURL(ctx context.Context, dest string) (string, error) {
	if m.SessionStore == nil {
		return "", ErrNotConfigured
	}
	return makeRedirectURL(loginURL, dest)
}

// LogoutURL returns a URL that, when visited, signs the user out,
// then redirects the user to the URL specified by dest. It is part of
// auth.UsersAPI interface.
func (m *CookieAuthMethod) LogoutURL(ctx context.Context, dest string) (string, error) {
	if m.SessionStore == nil {
		return "", ErrNotConfigured
	}
	return makeRedirectURL(logoutURL, dest)
}

////

// loginHandler initiates login flow by redirecting user to OpenID login page.
func (m *CookieAuthMethod) loginHandler(ctx *router.Context) {
	c, rw, r := ctx.Request.Context(), ctx.Writer, ctx.Request

	dest, err := normalizeURL(r.URL.Query().Get("r"))
	if err != nil {
		replyError(c, rw, err, "Bad redirect URI (%q) - %s", dest, err)
		return
	}

	cfg, err := FetchOpenIDSettings(c)
	if err != nil {
		replyError(c, rw, err, "Can't load OpenID settings - %s", err)
		return
	}

	// `state` will be propagated by OpenID backend and will eventually show up
	// in callback URI handler. See callbackHandler.
	state := map[string]string{
		"dest_url": dest,
		"host_url": r.Host,
	}
	authURI, err := authenticationURI(c, cfg, state)
	if err != nil {
		replyError(c, rw, err, "Can't generate authentication URI - %s", err)
		return
	}
	http.Redirect(rw, r, authURI, http.StatusFound)
}

// logoutHandler nukes active session and redirect back to destination URL.
func (m *CookieAuthMethod) logoutHandler(ctx *router.Context) {
	c, rw, r := ctx.Request.Context(), ctx.Writer, ctx.Request

	dest, err := normalizeURL(r.URL.Query().Get("r"))
	if err != nil {
		replyError(c, rw, err, "Bad redirect URI (%q) - %s", dest, err)
		return
	}

	// Close a session if there's one.
	sid, err := decodeSessionCookie(c, r)
	if err != nil {
		replyError(c, rw, err, "Error when decoding session cookie - %s", err)
		return
	}
	if sid != "" {
		(logging.Fields{"sid": sid}).Infof(c, "Closing the session")
		if err = m.SessionStore.CloseSession(c, sid); err != nil {
			replyError(c, rw, err, "Error when closing the session - %s", err)
			return
		}
	}

	// Nuke all session cookies to get to a completely clean state.
	removeCookie(rw, r, sessionCookieName)
	m.removeIncompatibleCookies(rw, r)

	// Redirect to the final destination.
	logging.Infof(c, "Redirecting to %s", dest)
	http.Redirect(rw, r, dest, http.StatusFound)
}

// callbackHandler handles redirect from OpenID backend. Parameters contain
// authorization code that can be exchanged for user profile.
func (m *CookieAuthMethod) callbackHandler(ctx *router.Context) {
	c, rw, r := ctx.Request.Context(), ctx.Writer, ctx.Request

	// This code path is hit when user clicks "Deny" on consent page.
	q := r.URL.Query()
	errorMsg := q.Get("error")
	if errorMsg != "" {
		replyError(c, rw, errors.New("login error"), "OpenID login error: %s", errorMsg)
		return
	}

	// Validate inputs.
	code := q.Get("code")
	if code == "" {
		replyError(c, rw, errors.New("login error"), "Missing 'code' parameter")
		return
	}
	stateTok := q.Get("state")
	if stateTok == "" {
		replyError(c, rw, errors.New("login error"), "Missing 'state' parameter")
		return
	}
	state, err := validateStateToken(c, stateTok)
	if err != nil {
		replyError(c, rw, err, "Failed to validate 'state' token")
		return
	}

	// Revalidate "dest_url". It was already validate in loginHandler when
	// generating state token, but just in case.
	dest, err := normalizeURL(state["dest_url"])
	if err != nil {
		replyError(c, rw, err, "Bad redirect URI (%q) - %s", dest, err)
		return
	}

	// Callback URI is hardcoded in OAuth2 client config and must always point
	// to default version on GAE. Yet we want to support logging to non-default
	// versions that have different hostnames. Do some redirect dance here to pass
	// control to required version if necessary (so that it can set cookie on
	// non-default version domain). Same handler with same params, just with
	// different hostname. For most common case of signing in into default version
	// this code path is not triggered.
	if state["host_url"] != r.Host {
		// There's no Scheme in r.URL. Append one, otherwise url.String() returns
		// relative (broken) URL. And replace the hostname with desired one.
		url := *r.URL
		if m.Insecure {
			url.Scheme = "http"
		} else {
			url.Scheme = "https"
		}
		url.Host = state["host_url"]
		logging.Warningf(c, "Redirecting to callback URI on another host %q", url.Host)
		http.Redirect(rw, r, url.String(), http.StatusFound)
		return
	}

	// Use authorization code to grab user profile.
	cfg, err := FetchOpenIDSettings(c)
	if err != nil {
		replyError(c, rw, err, "Can't load OpenID settings - %s", err)
		return
	}
	uid, user, err := handleAuthorizationCode(c, cfg, code)
	if err != nil {
		replyError(c, rw, err, "Error when fetching user profile - %s", err)
		return
	}

	// Grab previous session from the cookie to close it once new one is created.
	prevSid, err := decodeSessionCookie(c, r)
	if err != nil {
		replyError(c, rw, err, "Error when decoding session cookie - %s", err)
		return
	}

	// Create session in the session store.
	expTime := clock.Now(c).Add(sessionCookieToken.Expiration)
	sid, err := m.SessionStore.OpenSession(c, uid, user, expTime)
	if err != nil {
		replyError(c, rw, err, "Error when creating the session - %s", err)
		return
	}
	(logging.Fields{"sid": sid}).Infof(c, "Opened a new session")

	// Kill previous session now that new one is successfully created.
	if prevSid != "" {
		(logging.Fields{"sid": prevSid}).Infof(c, "Closing the previous session")
		if err = m.SessionStore.CloseSession(c, prevSid); err != nil {
			replyError(c, rw, err, "Error when closing the session - %s", err)
			return
		}
	}

	// Set the cookies.
	cookie, err := makeSessionCookie(c, sid, !m.Insecure)
	if err != nil {
		replyError(c, rw, err, "Can't make session cookie - %s", err)
		return
	}
	http.SetCookie(rw, cookie)
	m.removeIncompatibleCookies(rw, r)

	// Redirect to the final destination page.
	logging.Infof(c, "Redirecting to %s", dest)
	http.Redirect(rw, r, dest, http.StatusFound)
}

// removeIncompatibleCookies removes cookies specified by m.IncompatibleCookies.
func (m *CookieAuthMethod) removeIncompatibleCookies(rw http.ResponseWriter, r *http.Request) {
	for _, cookie := range m.IncompatibleCookies {
		removeCookie(rw, r, cookie)
	}
}

////

// normalizeURL verifies URL is parsable and that it is relative.
func normalizeURL(dest string) (string, error) {
	u, err := url.Parse(dest)
	if err != nil {
		return "", err
	}
	// Note: '//host/path' is a location on a server named 'host'.
	if u.IsAbs() || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "", errBadDestinationURL
	}
	// path.Clean removes trailing slash. It matters for URLs though. Keep it.
	keepSlash := strings.HasSuffix(u.Path, "/")
	u.Path = path.Clean(u.Path)
	if !strings.HasSuffix(u.Path, "/") && keepSlash {
		u.Path += "/"
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "", errBadDestinationURL
	}
	return u.String(), nil
}

// makeRedirectURL is used to generate login and logout URLs.
func makeRedirectURL(base, dest string) (string, error) {
	dest, err := normalizeURL(dest)
	if err != nil {
		return "", err
	}
	v := url.Values{}
	v.Set("r", dest)
	return base + "?" + v.Encode(), nil
}

// removeCookie sets a cookie to past expiration date so that browser can remove
// it. Also replaced value with junk, in case browser decides to ignore
// expiration time.
func removeCookie(rw http.ResponseWriter, r *http.Request, cookie string) {
	if prev, err := r.Cookie(cookie); err == nil {
		cpy := *prev
		cpy.Value = "deleted"
		cpy.Path = "/"
		cpy.MaxAge = -1
		cpy.Expires = time.Unix(1, 0)
		http.SetCookie(rw, &cpy)
	}
}

// replyError logs the error and replies with HTTP 500 (on transient errors) or
// HTTP 400 on fatal errors (that can happen only on bad requests).
func replyError(ctx context.Context, rw http.ResponseWriter, err error, msg string, args ...any) {
	code := http.StatusBadRequest
	if transient.Tag.In(err) {
		code = http.StatusInternalServerError
	}
	msg = fmt.Sprintf(msg, args...)
	logging.Errorf(ctx, "HTTP %d: %s", code, msg)
	http.Error(rw, msg, code)
}
