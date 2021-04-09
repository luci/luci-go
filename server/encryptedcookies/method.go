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

package encryptedcookies

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/openid"
	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/router"
)

// OpenIDConfig is a configuration related to OpenID Connect provider.
//
// All parameters are required.
type OpenIDConfig struct {
	// DiscoveryURL is where to grab discovery document with provider's config.
	DiscoveryURL string

	// ClientID identifies OAuth2 Web client representing the application.
	//
	// Can be obtained by registering the OAuth2 client with the identity
	// provider.
	ClientID string

	// ClientSecret is a secret associated with ClientID.
	//
	// Can be obtained by registering the OAuth2 client with the identity
	// provider.
	ClientSecret string

	// RedirectURI must be `https://<host>/auth/openid/callback`.
	//
	// The OAuth2 client should be configured to allow this redirect URL.
	RedirectURI string
}

// Method is an auth.Method implementation that uses encrypted cookies.
//
// Uses OpenID Connect to establish sessions and refresh tokens to verify
// OpenID identity provider still knows about the user.
type AuthMethod struct {
	// Configuration returns OpenID Connect configuration parameters.
	//
	// Required.
	OpenIDConfig func(ctx context.Context) (*OpenIDConfig, error)

	// Sessions keeps user sessions in some permanent storage.
	//
	// Required.
	Sessions session.Store

	// Insecure is true to allow http:// URLs and non-https cookies. Useful for
	// local development.
	Insecure bool

	// IncompatibleCookies is a list of cookies to remove when setting or clearing
	// the session cookie. It is useful to get rid of cookies from previously used
	// authentication methods.
	IncompatibleCookies []string
}

var _ interface {
	auth.Method
	auth.UsersAPI
	auth.Warmable
	auth.HasHandlers
} = (*AuthMethod)(nil)

const (
	loginURL    = "/auth/openid/login"
	logoutURL   = "/auth/openid/logout"
	callbackURL = "/auth/openid/callback"
)

// InstallHandlers installs HTTP handlers used in the login protocol.
//
// Implements auth.HasHandlers.
func (m *AuthMethod) InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET(loginURL, base, m.loginHandler)
	r.GET(logoutURL, base, m.logoutHandler)
	r.GET(callbackURL, base, m.callbackHandler)
}

// Warmup prepares local caches.
//
// Implements auth.Warmable.
func (m *AuthMethod) Warmup(ctx context.Context) error {
	return fmt.Errorf("not implemented")
}

