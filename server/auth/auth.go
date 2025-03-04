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
	"net/http"

	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/delegation"
	"go.chromium.org/luci/server/auth/internal/tracing"
	"go.chromium.org/luci/server/auth/signing"
	"go.chromium.org/luci/server/router"
)

var (
	// Authenticate errors (must be grpc-tagged).

	// ErrNotConfigured is returned by Authenticate and other functions if the
	// context wasn't previously initialized via 'Initialize'.
	ErrNotConfigured = errors.New("auth: the library is not properly configured", grpcutil.InternalTag)

	// ErrBadClientID is returned by Authenticate if caller is using an OAuth2
	// client ID not in the list of allowed IDs. More info is in the log.
	ErrBadClientID = errors.New("auth: OAuth client_id is not in the allowlist", grpcutil.PermissionDeniedTag)

	// ErrBadAudience is returned by Authenticate if token's audience is unknown.
	ErrBadAudience = errors.New("auth: bad token audience", grpcutil.PermissionDeniedTag)

	// ErrBadRemoteAddr is returned by Authenticate if request's remote_addr can't
	// be parsed.
	ErrBadRemoteAddr = errors.New("auth: bad remote addr", grpcutil.InternalTag)

	// ErrForbiddenIP is returned when an account is restricted by an IP allowlist
	// and request's remote_addr is not in it.
	ErrForbiddenIP = errors.New("auth: IP is not in the allowlist", grpcutil.PermissionDeniedTag)

	// ErrProjectHeaderForbidden is returned by Authenticate if an unknown caller
	// tries to use X-Luci-Project header. Only a preapproved set of callers are
	// allowed to use this header, see InternalServicesGroup.
	ErrProjectHeaderForbidden = errors.New("auth: the caller is not allowed to use X-Luci-Project", grpcutil.PermissionDeniedTag)

	// Other errors.

	// ErrNoUsersAPI is returned by LoginURL and LogoutURL if none of
	// the authentication methods support UsersAPI.
	ErrNoUsersAPI = errors.New("auth: methods do not support login or logout URL")

	// ErrNoForwardableCreds is returned by GetRPCTransport when attempting to
	// forward credentials (via AsCredentialsForwarder) that are not forwardable.
	ErrNoForwardableCreds = errors.New("auth: no forwardable credentials in the context")

	// ErrNoStateEndpoint is returned by StateEndpointURL if the state endpoint is
	// not exposed.
	ErrNoStateEndpoint = errors.New("auth: the state endpoint is not available")
)

const (
	// InternalServicesGroup is a name of a group with service accounts of LUCI
	// microservices of the current LUCI deployment (and only them!).
	//
	// Accounts in this group are allowed to use X-Luci-Project header to specify
	// that RPCs are done in a context of some particular project. For such
	// requests CurrentIdentity() == 'project:<X-Luci-Project value>'.
	//
	// This group should contain only **fully trusted** services, deployed and
	// managed by the LUCI deployment administrators. Adding "random" services
	// here is a security risk, since they will be able to impersonate any LUCI
	// project.
	InternalServicesGroup = "auth-luci-services"
)

// RequestMetadata is metadata used when authenticating a request.
//
// Can be constructed by:
//   - RequestMetadataForHTTP based on http.Request.
//   - authtest.NewFakeRequestMetadata based on fakes for unit tests.
type RequestMetadata interface {
	// Header returns a value of a given header or an empty string.
	//
	// Headers are also known as simply "metadata" in gRPC world.
	//
	// The key is case-insensitive. If the request has multiple headers matching
	// the key, returns only the first one.
	Header(key string) string

	// Cookie returns a cookie or an error if there's no such cookie.
	//
	// Transports that do not support cookies (e.g. gRPC) can always return
	// an error. They will just not work with authentication schemes based on
	// cookies.
	Cookie(key string) (*http.Cookie, error)

	// RemoteAddr returns the IP address the request came from or "" if unknown.
	//
	// It is used by default for IP allowlist checks if there's no EndUserIP
	// callback set in the auth library configuration. The EndUserIP callback is
	// usually set in environments where the server runs behind a proxy, when
	// the real end user IP is passed via some trusted header or other form of
	// metadata.
	//
	// If "", IP allowlist check will be skipped and the request will be assumed
	// to come from "0.0.0.0" aka "unspecified IPv4".
	RemoteAddr() string

	// Host returns the hostname the request was sent to or "" if unknown.
	//
	// Also known as HTTP2 `:authority` pseudo-header.
	Host() string
}

