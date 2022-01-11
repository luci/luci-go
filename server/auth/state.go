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
	"fmt"
	"net"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/realms"
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

	// Method returns an authentication method used for the current request or nil
	// if the request is anonymous.
	//
	// If non-nil, its one of the methods in Authenticator.Methods.
	Method() Method

	// User holds the identity and profile of the current caller.
	//
	// User.Identity usually matches PeerIdentity(), but can be different if
	// the delegation is used.
	//
	// This field is never nil. For anonymous call it contains User with identity
	// AnonymousIdentity.
	//
	// Do not modify it.
	User() *User

	// Session is the session object produced by the authentication method.
	//
	// It may hold some extra information pertaining to the request. It may be nil
	// if there's no extra information. The session can be used to transfer
	// information from the authentication method to other parts of the auth
	// stack that execute later.
	Session() Session

	// PeerIdentity identifies whoever is making the request.
	//
	// It's an identity directly extracted from user credentials (ignoring
	// delegation tokens).
	PeerIdentity() identity.Identity

	// PeerIP is IP address (IPv4 or IPv6) of whoever is making the request or
	// nil if not available.
	PeerIP() net.IP

	// UserCredentials is an end-user credentials as they were received if they
	// are allowed to be forwarded.
	//
	// Includes the primary OAuth token and any extra LUCI-specific headers.
	UserCredentials() (*oauth2.Token, map[string]string, error)
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

// HasPermission returns true if the current caller has the given permission
// in the realm.
//
// A non-existing realm is replaced with the corresponding root realm (e.g. if
// "projectA:some/realm" doesn't exist, "projectA:@root" will be used in its
// place). If the project doesn't exist or is not using realms yet, all its
// realms (including the root realm) are considered empty. HasPermission returns
// false in this case.
//
// Attributes are the context of this particular permission check and are used
// as inputs to `conditions` predicates in conditional bindings. If a service
// supports conditional bindings, it must document what attributes it passes
// with each permission it checks.
//
// Returns an error only if the check itself failed due to a misconfiguration
// or transient issues. This should usually result in an Internal error.
func HasPermission(c context.Context, perm realms.Permission, realm string, attrs realms.Attrs) (bool, error) {
	if s := GetState(c); s != nil {
		return s.DB().HasPermission(c, s.User().Identity, perm, realm, attrs)
	}
	return false, ErrNotConfigured
}

// HasPermissionDryRun compares result of HasPermission to 'expected'.
//
// Intended to be used during the migration between the old and new ACL models.
type HasPermissionDryRun struct {
	ExpectedResult bool   // the expected result of this dry run
	TrackingBug    string // identifier of a particular migration, for logs
	AdminGroup     string // if given, implicitly grant all permissions to its members
}

// Execute calls HasPermission and compares the result to the expectations.
//
// Logs information about the call and any errors or discrepancies found.
//
// Accepts same arguments as HasPermission. Intentionally returns nothing.
func (dr HasPermissionDryRun) Execute(c context.Context, perm realms.Permission, realm string, attrs realms.Attrs) {
	s := GetState(c)
	if s == nil { // this should not really be happening at all
		logging.Errorf(c, "HasPermissionDryRun: no state in the context")
		return
	}

	db := s.DB()
	ident := s.User().Identity

	// We use python naming convention in the log to make Go and Python dry run
	// logs look identical in case we want to parse them.
	logPfx := fmt.Sprintf("has_permission_dryrun(%q, %q, %q), authdb=%d", perm, realm, ident, authdb.Revision(db))
	if dr.TrackingBug != "" {
		logPfx = dr.TrackingBug + ": " + logPfx
	}

	allowDeny := func(b bool) string {
		if b {
			return "ALLOW"
		}
		return "DENY"
	}

	switch result, err := db.HasPermission(c, ident, perm, realm, attrs); {
	case err != nil:
		logging.Errorf(c, "%s: error - want %s, got: %s", logPfx, allowDeny(dr.ExpectedResult), err)
	case result == dr.ExpectedResult:
		logging.Infof(c, "%s: match - %s", logPfx, allowDeny(result))
	case dr.AdminGroup == "" || !dr.ExpectedResult:
		logging.Warningf(c, "%s: mismatch - got %s, want %s", logPfx, allowDeny(result), allowDeny(dr.ExpectedResult))
	default:
		// We expected ALLOW, but got DENY. Maybe the legacy ACL check relied on
		// the admin group. Check this separately.
		switch admin, err := db.IsMember(c, ident, []string{dr.AdminGroup}); {
		case err != nil:
			logging.Errorf(c, "%s: error - want ALLOW, got: %s", logPfx, err)
		case admin:
			logging.Infof(c, "%s: match - ADMIN_ALLOW", logPfx)
		default:
			logging.Warningf(c, "%s: mismatch - got DENY, want ALLOW", logPfx)
		}
	}
}

