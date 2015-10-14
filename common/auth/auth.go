// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package auth defines an opinionated wrapper around OAuth2.

It hides configurability of base oauth2 library and instead makes a predefined
set of choices regarding where the credentials should be stored and how OAuth2
should be used. It makes authentication flows look more uniform across tools
that use infra.libs.auth and allow credentials reuse across multiple binaries.

Also it knows about various environments Chrome Infra tools are running under
(GCE, Chrome Infra Golo, GAE, developers' machine) and switches default
authentication scheme accordingly (e.g. on GCE machine the default is to use
GCE metadata server).

All tools that use infra.libs.auth share same credentials by default, meaning
a user needs to authenticate only once to use them all. Credentials are cached
in ~/.config/chrome_infra/auth/* and reused by all processes running under
the same user account.
*/
package auth

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/cloud/compute/metadata"

	"github.com/luci/luci-go/common/auth/internal"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
)

var (
	// ErrLoginRequired is returned by Transport() in case long term credentials
	// are not cached and the user must go through interactive login.
	ErrLoginRequired = errors.New("interactive login is required")

	// ErrInsufficientAccess is returned by Login() or Transport() if access_token
	// can't be minted for given OAuth scopes. For example if GCE instance wasn't
	// granted access to requested scopes when it was created.
	ErrInsufficientAccess = internal.ErrInsufficientAccess
)

// Known Google API OAuth scopes.
const (
	OAuthScopeEmail = "https://www.googleapis.com/auth/userinfo.email"
)

// Method defines a method to use to obtain OAuth access_token.
type Method string

// Supported authentication methods.
const (
	// AutoSelectMethod can be used to allow the library to pick a method most
	// appropriate for current execution environment. It will search for a private
	// key for a service account, then (if running on GCE) will try to query GCE
	// metadata server, and only then pick UserCredentialsMethod that requires
	// interaction with a user.
	AutoSelectMethod Method = ""

	// UserCredentialsMethod is used for interactive OAuth 3-legged login flow.
	UserCredentialsMethod Method = "UserCredentialsMethod"

	// ServiceAccountMethod is used to authenticate as a service account using
	// a private key.
	ServiceAccountMethod Method = "ServiceAccountMethod"

	// GCEMetadataMethod is used on Compute Engine to use tokens provided by
	// Metadata server. See https://cloud.google.com/compute/docs/authentication
	GCEMetadataMethod Method = "GCEMetadataMethod"

	// CustomMethod is used if access token is provided via CustomTokenMinter.
	CustomMethod Method = "CustomMethod"
)

// LoginMode is used as enum in NewAuthenticator function.
type LoginMode string

const (
	// InteractiveLogin is passed to NewAuthenticator to instruct Authenticator to
	// ignore cached tokens and forcefully rerun full login flow in Transport().
	// Used by 'login' CLI command.
	InteractiveLogin LoginMode = "InteractiveLogin"

	// SilentLogin is passed to NewAuthenticator if authentication must be used
	// and it is NOT OK to run interactive login flow to get the tokens.
	// Transport() call will fail with ErrLoginRequired error if there's no cached
	// tokens. Should normally be used by all CLI tools that need to use
	// authentication. If in doubt what mode to use, pick this one.
	SilentLogin LoginMode = "SilentLogin"

	// OptionalLogin is passed to NewAuthenticator if it is OK not to use
	// authentication if there are no cached credentials or they are revoked or
	// expired. Interactive login will never be called, default unauthenticated
	// client will be used instead if no credentials are present. Should be used
	// by CLI tools where authentication is optional.
	OptionalLogin LoginMode = "OptionalLogin"
)