// Method implements a particular low-level authentication mechanism.
//
// It may also optionally implement a bunch of other interfaces:
//
//	UsersAPI: if the method supports login and logout URLs.
//	Warmable: if the method supports warm up.
//	HasHandlers: if the method needs to install HTTP handlers.
//
// Methods are not usually used directly, but passed to Authenticator{...} that
// knows how to apply them.
type Method interface {
	// Authenticate extracts user information from the incoming request.
	//
	// It returns:
	//   * (*User, Session or nil, nil) on success.
	//   * (nil, nil, nil) if the method is not applicable.
	//   * (nil, nil, error) if the method is applicable, but credentials are bad.
	//
	// The returned error may be tagged with an grpcutil error tag. Its code will
	// be used to derive the response status code. Internal error messages (e.g.
	// ones tagged with grpcutil.InternalTag or similar) are logged, but not sent
	// to clients. All other errors are sent to clients as is.
	Authenticate(context.Context, RequestMetadata) (*User, Session, error)
}

// UsersAPI may be additionally implemented by Method if it supports login and
// logout URLs.
type UsersAPI interface {
	// LoginURL returns a URL that, when visited, prompts the user to sign in,
	// then redirects the user to the URL specified by dest.
	LoginURL(ctx context.Context, dest string) (string, error)

	// LogoutURL returns a URL that, when visited, signs the user out,
	// then redirects the user to the URL specified by dest.
	LogoutURL(ctx context.Context, dest string) (string, error)
}

// Warmable may be additionally implemented by Method if it supports warm up.
type Warmable interface {
	// Warmup may be called to precache the data needed by the method.
	//
	// There's no guarantee when it will be called or if it will be called at all.
	// Should always do best-effort initialization. Errors are logged and ignored.
	Warmup(ctx context.Context) error
}

// HasHandlers may be additionally implemented by Method if it needs to
// install HTTP handlers.
type HasHandlers interface {
	// InstallHandlers installs necessary HTTP handlers into the router.
	InstallHandlers(r *router.Router, base router.MiddlewareChain)
}

// HasStateEndpoint may be additionally implemented by Method if it exposes
// an HTTP endpoints that returns the authentication state, OAuth and ID tokens
// for frontend applications.
type HasStateEndpoint interface {
	// StateEndpointURL returns an URL that serves StateEndpointResponse JSON.
	//
	// See StateEndpointResponse for the format and meaning of the response.
	//
	// Returns ErrNoStateEndpoint if the endpoint is not actually exposed. This
	// can happen if the method generally supports the state endpoint, but it is
	// turned off in the method's configuration.
	StateEndpointURL(ctx context.Context) (string, error)
}

// UserCredentialsGetter may be additionally implemented by Method if it knows
// how to extract end-user credentials from the incoming request. Currently
// understands only OAuth2 tokens.
type UserCredentialsGetter interface {
	// GetUserCredentials extracts an OAuth access token from the incoming request
	// or returns an error if it isn't possible.
	//
	// May omit token's expiration time if it isn't known.
	//
	// Guaranteed to be called only after the successful authentication, so it
	// doesn't have to recheck the validity of the token.
	GetUserCredentials(context.Context, RequestMetadata) (*oauth2.Token, error)
}

