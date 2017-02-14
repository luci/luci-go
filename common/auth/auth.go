// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package auth implements a wrapper around golang.org/x/oauth2.
//
// Its main improvement is the on-disk cache for OAuth tokens, which is
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
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/credentials"

	"cloud.google.com/go/compute/metadata"

	"github.com/luci/luci-go/common/auth/internal"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
)

var (
	// ErrLoginRequired is returned by Transport() in case long term credentials
	// are not cached and the user must go through interactive login.
	ErrLoginRequired = errors.New("interactive login is required")

	// ErrInsufficientAccess is returned by Login() or Transport() if access_token
	// can't be minted for given OAuth scopes. For example if GCE instance wasn't
	// granted access to requested scopes when it was created.
	ErrInsufficientAccess = internal.ErrInsufficientAccess

	// ErrBadCredentials is returned by authenticating RoundTripper if service
	// account key used to generate access tokens is revoked, malformed or can not
	// be read from disk.
	ErrBadCredentials = internal.ErrBadCredentials
)

// Known Google API OAuth scopes.
const (
	OAuthScopeEmail = "https://www.googleapis.com/auth/userinfo.email"
)

// Method defines a method to use to obtain OAuth access token.
type Method string

// Supported authentication methods.
const (
	// AutoSelectMethod can be used to allow the library to pick a method most
	// appropriate for given set of options and the current execution environment.
	//
	// For example, passing ServiceAccountJSONPath or ServiceAcountJSON makes
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
)

// LoginMode is used as enum in NewAuthenticator function.
type LoginMode string

const (
	// InteractiveLogin instructs Authenticator to ignore cached tokens (if any)
	// and forcefully rerun interactive login flow in Transport(), Client() and
	// other factories or Login() (whichever is called first).
	//
	// This is typically used with UserCredentialsMethod to generate an OAuth
	// refresh token and put it in the token cache, to make it available for later
	// non-interactive usage.
	//
	// When used with service account credentials (ServiceAccountMethod mode),
	// just precaches the access token.
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
	// TODO(vadimsh): When used with ServiceAccountMethod it is identical to
	// SilentLogin, since it makes no sense to ignore invalid service account
	// credentials when the caller is explicitly asking the authenticator to use
	// them.
	//
	// Has the original meaning when used with GCEMetadataMethod: it instructs to
	// skip authentication if the token returned by GCE metadata service doesn't
	// have all requested scopes.
	OptionalLogin LoginMode = "OptionalLogin"
)

// minAcceptedLifetime is minimal lifetime of a token returned by the token
// source or put into authentication headers.
//
// If token is expected to live less than this duration, it will be refreshed.
const minAcceptedLifetime = 2 * time.Minute

// Options are used by NewAuthenticator call.
//
// All fields are optional and have sane default values.
type Options struct {
	// Transport is underlying round tripper to use for requests.
	//
	// Default: http.DefaultTransport.
	Transport http.RoundTripper

	// Method defines how to grab OAuth2 tokens.
	//
	// Default: AutoSelectMethod.
	Method Method

	// Scopes is a list of OAuth scopes to request.
	//
	// Default: [OAuthScopeEmail].
	Scopes []string

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

	// ServiceAccountJSONPath is a path to a JSON blob with a private key to use.
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

	// SecretsDir can be used to set the path to a directory where tokens
	// are cached.
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

	// customTokenProvider is used in unit tests.
	customTokenProvider internal.TokenProvider
}

// SelectBestMethod returns a most appropriate authentication method for the
// given set of options and the current execution environment.
//
// Invoked by Authenticator if AutoSelectMethod is passed as Method in Options.
//
// Shouldn't be generally used directly. Exposed publicly just for documentation
// purposes.
func SelectBestMethod(opts *Options) Method {
	switch {
	case opts.ServiceAccountJSONPath != "" || len(opts.ServiceAccountJSON) != 0:
		return ServiceAccountMethod
	case metadata.OnGCE():
		return GCEMetadataMethod
	default:
		return UserCredentialsMethod
	}
}

// Authenticator is a factory for http.RoundTripper objects that know how to use
// cached OAuth credentials and how to send monitoring metrics (if tsmon package
// was imported).
//
// Authenticator also knows how to run interactive login flow, if required.
type Authenticator struct {
	// Immutable members.
	loginMode LoginMode
	opts      *Options
	transport http.RoundTripper
	ctx       context.Context

	// Mutable members.
	lock     sync.RWMutex
	cache    internal.TokenCache
	provider internal.TokenProvider
	err      error
	token    *oauth2.Token
}

