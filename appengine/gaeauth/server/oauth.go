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
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"golang.org/x/oauth2"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/user"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
)

var errBadAuthHeader = errors.New("oauth: bad Authorization header")

// EmailScope is a scope used to identifies user's email. Present in most tokens
// by default. Can be used as a base scope for authentication.
const EmailScope = "https://www.googleapis.com/auth/userinfo.email"

// OAuth2Method implements auth.Method on top of GAE OAuth2 API. It doesn't
// implement auth.UsersAPI.
type OAuth2Method struct {
	// Scopes is a list of OAuth scopes to check when authenticating the token.
	Scopes []string
}

var _ auth.UserCredentialsGetter = (*OAuth2Method)(nil)

// Authenticate extracts peer's identity from the incoming request.
func (m *OAuth2Method) Authenticate(c context.Context, r *http.Request) (*auth.User, error) {
	if info.IsDevAppServer(c) {
		// On "dev_appserver", we verify OAuth2 tokens using Google's OAuth2
		// verification endpoint.
		//
		// It is slow as hell on "dev_appserver", but good enough for local manual
		// testing.
		devMethod := auth.GoogleOAuth2Method{
			Scopes: m.Scopes,
		}
		return devMethod.Authenticate(c, r)
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

// GetUserCredentials implements auth.UserCredentialsGetter.
func (m *OAuth2Method) GetUserCredentials(c context.Context, r *http.Request) (*oauth2.Token, error) {
	chunks := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(chunks) != 2 || (chunks[0] != "OAuth" && chunks[0] != "Bearer") {
		return nil, errBadAuthHeader
	}
	return &oauth2.Token{
		AccessToken: chunks[1],
		TokenType:   "Bearer",
	}, nil
}