// Session holds some extra information pertaining to the request.
//
// It is stored in the context as part of State. Used by AsSessionUser RPC
// authority kind.
type Session interface {
	// AccessToken returns an OAuth access token identifying the session user.
	AccessToken(ctx context.Context) (*oauth2.Token, error)
	// IDToken returns an ID token identifying the session user.
	IDToken(ctx context.Context) (*oauth2.Token, error)
}

// User represents identity and profile of a user.
type User struct {
	// Identity is identity string of the user (may be AnonymousIdentity).
	// If User is returned by Authenticate(...), Identity string is always present
	// and valid.
	Identity identity.Identity `json:"identity,omitempty"`

	// Superuser is true if the user is site-level administrator. For example, on
	// GAE this bit is set for GAE-level administrators. Optional, default false.
	Superuser bool `json:"superuser,omitempty"`

	// Email is email of the user. Optional, default "". Don't use it as a key
	// in various structures. Prefer to use Identity() instead (it is always
	// available).
	Email string `json:"email,omitempty"`

	// Name is full name of the user. Optional, default "".
	Name string `json:"name,omitempty"`

	// Picture is URL of the user avatar. Optional, default "".
	Picture string `json:"picture,omitempty"`

	// ClientID is the ID of the pre-registered OAuth2 client so its identity can
	// be verified. Used only by authentication methods based on OAuth2.
	// See https://developers.google.com/console/help/#generatingoauth2 for more.
	ClientID string `json:"client_id,omitempty"`

	// Extra is any additional information the authentication method produces.
	//
	// Its exact type depends on the authentication method used. Usually the
	// authentication method will have an accompanying getter function that knows
	// how to interpret this field.
	Extra any `json:"-"`
}

// StateEndpointResponse defines a JSON structure of a state endpoint response.
//
// It represents the state of the authentication session based on the session
// cookie (or other ambient credential) in the request metadata.
//
// It is intended to be called via a same origin URL fetch request by the
// frontend code that needs an OAuth access token or an ID token representing
// the signed in user.
//
// If there's a valid authentication credential, the state endpoint replies with
// HTTP 200 status code and the JSON-serialized StateEndpointResponse struct
// with state details. The handler refreshes access and ID tokens if they expire
// soon.
//
// If there is no authentication credential or it has expired or was revoked,
// the state endpoint still replies with HTTP 200 code and the JSON-serialized
// StateEndpointResponse struct, except its `identity` field is
// `anonymous:anonymous` and no other fields are populated.
//
// On errors the state endpoint replies with a non-200 HTTP status code with a
// `plain/text` body containing the error message. This is an exceptional
// situation (usually internal transient errors caused by the session store
// unavailability or some misconfiguration in code). Replies with HTTP code
// equal or larger than 500 indicate transient errors and can be retried.
//
// The state endpoint is exposed only by auth methods that implement
// HasStateEndpoint interface (e.g. `encryptedcookies`), and only if they are
// configured to expose it.
type StateEndpointResponse struct {
	// Identity is a LUCI identity string of the user or `anonymous:anonymous` if
	// the user is not logged in.
	Identity string `json:"identity"`

	// Email is the email of the user account if the user is logged in.
	Email string `json:"email,omitempty"`

	// Picture is the https URL of the user profile picture if available.
	Picture string `json:"picture,omitempty"`

	// AccessToken is an OAuth access token of the logged in user.
	//
	// See RequiredScopes and OptionalScopes in AuthMethod for what scopes this
	// token can have.
	AccessToken string `json:"accessToken,omitempty"`

	// AccessTokenExpiry is an absolute expiration time (as a unix timestamp) of
	// the access token.
	//
	// It is at least 10 min in the future.
	AccessTokenExpiry int64 `json:"accessTokenExpiry,omitempty"`

	// AccessTokenExpiresIn is approximately how long the access token will be
	// valid since when the response was generated, in seconds.
	//
	// It is at least 600 sec.
	AccessTokenExpiresIn int32 `json:"accessTokenExpiresIn,omitempty"`

	// IDToken is an identity token of the logged in user.
	//
	// Its `aud` claim is equal to ClientID in OpenIDConfig passed to AuthMethod.
	IDToken string `json:"idToken,omitempty"`

	// IDTokenExpiry is an absolute expiration time (as a unix timestamp) of
	// the identity token.
	//
	// It is at least 10 min in the future.
	IDTokenExpiry int64 `json:"idTokenExpiry,omitempty"`

	// IDTokenExpiresIn is approximately how long the identity token will be
	// valid since when the response was generated, in seconds.
	//
	// It is at least 600 sec.
	IDTokenExpiresIn int32 `json:"idTokenExpiresIn,omitempty"`
}