// Options are used by NewAuthenticator call. All fields are optional and have
// sane default values.
type Options struct {
	// Context carries the local goroutine state such as transport and logger.
	// Context must be set to GAE request context if this library is used on
	// appengine. On non-appegine it is fine to pass nil to use default values.
	// Note that Authenticator will be bound to the context and should not
	// outlive it.
	Context context.Context
	// Logger is used to write log messages. If nil, extract it from the context.
	Logger logging.Logger
	// Transport is underlying round tripper to use for requests. If nil, will
	// be extracted from the context. If not there, http.DefaultTransport will
	// be used.
	Transport http.RoundTripper

	// Method defaults to AutoSelectMethod.
	Method Method
	// Scopes is a list of OAuth scopes to request, defaults to [OAuthScopeEmail].
	Scopes []string

	// ClientID is OAuth client_id to use with UserCredentialsMethod.
	// Default: provided by DefaultClient().
	ClientID string
	// ClientID is OAuth client_secret to use with UserCredentialsMethod.
	// Default: provided by DefaultClient().
	ClientSecret string

	// ServiceAccountJSONPath is a path to a JSON blob with a private key to use
	// with ServiceAccountMethod. See the "Credentials" page under "APIs & Auth"
	// for your project at Cloud Console.
	// Default: ~/.config/chrome_infra/auth/service_account.json.
	ServiceAccountJSONPath string
	// ServiceAccountJSON is a body of JSON key file to use. Overrides
	// ServiceAccountJSONPath if given.
	ServiceAccountJSON []byte

	// GCEAccountName is an account name to query to fetch token for from metadata
	// server when GCEMetadataMethod is used. If given account wasn't granted
	// required set of scopes during instance creation time, Transport() call
	// fails with ErrInsufficientAccess.
	// Default: "default" account.
	GCEAccountName string

	// TokenCacheFactory is a factory method to use to grab TokenCache object.
	// If not set, a file system cache will be used.
	TokenCacheFactory func(entryName string) (TokenCache, error)

	// CustomTokenMinter is a factory to make new tokens if CustomMethod is used.
	CustomTokenMinter TokenMinter
}

// Authenticator is a factory for http.RoundTripper objects that know how to use
// cached OAuth credentials. Authenticator also knows how to run interactive
// login flow, if required.
type Authenticator struct {
	// Immutable members.
	loginMode LoginMode
	opts      *Options
	transport http.RoundTripper
	log       logging.Logger

	// Mutable members.
	lock     sync.Mutex
	cache    TokenCache
	provider internal.TokenProvider
	err      error
	token    internal.Token
}

// TokenCache implements a scheme to cache access tokens between the calls. It
// will be used concurrently from multiple threads. Authenticator takes care
// of token serialization, TokenCache operates in terms of byte buffers.
type TokenCache interface {
	// Read returns the data previously stored with Write or (nil, nil) if not
	// there or expired. It is also OK if token cache doesn't do expiration check
	// itself. Cached byte buffer includes expiration time too.
	Read() ([]byte, error)

	// Write puts token data into the cache.
	Write(tok []byte, expiry time.Time) error

	// Clear clears the cache.
	Clear() error
}

// Token represents OAuth2 access token.
type Token struct {
	// AccessToken is actual token that authorizes and authenticates the requests.
	AccessToken string `json:"access_token"`

	// Expiry is the expiration time of the token or zero if it does not expire.
	Expiry time.Time `json:"expiry"`
}

// TokenMinter is a factory for access tokens when using CustomMode. It is
// invoked only if token is not in cache or has expired.
type TokenMinter interface {
	// MintToken return new access token for given scopes.
	MintToken(scopes []string) (Token, error)

	// CacheSeed is used to derive cache key name for tokens minted via
	// this object.
	CacheSeed() []byte
}

// NewAuthenticator returns a new instance of Authenticator given its options.
func NewAuthenticator(loginMode LoginMode, opts Options) *Authenticator {
	// Add default scope, sort scopes.
	if len(opts.Scopes) == 0 {
		opts.Scopes = []string{OAuthScopeEmail}
	}
	tmp := make([]string, len(opts.Scopes))
	copy(tmp, opts.Scopes)
	sort.Strings(tmp)
	opts.Scopes = tmp

	// Fill in blanks with default values.
	if opts.ClientID == "" || opts.ClientSecret == "" {
		opts.ClientID, opts.ClientSecret = DefaultClient()
	}
	if opts.GCEAccountName == "" {
		opts.GCEAccountName = "default"
	}
	if opts.Context == nil {
		opts.Context = context.Background()
	}
	if opts.Logger == nil {
		opts.Logger = logging.Get(opts.Context)
	}
	if opts.Transport == nil {
		opts.Transport = internal.TransportFromContext(opts.Context)
	}

	// See ensureInitialized for the rest of the initialization.
	auth := &Authenticator{
		loginMode: loginMode,
		opts:      &opts,
		log:       opts.Logger,
	}
	auth.transport = &authTransport{
		parent: auth,
		base:   opts.Transport,
	}
	return auth
}

