// Copyright 2014 The Chromium Authors. All rights reserved.
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
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"golang.org/x/net/context"

	"infra/libs/auth/internal"
	"infra/libs/build"
	"infra/libs/gce"
	"infra/libs/logging"
)

var (
	// ErrLoginRequired is returned by Transport() in case long term credentials
	// are not cached and the user must go through interactive login.
	ErrLoginRequired = errors.New("Interactive login is required")

	// ErrInsufficientAccess is returned by Login() or Transport() if access_token
	// can't be minted for given OAuth scopes. For example if GCE instance wasn't
	// granted access to requested scopes when it was created.
	ErrInsufficientAccess = internal.ErrInsufficientAccess

	// ErrNoTerminal is returned by Login() if interaction with a user is
	// required, but the process is not attached to a terminal.
	ErrNoTerminal = errors.New("Can't interact with a user: no terminal")
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
)

// LoginMode is used as enum in AuthenticatedClient function.
type LoginMode string

const (
	// InteractiveLogin is passed to AuthenticatedClient to forcefully rerun full
	// login flow and cache resulting tokens. Used by 'login' CLI command.
	InteractiveLogin LoginMode = "InteractiveLogin"

	// SilentLogin is passed to AuthenticatedClient if authentication must be used
	// and it is NOT OK to run interactive login flow to get the tokens. The call
	// will fail with ErrLoginRequired error if there's no cached tokens. Should
	// normally be used by all CLI tools that need to use authentication.
	SilentLogin LoginMode = "SilentLogin"

	// OptionalLogin is passed to AuthenticatedClient if it is OK not to use
	// authentication if there are no cached credentials. Interactive login will
	// never be called, default unauthenticated client will be returned instead.
	// Should be used by CLI tools where authentication is optional.
	OptionalLogin LoginMode = "OptionalLogin"
)

// Options are used by NewAuthenticator call. All fields are optional and have
// sane default values.
type Options struct {
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

	// GCEAccountName is an account name to query to fetch token for from metadata
	// server when GCEMetadataMethod is used. If given account wasn't granted
	// required set of scopes during instance creation time, Transport() call
	// fails with ErrInsufficientAccess.
	// Default: "default" account.
	GCEAccountName string

	// Context carries the underlying HTTP transport to use. If context is not
	// provided or doesn't contain the transport, http.DefaultTransport will be
	// used.
	Context context.Context

	// Log is logger to use, defaults to logging.DefaultLogger.
	Log logging.Logger
}

// Authenticator is a factory for http.RoundTripper objects that know how to use
// cached OAuth credentials. Authenticator also knows how to run interactive
// login flow, if required.
type Authenticator interface {
	// Transport returns http.RoundTripper that adds authentication details to
	// each request. An interactive authentication flow (if required) must be
	// complete before making a transport, otherwise ErrLoginRequired is returned.
	// Returned transport object can be safely reused across many http.Client's.
	Transport() (http.RoundTripper, error)

	// Login perform an interaction with the user to get a long term refresh token
	// and cache it. Blocks for user input, can use stdin. Returns ErrNoTerminal
	// if interaction with a user is required, but the process is not running
	// under a terminal. It overwrites currently cached credentials, if any.
	Login() error

	// PurgeCredentialsCache removes cached tokens.
	PurgeCredentialsCache() error
}

// DefaultAuthenticator is a shared Authenticator built with default options.
var DefaultAuthenticator Authenticator

func init() {
	DefaultAuthenticator = NewAuthenticator(Options{})
}

// NewAuthenticator returns a new instance of Authenticator given its options.
func NewAuthenticator(opts Options) Authenticator {
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
	if opts.ServiceAccountJSONPath == "" {
		opts.ServiceAccountJSONPath = filepath.Join(SecretsDir(), "service_account.json")
	}
	if opts.GCEAccountName == "" {
		opts.GCEAccountName = "default"
	}
	if opts.Context == nil {
		opts.Context = context.TODO()
	}
	if opts.Log == nil {
		opts.Log = logging.DefaultLogger
	}

	// See ensureInitialized for the rest of the initialization.
	auth := &authenticatorImpl{opts: &opts, log: opts.Log}
	auth.transport = &authTransport{
		parent: auth,
		base:   internal.TransportFromContext(opts.Context),
		log:    opts.Log,
	}
	return auth
}

// PurgeCredentialsCache deletes all cached credentials from the disk.
func PurgeCredentialsCache() error {
	log := logging.DefaultLogger
	dir := SecretsDir()
	secrets, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		log.Warningf("auth: failed to enumerate %s to purge credentials: %s", dir, err)
		return err
	}
	someFailed := false
	for _, info := range secrets {
		if info.Mode().IsRegular() && strings.HasSuffix(info.Name(), ".tok") {
			log.Debugf("auth: clearing cached token: %s", info.Name())
			err = os.Remove(filepath.Join(dir, info.Name()))
			if err != nil {
				someFailed = true
				log.Debugf("auth: failed to remove cached token %s: %s", info.Name(), err)
			}
		}
	}
	if someFailed {
		return fmt.Errorf("auth: not all cached tokens were purged")
	}
	return nil
}