// NewAuthenticator returns a new instance of Authenticator given its options.
//
// The authenticator is essentially a factory for http.RoundTripper that knows
// how to use OAuth2 tokens. It is bound to the given context: uses its logger,
// clock, transport and deadline.
func NewAuthenticator(ctx context.Context, loginMode LoginMode, opts Options) *Authenticator {
	ctx = logging.SetField(ctx, "pkg", "auth")

	// Add default scope, sort scopes.
	if len(opts.Scopes) == 0 {
		opts.Scopes = []string{OAuthScopeEmail}
	} else {
		opts.Scopes = append([]string(nil), opts.Scopes...) // copy
		sort.Strings(opts.Scopes)
	}

	// Fill in blanks with default values.
	if opts.GCEAccountName == "" {
		opts.GCEAccountName = "default"
	}
	if opts.Transport == nil {
		opts.Transport = http.DefaultTransport
	}

	// TODO(vadimsh): Check SecretsDir permissions. It should be 0700.
	if opts.SecretsDir != "" && !filepath.IsAbs(opts.SecretsDir) {
		var err error
		opts.SecretsDir, err = filepath.Abs(opts.SecretsDir)
		if err != nil {
			panic(fmt.Errorf("failed to get abs path to token cache dir: %s", err))
		}
	}

	// See ensureInitialized for the rest of the initialization.
	auth := &Authenticator{
		loginMode: loginMode,
		opts:      &opts,
		ctx:       ctx,
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
//   * nil if we have a valid cached token or can mint one on the fly.
//   * ErrLoginRequired if we have no cached token and need to bother the user.
//   * ErrInsufficientAccess if the configured auth method can't mint the token
//     we require (e.g when using GCE method and the instance doesn't have all
//     requested OAuth scopes).
//   * Generic error on other unexpected errors.
func (a *Authenticator) CheckLoginRequired() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if err := a.ensureInitialized(); err != nil {
		return err
	}

	// No cached token and token provider requires interaction with a user: need
	// to login. Only non-interactive token providers are allowed to mint tokens
	// on the fly, see refreshToken.
	if a.token == nil && a.provider.RequiresInteraction() {
		return ErrLoginRequired
	}

	return nil
}

// Login perform an interaction with the user to get a long term refresh token
// and cache it.
//
// Blocks for user input, can use stdin. It overwrites currently cached
// credentials, if any.
func (a *Authenticator) Login() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.ensureInitialized()
	if err != nil {
		return err
	}
	if !a.provider.RequiresInteraction() {
		return nil // can mint the token on the fly, no need for login
	}

	// Create initial token. This may require interaction with a user. Do not do
	// retries here, since Login is called when user is looking, let user do the
	// retries (since if MintToken() interacts with the user, retrying it
	// automatically will be extra confusing).
	a.token, err = a.provider.MintToken()
	if err != nil {
		return err
	}

	// Store the initial token in the cache. Don't abort if it fails, the token
	// is still usable from the memory.
	key, err := a.provider.CacheKey()
	if err == nil {
		err = a.cache.PutToken(key, a.token)
	}
	if err != nil {
		logging.Warningf(a.ctx, "Failed to write token to cache: %s", err)
	}

	return nil
}

// PurgeCredentialsCache removes cached tokens.
func (a *Authenticator) PurgeCredentialsCache() error {
	a.lock.Lock()
	defer a.lock.Unlock()
	if err := a.ensureInitialized(); err != nil {
		return err
	}
	a.token = nil
	key, err := a.provider.CacheKey()
	if err != nil {
		return err
	}
	return a.cache.DeleteToken(key)
}

// GetAccessToken returns a valid access token with specified minimum lifetime.
//
// Does not interact with the user. May return ErrLoginRequired.
func (a *Authenticator) GetAccessToken(lifetime time.Duration) (*oauth2.Token, error) {
	a.lock.Lock()
	if err := a.ensureInitialized(); err != nil {
		a.lock.Unlock()
		return nil, err
	}
	tok := a.token
	a.lock.Unlock()

	if tok == nil || internal.TokenExpiresInRnd(a.ctx, tok, lifetime) {
		var err error
		tok, err = a.refreshToken(tok, lifetime)
		if err != nil {
			return nil, err
		}
		// Note: no randomization here. It is a sanity check that verifies
		// refreshToken did its job.
		if internal.TokenExpiresIn(a.ctx, tok, lifetime) {
			return nil, fmt.Errorf("auth: failed to refresh the token")
		}
	}
	return tok, nil
}

