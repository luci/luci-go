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

package internal

import (
	"context"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/server/auth"
)

// GoogleAuthorizationEndpoint is Google's authorization endpoint URL.
const GoogleAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"

// OAuthClientProvider returns OAuth client details for known clients.
//
// Returns nil if the client is not known or an error if the check failed.
type OAuthClientProvider func(ctx context.Context, clientID string) (*OAuthClient, error)

// OAuthClient represents a known accepted OAuth client.
type OAuthClient struct {
	// ProviderName is the name of the identity provider shown on the web pages.
	ProviderName string
	// AuthorizationEndpoint is OAuth endpoint to redirect the user to.
	AuthorizationEndpoint string
}

// AuthDBClientProvider checks if a client is registered in the AuthDB.
func AuthDBClientProvider(ctx context.Context, clientID string) (*OAuthClient, error) {
	logging.Infof(ctx, "Checking OAuth client ID: %s", clientID)
	switch yes, err := auth.GetState(ctx).DB().IsAllowedOAuthClientID(ctx, "", clientID); {
	case err != nil:
		return nil, err
	case yes:
		// Only Google OAuth clients are supported currently.
		return &OAuthClient{
			ProviderName:          "Google Accounts",
			AuthorizationEndpoint: GoogleAuthorizationEndpoint,
		}, nil
	default:
		return nil, nil
	}
}