// Authenticator performs authentication of incoming requests.
//
// It is a stateless object configured with a list of methods to try when
// authenticating incoming requests. It implements Authenticate method that
// performs high-level authentication logic using the provided list of low-level
// auth methods.
//
// Note that most likely you don't need to instantiate this object directly.
// Use Authenticate middleware instead. Authenticator is exposed publicly only
// to be used in advanced cases, when you need to fine-tune authentication
// behavior.
type Authenticator struct {
	Methods []Method // a list of authentication methods to try
}

// GetMiddleware returns a middleware that uses this Authenticator for
// authentication.
//
// It uses a.Authenticate internally and handles errors appropriately.
//
// TODO(vadimsh): Refactor to be a function instead of a method and move to
// http.go.
func (a *Authenticator) GetMiddleware() router.Middleware {
	return func(c *router.Context, next router.Handler) {
		ctx, err := a.AuthenticateHTTP(c.Request.Context(), c.Request)
		if err != nil {
			code, ok := grpcutil.Tag.Value(err)
			if !ok {
				if transient.Tag.In(err) {
					code = codes.Internal
				} else {
					code = codes.Unauthenticated
				}
			}
			replyError(c.Request.Context(), c.Writer, grpcutil.CodeStatus(code), err)
		} else {
			c.Request = c.Request.WithContext(ctx)
			next(c)
		}
	}
}

// AuthenticateHTTP authenticates an HTTP request.
//
// See Authenticate for all details.
//
// This method is likely temporary until pRPC server switches to use gRPC
// interceptors for authentication.
func (a *Authenticator) AuthenticateHTTP(ctx context.Context, r *http.Request) (context.Context, error) {
	return a.Authenticate(ctx, RequestMetadataForHTTP(r))
}

