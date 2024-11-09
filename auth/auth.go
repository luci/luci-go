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

// Package auth implements a wrapper around golang.org/x/oauth2.
//
// Its main improvement is the on-disk cache for authentication tokens, which is
// especially important for 3-legged interactive OAuth flows: its usage
// eliminates annoying login prompts each time a program is used (because the
// refresh token can now be reused). The cache also allows to reduce unnecessary
// token refresh calls when sharing a service account between processes.
//
// The package also implements some best practices regarding interactive login
// flows in CLI programs. It makes it easy to implement a login process as
// a separate interactive step that happens before the main program loop.
//
// The antipattern it tries to prevent is "launch an interactive login flow
// whenever program hits 'Not Authorized' response from the server". This
// usually results in a very confusing behavior, when login prompts pop up
// unexpectedly at random time, random places and from multiple goroutines at
// once, unexpectedly consuming unintended stdin input.
package auth

import (
	"context"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"

	"go.chromium.org/luci/auth/internal"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/lucictx"
)

var (
	// ErrLoginRequired is returned by Transport or GetAccessToken in case long
	// term credentials are not cached and the user must go through an interactive
	// login flow.
	ErrLoginRequired = errors.New("interactive login is required")

	// ErrInsufficientAccess is returned by Login or Transport if an access token
	// can't be minted for given OAuth scopes. For example if a GCE instance
	// wasn't granted access to requested scopes when it was created.
	ErrInsufficientAccess = internal.ErrInsufficientAccess

	// ErrNoEmail is returned by GetEmail() if the cached credentials are not
	// associated with some particular email. This may happen, for example, when
	// using a refresh token that doesn't have 'userinfo.email' scope.
	ErrNoEmail = errors.New("the token is not associated with an email")

	// ErrBadOptions is returned by Login() or Transport() if Options passed
	// to authenticator indicate incompatible features. This likely indicates
	// a programming error.
	ErrBadOptions = errors.New("bad authenticator options")

	// ErrAudienceRequired is returned when UseIDTokens is set without specifying
	// the target audience for ID tokens.
	ErrAudienceRequired = errors.New("using ID tokens requires specifying an audience string")

	// ErrNoIDToken is returned by GetAccessToken when UseIDTokens option is true,
	// but the authentication method doesn't actually support ID tokens either
	// inherently by its nature (e.g. not implemented) or due to its configuration
	// (e.g. no necessary scopes).
	ErrNoIDToken = errors.New("ID tokens are not supported in this configuration")

	// ErrNoAccessToken is returned by GetAccessToken when UseIDTokens option is
	// false (i.e. the caller wants access tokens), but the authentication method
	// doesn't actually support access tokens.
	ErrNoAccessToken = errors.New("access tokens are not supported in this configuration")
)

// Some known Google API OAuth scopes.
const (
	OAuthScopeEmail = "https://www.googleapis.com/auth/userinfo.email"
	OAuthScopeIAM   = "https://www.googleapis.com/auth/iam"
)

const (
	// GCEServiceAccount is special value that can be passed instead of path to
	// a service account credentials file to indicate that GCE VM credentials
	// should be used instead of a real credentials file.
	GCEServiceAccount = ":gce"
)

// Method defines a method to use to obtain authentication token.
type Method string

// Supported authentication methods.
const (
	// AutoSelectMethod can be used to allow the library to pick a method most
	// appropriate for given set of options and the current execution environment.
	//
	// For example, passing ServiceAccountJSONPath or ServiceAccountJSON makes
	// Authenticator to pick ServiceAccountMethod.
	//
	// See SelectBestMethod function for details.
	AutoSelectMethod Method = ""

	// UserCredentialsMethod is used for interactive OAuth 3-legged login flow.
	//
	// Using this method requires specifying an OAuth client by passing ClientID
	// and ClientSecret in Options when calling NewAuthenticator.
	//
	// Additionally, SilentLogin and OptionalLogin (i.e. non-interactive) login
	// modes rely on a presence of a refresh token in the token cache, thus using
	// these modes with UserCredentialsMethod also requires configured token
	// cache (see SecretsDir field of Options).
	UserCredentialsMethod Method = "UserCredentialsMethod"

	// ServiceAccountMethod is used to authenticate as a service account using
	// a private key.
	//
	// Callers of NewAuthenticator must pass either a path to a JSON file with
	// service account key (as produced by Google Cloud Console) or a body of this
	// JSON file. See ServiceAccountJSONPath and ServiceAccountJSON fields in
	// Options.
	//
	// Using ServiceAccountJSONPath has an advantage: Authenticator always loads
	// the private key from the file before refreshing the token, it allows to
	// replace the key while the process is running.
	ServiceAccountMethod Method = "ServiceAccountMethod"

	// GCEMetadataMethod is used on Compute Engine to use tokens provided by
	// Metadata server. See https://cloud.google.com/compute/docs/authentication
	GCEMetadataMethod Method = "GCEMetadataMethod"

	// LUCIContextMethod is used by LUCI-aware applications to fetch tokens though
	// a local auth server (discoverable via "local_auth" key in LUCI_CONTEXT).
	//
	// This method is similar in spirit to GCEMetadataMethod: it uses some local
	// HTTP server as a provider of OAuth access tokens, which gives an ambient
	// authentication context to apps that use it.
	//
	// There are some big differences:
	//  1. LUCIContextMethod supports minting tokens for multiple different set
	//     of scopes, unlike GCE metadata server that always gives a token with
	//     preconfigured scopes (set when the GCE instance was created).
	//  2. LUCIContextMethod is not GCE-specific. It doesn't use magic link-local
	//     IP address. It can run on any machine.
	//  3. The access to the local auth server is controlled by file system
	//     permissions of LUCI_CONTEXT file (there's a secret in this file).
	//  4. There can be many local auth servers running at once (on different
	//     ports). Useful for bringing up sub-contexts, in particular in
	//     combination with ActAsServiceAccount ("sudo" mode) or for tests.
	//
	// See auth/integration/localauth package for the implementation of the server
	// side of the protocol.
	LUCIContextMethod Method = "LUCIContextMethod"
)

// LoginMode is used as enum in NewAuthenticator function.
type LoginMode string

const (
	// InteractiveLogin indicates to Authenticator that it is okay to run an
	// interactive login flow (via Login()) in Transport(), Client() or other
	// factories if there's no cached token.
	//
	// This is typically used with UserCredentialsMethod to generate an OAuth
	// refresh token and put it in the token cache at the start of the program,
	// when grabbing a transport.
	//
	// Has no effect when used with service account credentials.
	InteractiveLogin LoginMode = "InteractiveLogin"

	// SilentLogin indicates to Authenticator that it must return a transport that
	// implements authentication, but it is NOT OK to run interactive login flow
	// to make it.
	//
	// Transport() and other factories will fail with ErrLoginRequired error if
	// there's no cached token or one can't be generated on the fly in
	// non-interactive mode. This may happen when using UserCredentialsMethod.
	//
	// It is always OK to use SilentLogin mode with service accounts credentials
	// (ServiceAccountMethod mode), since no user interaction is necessary to
	// generate an access token in this case.
	SilentLogin LoginMode = "SilentLogin"

	// OptionalLogin indicates to Authenticator that it should return a transport
	// that implements authentication, but it is OK to return non-authenticating
	// transport if there are no valid cached credentials.
	//
	// An interactive login flow will never be invoked. An unauthenticated client
	// will be returned if no credentials are present.
	//
	// Can be used when making calls to backends that allow anonymous access. This
	// is especially useful with UserCredentialsMethod: a user may start using
	// the service right away (in anonymous mode), and later login (using Login()
	// method or any other way of initializing credentials cache) to get more
	// permissions.
	//
	// When used with ServiceAccountMethod it is identical to SilentLogin, since
	// it makes no sense to ignore invalid service account credentials when the
	// caller is explicitly asking the authenticator to use them.
	//
	// Has the original meaning when used with GCEMetadataMethod: it instructs to
	// skip authentication if the token returned by GCE metadata service doesn't
	// have all requested scopes.
	OptionalLogin LoginMode = "OptionalLogin"
)