// AuthenticatedClient performs login (if requested) and returns http.Client.
// See documentation for 'mode' for more details.
func AuthenticatedClient(mode LoginMode, auth Authenticator) (*http.Client, error) {
	if mode == InteractiveLogin {
		if err := auth.PurgeCredentialsCache(); err != nil {
			return nil, err
		}
	}
	transport, err := auth.Transport()
	if err == nil {
		return &http.Client{Transport: transport}, nil
	}
	if err != ErrLoginRequired || mode == SilentLogin {
		return nil, err
	}
	if mode == OptionalLogin {
		return http.DefaultClient, nil
	}
	if mode != InteractiveLogin {
		return nil, fmt.Errorf("Invalid mode argument: %s", mode)
	}
	if err = auth.Login(); err != nil {
		return nil, err
	}
	if transport, err = auth.Transport(); err != nil {
		return nil, err
	}
	return &http.Client{Transport: transport}, nil
}

// DefaultAuthenticatedClient performs login (if requested) and returns
// http.Client. It uses DefaultAuthenticator.
func DefaultAuthenticatedClient(mode LoginMode) (*http.Client, error) {
	return AuthenticatedClient(mode, DefaultAuthenticator)
}

////////////////////////////////////////////////////////////////////////////////
// Authenticator implementation.

type authenticatorImpl struct {
	// Immutable members.
	opts      *Options
	transport http.RoundTripper
	log       logging.Logger

	// Mutable members.
	lock     sync.Mutex
	cache    *tokenCache
	provider internal.TokenProvider
	err      error
	token    internal.Token
}

func (a *authenticatorImpl) Transport() (http.RoundTripper, error) {
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

func (a *authenticatorImpl) Login() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	err := a.ensureInitialized()
	if err != nil {
		return err
	}
	if !a.provider.RequiresInteraction() {
		a.log.Debugf("auth: no login required")
		return nil
	}

	// Active terminal is required for interaction with a user.
	if !logging.IsTerminal {
		return ErrNoTerminal
	}

	// Create initial token. This may require interaction with a user.
	a.log.Debugf("auth: launching interactive authentication flow")
	a.token, err = a.provider.MintToken()
	if err != nil {
		return err
	}

	// Store the initial token in the cache. Don't abort if it fails, the token
	// is still usable from the memory.
	if err = a.cacheToken(a.token); err != nil {
		a.log.Warningf("auth: failed to write token to cache: %v", err)
	}

	return nil
}

func (a *authenticatorImpl) PurgeCredentialsCache() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if err := a.ensureInitialized(); err != nil {
		return err
	}
	if err := a.cache.clear(); err != nil {
		return err
	}
	a.token = nil
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// Authenticator private methods.

// ensureInitialized is supposed to be called under the lock.
func (a *authenticatorImpl) ensureInitialized() error {
	if a.err != nil || a.provider != nil {
		return a.err
	}

	// selectDefaultMethod may do heavy calls, call it lazily here rather than in
	// NewAuthenticator. NewAuthenticator is called from init(), no need to delay
	// process startup.
	if a.opts.Method == AutoSelectMethod {
		a.opts.Method = selectDefaultMethod(a.opts)
	}
	a.log.Debugf("auth: using %s", a.opts.Method)
	a.provider, a.err = makeTokenProvider(a.opts)
	if a.err != nil {
		return a.err
	}

	// Setup the cache only when Method is known, cache filename depends on it.
	a.cache = &tokenCache{
		path: filepath.Join(SecretsDir(), cacheFileName(a.opts)+".tok"),
		log:  a.log,
	}

	// Broken token cache is not a fatal error. So just log it and forget, a new
	// token will be minted.
	var err error
	a.token, err = a.readTokenCache()
	if err != nil {
		a.log.Warningf("auth: failed to read token from cache: %v", err)
	}
	return nil
}

