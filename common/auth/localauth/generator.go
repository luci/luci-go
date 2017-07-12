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
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"github.com/luci/luci-go/common/auth"
)

// NewFlexibleGenerator constructs TokenGenerator that can generate tokens with
// arbitrary set of scopes, if given options allow.
//
// It creates one or more auth.Authenticator instances internally (per
// combination of requested scopes) using given 'opts' as a basis for options.
//
// Works only for options for which auth.AllowsArbitraryScopes(...) is true.
// In this case options point to a service account private key or something
// equivalent.
//
// The token generator will produce tokens that have exactly requested scopes.
// Value of opts.Scopes is ignored.
func NewFlexibleGenerator(ctx context.Context, opts auth.Options) (TokenGenerator, error) {
	if !auth.AllowsArbitraryScopes(ctx, opts) {
		return nil, fmt.Errorf("can't use given auth.Options to mint token with arbitrary scopes")
	}

	lock := sync.RWMutex{}
	authenticators := map[string]*auth.Authenticator{}

	return func(_ context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
		// We use '\n' as separator. It should not appear in the scopes.
		for _, s := range scopes {
			if strings.ContainsRune(s, '\n') {
				return nil, fmt.Errorf("bad scope: %q", s)
			}
		}
		// Note: scopes are already sorted per Server{...} contract.
		cacheKey := strings.Join(scopes, "\n")

		// Grab an authenticator for the requested set of scopes or create new one.
		lock.RLock()
		authenticator := authenticators[cacheKey]
		lock.RUnlock()
		if authenticator == nil {
			lock.Lock()
			authenticator = authenticators[cacheKey]
			if authenticator == nil {
				opts := opts
				opts.Scopes = scopes
				authenticator = auth.NewAuthenticator(ctx, auth.SilentLogin, opts)
				authenticators[cacheKey] = authenticator
			}
			lock.Unlock()
		}

		return authenticator.GetAccessToken(lifetime)
	}, nil
}

// NewRigidGenerator constructs TokenGenerator that always uses given
// authenticator to generate tokens, regardless of requested scopes.
//
// It is suitable for auth methods that rely on existing pre-configured
// credentials or state to generate tokens with some predefined set of scopes.
//
// For example, this is the case for UserCredentialsMethod (where the user has
// to go through a login flow to get a refresh token) or for GCEMetadataMethod
// (where GCE instance has to be launched with predefined list of scopes).
//
// The token generator will return the access token with scopes it can give,
// regardless of requested scopes. Also this token will always have all scopes
// given to it by Authenticator.
//
// Note that we can't even compare the requested scopes to the scopes provided
// by the authenticator, because some Google scopes are aliases for a large set
// of scopes. For example, a token with 'cloud-platform' scope can be used in
// APIs that expect 'iam' scope. So the generator will just give the token it
// has, hoping for the best. If the token is not sufficient, callers will get
// HTTP 403 or HTTP 401 errors when using it.
func NewRigidGenerator(ctx context.Context, authenticator *auth.Authenticator) (TokenGenerator, error) {
	// Bail early if we detect there are no cached refresh token for the requested
	// combination of scopes.
	if err := authenticator.CheckLoginRequired(); err != nil {
		return nil, err
	}
	return func(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
		return authenticator.GetAccessToken(lifetime)
	}, nil
}