// Options are used by NewAuthenticator call.
type Options struct {
	// Transport is underlying round tripper to use for requests.
	//
	// Default: http.DefaultTransport.
	Transport http.RoundTripper

	// Method defines how to grab authentication tokens.
	//
	// Default: AutoSelectMethod.
	Method Method

	// UseIDTokens indicates to use ID tokens instead of access tokens.
	//
	// All methods that use or return OAuth access tokens would use ID tokens
	// instead. This is useful, for example, when calling APIs that are hosted on
	// Cloud Run or served via Cloud Endpoints.
	//
	// When setting to true, make sure to specify a correct Audience if the
	// default one is not appropriate.
	//
	// When using UserCredentialsMethod implicitly appends OAuthScopeEmail to the
	// list of OAuth scopes, since this scope is needed to get ID tokens in this
	// mode.
	//
	// Default: false.
	UseIDTokens bool

	// Scopes is a list of OAuth scopes to request.
	//
	// Ignored when using ID tokens.
	//
	// Default: [OAuthScopeEmail].
	Scopes []string

	// Audience is the audience to put into ID tokens.
	//
	// It will become `aud` claim in the token. Should usually be some "https://"
	// URL. Services that validate ID tokens check this field.
	//
	// Ignored when not using ID tokens or when using UserCredentialsMethod (the
	// audience always matches OAuth2 ClientID in this case).
	//
	// Defaults: the value of ClientID to mimic UserCredentialsMethod.
	Audience string

	// ActAsServiceAccount is used to act as a specified service account email.
	//
	// When this option is set, there are two identities involved:
	//  1. A service account identity specified by `ActAsServiceAccount`.
	//  2. An identity conveyed by the authenticator options (via cached refresh
	//     token, or via `ServiceAccountJSON`, or other similar ways), i.e. the
	//     identity asserted by the authenticator in case `ActAsServiceAccount` is
	//     not set. It is referred to below as the Actor identity.
	//
	// The resulting authenticator will produce access tokens for service account
	// `ActAsServiceAccount`, using the Actor identity to generate them via some
	// "acting" API.
	//
	// If `ActViaLUCIRealm` is not set, the "acting" API is Google Cloud IAM.
	// The Actor credentials will internally be used to generate access tokens
	// with IAM scope (see `OAuthScopeIAM`). These tokens will then be used to
	// call `generateAccessToken` Cloud IAM RPC to obtain the final tokens that
	// belong to the service account `ActAsServiceAccount`. This requires the
	// Actor to have "iam.serviceAccounts.getAccessToken" Cloud IAM permission,
	// which is usually granted via "Service Account Token Creator" IAM role.
	//
	// If `ActViaLUCIRealm` is set, the "acting" API is the LUCI Token Server.
	// The Actor credentials will internally be used to generate access tokens
	// with just email scope (see `OAuthScopeEmail`). These tokens will then be
	// used to call `MintServiceAccountToken` RPC. This requires the following
	// LUCI permissions in the realm specified by `ActViaLUCIRealm`:
	//  1. The Actor needs "luci.serviceAccounts.mintToken" permission.
	//  2. The target service account needs "luci.serviceAccounts.existInRealm"
	//     permission.
	//  3. The LUCI project the realm belongs to must be authorized to use the
	//     target service account (currently via project_owned_accounts.cfg global
	//     config file).
	//
	// Regardless of what "acting" API is used, `Scopes` parameter specifies what
	// OAuth scopes to request for the final access token belonging to
	// `ActAsServiceAccount`.
	//
	// Default: none.
	ActAsServiceAccount string

	// ActViaLUCIRealm is a LUCI Realm to use to authorize access to the service
	// account when "acting" as it through a LUCI Token Server.
	//
	// See `ActAsServiceAccount` for a detailed explanation.
	//
	// Should have form "<project>:<realm>" (e.g. "chromium:ci"). It instructs
	// the Token Server to lookup acting permissions in a realm named "<realm>",
	// defined in `realms.cfg` project config file in a LUCI project named
	// "<project>".
	//
	// Using this option requires `TokenServerHost` to be set.
	//
	// Default: none.
	ActViaLUCIRealm string

	// TokenServerHost is a hostname of a LUCI Token Server to use when acting.
	//
	// Used only when `ActAsServiceAccount` and `ActViaLUCIRealm` are set.
	//
	// Default: none.
	TokenServerHost string

	// ClientID is OAuth client ID to use with UserCredentialsMethod.
	//
	// See https://developers.google.com/identity/protocols/OAuth2InstalledApp
	// (in particular everything related to "Desktop apps").
	//
	// Together with Scopes forms a cache key in the token cache, which in
	// practical terms means there can be only one concurrently "logged in" user
	// per [ClientID, Scopes] combination. So if multiple binaries use exact same
	// ClientID and Scopes, they'll share credentials cache (a login in one app
	// makes the user logged in in the other app too).
	//
	// If you don't want to share login information between tools, use separate
	// ClientID or SecretsDir values.
	//
	// If not set, UserCredentialsMethod auth method will not work.
	//
	// Default: none.
	ClientID string

	// ClientSecret is OAuth client secret to use with UserCredentialsMethod.
	//
	// Default: none.
	ClientSecret string

	// LoginSessionsHost is a hostname of a service that implements LoginSessions
	// pRPC service to use for performing interactive OAuth login flow instead
	// of using OOB redirect URI.
	//
	// Matters only when using UserCredentialsMethod method. When used, the stdout
	// must be attached to a real terminal (i.e. not redirected to a file or
	// pipe). This is a precaution against using this login method on bots or from
	// scripts which is never correct and can be dangerous.
	//
	// Default: none
	LoginSessionsHost string

	// ServiceAccountJSONPath is a path to a JSON blob with a private key to use.
	//
	// Can also be set to GCEServiceAccount (':gce') to indicate that the GCE VM
	// service account should be used instead. Useful in CLI interfaces. This
	// works only if Method is set to AutoSelectMethod (which is the default for
	// most CLI apps). If GCEServiceAccount is used on a machine without GCE
	// metadata server, authenticator methods return an error.
	//
	// Used only with ServiceAccountMethod.
	ServiceAccountJSONPath string

	// ServiceAccountJSON is a body of JSON key file to use.
	//
	// Overrides ServiceAccountJSONPath if given.
	ServiceAccountJSON []byte

	// GCEAccountName is an account name to query to fetch token for from metadata
	// server when GCEMetadataMethod is used.
	//
	// If given account wasn't granted required set of scopes during instance
	// creation time, Transport() call fails with ErrInsufficientAccess.
	//
	// Default: "default" account.
	GCEAccountName string

	// GCEAllowAsDefault indicates whether it is OK to pick GCE authentication
	// method as default if no other methods apply.
	//
	// Effective only when running on GCE and Method is set to AutoSelectMethod.
	//
	// In theory using GCE metadata server for authentication when it is
	// available looks attractive. In practice, especially if running in a
	// heterogeneous fleet with a mix of GCE and non-GCE machines, automatically
	// enabling GCE-based authentication is very surprising when it happens.
	//
	// Default: false (don't "sniff" GCE environment).
	GCEAllowAsDefault bool

	// GCESupportsArbitraryScopes is true if the GCE metadata server is expected
	// to produce tokens with an arbitrary set of scopes (not only ones it reports
	// via "/scopes" endpoint) when asked via "/token?scopes=...".
	//
	// As of Apr 2024, this is true for at least GAE and Cloud Run metadata
	// servers. It is not true for GCE VM metadata server.
	//
	// When false, the default token produced by the metadata server will always
	// be used, regardless of requested set of scopes (they will be ignored). This
	// matches the standard GCE VM metadata experience.
	//
	// This field is primarily exposed to be used by go.chromium.org/luci/server,
	// which knows details of the environment the server runs in.
	//
	// Ignored when not using GCEMetadataMethod.
	//
	// Default is conservative false.
	GCESupportsArbitraryScopes bool

	// SecretsDir can be used to set the path to a directory where tokens
	// are cached.
	//
	// Do not try to extract or use the tokens stored in this
	// directory, as the format is not guaranteed.
	//
	// If not set, tokens will be cached only in the process memory. For refresh
	// tokens it means the user would have to go through the login process each
	// time process is started. For service account tokens it means there'll be
	// HTTP round trip to OAuth backend to generate access token each time the
	// process is started.
	SecretsDir string

	// DisableMonitoring can be used to disable the monitoring instrumentation.
	//
	// The transport produced by this authenticator sends tsmon metrics IFF:
	//  1. DisableMonitoring is false (default).
	//  2. The context passed to 'NewAuthenticator' has monitoring initialized.
	DisableMonitoring bool

	// MonitorAs is used for 'client' field of monitoring metrics.
	//
	// The default is 'luci-go'.
	MonitorAs string

	// MinTokenLifetime defines a minimally acceptable lifetime of access tokens
	// generated internally by authenticating http.RoundTripper, TokenSource and
	// PerRPCCredentials.
	//
	// Not used when GetAccessToken is called directly (it accepts this parameter
	// as an argument).
	//
	// The default is 2 min. There's rarely a need to change it and using smaller
	// values may be dangerous (e.g. if the request gets stuck somewhere or the
	// token is cached incorrectly it may expire before it is checked).
	MinTokenLifetime time.Duration

	// testingCache is used in unit tests.
	testingCache internal.TokenCache
	// testingBaseTokenProvider is used in unit tests.
	testingBaseTokenProvider internal.TokenProvider
	// testingIAMTokenProvider is used in unit tests.
	testingIAMTokenProvider internal.TokenProvider
}

