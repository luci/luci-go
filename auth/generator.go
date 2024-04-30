// Copyright 2017 The LUCI Authors.
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
	"errors"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// TokenGenerator manages a collection of Authenticators to generate tokens with
// scopes or audiences not known in advance.
//
// It is primarily useful as a building block for `luci-auth context` mechanism.
type TokenGenerator struct {
	ctx                     context.Context
	opts                    Options
	allowsArbitraryScopes   bool
	allowsArbitraryAudience bool

	lock           sync.RWMutex
	authenticators map[string]*Authenticator
}

// NewTokenGenerator constructs a TokenGenerator that can generate tokens with
// an arbitrary set of scopes or audiences, if given options allow.
//
// It creates one or more Authenticator instances internally (per combination of
// requested parameters) using given `opts` as a basis for auth options.
//
// If given options allow minting tokens with arbitrary scopes, then opts.Scopes
// are ignored and they are instead substituted with the scopes requested in
// GenerateToken. This happens, for example, when `opts` indicate using service
// account keys, or IAM impersonation or LUCI context protocol. In all these
// cases the credentials are "powerful enough" to generate tokens with arbitrary
// scopes.
//
// If given options do not allow changing scopes (e.g. they are backed by an
// OAuth2 refresh token that has fixed scopes), then GenerateToken will return
// tokens with such fixed scopes regardless of what scopes are requested. This
// is still useful sometimes, since some fixed scopes can actually cover a lot
// of other scopes (e.g. "cloud-platform" scopes covers a ton of more fine grain
// Cloud scopes).
func NewTokenGenerator(ctx context.Context, opts Options) *TokenGenerator {
	if opts.Method == AutoSelectMethod {
		opts.Method = SelectBestMethod(ctx, opts)
	}
	opts.UseIDTokens = false // to preserve opts.Scopes in PopulateDefaults
	opts.PopulateDefaults()
	return &TokenGenerator{
		ctx:                     ctx,
		opts:                    opts,
		allowsArbitraryScopes:   allowsArbitraryScopes(&opts),
		allowsArbitraryAudience: allowsArbitraryAudience(&opts),
		authenticators:          map[string]*Authenticator{},
	}
}

// allowsArbitraryScopes returns true if given authenticator options allow
// generating tokens for arbitrary set of scopes.
//
// For example, using a private key to sign assertions allows to mint tokens
// for any set of scopes (since there's no restriction on what scopes we can
// put into JWT to be signed).
func allowsArbitraryScopes(opts *Options) bool {
	switch {
	case opts.Method == ServiceAccountMethod:
		// A private key can be used to generate tokens with any combination of
		// scopes.
		return true
	case opts.Method == GCEMetadataMethod && opts.GCESupportsArbitraryScopes:
		// We can ask the metadata server for any combination of scopes.
		return true
	case opts.Method == LUCIContextMethod:
		// We can ask the local auth server for any combination of scopes.
		return true
	case opts.ActAsServiceAccount != "":
		// When using derived tokens the authenticator can ask the corresponding API
		// (Cloud IAM's generateAccessToken or LUCI's MintServiceAccountToken) for
		// any scopes it wants.
		return true
	}
	return false
}

// allowsArbitraryAudience returns true if given authenticator options allow
// generating ID tokens for arbitrary audience.
func allowsArbitraryAudience(opts *Options) bool {
	// Only UserCredentialsMethod hardcodes audience in the token based on the
	// OAuth2 client ID used to generate it. Additionally, when we use an
	// impersonation API, we can request any audience we want, even when using
	// UserCredentialsMethod to authenticate calls to this impersonation API.
	return opts.Method != UserCredentialsMethod || opts.ActAsServiceAccount != ""
}

// Options returns the base options used to construct new authenticators.
//
// They are normalized with all defaults filled in.
func (g *TokenGenerator) Options() Options {
	return g.opts
}

// Authenticator returns an authenticator that uses access or ID tokens.
//
// Either `scopes` or `audience` (but not both) should be given. If `scopes`
// are given, the returned authenticator will use access tokens with these
// scopes. If `audience` is given, the returned authenticator will use ID tokens
// with this audience.
//
// The returned authenticator uses the context passed to NewTokenGenerator for
// logging.
func (g *TokenGenerator) Authenticator(scopes []string, audience string) (*Authenticator, error) {
	var cacheKey string

	switch {
	case len(scopes) != 0 && audience != "":
		return nil, errors.New("either scopes or audience should be given, not both")

	case len(scopes) != 0:
		// For auth methods that don't allow changing scopes, just use the
		// predefined ones and hope they are sufficient for the caller.
		if !g.allowsArbitraryScopes {
			scopes = g.opts.Scopes
		} else {
			scopes = normalizeScopes(scopes)
		}
		cacheKey = "oauth\x00" + strings.Join(scopes, "\x00")

	case audience != "":
		// For auth methods that don't allow changing audience, just use the
		// predefined one and hope it is sufficient for the caller.
		if !g.allowsArbitraryAudience {
			audience = g.opts.Audience
		}
		cacheKey = "oidc\x00" + audience

	default:
		return nil, errors.New("no scopes or audience are given")
	}

	g.lock.RLock()
	authenticator := g.authenticators[cacheKey]
	g.lock.RUnlock()

	if authenticator == nil {
		g.lock.Lock()
		defer g.lock.Unlock()
		authenticator = g.authenticators[cacheKey]
		if authenticator == nil {
			opts := g.opts
			opts.UseIDTokens = len(scopes) == 0
			opts.Scopes = scopes
			opts.Audience = audience
			authenticator = NewAuthenticator(g.ctx, SilentLogin, opts)
			g.authenticators[cacheKey] = authenticator
		}
	}

	return authenticator, nil
}

// GenerateOAuthToken returns an access token for a combination of scopes.
//
// The returned token lives for at least given `lifetime` duration, but it may
// live longer. If `lifetime` is zero, a default lifetime from auth.Options will
// be used.
func (g *TokenGenerator) GenerateOAuthToken(_ context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	if len(scopes) == 0 {
		return nil, errors.New("got empty list of OAuth scopes")
	}
	a, err := g.Authenticator(scopes, "")
	if err != nil {
		return nil, err
	}
	if lifetime == 0 {
		lifetime = g.opts.MinTokenLifetime
	}
	return a.GetAccessToken(lifetime)
}

// GenerateIDToken returns an ID token with the given audience in `aud` claim.
//
// The returned token lives for at least given `lifetime` duration, but it may
// live longer. If `lifetime` is zero, a default lifetime from auth.Options will
// be used.
func (g *TokenGenerator) GenerateIDToken(_ context.Context, audience string, lifetime time.Duration) (*oauth2.Token, error) {
	if audience == "" {
		return nil, errors.New("got empty ID token audience")
	}
	a, err := g.Authenticator(nil, audience)
	if err != nil {
		return nil, err
	}
	if lifetime == 0 {
		lifetime = g.opts.MinTokenLifetime
	}
	return a.GetAccessToken(lifetime)
}

// GetEmail returns an email associated with all tokens produced by this
// generator or ErrNoEmail if it's not available.
func (g *TokenGenerator) GetEmail() (string, error) {
	// First try to fish out the email from existing cached authenticators.
	var email string
	g.lock.RLock()
	for _, a := range g.authenticators {
		if email, _ = a.GetEmail(); email != "" {
			break
		}
	}
	g.lock.RUnlock()

	if email != "" {
		return email, nil
	}

	// Give up and construct a new authenticator just to get the email.
	a, err := g.Authenticator(g.opts.Scopes, "")
	if err != nil {
		return "", err
	}
	return a.GetEmail()
}
