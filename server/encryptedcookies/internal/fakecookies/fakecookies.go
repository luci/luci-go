// Copyright 2021 The LUCI Authors.
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

// Package fakecookies implements a cookie-based fake authentication method.
//
// It is used during the development instead of real encrypted cookies. It is
// absolutely insecure and must not be used in any real server.
package fakecookies

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/encryptedcookies/internal"
	"go.chromium.org/luci/server/router"
)

// AuthMethod is an auth.Method implementation that uses fake cookies.
type AuthMethod struct {
	// LimitCookieExposure, if set, makes the fake cookie behave the same way as
	// when this option is used with production cookies.
	//
	// See the module documentation.
	LimitCookieExposure bool
	// ExposedStateEndpoint is a URL path of the state endpoint, if any.
	ExposedStateEndpoint string

	m              sync.Mutex
	serverUser     *auth.User // see serverUserInfo
	serverUserInit bool       // true if already initialized (can still be nil)
}

var _ interface {
	auth.Method
	auth.UsersAPI
	auth.HasHandlers
	auth.HasStateEndpoint
} = (*AuthMethod)(nil)

const (
	loginURL          = "/auth/openid/login"
	logoutURL         = "/auth/openid/logout"
	defaultPictureURL = "/auth/openid/profile.svg"

	cookieName = "FAKE_LUCI_DEV_AUTH_COOKIE"
)

// InstallHandlers installs HTTP handlers used in the login protocol.
//
// Implements auth.HasHandlers.
func (m *AuthMethod) InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET(loginURL, base, m.loginHandlerGET)
	r.POST(loginURL, base, m.loginHandlerPOST)
	r.GET(logoutURL, base, m.logoutHandler)
	r.GET(defaultPictureURL, base, m.pictureHandler)
}