// PopulateDefaults populates empty fields of `opts` with default values.
//
// It is called automatically by NewAuthenticator. Use it only if you need to
// normalize and examine auth.Options before passing them to NewAuthenticator.
func (opts *Options) PopulateDefaults() {
	// Set the default scope, sort and dedup scopes.
	if len(opts.Scopes) == 0 {
		opts.Scopes = []string{OAuthScopeEmail} // also implies "openid"
	}
	opts.Scopes = normalizeScopes(opts.Scopes)

	// Fill in blanks with default values.
	if opts.Audience == "" {
		opts.Audience = opts.ClientID
	}
	if opts.GCEAccountName == "" {
		opts.GCEAccountName = "default"
	}
	if opts.Transport == nil {
		opts.Transport = http.DefaultTransport
	}
	if opts.MinTokenLifetime == 0 {
		opts.MinTokenLifetime = 2 * time.Minute
	}

	// TODO(vadimsh): Check SecretsDir permissions. It should be 0700.
	if opts.SecretsDir != "" && !filepath.IsAbs(opts.SecretsDir) {
		var err error
		opts.SecretsDir, err = filepath.Abs(opts.SecretsDir)
		if err != nil {
			panic(errors.Annotate(err, "failed to get abs path to token cache dir").Err())
		}
	}
}

// SelectBestMethod returns a most appropriate authentication method for the
// given set of options and the current execution environment.
//
// Invoked by Authenticator if AutoSelectMethod is passed as Method in Options.
// It picks the first applicable method in this order:
//   - ServiceAccountMethod (if the service account private key is configured).
//   - LUCIContextMethod (if running inside LUCI_CONTEXT with an auth server).
//   - GCEMetadataMethod (if running on GCE and GCEAllowAsDefault is true).
//   - UserCredentialsMethod (if no other method applies).
//
// Beware: it may do relatively heavy calls on first usage (to detect GCE
// environment). Fast after that.
func SelectBestMethod(ctx context.Context, opts Options) Method {
	// Asked to use JSON private key.
	if opts.ServiceAccountJSONPath != "" || len(opts.ServiceAccountJSON) != 0 {
		if opts.ServiceAccountJSONPath == GCEServiceAccount {
			return GCEMetadataMethod
		}
		return ServiceAccountMethod
	}

	// Have a local auth server and an account we are allowed to pick by default.
	// If no default account is given, don't automatically pick up this method.
	if la := lucictx.GetLocalAuth(ctx); la != nil && la.DefaultAccountId != "" {
		return LUCIContextMethod
	}

	// Running on GCE and callers are fine with automagically picking up GCE
	// metadata server.
	if opts.GCEAllowAsDefault && metadata.OnGCE() {
		return GCEMetadataMethod
	}

	return UserCredentialsMethod
}

// Authenticator is a factory for http.RoundTripper objects that know how to use
// cached credentials and how to send monitoring metrics (if tsmon package was
// imported).
//
// Authenticator also knows how to run interactive login flow, if required.
type Authenticator struct {
	// Immutable members.
	loginMode LoginMode
	opts      *Options
	transport http.RoundTripper
	ctx       context.Context

	// Mutable members.
	lock sync.RWMutex
	err  error

	// baseToken is a token (and its provider and cache) whose possession is
	// sufficient to get the final access token used for authentication of user
	// calls (see 'authToken' below).
	//
	// Methods like 'CheckLoginRequired' check that the base token exists in the
	// cache or can be generated on the fly.
	//
	// In actor mode, the base token has scopes necessary for the corresponding
	// acting API to work (e.g. IAM scope when using Cloud's generateAccessToken).
	// The base token is also always using whatever auth method was specified by
	// Options.Method.
	//
	// In non-actor mode, baseToken coincides with authToken: both point to the
	// exact same struct.
	baseToken *tokenWithProvider

	// authToken is a token (and its provider and cache) that is actually used for
	// authentication of user calls.
	//
	// It is a token returned by 'GetAccessToken'. It is always scoped to 'Scopes'
	// list, as passed to NewAuthenticator via Options.
	//
	// In actor mode, it is derived from the base token by using some "acting" API
	// (which one depends on Options, see ActAsServiceAccount comment). This
	// process is non-interactive and thus can always be performed as long as we
	// have the base token.
	//
	// In non-actor mode it is the main token generated by the authenticator. In
	// this case it coincides with baseToken: both point to the exact same object.
	authToken *tokenWithProvider
}