// Authenticate authenticates the request.
//
// Implements auth.Method.
func (m *AuthMethod) Authenticate(ctx context.Context, r *http.Request) (*auth.User, auth.Session, error) {
	cfg, err := m.checkConfigured(ctx)
	if err != nil {
		return nil, nil, err
	}
	// TODO
	_ = cfg
	return nil, nil, fmt.Errorf("not implemented")
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest.
//
// Implements auth.UsersAPI.
func (m *AuthMethod) LoginURL(ctx context.Context, dest string) (string, error) {
	if _, err := m.checkConfigured(ctx); err != nil {
		return "", err
	}
	return makeRedirectURL(loginURL, dest)
}

// LogoutURL returns a URL that, when visited, signs the user out,
// then redirects the user to the URL specified by dest.
//
// Implements auth.UsersAPI.
func (m *AuthMethod) LogoutURL(ctx context.Context, dest string) (string, error) {
	if _, err := m.checkConfigured(ctx); err != nil {
		return "", err
	}
	return makeRedirectURL(logoutURL, dest)
}

////////////////////////////////////////////////////////////////////////////////

// handler is one of /login, /logout or /callback handlers.
type handler func(ctx context.Context, r *http.Request, rw http.ResponseWriter, cfg *OpenIDConfig, discovery *openid.DiscoveryDoc) error

// checkConfigured verifies the method is configured.
//
// Panics on API violations (i.e. coding errors) and merely returns an error if
// OpenIDConfig callback doesn't produce a valid config.
//
// Returns the resulting OpenIDConfig.
func (m *AuthMethod) checkConfigured(ctx context.Context) (*OpenIDConfig, error) {
	if m.OpenIDConfig == nil {
		panic("bad encryptedcookies.AuthMethod usage: OpenIDConfig is nil")
	}
	if m.Sessions == nil {
		panic("bad encryptedcookies.AuthMethod usage: Sessions is nil")
	}
	cfg, err := m.OpenIDConfig(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch OpenID config").Err()
	}
	switch {
	case cfg.DiscoveryURL == "":
		return nil, errors.Reason("bad OpenID config: no discovery URL").Err()
	case cfg.ClientID == "":
		return nil, errors.Reason("bad OpenID config: no client IDL").Err()
	case cfg.ClientSecret == "":
		return nil, errors.Reason("bad OpenID config: no client secret").Err()
	case cfg.RedirectURI == "":
		return nil, errors.Reason("bad OpenID config: no redirect URI").Err()
	}
	return cfg, nil
}

// handler is a common wrapper for routes registered in InstallHandlers.
func (m *AuthMethod) handler(ctx *router.Context, cb handler) {
	cfg, err := m.checkConfigured(ctx.Context)
	if err == nil {
		var discovery *openid.DiscoveryDoc
		discovery, err = openid.FetchDiscoveryDoc(ctx.Context, cfg.DiscoveryURL)
		if err != nil {
			err = errors.Annotate(err, "failed to fetch the discovery doc").Tag(transient.Tag).Err()
		} else {
			err = cb(ctx.Context, ctx.Request, ctx.Writer, cfg, discovery)
		}
	}
	if err != nil {
		code := http.StatusBadRequest
		if transient.Tag.In(err) {
			code = http.StatusInternalServerError
		}
		http.Error(ctx.Writer, err.Error(), code)
	}
}

// loginHandler initiates the login flow.
func (m *AuthMethod) loginHandler(ctx *router.Context) {
	m.handler(ctx, func(ctx context.Context, r *http.Request, rw http.ResponseWriter, cfg *OpenIDConfig, discovery *openid.DiscoveryDoc) error {
		return fmt.Errorf("not implemented")
	})
}

// logoutHandler closes the session.
func (m *AuthMethod) logoutHandler(ctx *router.Context) {
	m.handler(ctx, func(ctx context.Context, r *http.Request, rw http.ResponseWriter, cfg *OpenIDConfig, discovery *openid.DiscoveryDoc) error {
		return fmt.Errorf("not implemented")
	})
}

// callbackHandler handles a redirect from the OpenID provider.
func (m *AuthMethod) callbackHandler(ctx *router.Context) {
	m.handler(ctx, func(ctx context.Context, r *http.Request, rw http.ResponseWriter, cfg *OpenIDConfig, discovery *openid.DiscoveryDoc) error {
		return fmt.Errorf("not implemented")
	})
}

////////////////////////////////////////////////////////////////////////////////

// normalizeURL verifies URL is parsable and that it is relative.
func normalizeURL(dest string) (string, error) {
	if dest == "" {
		return "/", nil
	}
	u, err := url.Parse(dest)
	if err != nil {
		return "", errors.Annotate(err, "bad destination URL %q", dest).Err()
	}
	// Note: '//host/path' is a location on a server named 'host'.
	if u.IsAbs() || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return "", errors.Reason("bad absolute destination URL %q", u).Err()
	}
	// path.Clean removes trailing slash. It matters for URLs though. Keep it.
	keepSlash := strings.HasSuffix(u.Path, "/")
	u.Path = path.Clean(u.Path)
	if !strings.HasSuffix(u.Path, "/") && keepSlash {
		u.Path += "/"
	}
	if !strings.HasPrefix(u.Path, "/") {
		return "", errors.Reason("bad destination URL %q", u).Err()
	}
	return u.String(), nil
}

// makeRedirectURL is used to generate login and logout URLs.
func makeRedirectURL(base, dest string) (string, error) {
	dest, err := normalizeURL(dest)
	if err != nil {
		return "", err
	}
	if dest == "/" {
		return base, nil
	}
	v := url.Values{"r": {dest}}
	return base + "?" + v.Encode(), nil
}
