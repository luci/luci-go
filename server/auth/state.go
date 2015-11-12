// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"net"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/auth/identity"
)

// State is stored in the context when handling an incoming request. It
// contains authentication related state of the current request.
type State interface {
	// Method returns authentication method used for current request or nil if
	// request is anonymous.
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
}

type stateContextKey int

// GetState return State stored in the context or nil if it is not available.
func GetState(c context.Context) State {
	if s, ok := c.Value(stateContextKey(0)).(State); ok && s != nil {
		return s
	}
	return nil
}

// CurrentUser represents the current caller. Shortcut for GetState(c).User().
// Returns user that represents identity.AnonymousIdentity if the context
// doesn't have State.
func CurrentUser(c context.Context) *User {
	if s := GetState(c); s != nil {
		return s.User()
	}
	return &User{Identity: identity.AnonymousIdentity}
}

// CurrentIdentity return identity of the current caller. Shortcut for
// GetState(c).User().Identity(). Returns identity.AnonymousIdentity if
// the context doesn't have State.
func CurrentIdentity(c context.Context) identity.Identity {
	if s := GetState(c); s != nil {
		return s.User().Identity
	}
	return identity.AnonymousIdentity
}

///

// state implements State. Immutable.
type state struct {
	db        DB
	method    Method
	user      *User
	peerIdent identity.Identity
	peerIP    net.IP
}

func (s *state) Method() Method                  { return s.method }
func (s *state) User() *User                     { return s.user }
func (s *state) PeerIdentity() identity.Identity { return s.peerIdent }
func (s *state) PeerIP() net.IP                  { return s.peerIP }