// readTokenCache may be called with a.lock held or not held. It works either way.
func (a *authenticatorImpl) readTokenCache() (internal.Token, error) {
	// 'read' returns (nil, nil) if cache is empty.
	buf, err := a.cache.read()
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
func (a *authenticatorImpl) cacheToken(tok internal.Token) error {
	buf, err := a.provider.MarshalToken(tok)
	if err != nil {
		return err
	}
	return a.cache.write(buf)
}

// currentToken lock a.lock inside. It MUST NOT be called when a.lock is held.
func (a *authenticatorImpl) currentToken() internal.Token {
	// TODO(vadimsh): Test with go test -race. The lock may be unnecessary.
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.token
}

// refreshToken compares current token to 'prev' and launches token refresh
// procedure if they still match. Returns a refreshed token (if a refresh
// procedure happened) or the current token (i.e. if it's different from prev).
// Acts as "Compare-And-Swap" where "Swap" is a token refresh procedure.
func (a *authenticatorImpl) refreshToken(prev internal.Token) (internal.Token, error) {
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
		if err == nil && cached != nil && !cached.Equals(prev) && !cached.Expired() {
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
			a.log.Debugf("auth: minting a new token")
			a.token, err = a.provider.MintToken()
			if err != nil {
				a.log.Warningf("auth: failed to mint a token: %v", err)
				return nil, false, err
			}
		} else {
			a.log.Debugf("auth: refreshing the token")
			a.token, err = a.provider.RefreshToken(a.token)
			if err != nil {
				a.log.Warningf("auth: failed to refresh the token: %v", err)
				return nil, false, err
			}
		}
		return a.token, true, nil
	}()

	if err != nil {
		return nil, err
	}

	// Store the new token in the cache outside the lock, no need for callers to
	// wait for this. Do not die if failed, token is still usable from the memory.
	if cache {
		if err = a.cacheToken(tok); err != nil {
			a.log.Warningf("auth: failed to write refreshed token to the cache: %v", err)
		}
	}
	return tok, nil
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
	parent *authenticatorImpl
	base   http.RoundTripper
	log    logging.Logger
}

// RoundTrip appends authorization details to the request.
func (t *authTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	tok := t.parent.currentToken()
	if tok == nil || tok.Expired() {
		tok, err = t.parent.refreshToken(tok)
		if err != nil {
			return
		}
		if tok == nil || tok.Expired() {
			err = fmt.Errorf("auth: failed to refresh the token")
			return
		}
	}
	clone := *req
	clone.Header = make(http.Header)
	for k, v := range req.Header {
		clone.Header[k] = v
	}
	for k, v := range tok.RequestHeaders() {
		clone.Header.Set(k, v)
	}
	return t.base.RoundTrip(&clone)
}

////////////////////////////////////////////////////////////////////////////////
// tokenCache implementation.

type tokenCache struct {
	path string
	log  logging.Logger
	lock sync.Mutex
}

func (c *tokenCache) read() (buf []byte, err error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.log.Debugf("auth: reading token from %s", c.path)
	buf, err = ioutil.ReadFile(c.path)
	if err != nil && os.IsNotExist(err) {
		err = nil
	}
	return
}

func (c *tokenCache) write(buf []byte) error {
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

func (c *tokenCache) clear() error {
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

func cacheFileName(opts *Options) string {
	// Construct a name of cache file from data that identifies requested
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
	return hex.EncodeToString(sum.Sum(nil))[:16]
}

// selectDefaultMethod is mocked in tests.
var selectDefaultMethod = func(opts *Options) Method {
	if opts.ServiceAccountJSONPath != "" {
		info, _ := os.Stat(opts.ServiceAccountJSONPath)
		if info != nil && info.Mode().IsRegular() {
			return ServiceAccountMethod
		}
	}
	if gce.IsRunningOnGCE() {
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
	switch opts.Method {
	case UserCredentialsMethod:
		return internal.NewUserAuthTokenProvider(
			opts.Context,
			opts.ClientID,
			opts.ClientSecret,
			opts.Scopes)
	case ServiceAccountMethod:
		return internal.NewServiceAccountTokenProvider(
			opts.Context,
			opts.ServiceAccountJSONPath,
			opts.Scopes)
	case GCEMetadataMethod:
		return internal.NewGCETokenProvider(
			opts.GCEAccountName,
			opts.Scopes)
	default:
		return nil, fmt.Errorf("Unrecognized authentication method: %s", opts.Method)
	}
}

// DefaultClient returns OAuth client_id and client_secret to use for 3 legged
// OAuth flow. Note that client_secret is not really a secret since it's
// hardcoded into the source code (and binaries). It's totally fine, as long
// as it's callback URI is configured to be 'localhost'. If someone decides to
// reuse such client_secret they have to run something on user's local machine
// to get the refresh_token.
func DefaultClient() (clientID string, clientSecret string) {
	if build.ReleaseBuild {
		clientID = "446450136466-2hr92jrq8e6i4tnsa56b52vacp7t3936.apps.googleusercontent.com"
		clientSecret = "uBfbay2KCy9t4QveJ-dOqHtp"
	} else {
		clientID = "502071599212-1mogcu4ekrulor34tjtt6t8oq07ihmf9.apps.googleusercontent.com"
		clientSecret = "2wuVIjpMBVOaCKom9gZtopfZ"
	}
	return
}

// SecretsDir returns an absolute path to a directory to keep secret files in.
func SecretsDir() string {
	return secretsDir()
}