// Authenticate authenticates the request.
//
// Implements auth.Method.
func (m *AuthMethod) Authenticate(ctx context.Context, r auth.RequestMetadata) (*auth.User, auth.Session, error) {
	cookie, _ := r.Cookie(cookieName)
	if cookie == nil {
		return nil, nil, nil // the method is not applicable, skip it
	}

	email, err := decodeFakeCookie(cookie.Value)
	if err != nil {
		logging.Warningf(ctx, "Skipping %s: %s", cookieName, err)
		return nil, nil, nil
	}
	ident, err := identity.MakeIdentity("user:" + email)
	if err != nil {
		logging.Warningf(ctx, "Skipping %s: %s", cookieName, err)
		return nil, nil, nil
	}

	user := &auth.User{
		Identity: ident,
		Email:    email,
		Name:     "Some User",
		Picture:  defaultPictureURL,
	}

	// If the local developer logs in using their email, we can actually produce
	// real auth tokens (since the server runs under this account too). We can
	// also try to extract the real profile information. Not a big deal if it is
	// not available. It is not essential, just adds more "realism" when it is
	// present.
	if email == serverEmail(ctx) {
		switch serverUser, err := m.serverUserInfo(ctx); {
		case err != nil:
			return nil, nil, transient.Tag.Apply(errors.Fmt("transient error getting server's user info: %w", err))
		case serverUser != nil:
			user = serverUser
		}
		return user, serverSelfSession{}, nil
	}

	// If the fake session user is not matching server's email, use a fake profile
	// and install an erroring session that asks the caller to log in as
	// the developer. We can't generate real tokens for fake users.
	return user, erroringSession{
		err: fmt.Errorf(
			"session-bound auth tokens are available only when logging in "+
				"with the account used by the local dev server itself: %s", email,
		),
	}, nil
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest.
//
// Implements auth.UsersAPI.
func (m *AuthMethod) LoginURL(ctx context.Context, dest string) (string, error) {
	return internal.MakeRedirectURL(loginURL, dest)
}

// LogoutURL returns a URL that, when visited, signs the user out,
// then redirects the user to the URL specified by dest.
//
// Implements auth.UsersAPI.
func (m *AuthMethod) LogoutURL(ctx context.Context, dest string) (string, error) {
	return internal.MakeRedirectURL(logoutURL, dest)
}

// StateEndpointURL returns an URL that serves the authentication state.
//
// Implements auth.HasStateEndpoint.
func (m *AuthMethod) StateEndpointURL(ctx context.Context) (string, error) {
	if m.ExposedStateEndpoint != "" {
		return m.ExposedStateEndpoint, nil
	}
	return "", auth.ErrNoStateEndpoint
}

// IsFakeCookiesSession returns true if the given auth.Session was produced by
// a fake cookies auth method.
func IsFakeCookiesSession(s auth.Session) bool {
	switch s.(type) {
	case serverSelfSession, erroringSession:
		return true
	default:
		return false
	}
}

////////////////////////////////////////////////////////////////////////////////

var loginPageTmpl = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<title>Dev Mode Fake Login</title>
<style>
body {
	font-family: "Roboto", sans-serif;
}
.container {
	width: 440px;
	padding-top: 50px;
	margin: auto;
}
.form {
	position: relative;
	max-width: 440px;
	padding: 45px;
	margin: 0 auto 100px;
	background: #ffffff;
	text-align: center;
	box-shadow: 0 0 20px 0 rgba(0, 0, 0, 0.2), 0 5px 5px 0 rgba(0, 0, 0, 0.24);
}
.form input {
	width: 100%;
	padding: 15px;
	margin: 0 0 15px;
	background: #f2f2f2;
	outline: 0;
	border: 0;
	box-sizing: border-box;
	font-size: 14px;
}
.form button {
	width: 100%;
	padding: 15px;
	outline: 0;
	border: 0;
	background: #404040;
	color: #ffffff;
	font-size: 14px;
	cursor: pointer;
}
.form button:hover, .form button:active, .form button:focus {
	background: #212121;
}
</style>
</head>
<body>
<div class="container">
	<div class="form">
		<form method="POST">
			<input type="text" placeholder="EMAIL" name="email" value="{{.Email}}"/>
			<button>LOGIN</button>
		</form>
	</div>
</div>
</body>
</html>`))

const profilePictureSVG = `<svg xmlns="http://www.w3.org/2000/svg" height="96px" width="96px" viewBox="0 0 24 24" fill="#455A64">
<path d="M0 0h24v24H0V0z" fill="none"/>
<path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2zm6.36 14.83c-1.43-1.74-4.9-2.33-6.36-2.33s-4.93.59-6.36 2.33C4.62 15.49 4 13.82 4 12c0-4.41 3.59-8 8-8s8 3.59 8 8c0 1.82-.62 3.49-1.64 4.83zM12 6c-1.94 0-3.5 1.56-3.5 3.5S10.06 13 12 13s3.5-1.56 3.5-3.5S13.94 6 12 6z"/>
</svg>`

// encodeFakeCookie prepares a cookie value that contains the given email.
func encodeFakeCookie(email string) string {
	return (url.Values{"email": {email}}).Encode()
}

// decodeFakeCookies is reverse of encodeFakeCookie.
func decodeFakeCookie(val string) (email string, err error) {
	v, err := url.ParseQuery(val)
	if err != nil {
		return "", err
	}
	return v.Get("email"), nil
}

// serverEmail returns the email the server runs as or "".
//
// In most cases the local dev server runs under the developer account.
func serverEmail(ctx context.Context) string {
	if s := auth.GetSigner(ctx); s != nil {
		if info, _ := s.ServiceInfo(ctx); info != nil {
			return info.ServiceAccountName
		}
	}
	return ""
}

// handler adapts `cb(...)` to match router.Handler.
func handler(ctx *router.Context, cb func(ctx context.Context, r *http.Request, rw http.ResponseWriter) error) {
	if err := cb(ctx.Request.Context(), ctx.Request, ctx.Writer); err != nil {
		http.Error(ctx.Writer, err.Error(), http.StatusInternalServerError)
	}
}

// loginHandlerGET initiates the login flow.
func (m *AuthMethod) loginHandlerGET(ctx *router.Context) {
	handler(ctx, func(ctx context.Context, r *http.Request, rw http.ResponseWriter) error {
		if _, err := internal.NormalizeURL(r.URL.Query().Get("r")); err != nil {
			return errors.Fmt("bad redirect URI: %w", err)
		}
		email := serverEmail(ctx)
		if email == "" {
			email = "someone@example.com"
		}
		return loginPageTmpl.Execute(rw, map[string]string{"Email": email})
	})
}

// loginHandlerPOST completes the login flow.
func (m *AuthMethod) loginHandlerPOST(ctx *router.Context) {
	handler(ctx, func(ctx context.Context, r *http.Request, rw http.ResponseWriter) error {
		dest, err := internal.NormalizeURL(r.URL.Query().Get("r"))
		if err != nil {
			return errors.Fmt("bad redirect URI: %w", err)
		}
		email := r.FormValue("email")
		if _, err := identity.MakeIdentity("user:" + email); err != nil {
			return errors.Fmt("bad email: %w", err)
		}

		var curPath, prevPath string
		var sameSite http.SameSite
		if m.LimitCookieExposure {
			curPath = internal.LimitedCookiePath
			prevPath = internal.UnlimitedCookiePath
			sameSite = http.SameSiteStrictMode
		} else {
			curPath = internal.UnlimitedCookiePath
			prevPath = internal.LimitedCookiePath
			sameSite = 0 // use browser's default
		}

		http.SetCookie(rw, &http.Cookie{
			Name:     cookieName,
			Value:    encodeFakeCookie(email),
			Path:     curPath,
			SameSite: sameSite,
			HttpOnly: true,
			Secure:   false,
			MaxAge:   60 * 60 * 24 * 14, // 2 weeks
		})
		internal.RemoveCookie(rw, r, cookieName, prevPath)

		http.Redirect(rw, r, dest, http.StatusFound)
		return nil
	})
}

// logoutHandler closes the session.
func (m *AuthMethod) logoutHandler(ctx *router.Context) {
	handler(ctx, func(ctx context.Context, r *http.Request, rw http.ResponseWriter) error {
		dest, err := internal.NormalizeURL(r.URL.Query().Get("r"))
		if err != nil {
			return errors.Fmt("bad redirect URI: %w", err)
		}
		internal.RemoveCookie(rw, r, cookieName, internal.UnlimitedCookiePath)
		internal.RemoveCookie(rw, r, cookieName, internal.LimitedCookiePath)
		http.Redirect(rw, r, dest, http.StatusFound)
		return nil
	})
}

// pictureHandler returns hardcoded SVG user profile picture.
func (m *AuthMethod) pictureHandler(ctx *router.Context) {
	ctx.Writer.Header().Set("Content-Type", "image/svg+xml")
	ctx.Writer.Header().Set("Cache-Control", "public, max-age=86400")
	ctx.Writer.Write([]byte(profilePictureSVG))
}

// serverUserInfo grabs *auth.User info based on server's own credentials.
//
// We use Google ID provider's /userinfo endpoint and access tokens. Note that
// we can't extract the profile information from the ID token since it may not
// be there anymore (if the token was refreshed already).
//
// Returns (nil, nil) if the user info is not available for some reason (e.g.
// when running the server under a service account). All errors should be
// considered transient.
func (m *AuthMethod) serverUserInfo(ctx context.Context) (*auth.User, error) {
	m.m.Lock()
	defer m.m.Unlock()
	if m.serverUserInit {
		return m.serverUser, nil
	}

	// See the comment in serverSelfSession.AccessToken regarding scopes.
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return nil, err
	}

	req, _ := http.NewRequest("GET", "https://openidconnect.googleapis.com/v1/userinfo", nil)
	resp, err := (&http.Client{Transport: tr}).Do(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 500 {
		return nil, errors.Fmt("HTTP %d: %q", resp.StatusCode, body)
	}

	if resp.StatusCode != 200 {
		logging.Warningf(ctx, "When fetching server's own user info: HTTP %d, body %q", resp.StatusCode, body)
		m.serverUserInit = true // we are done, no user info available
		return nil, nil
	}

	var claims struct {
		Email   string `json:"email"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
	}
	if err := json.Unmarshal(body, &claims); err != nil {
		return nil, errors.Fmt("failed to deserialize userinfo endpoint response: %w", err)
	}

	m.serverUserInit = true
	m.serverUser = &auth.User{
		Identity: identity.Identity("user:" + claims.Email),
		Email:    claims.Email,
		Name:     claims.Name,
		Picture:  claims.Picture,
	}
	return m.serverUser, nil
}

