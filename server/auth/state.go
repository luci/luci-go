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

package auth

import (
	"context"
	"net"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth/authdb"
)

// State is stored in the context when handling an incoming request. It
// contains authentication related state of the current request.
type State interface {
	// Authenticator is an Authenticator used to authenticate the request.
	Authenticator() *Authenticator

	// DB is authdb.DB snapshot with authorization information to use when
	// processing this request.
	//
	// Use directly only when you know what your are doing. Prefer to use wrapping
	// functions (e.g. IsMember) instead.
	DB() authdb.DB

	// Method returns authentication method used for current request or nil if
	// request is anonymous.
	//
	// If non-nil, its one of the methods in Authenticator.Methods.
	Method() Method

	// User holds the identity and profile of the current caller. User.Identity
	// usually matches PeerIdentity(), but can be different if delegation is used.
	// This field is never nil. For anonymous call it contains User with identity
	// AnonymousIdentity. Do not modify it.
	User() *User

	// PeerIdentity identifies whoever is making the request. It's an identity
	// directly extracted from user credentials (ignoring delegation tokens).
	PeerIdentity() identity.Identity

	// PeerIP is IP address (IPv4 or IPv6) of whoever is making the request or
	// nil if not available.
	PeerIP() net.IP

	// UserCredentials is an end-user credentials as they were received if they
	// are allowed to be forwarded.
	UserCredentials() (*oauth2.Token, error)
}

type stateContextKey int

// WithState injects State into the context.
//
// Mostly useful from tests. Must not be normally used from production code,
// 'Authenticate' sets the state itself.
func WithState(c context.Context, s State) context.Context {
	return context.WithValue(c, stateContextKey(0), s)
}

// GetState return State stored in the context by 'Authenticate' call, the
// background state if 'Authenticate' wasn't used or nil if the auth library
// wasn't configured.
//
// The background state roughly is similar to the state of anonymous call.
// Various background non user-facing handlers (crons, task queues) that do not
// use 'Authenticate' see this state by default. Its most important role is to
// provide access to authdb.DB (and all functionality that depends on it) to
// background handlers.
func GetState(c context.Context) State {
	if s, ok := c.Value(stateContextKey(0)).(State); ok && s != nil {
		return s
	}
	if getConfig(c) != nil {
		return backgroundState{c}
	}
	return nil
}

// CurrentUser represents the current caller.
//
// Shortcut for GetState(c).User(). Returns user with AnonymousIdentity if
// the context doesn't have State.
func CurrentUser(c context.Context) *User {
	if s := GetState(c); s != nil {
		return s.User()
	}
	return &User{Identity: identity.AnonymousIdentity}
}

// CurrentIdentity return identity of the current caller.
//
// Shortcut for GetState(c).User().Identity(). Returns AnonymousIdentity if
// the context doesn't have State.
func CurrentIdentity(c context.Context) identity.Identity {
	if s := GetState(c); s != nil {
		return s.User().Identity
	}
	return identity.AnonymousIdentity
}

// IsMember returns true if the current caller is in any of the given groups.
//
// Unknown groups are considered empty (the function returns false).
//
// May return errors if the check can not be performed (e.g. on datastore
// issues).
func IsMember(c context.Context, groups ...string) (bool, error) {
	if s := GetState(c); s != nil {
		return s.DB().IsMember(c, s.User().Identity, groups)
	}
	return false, ErrNotConfigured
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest.
//
// Shortcut for GetState(c).Authenticator().LoginURL(...).
func LoginURL(c context.Context, dest string) (string, error) {
	if s := GetState(c); s != nil {
		return s.Authenticator().LoginURL(c, dest)
	}
	return "", ErrNotConfigured
}

// LogoutURL returns a URL that, when visited, signs the user out, then
// redirects the user to the URL specified by dest.
//
// Shortcut for GetState(c).Authenticator().LogoutURL(...).
func LogoutURL(c context.Context, dest string) (string, error) {
	if s := GetState(c); s != nil {
		return s.Authenticator().LogoutURL(c, dest)
	}
	return "", ErrNotConfigured
}

///

// state implements State. Immutable.
type state struct {
	authenticator *Authenticator
	db            authdb.DB
	method        Method
	user          *User
	peerIdent     identity.Identity
	peerIP        net.IP

	// For AsCredentialsForwarder. Populated only when not using delegation.
	// 'endUserErr' (if not nil) would be returned by GetRPCTransport when
	// attempting to forward the credentials.
	endUserTok *oauth2.Token
	endUserErr error
}

func (s *state) Authenticator() *Authenticator           { return s.authenticator }
func (s *state) DB() authdb.DB                           { return s.db }
func (s *state) Method() Method                          { return s.method }
func (s *state) User() *User                             { return s.user }
func (s *state) PeerIdentity() identity.Identity         { return s.peerIdent }
func (s *state) PeerIP() net.IP                          { return s.peerIP }
func (s *state) UserCredentials() (*oauth2.Token, error) { return s.endUserTok, s.endUserErr }

///

// backgroundState corresponds to the state of auth library before any
// authentication is performed.
type backgroundState struct {
	ctx context.Context
}

func isBackgroundState(s State) bool {
	_, yes := s.(backgroundState)
	return yes
}

func (s backgroundState) DB() authdb.DB {
	db, err := GetDB(s.ctx)
	if err != nil {
		return authdb.ErroringDB{Error: err}
	}
	return db
}

func (s backgroundState) Authenticator() *Authenticator           { return nil }
func (s backgroundState) Method() Method                          { return nil }
func (s backgroundState) User() *User                             { return &User{Identity: identity.AnonymousIdentity} }
func (s backgroundState) PeerIdentity() identity.Identity         { return identity.AnonymousIdentity }
func (s backgroundState) PeerIP() net.IP                          { return nil }
func (s backgroundState) UserCredentials() (*oauth2.Token, error) { return nil, ErrNoForwardableCreds }
