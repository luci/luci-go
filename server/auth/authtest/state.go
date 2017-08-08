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

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/identity"
)

// FakeState implements auth.State returning predefined values.
//
// Inject it into the context when testing handlers that expect auth state:
//
//   ctx = auth.WithState(ctx, &authtest.FakeState{
//     Identity: "user:user@example.com",
//     IdentityGroups: []string{"admins"},
//   })
//   auth.IsMember(ctx, "admins") -> returns true.
type FakeState struct {
	// Identity is main identity associated with the request.
	//
	// identity.AnonymousIdentity if not set.
	Identity identity.Identity

	// IdentityGroups is list of groups the calling identity belongs to.
	IdentityGroups []string

	// Error if not nil is returned by IsMember checks.
	Error error

	// FakeDB is a mock authdb.DB implementation to use.
	//
	// If not nil, overrides 'IdentityGroups' and 'Error'.
	FakeDB authdb.DB

	// PeerIdentityOverride may be set for PeerIdentity() to return custom value.
	//
	// By default PeerIdentity() returns Identity (i.e. no delegation is
	// happening).
	PeerIdentityOverride identity.Identity

	// PeerIPOverride may be set for PeerIP() to return custom value.
	//
	// By default PeerIP() returns "127.0.0.1".
	PeerIPOverride net.IP
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
	return &FakeErroringDB{
		FakeDB: FakeDB{s.User().Identity: s.IdentityGroups},
		Error:  s.Error,
	}
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
	}
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