// Transport optionally performs a login and returns http.RoundTripper. It is
// high level wrapper around Login() and TransportIfAvailable() calls. See
// documentation for LoginMode for more details.
func (a *Authenticator) Transport() (http.RoundTripper, error) {
	if a.loginMode == InteractiveLogin {
		if err := a.PurgeCredentialsCache(); err != nil {
			return nil, err
		}
	}
	transport, err := a.TransportIfAvailable()
	switch {
	case err == nil:
		return transport, nil
	case err != ErrLoginRequired || a.loginMode == SilentLogin:
		return nil, err
	case a.loginMode == OptionalLogin:
		return http.DefaultTransport, nil
	case a.loginMode != InteractiveLogin:
		return nil, fmt.Errorf("invalid mode argument: %s", a.loginMode)
	}
	if err = a.Login(); err != nil {
		return nil, err
	}
	return a.TransportIfAvailable()
}

// Client optionally performs a login and returns http.Client. It uses transport
// returned by Transport(). See documentation for LoginMode for more details.
func (a *Authenticator) Client() (*http.Client, error) {
	transport, err := a.Transport()
	if err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport}, nil
}

// TransportIfAvailable returns http.RoundTripper that adds authentication
// details to each request. An interactive authentication flow (if required)
// must be complete before making a transport, otherwise ErrLoginRequired is
// returned.
func (a *Authenticator) TransportIfAvailable() (http.RoundTripper, error) {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.ensureInitialized()
	if err != nil {
		return nil, err
	}

	// No cached token and token provider requires interaction with a user: need
	// to login. Only non-interactive token providers are allowed to mint tokens
	// on the fly, see refreshToken.
	if a.token == nil && a.provider.RequiresInteraction() {
		return nil, ErrLoginRequired
	}
	return a.transport, nil
}