// Authenticate authenticates the request and adds State into the context.
//
// Returns an error if credentials are provided, but invalid. If no credentials
// are provided (i.e. the request is anonymous), finishes successfully, but in
// that case CurrentIdentity() returns AnonymousIdentity.
//
// The returned error may be tagged with an grpcutil error tag. Its code should
// be used to derive the response status code. Internal error messages (e.g.
// ones tagged with grpcutil.InternalTag or similar) should be logged, but not
// sent to clients. All other errors should be sent to clients as is.
func (a *Authenticator) Authenticate(ctx context.Context, r RequestMetadata) (_ context.Context, err error) {
	tracedCtx, span := tracing.Start(ctx, "go.chromium.org/luci/server/auth.Authenticate")
	report := durationReporter(tracedCtx, authenticateDuration)

	// This variable is changed throughout the function's execution. It it used
	// in the defer to figure out at what stage the call failed.
	stage := ""

	// This defer reports the outcome of the authentication to the monitoring.
	defer func() {
		switch {
		case err == nil:
			report(nil, "SUCCESS")
		case err == ErrNotConfigured:
			report(err, "ERROR_NOT_CONFIGURED")
		case err == ErrBadClientID:
			report(err, "ERROR_FORBIDDEN_OAUTH_CLIENT")
		case err == ErrBadAudience:
			report(err, "ERROR_FORBIDDEN_AUDIENCE")
		case err == ErrBadRemoteAddr:
			report(err, "ERROR_BAD_REMOTE_ADDR")
		case err == ErrForbiddenIP:
			report(err, "ERROR_FORBIDDEN_IP")
		case err == ErrProjectHeaderForbidden:
			report(err, "ERROR_PROJECT_HEADER_FORBIDDEN")
		case transient.Tag.In(err):
			report(err, "ERROR_TRANSIENT_IN_"+stage)
		default:
			report(err, "ERROR_IN_"+stage)
		}
		tracing.End(span, err)
	}()

	// We will need working DB factory below to check IP allowlist.
	cfg := getConfig(tracedCtx)
	if cfg == nil || cfg.DBProvider == nil || len(a.Methods) == 0 {
		return nil, ErrNotConfigured
	}

	// The future state that will be placed into the context.
	s := state{authenticator: a, endUserErr: ErrNoForwardableCreds}

	// Pick the first authentication method that applies.
	stage = "AUTH"
	for _, m := range a.Methods {
		var err error
		if s.user, s.session, err = m.Authenticate(tracedCtx, r); err != nil {
			return nil, err
		}
		if s.user != nil {
			if err = s.user.Identity.Validate(); err != nil {
				stage = "ID_REGEXP_CHECK"
				return nil, err
			}
			s.method = m
			break
		}
	}

	// If no authentication method is applicable, default to anonymous identity.
	if s.method == nil {
		s.user = &User{Identity: identity.AnonymousIdentity}
		s.session = nil
	}

	// peerIdent always matches the identity of a remote peer. It may end up being
	// different from s.user.Identity if the delegation tokens or project
	// identities are used (see below). They affect s.user.Identity but don't
	// touch s.peerIdent.
	s.peerIdent = s.user.Identity

	// Grab a snapshot of auth DB to use it consistently for the duration of this
	// request.
	stage = "AUTHDB_FETCH"
	s.db, err = cfg.DBProvider(tracedCtx)
	if err != nil {
		return nil, err
	}

	// If using OAuth2, make sure the ClientID is allowlisted.
	if s.user.ClientID != "" {
		stage = "OAUTH_CLIENT_ID_CHECK"
		if err := checkClientID(tracedCtx, cfg, s.db, s.user.Email, s.user.ClientID); err != nil {
			return nil, err
		}
	}

	// Extract peer's IP address and, if necessary, check it against an allowlist.
	stage = "IP_CHECK"
	if s.peerIP, err = checkEndUserIP(tracedCtx, cfg, s.db, r, s.peerIdent); err != nil {
		return nil, err
	}

	// Check X-Delegation-Token-V1 and X-Luci-Project headers. They are used in
	// LUCI-specific protocols to allow LUCI micro-services to act on behalf of
	// end-users or projects.
	var delegationToken string
	var projectHeader string
	if delegationToken = r.Header(delegation.HTTPHeaderName); delegationToken != "" {
		stage = "DELEGATION_TOKEN_CHECK"
		if s.user, err = checkDelegationToken(tracedCtx, cfg, s.db, delegationToken, s.peerIdent); err != nil {
			return nil, err
		}
	} else if projectHeader = r.Header(XLUCIProjectHeader); projectHeader != "" {
		stage = "PROJECT_HEADER_CHECK"
		if s.user, err = checkProjectHeader(tracedCtx, s.db, projectHeader, s.peerIdent); err != nil {
			return nil, err
		}
	}

	// If the main authentication mechanism is based on forwardable OAuth tokens,
	// grab all forwardable headers for GetRPCTransport(AsCredentialsForwarder).
	if credsGetter, _ := s.method.(UserCredentialsGetter); credsGetter != nil {
		s.endUserTok, s.endUserErr = credsGetter.GetUserCredentials(tracedCtx, r)
		if s.endUserErr == nil && (delegationToken != "" || projectHeader != "") {
			s.endUserExtraHeaders = make(map[string]string, 2)
			if delegationToken != "" {
				s.endUserExtraHeaders[delegation.HTTPHeaderName] = delegationToken
			}
			if projectHeader != "" {
				s.endUserExtraHeaders[XLUCIProjectHeader] = projectHeader
			}
		}
	}

	// Inject the auth state into the original context (not the traced one).
	return WithState(ctx, &s), nil
}

