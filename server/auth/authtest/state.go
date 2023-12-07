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

package authtest

import (
	"net"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/realms"
)

// FakeState implements auth.State by returning predefined values.
//
// Inject it into the context when testing handlers that expect an auth state:
//
//	ctx = auth.WithState(ctx, &authtest.FakeState{
//	  Identity: "user:user@example.com",
//	  IdentityGroups: []string{"admins"},
//	  IdentityPermissions: []authtest.RealmPermission{
//	    {"proj:realm1", perm1},
//	    {"proj:realm1", perm2},
//	  }
//	})
//	auth.IsMember(ctx, "admins") -> returns true.
//	auth.HasPermission(ctx, perm1, "proj:realm1", nil) -> returns true.
//
// Note that IdentityGroups, IdentityPermissions, PeerIPAllowlist and Error
// are effective only when FakeDB is nil. They are used as a shortcut to
// construct the corresponding FakeDB on the fly. If you need to prepare a more
// complex fake state, pass NewFakeDB(...) as FakeDB instead:
//
//	ctx = auth.WithState(ctx, &authtest.FakeState{
//	  Identity: "user:user@example.com",
//	  FakeDB: NewFakeDB(
//	    authtest.MockMembership("user:user@example.com", "group"),
//	    authtest.MockMembership("user:another@example.com", "group"),
//	    authtest.MockPermission("user:user@example.com", "proj:realm1", perm1),
//	    ...
//	  ),
//	})
type FakeState struct {
	// Identity is main identity associated with the request.
	//
	// identity.AnonymousIdentity if not set.
	Identity identity.Identity

	// IdentityGroups is list of groups the calling identity belongs to.
	IdentityGroups []string

	// IdentityPermissions is a list of (realm, permission) tuples that define
	// caller's permissions.
	IdentityPermissions []RealmPermission

	// PeerIPAllowlist is a list of IP allowlists the caller IP belongs to.
	PeerIPAllowlist []string

	// Error, if not nil, is returned by auth DB checks.
	Error error

	// FakeDB is an authdb.DB implementation to use.
	//
	// If not nil, takes precedence over IdentityGroups, IdentityPermissions,
	// PeerIPAllowlist and Error.
	FakeDB authdb.DB

	// SessionOverride may be set for Session() to return custom value.
	//
	// By default Session() returns nil.
	SessionOverride auth.Session

	// PeerIdentityOverride may be set for PeerIdentity() to return custom value.
	//
	// By default PeerIdentity() returns Identity (i.e. no delegation is
	// happening).
	PeerIdentityOverride identity.Identity

	// PeerIPOverride may be set for PeerIP() to return custom value.
	//
	// By default PeerIP() returns "127.0.0.1".
	PeerIPOverride net.IP

	// UserCredentialsOverride may be set to override UserCredentials().
	//
	// By default UserCredentials() returns ErrNoForwardableCreds error.
	UserCredentialsOverride *oauth2.Token

	// UserExtra is returned as Extra field of User() return value.
	UserExtra any
}

// RealmPermission is used to populate IdentityPermissions in FakeState.
type RealmPermission struct {
	Realm      string
	Permission realms.Permission
}

var _ auth.State = (*FakeState)(nil)

// Authenticator is part of State interface.
func (s *FakeState) Authenticator() *auth.Authenticator {
	return &auth.Authenticator{
		Methods: []auth.Method{
			&FakeAuth{User: s.User()},
		},
	}
}

// DB is part of State interface.
func (s *FakeState) DB() authdb.DB {
	if s.FakeDB != nil {
		return s.FakeDB
	}

	ident := s.User().Identity
	peerIP := s.PeerIP().String()

	// We construct it on the fly each time to allow FakeState users to modify
	// Identity, IdentityGroups, IdentityPermissions, etc. dynamically.
	mocks := []MockedDatum{}
	for _, group := range s.IdentityGroups {
		mocks = append(mocks, MockMembership(ident, group))
	}
	for _, perm := range s.IdentityPermissions {
		mocks = append(mocks, MockPermission(ident, perm.Realm, perm.Permission))
	}
	for _, wl := range s.PeerIPAllowlist {
		mocks = append(mocks, MockIPAllowlist(peerIP, wl))
	}
	if s.Error != nil {
		mocks = append(mocks, MockError(s.Error))
	}
	return NewFakeDB(mocks...)
}

// Method is part of State interface.
func (s *FakeState) Method() auth.Method {
	return s.Authenticator().Methods[0]
}

// User is part of State interface.
func (s *FakeState) User() *auth.User {
	ident := identity.AnonymousIdentity
	if s.Identity != "" {
		ident = s.Identity
	}
	return &auth.User{
		Identity: ident,
		Email:    ident.Email(),
		Extra:    s.UserExtra,
	}
}

// Session is part of State interface.
func (s *FakeState) Session() auth.Session {
	return s.SessionOverride
}

// PeerIdentity is part of State interface.
func (s *FakeState) PeerIdentity() identity.Identity {
	if s.PeerIdentityOverride == "" {
		return s.User().Identity
	}
	return s.PeerIdentityOverride
}

// PeerIP is part of State interface.
func (s *FakeState) PeerIP() net.IP {
	if s.PeerIPOverride == nil {
		return net.ParseIP("127.0.0.1")
	}
	return s.PeerIPOverride
}

// UserCredentials is part of State interface.
func (s *FakeState) UserCredentials() (*oauth2.Token, map[string]string, error) {
	if s.UserCredentialsOverride != nil {
		return s.UserCredentialsOverride, nil, nil
	}
	return nil, nil, auth.ErrNoForwardableCreds
}