// TokenSource optionally performs a login and returns oauth2.TokenSource.
//
// Can be used for interoperability with libraries that use golang.org/x/oauth2.
//
// It doesn't support 'OptionalLogin' mode, since oauth2.TokenSource must return
// some token. Otherwise its logic is similar to Transport(). In particular it
// may return ErrLoginRequired if interactive login is required, but the
// authenticator is in the silent mode. See LoginMode enum for more details.
//
// TODO(vadimsh): Move up, closer to Transport().
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
//
// TODO(vadimsh): Move up, closer to Transport().
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

////////////////////////////////////////////////////////////////////////////////
// credentials.PerRPCCredentials implementation.

type perRPCCreds struct {
	a *Authenticator
}

func (creds perRPCCreds) GetRequestMetadata(c context.Context, uri ...string) (map[string]string, error) {
	if len(uri) == 0 {
		panic("perRPCCreds: no URI given")
	}
	if creds.a == nil {
		return nil, nil
	}
	tok, err := creds.a.GetAccessToken(minAcceptedLifetime)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"Authorization": tok.TokenType + " " + tok.AccessToken,
	}, nil
}

func (creds perRPCCreds) RequireTransportSecurity() bool { return true }

////////////////////////////////////////////////////////////////////////////////
// oauth2.TokenSource implementation.

type tokenSource struct {
	a *Authenticator
}

// Token is part of oauth2.TokenSource inteface.
func (s tokenSource) Token() (*oauth2.Token, error) {
	return s.a.GetAccessToken(minAcceptedLifetime)
}

////////////////////////////////////////////////////////////////////////////////
// Authenticator private methods.