// ShouldEnforceRealmACL is true if the service should enforce the realm's ACLs.
//
// Based on `enforce_in_service` realm data. Exists temporarily during the
// realms migration.
//
// TODO(crbug.com/1051724): Remove when no longer used.
func ShouldEnforceRealmACL(c context.Context, realm string) (bool, error) {
	s := GetState(c)
	if s == nil {
		return false, ErrNotConfigured
	}

	data, err := s.DB().GetRealmData(c, realm)
	switch {
	case err != nil:
		return false, errors.Annotate(err, "failed to load realm data").Err()
	case data == nil:
		return false, nil // no realms.cfg in the project at all
	case len(data.EnforceInService) == 0:
		return false, nil // enforced nowhere
	}

	info, err := GetSigner(c).ServiceInfo(c)
	if err != nil {
		return false, errors.Annotate(err, "failed to get our own service info").Err()
	}

	for _, id := range data.EnforceInService {
		if id == info.AppID {
			return true, nil
		}
	}
	return false, nil
}

// IsInWhitelist returns true if the current caller is in the given IP
// whitelist.
//
// Unknown whitelists are considered empty (the function returns false).
//
// May return errors if the check can not be performed (e.g. on datastore
// issues).
func IsInWhitelist(c context.Context, whitelist string) (bool, error) {
	if s := GetState(c); s != nil {
		return s.DB().IsInWhitelist(c, s.PeerIP(), whitelist)
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
	session       Session
	peerIdent     identity.Identity
	peerIP        net.IP

	// For AsCredentialsForwarder. 'endUserErr' (if not nil) would be returned by
	// GetRPCTransport when attempting to forward the credentials.
	endUserTok          *oauth2.Token
	endUserExtraHeaders map[string]string
	endUserErr          error
}

func (s *state) Authenticator() *Authenticator   { return s.authenticator }
func (s *state) DB() authdb.DB                   { return s.db }
func (s *state) Method() Method                  { return s.method }
func (s *state) User() *User                     { return s.user }
func (s *state) Session() Session                { return s.session }
func (s *state) PeerIdentity() identity.Identity { return s.peerIdent }
func (s *state) PeerIP() net.IP                  { return s.peerIP }
func (s *state) UserCredentials() (*oauth2.Token, map[string]string, error) {
	return s.endUserTok, s.endUserExtraHeaders, s.endUserErr
}

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

func (s backgroundState) Authenticator() *Authenticator   { return nil }
func (s backgroundState) Method() Method                  { return nil }
func (s backgroundState) User() *User                     { return &User{Identity: identity.AnonymousIdentity} }
func (s backgroundState) Session() Session                { return nil }
func (s backgroundState) PeerIdentity() identity.Identity { return identity.AnonymousIdentity }
func (s backgroundState) PeerIP() net.IP                  { return nil }
func (s backgroundState) UserCredentials() (*oauth2.Token, map[string]string, error) {
	return nil, nil, ErrNoForwardableCreds
}