// usersAPI returns implementation of UsersAPI by examining Methods.
//
// Returns nil if none of Methods implement UsersAPI.
func (a *Authenticator) usersAPI() UsersAPI {
	for _, m := range a.Methods {
		if api, ok := m.(UsersAPI); ok {
			return api
		}
	}
	return nil
}

// LoginURL returns a URL that, when visited, prompts the user to sign in,
// then redirects the user to the URL specified by dest.
//
// Returns ErrNoUsersAPI if none of the authentication methods support login
// URLs.
func (a *Authenticator) LoginURL(ctx context.Context, dest string) (string, error) {
	if api := a.usersAPI(); api != nil {
		return api.LoginURL(ctx, dest)
	}
	return "", ErrNoUsersAPI
}

// LogoutURL returns a URL that, when visited, signs the user out, then
// redirects the user to the URL specified by dest.
//
// Returns ErrNoUsersAPI if none of the authentication methods support login
// URLs.
func (a *Authenticator) LogoutURL(ctx context.Context, dest string) (string, error) {
	if api := a.usersAPI(); api != nil {
		return api.LogoutURL(ctx, dest)
	}
	return "", ErrNoUsersAPI
}

////

// replyError logs the error and writes a response to ResponseWriter.
//
// For codes < 500, the error is logged at Warning level and written to the
// response as is. For codes >= 500 the error is logged at Error level and
// the generic error message is written instead.
func replyError(ctx context.Context, rw http.ResponseWriter, code int, err error) {
	if code < 500 {
		logging.Warningf(ctx, "HTTP %d: %s", code, err)
		http.Error(rw, err.Error(), code)
	} else {
		logging.Errorf(ctx, "HTTP %d: %s", code, err)
		http.Error(rw, http.StatusText(code), code)
	}
}

// checkClientID returns nil if the clientID is allowed, ErrBadClientID if not,
// and a transient errors if the check itself failed.
func checkClientID(ctx context.Context, cfg *Config, db authdb.DB, email, clientID string) error {
	// Check the global allowlist in the AuthDB.
	switch valid, err := db.IsAllowedOAuthClientID(ctx, email, clientID); {
	case err != nil:
		return errors.Annotate(err, "failed to check client ID allowlist").Tag(transient.Tag).Err()
	case valid:
		return nil
	}

	// It may be an app-specific client ID supplied via cfg.FrontendClientID.
	if cfg.FrontendClientID != nil {
		switch frontendClientID, err := cfg.FrontendClientID(ctx); {
		case err != nil:
			return errors.Annotate(err, "failed to grab frontend client ID").Tag(transient.Tag).Err()
		case clientID == frontendClientID:
			return nil
		}
	}

	logging.Errorf(ctx, "auth: %q is using client_id %q not in the allowlist", email, clientID)
	return ErrBadClientID
}