////////////////////////////////////////////////////////////////////////////////

// serverSelfSession implements auth.Session by using server's own credentials.
//
// This is useful only when the session user matches the account the server
// is running as. This can happen only locally in the dev mode.
type serverSelfSession struct{}

func (serverSelfSession) AccessToken(ctx context.Context) (*oauth2.Token, error) {
	// Strictly speaking we need only userinfo.email scope, but its refresh token
	// might not be present locally. But a token with scopes.CloudScopeSet()
	// (which includes the userinfo.email scope) is guaranteed to be present,
	// since the server checks for it when it starts.
	ts, err := auth.GetTokenSource(
		ctx,
		auth.AsSelf,
		auth.WithScopes(scopes.CloudScopeSet()...),
	)
	if err != nil {
		return nil, err
	}
	return ts.Token()
}

func (serverSelfSession) IDToken(ctx context.Context) (*oauth2.Token, error) {
	// In a real scenario ID token audience always matches the OAuth client ID
	// used during the login. We use some similarly looking fake. Note that this
	// fake is ignored when running locally using a token established with
	// `luci-auth login` (there's no way to substitute audiences of such local
	// tokens).
	ts, err := auth.GetTokenSource(
		ctx,
		auth.AsSelf,
		auth.WithIDTokenAudience("fake-client-id.apps.example.com"),
	)
	if err != nil {
		return nil, err
	}
	return ts.Token()
}

// erroringSession returns the given error from all methods.
type erroringSession struct {
	err error
}

func (s erroringSession) AccessToken(ctx context.Context) (*oauth2.Token, error) {
	return nil, s.err
}

func (s erroringSession) IDToken(ctx context.Context) (*oauth2.Token, error) {
	return nil, s.err
}
