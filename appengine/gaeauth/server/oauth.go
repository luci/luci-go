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

package server

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/urlfetch"
	"go.chromium.org/gae/service/user"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/googleoauth"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/mutexpool"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/identity"
	"go.chromium.org/luci/server/caching"
)

// EmailScope is a scope used to identifies user's email. Present in most tokens
// by default. Can be used as a base scope for authentication.
const EmailScope = "https://www.googleapis.com/auth/userinfo.email"

// UserOAuth2Method implements auth.Method on top of GAE OAuth2 API. It doesn't
// implement auth.UsersAPI.
type UserOAuth2Method struct {
	// Scopes is a list of OAuth scopes to check when authenticating the token.
	Scopes []string

	initDevOnce sync.Once
	devMethod   GoogleOAuth2Method
}

// Authenticate extracts peer's identity from the incoming request.
func (m *UserOAuth2Method) Authenticate(c context.Context, r *http.Request) (*auth.User, error) {
	if info.IsDevAppServer(c) {
		m.initDevOnce.Do(func() {
			m.devMethod.Scopes = m.Scopes
			m.devMethod.ClientFunc = func(c context.Context) (*http.Client, error) {
				return &http.Client{Transport: urlfetch.Get(c)}, nil
			}
		})
		return m.devMethod.Authenticate(c, r)
	}

	header := r.Header.Get("Authorization")
	if header == "" || len(m.Scopes) == 0 {
		return nil, nil // this method is not applicable
	}

	// GetOAuthUser RPC is notoriously flaky. Do a bunch of retries on errors.
	var err error
	for attemp := 0; attemp < 4; attemp++ {
		var u *user.User
		u, err = user.CurrentOAuth(c, m.Scopes...)
		if err != nil {
			logging.Warningf(c, "oauth: failed to execute GetOAuthUser - %s", err)
			continue
		}
		if u == nil {
			return nil, nil
		}
		if u.ClientID == "" {
			return nil, fmt.Errorf("oauth: ClientID is unexpectedly empty")
		}
		id, idErr := identity.MakeIdentity("user:" + u.Email)
		if idErr != nil {
			return nil, idErr
		}
		return &auth.User{
			Identity:  id,
			Superuser: u.Admin,
			Email:     u.Email,
			ClientID:  u.ClientID,
		}, nil
	}
	return nil, transient.Tag.Apply(err)
}

// GoogleOAuth2Method implements auth.Method on top of Google's OAuth2
// endpoint.
//
// It is useful for development verification (e.g., "dev_appserver") or
// verification in environments without the User API (e.g., Flex).
type GoogleOAuth2Method struct {
	// Scopes is a list of OAuth scopes to check when authenticating the token.
	Scopes []string

	// Client returns the HTTP client to use. If nil, the default HTTP client will
	// be used.
	ClientFunc func(context.Context) (*http.Client, error)

	// mp is a mutex pool that locks around a given token to ensure that
	// multiple concurrent accesses to the same token collapse into a single
	// access. The rest will pull from cache.
	mp mutexpool.P

	// tokenInfoEndpoint is used in unit test to mock production endpoint.
	tokenInfoEndpoint string
}

type tokenCheckCache string

// Authenticate implements Method.
func (m *GoogleOAuth2Method) Authenticate(c context.Context, r *http.Request) (user *auth.User, err error) {
	// Extract the access token from the Authorization header.
	header := r.Header.Get("Authorization")
	if header == "" || len(m.Scopes) == 0 {
		return nil, nil // this method is not applicable
	}

	chunks := strings.SplitN(header, " ", 2)
	if len(chunks) != 2 || (chunks[0] != "OAuth" && chunks[0] != "Bearer") {
		return nil, errors.New("oauth: bad Authorization header")
	}
	accessToken := chunks[1]

	cacheKey := tokenCheckCache(accessToken)
	m.mp.WithMutex(cacheKey, func() {
		// Maybe already verified this token?
		pc := caching.ProcessCache(c)
		if pc != nil {
			if cachedUser, ok := pc.Get(c, cacheKey); ok {
				user = cachedUser.(*auth.User)
				return
			}
		}

		var exp time.Duration
		user, exp, err = m.authenticateAgainstGoogle(c, accessToken)
		if err != nil {
			return
		}

		// Cache the resulting token for next time.
		if pc != nil {
			pc.Put(c, cacheKey, user, exp)
		}
	})
	return
}

// authenticateAgainstGoogle uses OAuth2 tokeninfo endpoint via URL fetch.
//
// It is slow as hell, but good enough for local manual testing and really
// the only viable option for Flex environment. Therefore, it's important
// that we cache the results aggressively.
func (m *GoogleOAuth2Method) authenticateAgainstGoogle(c context.Context, accessToken string) (*auth.User, time.Duration, error) {
	var client *http.Client
	if m.ClientFunc != nil {
		var err error
		client, err = m.ClientFunc(c)
		if err != nil {
			return nil, 0, err
		}
	}

	// Fetch an info dict associated with the token.
	logging.Infof(c, "oauth: Querying tokeninfo endpoint")
	tokenInfo, err := googleoauth.GetTokenInfo(c, googleoauth.TokenInfoParams{
		AccessToken: accessToken,
		Client:      client,
		Endpoint:    m.tokenInfoEndpoint,
	})
	if err != nil {
		if err == googleoauth.ErrBadToken {
			return nil, 0, err
		}
		return nil, 0, errors.Annotate(err, "oauth: transient error when validating token").
			Tag(transient.Tag).Err()
	}

	// Verify the token contains a validated email.
	switch {
	case tokenInfo.Email == "":
		return nil, 0, fmt.Errorf("oauth: token is not associated with an email")
	case !tokenInfo.EmailVerified:
		return nil, 0, fmt.Errorf("oauth: email %s is not verified", tokenInfo.Email)
	}
	if tokenInfo.ExpiresIn <= 0 {
		return nil, 0, fmt.Errorf("oauth: 'expires_in' field is not a positive integer")
	}

	// Verify `scopes` is subset of tokenInfo.Scope.
	tokenScopes := map[string]bool{}
	for _, s := range strings.Split(tokenInfo.Scope, " ") {
		tokenScopes[s] = true
	}
	for _, s := range m.Scopes {
		if !tokenScopes[s] {
			return nil, 0, fmt.Errorf("oauth: token doesn't have scope %q", s)
		}
	}

	// Good enough.
	id, err := identity.MakeIdentity("user:" + tokenInfo.Email)
	if err != nil {
		return nil, 0, err
	}

	exp := time.Duration(tokenInfo.ExpiresIn) * time.Second
	u := &auth.User{
		Identity: id,
		Email:    tokenInfo.Email,
		ClientID: tokenInfo.Aud,
	}

	return u, exp, nil
}
