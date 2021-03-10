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

package localauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth"
)

// NewTokenGenerator constructs a TokenGenerator that can generate tokens with
// an arbitrary set of scopes or audiences, if given options allow.
//
// It creates one or more auth.Authenticator instances internally (per
// combination of requested parameters) using given 'opts' as a basis for
// auth options.
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
func NewTokenGenerator(ctx context.Context, opts auth.Options) (TokenGenerator, error) {
	if opts.Method == auth.AutoSelectMethod {
		opts.Method = auth.SelectBestMethod(ctx, opts)
	}
	opts.UseIDTokens = false // to preserve opts.Scopes in PopulateDefaults
	opts.PopulateDefaults()
	return &generator{
		ctx:                     ctx,
		opts:                    opts,
		allowsArbitraryScopes:   allowsArbitraryScopes(&opts),
		allowsArbitraryAudience: allowsArbitraryAudience(&opts),
		authenticators:          map[string]*auth.Authenticator{},
	}, nil
}

// allowsArbitraryScopes returns true if given authenticator options allow
// generating tokens for arbitrary set of scopes.
//
// For example, using a private key to sign assertions allows to mint tokens
// for any set of scopes (since there's no restriction on what scopes we can
// put into JWT to be signed).
func allowsArbitraryScopes(opts *auth.Options) bool {
	switch {
	case opts.Method == auth.ServiceAccountMethod:
		// A private key can be used to generate tokens with any combination of
		// scopes.
		return true
	case opts.Method == auth.LUCIContextMethod:
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
func allowsArbitraryAudience(opts *auth.Options) bool {
	// Only UserCredentialsMethod hardcodes audience in the token based on the
	// OAuth2 client ID used to generate it. Additionally, when we use an
	// impersonation API, we can request any audience we want, even when using
	// UserCredentialsMethod to authenticate calls to this impersonation API.
	return opts.Method != auth.UserCredentialsMethod || opts.ActAsServiceAccount != ""
}

type generator struct {
	ctx                     context.Context
	opts                    auth.Options
	allowsArbitraryScopes   bool
	allowsArbitraryAudience bool

	lock           sync.RWMutex
	authenticators map[string]*auth.Authenticator
}

func (g *generator) authenticator(scopes []string, audience string) (*auth.Authenticator, error) {
	var cacheKey string

	switch {
	case len(scopes) != 0:
		// For auth methods that don't allow changing scopes, just use the predefined
		// ones and hope they are sufficient for the caller.
		if !g.allowsArbitraryScopes {
			scopes = g.opts.Scopes
		}
		// We use '\n' as separator. It should not appear in the scopes.
		for _, s := range scopes {
			if strings.ContainsRune(s, '\n') {
				return nil, fmt.Errorf("bad scope: %q", s)
			}
		}
		// Note: scopes are already sorted per Server{...} contract.
		cacheKey = "access_token\n" + strings.Join(scopes, "\n")
	case audience != "":
		// For auth methods that don't allow changing audience, just use the
		// predefined one and hope it is sufficient for the caller.
		if !g.allowsArbitraryAudience {
			audience = g.opts.Audience
		}
		cacheKey = "id_token\n" + audience
	default:
		// This should not be happening.
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
			authenticator = auth.NewAuthenticator(g.ctx, auth.SilentLogin, opts)
			g.authenticators[cacheKey] = authenticator
		}
	}

	return authenticator, nil
}

func (g *generator) GenerateOAuthToken(_ context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	if len(scopes) == 0 {
		return nil, errors.New("got empty list of OAuth scopes")
	}
	a, err := g.authenticator(scopes, "")
	if err != nil {
		return nil, err
	}
	return a.GetAccessToken(lifetime)
}

func (g *generator) GenerateIDToken(_ context.Context, audience string, lifetime time.Duration) (*oauth2.Token, error) {
	if audience == "" {
		return nil, errors.New("got empty ID token audience")
	}
	a, err := g.authenticator(nil, audience)
	if err != nil {
		return nil, err
	}
	return a.GetAccessToken(lifetime)
}

func (g *generator) GetEmail() (string, error) {
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
	a, err := g.authenticator([]string{auth.OAuthScopeEmail}, "")
	if err != nil {
		return "", err
	}
	return a.GetEmail()
}
