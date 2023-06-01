// Copyright 2021 The LUCI Authors.
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

package frontend

import (
	"encoding/json"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
)

type authState struct {
	// Identity string of the user (anonymous:anonymous if the user is not
	// logged in).
	Identity string `json:"identity,omitempty"`
	// The email of the user. Optional, default "".
	Email string `json:"email,omitempty"`
	// The URL of the user avatar. Optional, default "".
	Picture string `son:"picture,omitempty"`
	// The token that authorizes and authenticates the requests.
	AccessToken string `json:"accessToken,omitempty"`
	// Expiration time (unix timestamp) of the access token.
	//
	// If zero, the access token does not expire.
	AccessTokenExpiry int64 `json:"accessTokenExpiry,omitempty"`
}

// getAuthState returns data about the current user and the access token
// associated with the current session.
func getAuthState(c *router.Context) error {
	user := auth.CurrentUser(c.Request.Context())
	var state *authState
	if user.Identity == identity.AnonymousIdentity {
		state = &authState{
			Identity: string(user.Identity),
		}
	} else {
		session := auth.GetState(c.Request.Context()).Session()
		if session == nil {
			return errors.New("request not authenticated via secure cookies", grpcutil.UnauthenticatedTag)
		}

		token, err := session.AccessToken(c.Request.Context())
		if err != nil {
			return err
		}
		state = &authState{
			Identity:          string(user.Identity),
			Email:             user.Email,
			Picture:           user.Picture,
			AccessToken:       token.AccessToken,
			AccessTokenExpiry: token.Expiry.Unix(),
		}
	}

	c.Writer.Header().Add("Content-Type", "application/json")
	err := json.NewEncoder(c.Writer).Encode(state)
	return errors.Annotate(err, "failed to JSON encode output").Tag(grpcutil.InternalTag).Err()
}
