// Copyright 2023 The LUCI Authors.
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

package encryptedcookies

import (
	"encoding/json"
	"net/http"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

// AuthState defines a JSON structure of "/auth/openid/state" response.
//
// It represents the state of the authentication session based on the session
// cookie in the request headers.
//
// "/auth/openid/state" is intended to be called via a same origin URL fetch
// request by the frontend code that needs an OAuth access token or an ID token
// representing the signed in user.
//
// If there's a valid authentication cookie, the state endpoint replies with
// HTTP 200 status code and the JSON-serialized AuthState struct with state
// details. The handler refreshes access and ID tokens if they expire soon.
//
// If there is no authentication cookie or it has expired or was revoked, the
// state endpoint still replies with HTTP 200 code and the JSON-serialized
// AuthState struct, except its `identity` field is `anonymous:anonymous` and
// no other fields are populated.
//
// On errors the state endpoint replies with a non-200 HTTP status code with a
// `plain/text` body containing the error message. This is an exceptional
// situation (usually internal transient errors caused by the session store
// unavailability or some misconfiguration in code).
//
// The state endpoint is disabled by default, see ExposeStateEndpoint field in
// AuthMethod.
type AuthState struct {
	// Identity is a LUCI identity string of the user or `anonymous:anonymous` if
	// the user is not logged in.
	Identity string `json:"identity"`

	// Email is the email of the user account if the user is logged in.
	Email string `json:"email,omitempty"`

	// Picture is the https URL of the user profile picture if available.
	Picture string `json:"picture,omitempty"`

	// AccessToken is an OAuth access token of the logged in user.
	//
	// See RequiredScopes and OptionalScopes in AuthMethod for what scopes this
	// token can have.
	AccessToken string `json:"accessToken,omitempty"`

	// AccessTokenExpiry is an absolute expiration time (as a unix timestamp) of
	// the access token.
	//
	// It is at least 10 min in the future.
	AccessTokenExpiry int64 `json:"accessTokenExpiry,omitempty"`

	// AccessTokenExpiresIn is approximately how long the access token will be
	// valid since when the response was generated, in seconds.
	//
	// It is at least 600 sec.
	AccessTokenExpiresIn int32 `json:"accessTokenExpiresIn,omitempty"`

	// IDToken is an identity token of the logged in user.
	//
	// Its `aud` claim is equal to ClientID in OpenIDConfig passed to AuthMethod.
	IDToken string `json:"idToken,omitempty"`

	// IDTokenExpiry is an absolute expiration time (as a unix timestamp) of
	// the identity token.
	//
	// It is at least 10 min in the future.
	IDTokenExpiry int64 `json:"idTokenExpiry,omitempty"`

	// IDTokenExpiresIn is approximately how long the identity token will be
	// valid since when the response was generated, in seconds.
	//
	// It is at least 600 sec.
	IDTokenExpiresIn int32 `json:"idTokenExpiresIn,omitempty"`
}

// stateHandler serves JSON with the session state, see AuthState.
func stateHandlerImpl(ctx *router.Context, sessionCheck func(auth.Session) bool) {
	// Only allow the user to directly navigate to this page (for testing),
	// i.e. "none", or a same-origin request, i.e. "same-origin".
	// The user's OAuth token should never be returned in a cross-origin
	// request.
	fetchSite := ctx.Request.Header.Get("Sec-Fetch-Site")
	if fetchSite != "none" && fetchSite != "same-origin" {
		http.Error(ctx.Writer, "Request must be a same-origin request.", http.StatusForbidden)
		return
	}

	state := auth.GetState(ctx.Request.Context())
	user := state.User()

	reply := AuthState{
		Identity: string(user.Identity),
		Email:    user.Email,
		Picture:  user.Picture,
	}

	replyErr := func(msg, details string) {
		http.Error(ctx.Writer, msg, http.StatusInternalServerError)
		logging.Errorf(ctx.Request.Context(), "%s: %s", msg, details)
	}

	if user.Identity != identity.AnonymousIdentity {
		// If the request was authenticated via encrypted cookies, the session must
		// be present.
		session := state.Session()
		if session == nil {
			replyErr("The auth session is unexpectedly missing", "this is likely misconfiguration of the authentication middleware chain")
			return
		}
		// Sanity check we authenticated the request using the correct method and
		// not some other mechanism that populates state.Session().
		if !sessionCheck(session) {
			replyErr("Unexpected auth session type", "this is likely misconfiguration of the authentication middleware chain")
			return
		}
		accessToken, err := session.AccessToken(ctx.Request.Context())
		if err != nil {
			replyErr("Failure getting access token", err.Error())
			return
		}
		idToken, err := session.IDToken(ctx.Request.Context())
		if err != nil {
			replyErr("Failure getting ID token", err.Error())
			return
		}
		now := clock.Now(ctx.Request.Context())
		reply.AccessToken = accessToken.AccessToken
		reply.AccessTokenExpiry = accessToken.Expiry.Unix()
		reply.AccessTokenExpiresIn = int32(accessToken.Expiry.Sub(now).Seconds())
		reply.IDToken = idToken.AccessToken
		reply.IDTokenExpiry = idToken.Expiry.Unix()
		reply.IDTokenExpiresIn = int32(idToken.Expiry.Sub(now).Seconds())
	}

	bytes, err := json.MarshalIndent(&reply, "", "  ")
	if err != nil {
		replyErr("Error JSON-marshaling response", err.Error())
		return
	}
	ctx.Writer.Header().Add("Content-Type", "application/json")
	if _, err := ctx.Writer.Write(bytes); err != nil {
		logging.Errorf(ctx.Request.Context(), "Writing JSON response: %s", err)
	}
}