// ensureInitialized instantiates TokenProvider and reads token from cache.
//
// It is supposed to be called under the lock.
func (a *Authenticator) ensureInitialized() error {
	if a.err != nil || a.provider != nil {
		return a.err
	}

	// SelectBestMethod may do heavy calls like talking to GCE metadata server,
	// call it lazily here rather than in NewAuthenticator.
	if a.opts.Method == AutoSelectMethod {
		a.opts.Method = SelectBestMethod(a.opts)
	}
	a.provider, a.err = makeTokenProvider(a.ctx, a.opts)
	if a.err != nil {
		return a.err // note: this can be ErrInsufficientAccess
	}

	// Initialize token caches. Use disk cache only if SecretsDir is given and
	// the provider is not "lightweight" (so it makes sense to actually hit
	// the disk, rather then call the provider each time new token is needed).
	//
	// Note also that tests set a.cache before ensureInitialized() is called, so
	// don't overwrite it here.
	if a.cache == nil && !a.provider.Lightweight() {
		if a.opts.SecretsDir != "" {
			a.cache = &internal.DiskTokenCache{
				Context:    a.ctx,
				SecretsDir: a.opts.SecretsDir,
			}
		} else {
			logging.Warningf(a.ctx, "Disabling disk token cache. Not configured.")
		}
	}
	if a.cache == nil {
		a.cache = internal.ProcTokenCache
	}

	// Interactive providers need to know whether there's a cached token (to ask
	// to run interactive login if there's none). Non-interactive providers do not
	// care about state of the cache that much (they know how to update it
	// themselves). So examine the cache here only when using interactive
	// provider. Non interactive providers will do it lazily on a first
	// refreshToken(...) call.
	if a.provider.RequiresInteraction() {
		// Broken token cache is not a fatal error. So just log it and forget, a new
		// token will be minted.
		key, err := a.provider.CacheKey()
		if err == nil {
			a.token, err = a.cache.GetToken(key)
		}
		if err != nil {
			logging.Warningf(a.ctx, "Failed to read token from cache: %s", err)
		}
	}

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
//   (true, nil) if successfully initialized the authenticator with some token.
//   (false, nil) to disable authentication (for OptionalLogin mode).
//   (false, err) on errors.
func (a *Authenticator) doLoginIfRequired(requiresAuth bool) (useAuth bool, err error) {
	if a.loginMode == InteractiveLogin {
		if err := a.PurgeCredentialsCache(); err != nil {
			return false, err
		}
	}
	effectiveMode := a.loginMode
	if requiresAuth && effectiveMode == OptionalLogin {
		effectiveMode = SilentLogin
	}
	switch err := a.CheckLoginRequired(); {
	case err == nil:
		return true, nil // have a valid cached token
	case err == ErrInsufficientAccess && effectiveMode == OptionalLogin:
		return false, nil // have the token, but it doesn't have enough scopes
	case err != ErrLoginRequired:
		return false, err // some error we can't handle (we handle only ErrLoginRequired)
	case effectiveMode == SilentLogin:
		return false, ErrLoginRequired // can't run Login in SilentLogin mode
	case effectiveMode == OptionalLogin:
		return false, nil // we can skip auth in OptionalLogin if we have no token
	case effectiveMode != InteractiveLogin:
		return false, fmt.Errorf("invalid mode argument: %s", effectiveMode)
	}
	if err := a.Login(); err != nil {
		return false, err
	}
	return true, nil
}

// currentToken returns currently loaded token (or nil).
//
// It lock a.lock inside. It MUST NOT be called when a.lock is held.
func (a *Authenticator) currentToken() *oauth2.Token {
	a.lock.RLock()
	defer a.lock.RUnlock()
	return a.token
}

// refreshToken compares current token to 'prev' and launches token refresh
// procedure if they still match.
//
// Returns a refreshed token (if a refresh procedure happened) or the current
// token, if it's already different from 'prev'. Acts as "Compare-And-Swap"
// where "Swap" is a token refresh procedure.
//
// If token can't be refreshed (e.g. it was revoked), sets current token to nil
// and returns ErrLoginRequired or ErrBadCredentials.
func (a *Authenticator) refreshToken(prev *oauth2.Token, lifetime time.Duration) (*oauth2.Token, error) {
	// Note: 'refreshToken' is called only when the caller used TokenExpiresInRnd
	// check already and determined it is time to refresh the token. No expiration
	// randomization is needed at this point, it has already been done.

	cacheKey, err := a.provider.CacheKey()
	if err != nil {
		// An error here is truly fatal. It is something like "can't read service
		// account JSON from disk". There's no way to refresh a token without it.
		return nil, err
	}

	// Refresh the token under the lock.
	tok, cache, err := func() (*oauth2.Token, bool, error) {
		a.lock.Lock()
		defer a.lock.Unlock()

		// Some other goroutine already updated the token, just return new token.
		if a.token != nil && !internal.EqualTokens(a.token, prev) {
			return a.token, false, nil
		}

		// Rescan the cache. Maybe some other process updated the token. This branch
		// is also responsible for lazy-loading of token for non-interactive
		// providers, see ensureInitialized().
		if cached, _ := a.cache.GetToken(cacheKey); cached != nil {
			a.token = cached
			if !internal.EqualTokens(cached, prev) && !internal.TokenExpiresIn(a.ctx, cached, lifetime) {
				return cached, false, nil
			}
		}

		// Mint a new token or refresh the existing one.
		var err error
		if a.token == nil {
			// Can't do user interaction outside of Login.
			if a.provider.RequiresInteraction() {
				return nil, false, ErrLoginRequired
			}
			logging.Debugf(a.ctx, "Minting a new token, using %s", a.opts.Method)
			a.token, err = a.mintTokenWithRetries()
			if err != nil {
				logging.Warningf(a.ctx, "Failed to mint a token: %s", err)
				return nil, false, err
			}
		} else {
			logging.Debugf(a.ctx, "Refreshing the token")
			a.token, err = a.refreshTokenWithRetries(a.token)
			if err != nil {
				logging.Warningf(a.ctx, "Failed to refresh the token: %s", err)
				if err == internal.ErrBadRefreshToken && a.loginMode == OptionalLogin {
					logging.Warningf(a.ctx, "Switching to anonymous calls")
				}
				return nil, false, err
			}
		}
		lifetime := a.token.Expiry.Sub(clock.Now(a.ctx))
		logging.Debugf(a.ctx, "Token expires in %s", lifetime)
		return a.token, true, nil
	}()

	if err == internal.ErrBadRefreshToken || err == internal.ErrBadCredentials {
		// Do not keep broken token in the cache. It is unusable.
		if err := a.cache.DeleteToken(cacheKey); err != nil {
			logging.Warningf(a.ctx, "Failed to remove broken token from the cache: %s", err)
		}
		// Bad refresh token can be fixed by interactive logic. Bad credentials are
		// not fixable, so return the error as is in that case.
		if err == internal.ErrBadRefreshToken {
			err = ErrLoginRequired
		}
	}

	if err != nil {
		return nil, err
	}

	// Update the cache outside the lock, no need for callers to wait for this.
	// Do not die if failed, token is still usable from the memory.
	if cache && tok != nil {
		if err = a.cache.PutToken(cacheKey, tok); err != nil {
			logging.Warningf(a.ctx, "Failed to write refreshed token to the cache: %s", err)
		}
	}

	return tok, nil
}

////////////////////////////////////////////////////////////////////////////////
// Handling of transient errors.

// retryParams defines retry strategy for handling transient errors when minting
// or refreshing tokens.
func retryParams() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay:    10 * time.Millisecond,
			Retries:  50,
			MaxTotal: 10 * time.Second,
		},
		Multiplier: 1.5,
	}
}