// NewAuthenticator returns a new instance of Authenticator given its options.
//
// The authenticator is essentially a factory for http.RoundTripper that knows
// how to use and update cached credentials tokens. It is bound to the given
// context: uses its logger, clock and deadline.
func NewAuthenticator(ctx context.Context, loginMode LoginMode, opts Options) *Authenticator {
	opts.PopulateDefaults()

	// See ensureInitialized for the rest of the initialization.
	auth := &Authenticator{
		ctx:       ctx,
		loginMode: loginMode,
		opts:      &opts,
	}
	auth.transport = NewModifyingTransport(opts.Transport, auth.authTokenInjector)

	// Include the token refresh time into the monitored request time.
	if globalInstrumentTransport != nil && !opts.DisableMonitoring {
		monitorAs := opts.MonitorAs
		if monitorAs == "" {
			monitorAs = "luci-go"
		}
		instrumented := globalInstrumentTransport(ctx, auth.transport, monitorAs)
		if instrumented != auth.transport {
			logging.Debugf(ctx, "Enabling monitoring instrumentation (client == %q)", monitorAs)
			auth.transport = instrumented
		}
	}

	return auth
}

// Transport optionally performs a login and returns http.RoundTripper.
//
// It is a high level wrapper around CheckLoginRequired() and Login() calls. See
// documentation for LoginMode for more details.
func (a *Authenticator) Transport() (http.RoundTripper, error) {
	switch useAuth, err := a.doLoginIfRequired(false); {
	case err != nil:
		return nil, err
	case useAuth:
		return a.transport, nil // token-injecting transport
	default:
		return a.opts.Transport, nil // original non-authenticating transport
	}
}

// Client optionally performs a login and returns http.Client.
//
// It uses transport returned by Transport(). See documentation for LoginMode
// for more details.
func (a *Authenticator) Client() (*http.Client, error) {
	transport, err := a.Transport()
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport}, nil
}

// TokenSource optionally performs a login and returns oauth2.TokenSource.
//
// Can be used for interoperability with libraries that use golang.org/x/oauth2.
//
// It doesn't support 'OptionalLogin' mode, since oauth2.TokenSource must return
// some token. Otherwise its logic is similar to Transport(). In particular it
// may return ErrLoginRequired if interactive login is required, but the
// authenticator is in the silent mode. See LoginMode enum for more details.
func (a *Authenticator) TokenSource() (oauth2.TokenSource, error) {
	if _, err := a.doLoginIfRequired(true); err != nil {
		return nil, err
	}
	return tokenSource{a}, nil
}

// PerRPCCredentials optionally performs a login and returns PerRPCCredentials.
//
// It can be used to authenticate outbound gPRC RPC's.
//
// Has same logic as Transport(), in particular supports OptionalLogin mode.
// See Transport() for more details.
func (a *Authenticator) PerRPCCredentials() (credentials.PerRPCCredentials, error) {
	switch useAuth, err := a.doLoginIfRequired(false); {
	case err != nil:
		return nil, err
	case useAuth:
		return perRPCCreds{a}, nil // token-injecting PerRPCCredentials
	default:
		return perRPCCreds{}, nil // noop PerRPCCredentials
	}
}

// GetAccessToken returns a valid token with the specified minimum lifetime.
//
// Returns either an access token or an ID token based on UseIDTokens
// authenticator option.
//
// Does not interact with the user. May return ErrLoginRequired.
func (a *Authenticator) GetAccessToken(lifetime time.Duration) (*oauth2.Token, error) {
	tok, err := a.currentToken()
	if err != nil {
		return nil, err
	}

	// If interested in using ID tokens, but there's no ID token cached yet, force
	// a refresh. Note that auth methods that don't support ID tokens at all
	// return internal.NoIDToken in place of the ID token (and it is != ""). This
	// case is handled below.
	forceRefresh := tok != nil && a.opts.UseIDTokens && tok.IDToken == ""

	// Impose some arbitrary limit, since <= 0 lifetime won't work.
	if lifetime < time.Second {
		lifetime = time.Second
	}

	if tok == nil || forceRefresh || internal.TokenExpiresInRnd(a.ctx, tok, lifetime) {
		// Give 5 sec extra to make sure callers definitely receive a token that
		// has at least 'lifetime' seconds of life left. Without this margin, we
		// can get into an unlucky situation where the token is valid here, but
		// no longer valid (has fewer than 'lifetime' life left) up the stack, due
		// to the passage of time.
		var err error
		tok, err = a.refreshToken(tok, lifetime+5*time.Second)
		if err != nil {
			if err == ErrLoginRequired {
				return nil, err
			}
			return nil, errors.Annotate(err, "failed to refresh auth token").Err()
		}
		// Note: no randomization here. It is a sanity check that verifies
		// refreshToken did its job.
		if internal.TokenExpiresIn(a.ctx, tok, lifetime) {
			return nil, errors.Reason("failed to refresh auth token: still stale even after refresh").Err()
		}
	}

	if a.opts.UseIDTokens {
		if tok.IDToken == "" || tok.IDToken == internal.NoIDToken {
			return nil, ErrNoIDToken
		}
		return &oauth2.Token{
			AccessToken: tok.IDToken,
			Expiry:      tok.Expiry,
			TokenType:   "Bearer",
		}, nil
	}

	// This should not be happening, but in case it does, better to return an
	// error instead of a phony access token.
	if tok.Token.AccessToken == internal.NoAccessToken {
		return nil, ErrNoAccessToken
	}

	return &tok.Token, nil
}

// GetEmail returns an email associated with the credentials.
//
// In most cases this is a fast call that hits the cache. In some rare cases it
// may do an RPC to the Token Info endpoint to grab an email associated with the
// token.
//
// Returns ErrNoEmail if the email is not available. This may happen, for
// example, when using a refresh token that doesn't have 'userinfo.email' scope.
// Callers must expect this error to show up and should prepare a fallback.
//
// Returns an error if the email can't be fetched due to some other transient
// or fatal error. In particular, returns ErrLoginRequired if interactive login
// is required to get the token in the first place.
func (a *Authenticator) GetEmail() (string, error) {
	// Grab last known token and its associated email. Note that this call also
	// initializes the guts of the authenticator, including a.authToken.
	tok, err := a.currentToken()
	switch {
	case err != nil:
		return "", err
	case tok != nil && tok.Email == internal.NoEmail:
		return "", ErrNoEmail
	case tok != nil && tok.Email != internal.UnknownEmail:
		return tok.Email, nil
	}

	// There's no token cached yet (and thus email is not known). First try to ask
	// the provider for email only. Most providers can return it without doing any
	// RPCs or heavy calls. If this is not supported, resort to a heavier code
	// paths that actually refreshes the token and grabs its email along the way.
	a.lock.RLock()
	email := a.authToken.provider.Email()
	a.lock.RUnlock()
	switch {
	case email == internal.NoEmail:
		return "", ErrNoEmail
	case email != "":
		return email, nil
	}

	// The provider doesn't know the email. We need a forceful token refresh to
	// grab it (or discover it is NoEmail). This is relatively rare. It happens
	// only when using UserAuth TokenProvider and there's no cached token at all
	// or it is in old format that don't have email field.
	//
	// Pass -1 as lifetime to force trigger the refresh right now.
	tok, err = a.refreshToken(tok, -1)
	switch {
	case err == ErrLoginRequired:
		return "", err
	case err != nil:
		return "", errors.Annotate(err, "failed to refresh auth token").Err()
	case tok.Email == internal.NoEmail:
		return "", ErrNoEmail
	case tok.Email == internal.UnknownEmail: // this must not happen, but let's be cautious
		return "", errors.Reason("internal error when fetching the email, see logs").Err()
	default:
		return tok.Email, nil
	}
}