// checkEndUserIP parses the caller IP address and checks it against an
// allowlist (if necessary). Returns ErrBadRemoteAddr if the IP is malformed,
// ErrForbiddenIP if the IP is not allowlisted or a transient error if the check
// itself failed.
func checkEndUserIP(ctx context.Context, cfg *Config, db authdb.DB, r RequestMetadata, peerID identity.Identity) (net.IP, error) {
	var ipAddr string
	if cfg.EndUserIP != nil {
		ipAddr = cfg.EndUserIP(r)
	} else {
		ipAddr = r.RemoteAddr()
	}
	peerIP, err := parseRemoteIP(ipAddr)
	if err != nil {
		logging.Errorf(ctx, "auth: bad remote_addr %q in a call from %q - %s", ipAddr, peerID, err)
		return nil, ErrBadRemoteAddr
	}
	if peerIP.IsUnspecified() {
		return peerIP, nil
	}

	// Some callers may be constrained by an IP allowlist.
	switch ipAllowlist, err := db.GetAllowlistForIdentity(ctx, peerID); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to get IP allowlist for identity %q", peerID).Tag(transient.Tag).Err()
	case ipAllowlist != "":
		switch allowed, err := db.IsAllowedIP(ctx, peerIP, ipAllowlist); {
		case err != nil:
			return nil, errors.Annotate(err, "failed to check IP %s is in the allowlist %q", peerIP, ipAllowlist).Tag(transient.Tag).Err()
		case !allowed:
			return nil, ErrForbiddenIP
		}
	}

	return peerIP, nil
}

// checkDelegationToken checks correctness of a delegation token and returns
// a delegated *User.
func checkDelegationToken(ctx context.Context, cfg *Config, db authdb.DB, token string, peerID identity.Identity) (*User, error) {
	// Log the token fingerprint (even before parsing the token), it can be used
	// to grab the info about the token from the token server logs.
	logging.Fields{
		"fingerprint": tokenFingerprint(token),
	}.Debugf(ctx, "auth: Received delegation token")

	// Need to grab our own identity to verify that the delegation token is
	// minted for consumption by us and not some other service.
	ownServiceIdentity, err := getOwnServiceIdentity(ctx, cfg.Signer)
	if err != nil {
		return nil, err
	}
	delegatedIdentity, err := delegation.CheckToken(ctx, delegation.CheckTokenParams{
		Token:                token,
		PeerID:               peerID,
		CertificatesProvider: db,
		GroupsChecker:        db,
		OwnServiceIdentity:   ownServiceIdentity,
	})
	if err != nil {
		return nil, err
	}

	// Log that peerID is pretending to be delegatedIdentity.
	logging.Fields{
		"peerID":      peerID,
		"delegatedID": delegatedIdentity,
	}.Debugf(ctx, "auth: Using delegation")

	return &User{Identity: delegatedIdentity}, nil
}

// checkProjectHeader verifies the caller is allowed to use X-Luci-Project
// mechanism and returns a *User (with project-scoped identity) to use for
// the request.
func checkProjectHeader(ctx context.Context, db authdb.DB, project string, peerID identity.Identity) (*User, error) {
	// See comment for InternalServicesGroup.
	switch yes, err := db.IsMember(ctx, peerID, []string{InternalServicesGroup}); {
	case err != nil:
		return nil, errors.Annotate(err, "error when checking if %q is in %q", peerID, InternalServicesGroup).Tag(transient.Tag).Err()
	case !yes:
		return nil, ErrProjectHeaderForbidden
	}

	// Verify the actual value passes the regexp check.
	projIdent, err := identity.MakeIdentity("project:" + project)
	if err != nil {
		return nil, errors.Annotate(err, "bad %s", XLUCIProjectHeader).Err()
	}

	// Log that peerID is using project-scoped identity.
	logging.Fields{
		"peerID":    peerID,
		"projectID": projIdent,
	}.Debugf(ctx, "auth: Using project identity")

	return &User{Identity: projIdent}, nil
}

// getOwnServiceIdentity returns 'service:<appID>' identity of the current
// service.
func getOwnServiceIdentity(ctx context.Context, signer signing.Signer) (identity.Identity, error) {
	if signer == nil {
		return "", ErrNotConfigured
	}
	switch serviceInfo, err := signer.ServiceInfo(ctx); {
	case err != nil:
		return "", err
	case serviceInfo.AppID == "":
		return "", errors.Reason("auth: don't known our own app ID to check the delegation token is for us").Err()
	default:
		return identity.MakeIdentity("service:" + serviceInfo.AppID)
	}
}
