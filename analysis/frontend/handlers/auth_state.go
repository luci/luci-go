// Copyright 2022 The LUCI Authors.
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

package handlers

import (
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

type authState struct {
	// Identity string of the user (anonymous:anonymous if the user is not
	// logged in).
	Identity string `json:"identity"`
	// The email of the user. Optional, default "".
	Email string `json:"email"`
	// The URL of the user avatar. Optional, default "".
	Picture string `json:"picture"`
	// The token that authorizes and authenticates the requests.
	// Used for authenticating to many LUCI services, including Weetbix.
	AccessToken string `json:"accessToken"`
	// Expiration time (unix timestamp) of the access token.
	// If zero, the access token does not expire.
	AccessTokenExpiry int64 `json:"accessTokenExpiry"`
	// The OAuth ID token. Used for authenticating to Monorail.
	IDToken string `json:"idToken"`
	// Expiration time (unix timestamp) of the ID token.
	// If zero, the access token does not expire.
	IDTokenExpiry int64 `json:"idTokenExpiry"`
}

// GetAuthState returns data about the current user and the access token
// associated with the current session.
func (*Handlers) GetAuthState(ctx *router.Context) {
	fetchSite := ctx.Request.Header.Get("Sec-Fetch-Site")
	// Only allow the user to directly navigate to this page (for testing),
	// i.e. "none", or a same-origin request, i.e. "same-origin".
	// The user's OAuth token should never be returned in a cross-origin
	// request.
	if fetchSite != "none" && fetchSite != "same-origin" {
		http.Error(ctx.Writer, "Request must be a same-origin request.", http.StatusForbidden)
		return
	}

	user := auth.CurrentUser(ctx.Context)
	var state *authState
	if user.Identity == identity.AnonymousIdentity {
		state = &authState{
			Identity: string(user.Identity),
		}
	} else {
		session := auth.GetState(ctx.Context).Session()
		if session == nil {
			http.Error(ctx.Writer, "Request not authenticated via secure cookies.", http.StatusUnauthorized)
			return
		}

		accessToken, err := session.AccessToken(ctx.Context)
		if err != nil {
			logging.Errorf(ctx.Context, "Obtain access token: %s", err)
			http.Error(ctx.Writer, "Internal server error.", http.StatusInternalServerError)
			return
		}
		idToken, err := session.IDToken(ctx.Context)
		if err != nil {
			// If get here when running the server locally, it may mean you need to run
			// "luci-auth".
			logging.Errorf(ctx.Context, "Obtain ID token: %s", err)
			http.Error(ctx.Writer, "Internal server error.", http.StatusInternalServerError)
			return
		}
		state = &authState{
			Identity:          string(user.Identity),
			Email:             user.Email,
			Picture:           user.Picture,
			AccessToken:       accessToken.AccessToken,
			AccessTokenExpiry: accessToken.Expiry.Unix(),
			IDToken:           idToken.AccessToken,
			IDTokenExpiry:     idToken.Expiry.Unix(),
		}
	}

	respondWithJSON(ctx, state)
}