// CheckLoginRequired decides whether an interactive login is required.
//
// It examines the token cache and the configured authentication method to
// figure out whether we can attempt to grab an access token without involving
// the user interaction.
//
// Note: it does not check that the cached refresh token is still valid (i.e.
// not revoked). A revoked token will result in ErrLoginRequired error on a
// first attempt to use it.
//
// Returns:
//   - nil if we have a valid cached token or can mint one on the fly.
//   - ErrLoginRequired if we have no cached token and need to bother the user.
//   - ErrInsufficientAccess if the configured auth method can't mint the token
//     we require (e.g. when using GCE method and the instance doesn't have all
//     requested OAuth scopes).
//   - Generic error on other unexpected errors.
func (a *Authenticator) CheckLoginRequired() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if err := a.ensureInitialized(); err != nil {
		return err
	}

	// No cached base token and the token provider requires interaction with the
	// user: need to login. Only non-interactive token providers are allowed to
	// mint tokens on the fly, see refreshToken.
	if a.baseToken.token == nil && a.baseToken.provider.RequiresInteraction() {
		return ErrLoginRequired
	}

	return nil
}

// Login performs an interaction with the user to get a long term refresh token
// and cache it.
//
// Blocks for user input, can use stdin. It overwrites currently cached
// credentials, if any.
//
// When used with non-interactive token providers (e.g. based on service
// accounts), just clears the cached access token, so next the next
// authenticated call gets a fresh one.
func (a *Authenticator) Login() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.ensureInitialized()
	if err != nil {
		return err
	}

	// Remove any cached tokens to trigger full relogin.
	if err := a.purgeCredentialsCacheLocked(); err != nil {
		return err
	}

	if !a.baseToken.provider.RequiresInteraction() {
		return nil // can mint the token on the fly, no need for login
	}

	// Create an initial base token. This may require interaction with a user. Do
	// not do retries here, since Login is called when the user is looking, let
	// the user do the retries (since if MintToken() interacts with the user,
	// retrying it automatically will be extra confusing).
	a.baseToken.token, err = a.baseToken.provider.MintToken(a.ctx, nil)
	if err != nil {
		return err
	}

	// Store the initial token in the cache. Don't abort if it fails, the token
	// is still usable from the memory.
	if err := a.baseToken.putToCache(a.ctx); err != nil {
		logging.Warningf(a.ctx, "Failed to write token to cache: %s", err)
	}

	return nil
}

// PurgeCredentialsCache removes cached tokens.
//
// Does not revoke them!
func (a *Authenticator) PurgeCredentialsCache() error {
	a.lock.Lock()
	defer a.lock.Unlock()
	if err := a.ensureInitialized(); err != nil {
		return err
	}
	return a.purgeCredentialsCacheLocked()
}

func (a *Authenticator) purgeCredentialsCacheLocked() error {
	// No need to purge twice if baseToken == authToken, which is the case in
	// non-actor mode.
	var merr errors.MultiError
	if a.baseToken != a.authToken {
		merr = errors.NewMultiError(
			a.baseToken.purgeToken(a.ctx),
			a.authToken.purgeToken(a.ctx))
	} else {
		merr = errors.NewMultiError(a.baseToken.purgeToken(a.ctx))
	}

	switch total, first := merr.Summary(); {
	case total == 0:
		return nil
	case total == 1:
		return first
	default:
		return merr
	}
}

////////////////////////////////////////////////////////////////////////////////
// credentials.PerRPCCredentials implementation.

type perRPCCreds struct {
	a *Authenticator
}

func (creds perRPCCreds) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if len(uri) == 0 {
		panic("perRPCCreds: no URI given")
	}
	if creds.a == nil {
		return nil, nil
	}
	tok, err := creds.a.GetAccessToken(creds.a.opts.MinTokenLifetime)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization": tok.Type() + " " + tok.AccessToken,
	}, nil
}

func (creds perRPCCreds) RequireTransportSecurity() bool { return true }

////////////////////////////////////////////////////////////////////////////////
// oauth2.TokenSource implementation.

type tokenSource struct {
	a *Authenticator
}

// Token is part of oauth2.TokenSource interface.
func (s tokenSource) Token() (*oauth2.Token, error) {
	return s.a.GetAccessToken(s.a.opts.MinTokenLifetime)
}

////////////////////////////////////////////////////////////////////////////////
// Authenticator private methods.

// actingMode returns possible ways the authenticator can "act" as an account.
type actingMode int

const (
	actingModeNone actingMode = 0
	actingModeIAM  actingMode = 1
	actingModeLUCI actingMode = 2
)

// actingMode returns an acting mode based on Options.
func (a *Authenticator) actingMode() actingMode {
	switch {
	case a.opts.ActAsServiceAccount == "":
		return actingModeNone
	case a.opts.ActViaLUCIRealm != "":
		return actingModeLUCI
	default:
		return actingModeIAM
	}
}

// checkInitialized is (true, <err>) if initialization happened (successfully or
// not) of (false, nil) if not.
func (a *Authenticator) checkInitialized() (bool, error) {
	if a.err != nil || a.baseToken != nil {
		return true, a.err
	}
	return false, nil
}

