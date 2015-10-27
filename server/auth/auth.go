// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"net"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/auth/identity"
	"github.com/luci/luci-go/server/middleware"
)

var (
	// ErrNoUsersAPI is returned by LoginURL and LogoutURL if none of
	// the authentication methods support UsersAPI.
	ErrNoUsersAPI = errors.New("auth: methods do not support login or logout URL")
)

// Method implements particular kind of low level authentication mechanism for
// incoming requests. It may also optionally implement UsersAPI (if the method
// support login and logout URLs), HandlersInstaller (it the method needs
// some HTTP handlers to be exposed) and Warmable (if the method support Warmup
// method). Use type sniffing to figure out.
type Method interface {
	// Authenticate extracts user information from the incoming request.
	// It returns:
	//   * (*User, nil) on success.
	//   * (nil, nil) if the method is not applicable.
	//   * (nil, error) if the method is applicable, but credentials are invalid.
	Authenticate(context.Context, *http.Request) (*User, error)
}

// UsersAPI may be additionally implemented by Method if it supports login and
// logout URLs.
type UsersAPI interface {
	// LoginURL returns a URL that, when visited, prompts the user to sign in,
	// then redirects the user to the URL specified by dest.
	LoginURL(c context.Context, dest string) (string, error)

	// LogoutURL returns a URL that, when visited, signs the user out,
	// then redirects the user to the URL specified by dest.
	LogoutURL(c context.Context, dest string) (string, error)
}

// HandlersInstaller may be additionally implemented by Method if it needs some
// HTTP handlers to be exposed.
type HandlersInstaller interface {
	// InstallHandlers installs HTTP handlers require by the auth method. They
	// use middleware chain specified by `base`.
	InstallHandlers(r *httprouter.Router, base middleware.Base)
}

// Warmable may be additionally implemented by Method if it has some local
// caches that should be warmed up. Warmup is performed on best effort basis and
// not guaranteed to happen or finish before Method is used.
type Warmable interface {
	// Warmup prepares local caches.
	Warmup(c context.Context) error
}

// User represents identity and profile of a user.
type User struct {
	// Identity is identity string of the user (may be AnonymousIdentity).
	// If User is returned by Authenticate(...), Identity string is always present
	// and valid.
	Identity identity.Identity

	// Superuser is true if the user is site-level administrator. For example, on
	// GAE this bit is set for GAE-level administrators. Optional, default false.
	Superuser bool

	// Email is email of the user. Optional, default "". Don't use it as a key
	// in various structures. Prefer to use Identity() instead (it is always
	// available).
	Email string

	// Name is full name of the user. Optional, default "".
	Name string

	// Picture is URL of the user avatar. Optional, default "".
	Picture string

	// ClientID is the ID of the pre-registered OAuth2 client so its identity can
	// be verified. Used only by authentication methods based on OAuth2.
	// See https://developers.google.com/console/help/#generatingoauth2 for more.
	ClientID string
}

// Authenticator perform authentication of incoming requests. It is stateful
// object (holds in-memory caches), so declare it only once and then reuse
// across many requests.
type Authenticator struct {
	// methods is a list of authentication methods to try.
	methods []Method
}

// NewAuthenticator returns new instance of Authenticator configured to use
// given list of methods. Methods will be checked sequentially when
// authenticating a request until a first hit.
func NewAuthenticator(methods []Method) *Authenticator {
	return &Authenticator{methods}
}

// InstallHandlers installs HTTP handlers require by auth methods. They
// use middleware chain specified by `base`.
func (a *Authenticator) InstallHandlers(r *httprouter.Router, base middleware.Base) {
	for _, m := range a.methods {
		if i, ok := m.(HandlersInstaller); ok {
			i.InstallHandlers(r, base)
		}
	}
}

// Warmup prepares local caches. It is optional
func (a *Authenticator) Warmup(c context.Context) error {
	me := errors.NewLazyMultiError(len(a.methods))
	for i, m := range a.methods {
		if w, ok := m.(Warmable); ok {
			me.Assign(i, w.Warmup(c))
		}
	}
	return me.Get()
}

// Authenticate authenticates incoming requests and returns new context.Context
// with State stored into it. Returns error if credentials are provided, but
// invalid. If no credentials are provided (i.e. the request is anonymous),
// finishes successfully, but in that case State.Identity() will return
// AnonymousIdentity.
func (a *Authenticator) Authenticate(c context.Context, r *http.Request) (context.Context, error) {
	// Pick first authentication method that applies.
	var s state
	for _, m := range a.methods {
		var err error
		s.user, err = m.Authenticate(c, r)
		if err != nil {
			return nil, err
		}
		if s.user != nil {
			if err = s.user.Identity.Validate(); err != nil {
				return nil, err
			}
			s.method = m
			break
		}
	}

	// If no authentication method is applicable, default to anonymous identity.
	if s.method == nil {
		s.user = &User{Identity: identity.AnonymousIdentity}
	}
	s.peerIdent = s.user.Identity
	s.peerIP = net.ParseIP(strings.SplitN(r.RemoteAddr, ":", 2)[0])

	// TODO(vadimsh): Check IP whitelist.
	// TODO(vadimsh): Check host token.
	// TODO(vadimsh): Check delegation token.

	// Inject auth state. Also make sure this instance of authenticator is in
	// the returned context.
	c = context.WithValue(c, stateContextKey(0), &s)
	c = SetAuthenticator(c, a)
	return c, nil
}

// usersAPI returns implementation of UsersAPI by examining Methods. Returns nil
// if none of Methods implement UsersAPI.
func (a *Authenticator) usersAPI() UsersAPI {
	for _, m := range a.methods {
		if api, ok := m.(UsersAPI); ok {
			return api
		}
	}
	return nil
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest.
func (a *Authenticator) LoginURL(c context.Context, dest string) (string, error) {
	if api := a.usersAPI(); api != nil {
		return api.LoginURL(c, dest)
	}
	return "", ErrNoUsersAPI
}

// LogoutURL returns a URL that, when visited, signs the user out,
// then redirects the user to the URL specified by dest.
func (a *Authenticator) LogoutURL(c context.Context, dest string) (string, error) {
	if api := a.usersAPI(); api != nil {
		return api.LogoutURL(c, dest)
	}
	return "", ErrNoUsersAPI
}
