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

// stateHandler serves JSON with the session state, see StateEndpointResponse.
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

	reply := auth.StateEndpointResponse{
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