// ensureInitialized instantiates TokenProvider and reads token from cache.
//
// It is supposed to be called under the lock.
func (a *Authenticator) ensureInitialized() error {
	// Already initialized (successfully or not)?
	if initialized, err := a.checkInitialized(); initialized {
		return err
	}

	// ActViaLUCIRealm makes sense only with ActAsServiceAccount.
	if a.opts.ActViaLUCIRealm != "" && a.opts.ActAsServiceAccount == "" {
		a.err = ErrBadOptions
		return a.err
	}

	// SelectBestMethod may do heavy calls like talking to GCE metadata server,
	// call it lazily here rather than in NewAuthenticator.
	if a.opts.Method == AutoSelectMethod {
		a.opts.Method = SelectBestMethod(a.ctx, *a.opts)
	}

	// In Actor mode, switch the base token to have scopes required to call
	// the API used to generate target auth tokens. In non-actor mode, the base
	// token is also the target auth token, so scope it to whatever scopes were
	// requested via Options.
	var scopes []string
	var useIDTokens bool
	switch a.actingMode() {
	case actingModeNone:
		scopes = a.opts.Scopes
		useIDTokens = a.opts.UseIDTokens
		// To get ID tokens when using end-user credentials we need userinfo scope.
		if useIDTokens && a.opts.Method == UserCredentialsMethod {
			if !slices.Contains(scopes, OAuthScopeEmail) {
				scopes = normalizeScopes(append(scopes, OAuthScopeEmail))
			}
		}
	case actingModeIAM:
		scopes = []string{OAuthScopeIAM}
		useIDTokens = false // IAM uses OAuth tokens
	case actingModeLUCI:
		scopes = []string{OAuthScopeEmail}
		useIDTokens = false // LUCI currently uses OAuth tokens
	default:
		panic("impossible")
	}
	a.baseToken = &tokenWithProvider{}
	a.baseToken.provider, a.err = makeBaseTokenProvider(a.ctx, a.opts, scopes, useIDTokens)
	if a.err != nil {
		return a.err // note: this can be ErrInsufficientAccess
	}

	// In non-actor mode, the token we must check in 'CheckLoginRequired' is the
	// same as returned by 'GetAccessToken'. In actor mode, they are different.
	// See comments for 'baseToken' and 'authToken'.
	switch a.actingMode() {
	case actingModeNone:
		a.authToken = a.baseToken
	case actingModeIAM:
		a.authToken = &tokenWithProvider{}
		a.authToken.provider, a.err = makeIAMTokenProvider(a.ctx, a.opts)
	case actingModeLUCI:
		a.authToken = &tokenWithProvider{}
		a.authToken.provider, a.err = makeLUCITokenProvider(a.ctx, a.opts)
	default:
		panic("impossible")
	}
	if a.err != nil {
		return a.err
	}

	// Initialize the token cache. Use the disk cache only if SecretsDir is given
	// and any of the providers is not "lightweight" (so it makes sense to
	// actually hit the disk, rather then call the provider each time new token is
	// needed).
	if a.opts.testingCache != nil {
		a.baseToken.cache = a.opts.testingCache
		a.authToken.cache = a.opts.testingCache
	} else {
		cache := internal.ProcTokenCache
		if !a.baseToken.provider.Lightweight() || !a.authToken.provider.Lightweight() {
			if a.opts.SecretsDir != "" {
				cache = &internal.DiskTokenCache{
					Context:    a.ctx,
					SecretsDir: a.opts.SecretsDir,
				}
			} else {
				logging.Warningf(a.ctx, "Disabling auth disk token cache. Not configured.")
			}
		}
		// Use the disk cache only for non-lightweight providers to avoid
		// unnecessarily leaks of tokens to the disk.
		if a.baseToken.provider.Lightweight() {
			a.baseToken.cache = internal.ProcTokenCache
		} else {
			a.baseToken.cache = cache
		}
		if a.authToken.provider.Lightweight() {
			a.authToken.cache = internal.ProcTokenCache
		} else {
			a.authToken.cache = cache
		}
	}

	// Interactive providers need to know whether there's a cached token (to ask
	// to run interactive login if there's none). Non-interactive providers do not
	// care about state of the cache that much (they know how to update it
	// themselves). So examine the cache here only when using interactive
	// provider. Non interactive providers will do it lazily on a first
	// refreshToken(...) call.
	if a.baseToken.provider.RequiresInteraction() {
		// Broken token cache is not a fatal error. So just log it and forget, a new
		// token will be minted in Login.
		if err := a.baseToken.fetchFromCache(a.ctx); err != nil {
			logging.Warningf(a.ctx, "Failed to read auth token from cache: %s", err)
		}
	}

	// Note: a.authToken.provider is either equal to a.baseToken.provider (if not
	// using actor mode), or (when using actor mode) it doesn't require
	// interaction (because all "acting" providers are non-interactive). So don't
	// bother fetching 'authToken' from the cache. It will be fetched lazily on
	// the first use.

	return nil
}

// doLoginIfRequired optionally performs an interactive login.
//
// This is the main place where LoginMode handling is performed. Used by various
// factories (Transport, PerRPCCredentials, TokenSource, ...).
//
// If requiresAuth is false, we respect OptionalLogin mode. If true - we treat
// OptionalLogin mode as SilentLogin: some authentication mechanisms (like
// oauth2.TokenSource) require valid tokens no matter what. The corresponding
// factories set requiresAuth to true.
//
// Returns:
//
//	(true, nil) if successfully initialized the authenticator with some token.
//	(false, nil) to disable authentication (for OptionalLogin mode).
//	(false, err) on errors.
func (a *Authenticator) doLoginIfRequired(requiresAuth bool) (useAuth bool, err error) {
	err = a.CheckLoginRequired() // also initializes guts for effectiveLoginMode()
	effectiveMode := a.effectiveLoginMode()
	if requiresAuth && effectiveMode == OptionalLogin {
		effectiveMode = SilentLogin
	}
	switch {
	case err == nil:
		return true, nil // have a valid cached base token
	case err == ErrInsufficientAccess && effectiveMode == OptionalLogin:
		return false, nil // have the base token, but it doesn't have enough scopes
	case err != ErrLoginRequired:
		return false, err // some error we can't handle (we handle only ErrLoginRequired)
	case effectiveMode == SilentLogin:
		return false, ErrLoginRequired // can't run Login in SilentLogin mode
	case effectiveMode == OptionalLogin:
		return false, nil // we can skip auth in OptionalLogin if we have no token
	case effectiveMode != InteractiveLogin:
		return false, errors.Reason("invalid mode argument: %s", effectiveMode).Err()
	}
	if err := a.Login(); err != nil {
		return false, err
	}
	return true, nil
}

// effectiveLoginMode returns a login mode to use, considering what kind of a
// token provider is being used.
//
// See comments for OptionalLogin for more info. The gist of it: we treat
// OptionalLogin as SilentLogin when using a service account private key.
func (a *Authenticator) effectiveLoginMode() (lm LoginMode) {
	// a.opts.Method is modified under a lock, need to grab the lock to avoid a
	// race. Note that a.loginMode is immutable and can be read outside the
	// lock. We skip the locking if we know for sure that the return value will be
	// same as a.loginMode (which is the case for a.loginMode != OptionalLogin).
	lm = a.loginMode
	if lm == OptionalLogin {
		a.lock.RLock()
		if a.opts.Method == ServiceAccountMethod {
			lm = SilentLogin
		}
		a.lock.RUnlock()
	}
	return
}

// currentToken returns last known authentication token (or nil).
//
// If the token is not loaded yet, will attempt to load it from the on-disk
// cache. Returns nil if it's not there.
//
// It lock a.lock inside. It MUST NOT be called when a.lock is held. It will
// lazily call 'ensureInitialized' if necessary, returning its error.
func (a *Authenticator) currentToken() (tok *internal.Token, err error) {
	a.lock.RLock()
	initialized, err := a.checkInitialized()
	if initialized && err == nil {
		tok = a.authToken.token
	}
	a.lock.RUnlock()
	if err != nil {
		return
	}

	if !initialized || tok == nil {
		a.lock.Lock()
		defer a.lock.Unlock()

		if !initialized {
			if err = a.ensureInitialized(); err != nil {
				return
			}
			tok = a.authToken.token
		}

		if tok == nil {
			// Reading the token from cache is best effort. A broken cache is treated
			// like a cache miss.
			if cacheErr := a.authToken.fetchFromCache(a.ctx); cacheErr != nil {
				logging.Warningf(a.ctx, "Failed to read auth token from cache: %s", cacheErr)
			}
			tok = a.authToken.token
		}
	}

	return
}