// Login perform an interaction with the user to get a long term refresh token
// and cache it. Blocks for user input, can use stdin. Returns ErrNoTerminal
// if interaction with a user is required, but the process is not running
// under a terminal. It overwrites currently cached credentials, if any.
func (a *Authenticator) Login() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.ensureInitialized()
	if err != nil {
		return err
	}
	if !a.provider.RequiresInteraction() {
		return nil
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
	if err = a.cacheToken(a.token); err != nil {
		a.log.Warningf("auth: failed to write token to cache: %s", err)
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
	return a.cache.Clear()
}

// GetAccessToken returns a valid access token with specified minimum lifetime.
// Does not interact with the user. May return ErrLoginRequered.
func (a *Authenticator) GetAccessToken(lifetime time.Duration) (Token, error) {
	a.lock.Lock()
	if err := a.ensureInitialized(); err != nil {
		a.lock.Unlock()
		return Token{}, err
	}
	tok := a.token
	a.lock.Unlock()

	if a.expiresIn(tok, lifetime) {
		var err error
		tok, err = a.refreshToken(tok, lifetime)
		if err != nil {
			return Token{}, err
		}
		if a.expiresIn(tok, lifetime) {
			return Token{}, fmt.Errorf("auth: failed to refresh the token")
		}
	}
	return Token{
		AccessToken: tok.AccessToken(),
		Expiry:      tok.Expiry(),
	}, nil
}

////////////////////////////////////////////////////////////////////////////////
// Authenticator private methods.

// ensureInitialized is supposed to be called under the lock.
func (a *Authenticator) ensureInitialized() error {
	if a.err != nil || a.provider != nil {
		return a.err
	}

	// selectDefaultMethod may do heavy calls, call it lazily here rather than in
	// NewAuthenticator.
	if a.opts.Method == AutoSelectMethod {
		a.opts.Method = selectDefaultMethod(a.opts)
	}
	a.provider, a.err = makeTokenProvider(a.opts)
	if a.err != nil {
		return a.err
	}

	// Setup the cache only when Method is known, cache filename depends on it.
	cacheName := cacheEntryName(a.opts, a.provider)
	if a.opts.TokenCacheFactory != nil {
		a.cache, a.err = a.opts.TokenCacheFactory(cacheName)
		if a.err != nil {
			return a.err
		}
	} else {
		a.cache = &tokenFileCache{
			path: filepath.Join(SecretsDir(), cacheName+".tok"),
			log:  a.log,
		}
	}

	// Broken token cache is not a fatal error. So just log it and forget, a new
	// token will be minted.
	var err error
	a.token, err = a.readTokenCache()
	if err != nil {
		a.log.Warningf("auth: failed to read token from cache: %s", err)
	}
	return nil
}

// readTokenCache may be called with a.lock held or not held. It works either way.
func (a *Authenticator) readTokenCache() (internal.Token, error) {
	// 'read' returns (nil, nil) if cache is empty.
	buf, err := a.cache.Read()
	if err != nil || buf == nil {
		return nil, err
	}
	token, err := a.provider.UnmarshalToken(buf)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// cacheToken may be called with a.lock held or not held. It works either way.
func (a *Authenticator) cacheToken(tok internal.Token) error {
	buf, err := a.provider.MarshalToken(tok)
	if err != nil {
		return err
	}
	return a.cache.Write(buf, tok.Expiry())
}

// currentToken lock a.lock inside. It MUST NOT be called when a.lock is held.
func (a *Authenticator) currentToken() internal.Token {
	// TODO(vadimsh): Test with go test -race. The lock may be unnecessary.
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.token
}

// expiresIn returns True if token is not valid or expires within given
// duration.
func (a *Authenticator) expiresIn(t internal.Token, lifetime time.Duration) bool {
	if t == nil {
		return true
	}
	if t.AccessToken() == "" {
		return true
	}
	if t.Expiry().IsZero() {
		return false
	}
	expiry := t.Expiry().Add(-lifetime)
	return expiry.Before(clock.Now(a.opts.Context))
}

// refreshToken compares current token to 'prev' and launches token refresh
// procedure if they still match. Returns a refreshed token (if a refresh
// procedure happened) or the current token (i.e. if it's different from prev).
// Acts as "Compare-And-Swap" where "Swap" is a token refresh procedure.
// If token can't be refreshed (e.g. it was revoked), sets current token to nil
// and returns ErrLoginRequired.
func (a *Authenticator) refreshToken(prev internal.Token, lifetime time.Duration) (internal.Token, error) {
	// Refresh the token under the lock.
	tok, cache, err := func() (internal.Token, bool, error) {
		a.lock.Lock()
		defer a.lock.Unlock()

		// Some other goroutine already updated the token, just return the token.
		if a.token != nil && !a.token.Equals(prev) {
			return a.token, false, nil
		}

		// Rescan the cache. Maybe some other process updated the token.
		cached, err := a.readTokenCache()
		if err == nil && !a.expiresIn(cached, lifetime) && !cached.Equals(prev) {
			a.log.Debugf("auth: some other process put refreshed token in the cache")
			a.token = cached
			return a.token, false, nil
		}

		// Mint a new token or refresh the existing one.
		if a.token == nil {
			// Can't do user interaction outside of Login.
			if a.provider.RequiresInteraction() {
				return nil, false, ErrLoginRequired
			}
			a.log.Debugf("auth: minting a new token, using %s", a.opts.Method)
			a.token, err = a.mintTokenWithRetries()
			if err != nil {
				a.log.Warningf("auth: failed to mint a token: %s", err)
				return nil, false, err
			}
		} else {
			a.log.Debugf("auth: refreshing the token")
			a.token, err = a.refreshTokenWithRetries(a.token)
			if err != nil {
				a.log.Warningf("auth: failed to refresh the token: %s", err)
				if err == internal.ErrBadRefreshToken && a.loginMode == OptionalLogin {
					a.log.Warningf("auth: switching to anonymous calls")
				}
				return nil, false, err
			}
		}
		lifetime := a.token.Expiry().Sub(clock.Now(a.opts.Context))
		a.log.Debugf("auth: token expires in %s", lifetime)
		return a.token, true, nil
	}()

	if err == internal.ErrBadRefreshToken {
		// Do not keep revoked token in the cache. It is unusable.
		if err := a.cache.Clear(); err != nil {
			a.log.Warningf("auth: failed to remove revoked token from the cache: %s", err)
		}
		return nil, ErrLoginRequired
	}

	if err != nil {
		return nil, err
	}

	// Update the cache outside the lock, no need for callers to wait for this.
	// Do not die if failed, token is still usable from the memory.
	if cache && tok != nil {
		if err = a.cacheToken(tok); err != nil {
			a.log.Warningf("auth: failed to write refreshed token to the cache: %s", err)
		}
	}

	return tok, nil
}

// mintTokenWithRetries calls provider's MintToken() retrying on transient
// errors a bunch of times. Called only for non-interactive providers.
func (a *Authenticator) mintTokenWithRetries() (internal.Token, error) {
	// TODO(vadimsh): Implement retries.
	return a.provider.MintToken()
}

// refreshTokenWithRetries calls providers' RefreshToken(...) retrying on
// transient errors a bunch of times.
func (a *Authenticator) refreshTokenWithRetries(t internal.Token) (internal.Token, error) {
	// TODO(vadimsh): Implement retries.
	return a.provider.RefreshToken(a.token)
}

////////////////////////////////////////////////////////////////////////////////
// authTransport implementation.

// TODO(vadimsh): Support CancelRequest if underlying transport supports it.
// It's tricky. http.Client uses type cast to figure out whether transport
// supports request cancellation or not. So new authTransportWithCancelation
// should be used when parent transport provides CancelRequest. Also
// authTransport should keep a mapping between original http.Request objects
// and ones with access tokens attached (to know what to pass to
// parent transport CancelRequest).

type authTransport struct {
	parent *Authenticator
	base   http.RoundTripper
}

// RoundTrip appends authorization details to the request.
func (t *authTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Attempt to refresh the token, if required. Revert to non-authed call if
	// token can't be refreshed and running in OptionalLogin mode.
	tok := t.parent.currentToken()
	if tok == nil || t.parent.expiresIn(tok, time.Minute) {
		var err error
		tok, err = t.parent.refreshToken(tok, time.Minute)
		switch {
		case err == ErrLoginRequired && t.parent.loginMode == OptionalLogin:
			return t.base.RoundTrip(req)
		case err != nil:
			return nil, err
		case t.parent.expiresIn(tok, time.Minute):
			return nil, fmt.Errorf("auth: failed to refresh the token")
		}
	}
	// Original request must not be modified, make a shallow clone.
	clone := *req
	clone.Header = make(http.Header)
	for k, v := range req.Header {
		clone.Header[k] = v
	}
	clone.Header.Set("Authorization", "Bearer "+tok.AccessToken())
	return t.base.RoundTrip(&clone)
}

////////////////////////////////////////////////////////////////////////////////
// tokenFileCache implementation.

// tokenFileCache implements TokenCache on top of  the file system.
type tokenFileCache struct {
	path string
	log  logging.Logger
	lock sync.Mutex
}

func (c *tokenFileCache) Read() (buf []byte, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	buf, err = ioutil.ReadFile(c.path)
	if err != nil && os.IsNotExist(err) {
		err = nil
	}
	return
}

func (c *tokenFileCache) Write(buf []byte, exp time.Time) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.log.Debugf("auth: writing token to %s", c.path)
	err := os.MkdirAll(filepath.Dir(c.path), 0700)
	if err != nil {
		return err
	}
	// TODO(vadimsh): Make it atomic across multiple processes.
	return ioutil.WriteFile(c.path, buf, 0600)
}

func (c *tokenFileCache) Clear() error {
	c.lock.Lock()
	defer c.lock.Unlock()
	err := os.Remove(c.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Utility functions.

func cacheEntryName(opts *Options, p internal.TokenProvider) string {
	// Construct a name of cache entry from data that identifies requested
	// credential, to allow multiple differently configured instances of
	// Authenticator to coexist.
	sum := sha1.New()
	sum.Write([]byte(opts.Method))
	sum.Write([]byte{0})
	sum.Write([]byte(opts.ClientID))
	sum.Write([]byte{0})
	sum.Write([]byte(opts.ClientSecret))
	sum.Write([]byte{0})
	for _, scope := range opts.Scopes {
		sum.Write([]byte(scope))
		sum.Write([]byte{0})
	}
	sum.Write([]byte(opts.GCEAccountName))
	sum.Write(p.CacheSeed())
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// defaultServiceAccountPath returns a path to JSON key to look for by default.
func defaultServiceAccountPath() string {
	return filepath.Join(SecretsDir(), "service_account.json")
}

// selectDefaultMethod is mocked in tests.
var selectDefaultMethod = func(opts *Options) Method {
	if len(opts.ServiceAccountJSON) != 0 {
		return ServiceAccountMethod
	}
	// Try to find service account JSON file at provided or default location.
	serviceAccountPath := opts.ServiceAccountJSONPath
	if serviceAccountPath == "" {
		serviceAccountPath = defaultServiceAccountPath()
	}
	info, _ := os.Stat(serviceAccountPath)
	if info != nil && info.Mode().IsRegular() {
		return ServiceAccountMethod
	}
	if metadata.OnGCE() {
		return GCEMetadataMethod
	}
	return UserCredentialsMethod
}

// secretsDir is mocked in tests. Called by publicly visible SecretsDir().
var secretsDir = func() string {
	usr, err := user.Current()
	if err != nil {
		panic(err.Error())
	}
	// TODO(vadimsh): On Windows use SHGetFolderPath with CSIDL_LOCAL_APPDATA to
	// locate a directory to store app files.
	return filepath.Join(usr.HomeDir, ".config", "chrome_infra", "auth")
}

// makeTokenProvider is mocked in tests. Called by ensureInitialized.
var makeTokenProvider = func(opts *Options) (internal.TokenProvider, error) {
	// Note: on appengine opts.Context actually carries the transport used to
	// run OAuth2 flows. oauth2 library provides no way to customize it. So on
	// appengine opts.Context must be set and it must be derived from GAE request
	// context. opts.Transport is not used for token exchange flow itself, only
	// for authorized requests.
	switch opts.Method {
	case UserCredentialsMethod:
		return internal.NewUserAuthTokenProvider(
			opts.Context,
			opts.Logger,
			opts.ClientID,
			opts.ClientSecret,
			opts.Scopes)
	case ServiceAccountMethod:
		keyBody := opts.ServiceAccountJSON
		if len(keyBody) == 0 {
			serviceAccountPath := opts.ServiceAccountJSONPath
			if serviceAccountPath == "" {
				serviceAccountPath = defaultServiceAccountPath()
			}
			var err error
			if keyBody, err = ioutil.ReadFile(serviceAccountPath); err != nil {
				return nil, err
			}
		}
		return internal.NewServiceAccountTokenProvider(opts.Context, keyBody, opts.Scopes)
	case GCEMetadataMethod:
		return internal.NewGCETokenProvider(opts.GCEAccountName, opts.Scopes)
	case CustomMethod:
		if opts.CustomTokenMinter == nil {
			return nil, fmt.Errorf("CustomTokenMinter must be set")
		}
		return &customTokenProvider{
			c:      opts.Context,
			scopes: opts.Scopes,
			minter: opts.CustomTokenMinter,
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized authentication method: %s", opts.Method)
	}
}

// DefaultClient returns OAuth client_id and client_secret to use for 3 legged
// OAuth flow. Note that client_secret is not really a secret since it's
// hardcoded into the source code (and binaries). It's totally fine, as long
// as it's callback URI is configured to be 'localhost'. If someone decides to
// reuse such client_secret they have to run something on user's local machine
// to get the refresh_token.
func DefaultClient() (clientID string, clientSecret string) {
	clientID = "446450136466-2hr92jrq8e6i4tnsa56b52vacp7t3936.apps.googleusercontent.com"
	clientSecret = "uBfbay2KCy9t4QveJ-dOqHtp"
	return
}

// SecretsDir returns an absolute path to a directory to keep secret files in.
func SecretsDir() string {
	return secretsDir()
}

////////////////////////////////////////////////////////////////////////////////
// CustomMethod implementation.

// customToken adapts Token to internal.Token interface.
type customToken struct {
	Token
}

func (t *customToken) Expiry() time.Time {
	return t.Token.Expiry
}

func (t *customToken) AccessToken() string {
	return t.Token.AccessToken
}

func (t *customToken) Equals(another internal.Token) bool {
	if another == nil {
		return t == nil
	}
	casted, ok := another.(*customToken)
	if !ok {
		return false
	}
	return t.Token == casted.Token
}

// customTokenProvider implements internal.TokenProvider interface on top of
// TokenMinter.
type customTokenProvider struct {
	c      context.Context
	scopes []string
	minter TokenMinter
}

func (p *customTokenProvider) RequiresInteraction() bool {
	return false
}

func (p *customTokenProvider) CacheSeed() []byte {
	return p.minter.CacheSeed()
}

func (p *customTokenProvider) MintToken() (internal.Token, error) {
	tok, err := p.minter.MintToken(p.scopes)
	if err != nil {
		return nil, err
	}
	return &customToken{tok}, nil
}

func (p *customTokenProvider) RefreshToken(internal.Token) (internal.Token, error) {
	// Refreshing is the same as making a new one.
	return p.MintToken()
}

func (p *customTokenProvider) MarshalToken(t internal.Token) ([]byte, error) {
	casted, ok := t.(*customToken)
	if !ok {
		return nil, fmt.Errorf("wrong token kind: %T", t)
	}
	return json.Marshal(&casted.Token)
}

func (p *customTokenProvider) UnmarshalToken(data []byte) (internal.Token, error) {
	tok := Token{}
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &customToken{tok}, nil
}
