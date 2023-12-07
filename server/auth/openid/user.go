// Copyright 2020 The LUCI Authors.
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

package openid

import (
	"context"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/auth"
)

// UserFromIDToken validates the ID token and extracts user information from it.
//
// Returns the partially validated token and auth.User extracted from it.
//
// The caller is still responsible to verify token's Audience field.
func UserFromIDToken(ctx context.Context, token string, discovery *DiscoveryDoc) (*IDToken, *auth.User, error) {
	// Validate the discovery doc has necessary fields to proceed.
	switch {
	case discovery.Issuer == "":
		return nil, nil, errors.Reason("openid: bad discovery doc, empty issuer").Err()
	case discovery.JwksURI == "":
		return nil, nil, errors.Reason("openid: bad discovery doc, empty jwks_uri").Err()
	}

	// Grab the signing keys needed to verify the token. This is almost always
	// hitting the local process cache and thus must be fast.
	signingKeys, err := discovery.SigningKeys(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Unpack the ID token to grab the user information from it.
	verifiedToken, err := VerifyIDToken(ctx, token, signingKeys, discovery.Issuer)
	if err != nil {
		return nil, nil, err
	}

	// Ignore non https:// URLs for pictures. We serve all pages over HTTPS and
	// don't want to break this rule just for a pretty picture.
	picture := verifiedToken.Picture
	if picture != "" && !strings.HasPrefix(picture, "https://") {
		picture = ""
	}

	// Build the identity string from the email. This essentially validates it
	// against a regexp.
	id, err := identity.MakeIdentity("user:" + verifiedToken.Email)
	if err != nil {
		return nil, nil, err
	}

	return verifiedToken, &auth.User{
		Identity: id,
		Email:    verifiedToken.Email,
		Name:     verifiedToken.Name,
		Picture:  picture,
	}, nil
}