// refreshToken compares current auth token to 'prev' and launches token refresh
// procedure if they still match.
//
// Returns a refreshed token (if a refresh procedure happened) or the current
// token, if it's already different from 'prev'. Acts as "Compare-And-Swap"
// where "Swap" is a token refresh procedure.
//
// If the token can't be refreshed (e.g. the base token or the credentials were
// revoked), sets the current auth token to nil and returns an error.
func (a *Authenticator) refreshToken(prev *internal.Token, lifetime time.Duration) (*internal.Token, error) {
	return a.authToken.compareAndRefresh(a.ctx, compareAndRefreshOp{
		lock:     &a.lock,
		prev:     prev,
		lifetime: lifetime,
		refreshCb: func(ctx context.Context, prev *internal.Token) (*internal.Token, error) {
			// In Actor mode, need to make sure we have a sufficiently fresh base
			// token first, since it's needed to call "acting" API to get a new auth
			// token for the target service account. 1 min should be more than enough
			// to make an RPC.
			var base *internal.Token
			if a.actingMode() != actingModeNone {
				var err error
				if base, err = a.getBaseTokenLocked(ctx, time.Minute); err != nil {
					return nil, err
				}
			}
			return a.authToken.renewToken(ctx, prev, base)
		},
	})
}

// getBaseTokenLocked is used to get an actor token when running in actor mode.
//
// It is called with a.lock locked.
func (a *Authenticator) getBaseTokenLocked(ctx context.Context, lifetime time.Duration) (*internal.Token, error) {
	// Already have a good token?
	if !internal.TokenExpiresInRnd(ctx, a.baseToken.token, lifetime) {
		return a.baseToken.token, nil
	}

	// Need to make one.
	return a.baseToken.compareAndRefresh(ctx, compareAndRefreshOp{
		lock:     nil, // already holding the lock
		prev:     a.baseToken.token,
		lifetime: lifetime,
		refreshCb: func(ctx context.Context, prev *internal.Token) (*internal.Token, error) {
			return a.baseToken.renewToken(ctx, prev, nil)
		},
	})
}

////////////////////////////////////////////////////////////////////////////////
// Transport implementation.

// authTokenInjector injects an authentication token into request headers.
//
// Used as a callback for NewModifyingTransport.
func (a *Authenticator) authTokenInjector(req *http.Request) error {
	switch tok, err := a.GetAccessToken(a.opts.MinTokenLifetime); {
	case err == ErrLoginRequired && a.effectiveLoginMode() == OptionalLogin:
		return nil // skip auth, no need for modifications
	case err != nil:
		return err
	default:
		tok.SetAuthHeader(req)
		return nil
	}
}

////////////////////////////////////////////////////////////////////////////////
// tokenWithProvider implementation.

// tokenWithProvider wraps a token with provider that can update it and a cache
// that stores it.
type tokenWithProvider struct {
	token    *internal.Token        // in-memory cache of the token
	provider internal.TokenProvider // knows how to generate 'token'
	cache    internal.TokenCache    // persistent cache for the token
}

// fetchFromCache updates 't.token' by reading it from the cache.
func (t *tokenWithProvider) fetchFromCache(ctx context.Context) error {
	key, err := t.provider.CacheKey(ctx)
	if err != nil {
		return err
	}
	tok, err := t.cache.GetToken(key)
	if err != nil {
		return err
	}
	t.token = tok
	return nil
}

// putToCache puts 't.token' value into the cache.
func (t *tokenWithProvider) putToCache(ctx context.Context) error {
	key, err := t.provider.CacheKey(ctx)
	if err != nil {
		return err
	}
	return t.cache.PutToken(key, t.token)
}

// purgeToken removes the token from both on-disk cache and memory.
func (t *tokenWithProvider) purgeToken(ctx context.Context) error {
	t.token = nil
	key, err := t.provider.CacheKey(ctx)
	if err != nil {
		return err
	}
	return t.cache.DeleteToken(key)
}

// compareAndRefreshOp is parameters for 'compareAndRefresh' call.
type compareAndRefreshOp struct {
	lock      sync.Locker     // optional lock to grab when comparing and refreshing
	prev      *internal.Token // previously known token (the one we are refreshing)
	lifetime  time.Duration   // minimum acceptable token lifetime or <0 to force a refresh
	refreshCb func(ctx context.Context, existing *internal.Token) (*internal.Token, error)
}

// compareAndRefresh compares currently stored token to 'prev' and calls the
// given callback (under the lock, if not nil) to refresh it if they are still
// equal.
//
// Returns a refreshed token (if a refresh procedure happened) or the current
// token, if it's already different from 'prev'. Acts as "Compare-And-Swap"
// where "Swap" is a token refresh callback.
//
// If the callback returns an error (meaning the token can't be refreshed), sets
// the token to nil and returns the error.
func (t *tokenWithProvider) compareAndRefresh(ctx context.Context, params compareAndRefreshOp) (*internal.Token, error) {
	cacheKey, err := t.provider.CacheKey(ctx)
	if err != nil {
		// An error here is truly fatal. It is something like "can't read service
		// account JSON from disk". There's no way to refresh a token without it.
		return nil, err
	}

	// To give a context to "Minting a new token" messages and similar below.
	ctx = logging.SetFields(ctx, logging.Fields{
		"key":    cacheKey.Key,
		"scopes": strings.Join(cacheKey.Scopes, " "),
	})

	// Check that the token still need a refresh and do it (under the lock).
	tok, cacheIt, err := func() (*internal.Token, bool, error) {
		if params.lock != nil {
			params.lock.Lock()
			defer params.lock.Unlock()
		}

		// Some other goroutine already updated the token, just return the new one.
		if t.token != nil && !internal.EqualTokens(t.token, params.prev) {
			return t.token, false, nil
		}

		// Rescan the cache. Maybe some other process updated the token. This branch
		// is also responsible for lazy-loading of tokens from cache for
		// non-interactive providers, see ensureInitialized().
		if cached, _ := t.cache.GetToken(cacheKey); cached != nil {
			t.token = cached
			if !internal.EqualTokens(cached, params.prev) && params.lifetime > 0 && !internal.TokenExpiresIn(ctx, cached, params.lifetime) {
				return cached, false, nil
			}
		}

		// No one updated the token yet. It should be us. Mint a new token or
		// refresh the existing one.
		start := clock.Now(ctx)
		newTok, err := params.refreshCb(ctx, t.token)
		if err != nil {
			t.token = nil
			return nil, false, err
		}
		now := clock.Now(ctx)
		logging.Debugf(
			ctx, "The token refreshed in %s, expires in %s",
			now.Sub(start), newTok.Expiry.Round(0).Sub(now))
		t.token = newTok
		return newTok, true, nil
	}()

	if err == internal.ErrBadRefreshToken || err == internal.ErrBadCredentials {
		// Do not keep the broken token in the cache. It is unusable. Do this
		// outside the lock to avoid blocking other callers. Note that t.token is
		// already nil.
		if err := t.cache.DeleteToken(cacheKey); err != nil {
			logging.Warningf(ctx, "Failed to remove broken token from the cache: %s", err)
		}
		// A bad refresh token can be fixed by interactive login, so adjust the
		// error accordingly in this case.
		if err == internal.ErrBadRefreshToken {
			err = ErrLoginRequired
		}
	}

	if err != nil {
		return nil, err
	}

	// Update the cache outside the lock, no need for callers to wait for this.
	// Do not die if failed, the token is still usable from the memory.
	if cacheIt && tok != nil {
		if err := t.cache.PutToken(cacheKey, tok); err != nil {
			logging.Warningf(ctx, "Failed to write refreshed token to the cache: %s", err)
		}
	}

	return tok, nil
}