// mintTokenWithRetries calls provider's MintToken() retrying on transient
// errors a bunch of times. Called only for non-interactive providers.
func (a *Authenticator) mintTokenWithRetries() (tok *oauth2.Token, err error) {
	err = retry.Retry(a.ctx, retry.TransientOnly(retryParams), func() error {
		tok, err = a.provider.MintToken()
		return err
	}, nil)
	return
}

// refreshTokenWithRetries calls providers' RefreshToken(...) retrying on
// transient errors a bunch of times.
func (a *Authenticator) refreshTokenWithRetries(t *oauth2.Token) (tok *oauth2.Token, err error) {
	err = retry.Retry(a.ctx, retry.TransientOnly(retryParams), func() error {
		tok, err = a.provider.RefreshToken(a.token)
		return err
	}, nil)
	return
}

// authTokenInjector injects an authentication token into request headers.
//
// Used as a callback for NewModifyingTransport.
func (a *Authenticator) authTokenInjector(req *http.Request) error {
	// Attempt to refresh the token, if required. Revert to non-authed call if
	// token can't be refreshed and running in OptionalLogin mode.
	tok := a.currentToken()
	if tok == nil || internal.TokenExpiresInRnd(a.ctx, tok, minAcceptedLifetime) {
		var err error
		tok, err = a.refreshToken(tok, minAcceptedLifetime)
		switch {
		case err == ErrLoginRequired && a.loginMode == OptionalLogin:
			return nil // skip auth, no need for modifications
		case err != nil:
			return err
		// Note: no randomization here. It is a sanity check that verifies
		// refreshToken did its job.
		case internal.TokenExpiresIn(a.ctx, tok, minAcceptedLifetime):
			return fmt.Errorf("auth: failed to refresh the token")
		}
	}
	tok.SetAuthHeader(req)
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

// makeTokenProvider creates TokenProvider implementation based on options.
//
// Called by ensureInitialized.
func makeTokenProvider(ctx context.Context, opts *Options) (internal.TokenProvider, error) {
	// customTokenProvider is set only in unit tests.
	if opts.customTokenProvider != nil {
		return opts.customTokenProvider, nil
	}

	// Note: on appengine context.Context actually carries the transport used to
	// run OAuth2 flows. oauth2 library provides no way to customize it. So on
	// appengine opts.Context must be set and it must be derived from GAE request
	// context. opts.Transport is not used for token exchange flow itself, only
	// for authorized requests.
	switch opts.Method {
	case UserCredentialsMethod:
		return internal.NewUserAuthTokenProvider(
			ctx,
			opts.ClientID,
			opts.ClientSecret,
			opts.Scopes)
	case ServiceAccountMethod:
		serviceAccountPath := ""
		if len(opts.ServiceAccountJSON) == 0 {
			serviceAccountPath = opts.ServiceAccountJSONPath
		}
		return internal.NewServiceAccountTokenProvider(
			ctx,
			opts.ServiceAccountJSON,
			serviceAccountPath,
			opts.Scopes)
	case GCEMetadataMethod:
		return internal.NewGCETokenProvider(ctx, opts.GCEAccountName, opts.Scopes)
	default:
		return nil, fmt.Errorf("auth: unrecognized authentication method: %s", opts.Method)
	}
}