// renewToken is called to mint a new token or update existing one.
//
// It is called from non-interactive 'refreshToken' method, and thus it can't
// use interactive login flow.
func (t *tokenWithProvider) renewToken(ctx context.Context, prev, base *internal.Token) (*internal.Token, error) {
	if prev == nil {
		if t.provider.RequiresInteraction() {
			return nil, ErrLoginRequired
		}
		logging.Debugf(ctx, "Minting a new token")
		tok, err := t.mintTokenWithRetries(ctx, base)
		if err != nil {
			logging.Warningf(ctx, "Failed to mint a token: %s", err)
			return nil, err
		}
		return tok, nil
	}

	logging.Debugf(ctx, "Refreshing the token")
	tok, err := t.refreshTokenWithRetries(ctx, prev, base)
	if err != nil {
		logging.Warningf(ctx, "Failed to refresh the token: %s", err)
		return nil, err
	}
	return tok, nil
}

// retryParams defines retry strategy for handling transient errors when minting
// or refreshing tokens.
func retryParams() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:    10 * time.Millisecond,
			Retries:  50,
			MaxTotal: 2 * time.Minute,
		},
		Multiplier: 2,
	}
}

// mintTokenWithRetries calls provider's MintToken() retrying on transient
// errors a bunch of times. Called only for non-interactive providers.
func (t *tokenWithProvider) mintTokenWithRetries(ctx context.Context, base *internal.Token) (tok *internal.Token, err error) {
	err = retry.Retry(ctx, transient.Only(retryParams), func() error {
		tok, err = t.provider.MintToken(ctx, base)
		return err
	}, retry.LogCallback(ctx, "token-mint"))
	return
}

// refreshTokenWithRetries calls providers' RefreshToken(...) retrying on
// transient errors a bunch of times.
func (t *tokenWithProvider) refreshTokenWithRetries(ctx context.Context, prev, base *internal.Token) (tok *internal.Token, err error) {
	err = retry.Retry(ctx, transient.Only(retryParams), func() error {
		tok, err = t.provider.RefreshToken(ctx, prev, base)
		return err
	}, retry.LogCallback(ctx, "token-refresh"))
	return
}

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// normalizeScopes sorts the list of scopes and removes dups.
//
// Doesn't modify the original slice.
func normalizeScopes(s []string) []string {
	for i := 1; i < len(s); i++ {
		if s[i] <= s[i-1] { // not sorted or has dups
			sorted := stringset.NewFromSlice(s...)
			return sorted.ToSortedSlice()
		}
	}
	return s // already sorted and dedupped
}

// prepPhonyIDTokenScope checks `useIDTokens`.
//
// If it is true, requires the audience to be set and replaces scopes with
// a phony "audience:<value>" scope to be used as a cache key (and ignored by
// the providers, since they don't use OAuth2 scopes when minting ID tokens).
// See also comment for Scopes in internal.CacheKey.
//
// If `useIDTokens` is false, clears `audience`.
//
// As a result, the audience is set if and only if `useIDTokens` is true.
func prepPhonyIDTokenScope(useIDTokens bool, scopes []string, audience string) (scopesOut []string, audienceOut string, err error) {
	if useIDTokens {
		if audience == "" {
			return nil, "", ErrAudienceRequired
		}
		return []string{"audience:" + audience}, audience, nil
	}
	return scopes, "", nil
}

// makeBaseTokenProvider creates TokenProvider implementation based on options.
//
// opts.Scopes and opts.UseIDTokens are ignored, `scopes` and `useIDTokens` are
// used instead. This is used in actor mode to supply parameters necessary to
// use an "acting" API: they generally do not match what's in `opts`.
//
// Called by ensureInitialized.
func makeBaseTokenProvider(ctx context.Context, opts *Options, scopes []string, useIDTokens bool) (internal.TokenProvider, error) {
	if opts.testingBaseTokenProvider != nil {
		return opts.testingBaseTokenProvider, nil
	}

	// Only UserCredentialsMethod can generate ID tokens and access tokens at
	// the same time. All other methods can do only ID tokens or only access
	// tokens. prepPhonyIDTokenScope checks/mutates the parameters accordingly,
	// see its doc.
	audience := opts.Audience
	if opts.Method != UserCredentialsMethod {
		var err error
		scopes, audience, err = prepPhonyIDTokenScope(useIDTokens, scopes, audience)
		if err != nil {
			return nil, err
		}
	}

	switch opts.Method {
	case UserCredentialsMethod:
		if opts.ClientID == "" || opts.ClientSecret == "" {
			return nil, errors.Reason("OAuth client is not configured, can't use interactive login").Err()
		}
		if internal.NewLoginSessionTokenProvider == nil {
			return nil, errors.Reason("support for interactive login flow is not compiled into this binary").Err()
		}
		if opts.LoginSessionsHost == "" {
			return nil, errors.Reason("no login session host configured").Err()
		}
		// Note: LoginSessionTokenProvider supports ID tokens
		// and OAuth access tokens at the same time.
		// It ignores audience (it always matches ClientID).
		return internal.NewLoginSessionTokenProvider(
			ctx,
			opts.LoginSessionsHost,
			opts.ClientID,
			opts.ClientSecret,
			scopes,
			opts.Transport)
	case ServiceAccountMethod:
		serviceAccountPath := ""
		if len(opts.ServiceAccountJSON) == 0 {
			serviceAccountPath = opts.ServiceAccountJSONPath
		}
		return internal.NewServiceAccountTokenProvider(
			ctx,
			opts.ServiceAccountJSON,
			serviceAccountPath,
			scopes,
			audience)
	case GCEMetadataMethod:
		return internal.NewGCETokenProvider(
			ctx,
			opts.GCEAccountName,
			opts.GCESupportsArbitraryScopes,
			scopes,
			audience)
	case LUCIContextMethod:
		return internal.NewLUCIContextTokenProvider(
			ctx,
			scopes,
			audience,
			opts.Transport)
	default:
		return nil, errors.Reason("unrecognized authentication method: %s", opts.Method).Err()
	}
}

// makeIAMTokenProvider creates TokenProvider to use in actingModeIAM mode.
//
// Called by ensureInitialized.
func makeIAMTokenProvider(ctx context.Context, opts *Options) (internal.TokenProvider, error) {
	if opts.testingIAMTokenProvider != nil {
		return opts.testingIAMTokenProvider, nil
	}
	scopes, audience, err := prepPhonyIDTokenScope(opts.UseIDTokens, opts.Scopes, opts.Audience)
	if err != nil {
		return nil, err
	}
	return internal.NewIAMTokenProvider(
		ctx,
		opts.ActAsServiceAccount,
		scopes,
		audience,
		opts.Transport)
}

// makeLUCITokenProvider creates TokenProvider to use in actingModeLUCI mode.
//
// Called by ensureInitialized.
func makeLUCITokenProvider(ctx context.Context, opts *Options) (internal.TokenProvider, error) {
	if opts.TokenServerHost == "" {
		return nil, ErrBadOptions
	}
	if internal.NewLUCITSTokenProvider == nil {
		return nil, errors.New("support for impersonation through LUCI is not compiled into this binary")
	}
	scopes, audience, err := prepPhonyIDTokenScope(opts.UseIDTokens, opts.Scopes, opts.Audience)
	if err != nil {
		return nil, err
	}
	return internal.NewLUCITSTokenProvider(
		ctx,
		opts.TokenServerHost,
		opts.ActAsServiceAccount,
		opts.ActViaLUCIRealm,
		scopes,
		audience,
		opts.Transport)
}
