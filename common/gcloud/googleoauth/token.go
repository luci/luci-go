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

package googleoauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/logging"
)

var (
	genAccessTokenEndpoint = "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/%s:generateAccessToken"
)

type GenTokenFlowParams struct {
	// ServiceAccount is a service account name to get an access token for.
	ServiceAccount string

	// Scopes is a list of OAuth2 scopes to claim.
	Scopes []string

	// Client is a non-authenticating client to use for the exchange.
	//
	// If not set, http.DefaultClient will be used.
	Client *http.Client

	// tokenEndpoint is used in tests to mock www.googleapis.com.
	tokenEndpoint string
}


// GetAccessToken grabs an OAuth2 token using IAM generateAccessToken API.
//
// Behaves like GetAccessToken but using more efficient flow by directly calling
//   https://cloud.google.com/iam/credentials/reference/rest/v1/projects.serviceAccounts/generateAccessToken
// in order to obtain an OAuth2 token directly instead of first obtaining a JWT.
//
// The returned token usually have 1 hour lifetime
//
// Does not retry transient errors. Returns signing and HTTP connection errors
// as is. Unsuccessful HTTP requests result in *googleapi.Error.
func GetAccessToken(ctx context.Context, params GenTokenFlowParams) (*oauth2.Token, error) {
	if params.Client == nil {
		params.Client = http.DefaultClient
	}
	if params.tokenEndpoint == "" {
		params.tokenEndpoint = fmt.Sprintf(genAccessTokenEndpoint, url.QueryEscape(params.ServiceAccount))
	}

	v := url.Values{}
	for _, scope := range params.Scopes {
		v.Add("scope", scope)
	}
	resp, err := ctxhttp.PostForm(ctx, params.Client, params.tokenEndpoint, v)
	if err != nil {
		logging.WithError(err).Errorf(ctx, "POST %s failed", params.tokenEndpoint)
		return nil, err
	}
	defer googleapi.CloseBody(resp)
	if err := googleapi.CheckResponse(resp); err != nil {
		logging.WithError(err).Errorf(ctx, "POST %s failed", params.tokenEndpoint)
		return nil, err
	}
	var token struct {
		AccessToken string `json:accessToken`
		ExpireTime   string `json:expireTime`
		TokenType string
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		logging.WithError(err).Errorf(ctx, "Bad token endpoint response")
		return nil, err
	}

	expires, err := time.Parse(time.RFC3339, token.ExpireTime)
	if err != nil {
		err = fmt.Errorf("Unable to parse 'expireTime': %s", token.ExpireTime)
		logging.WithError(err).Errorf(ctx, "Bad token endpoint response, unable to parse expireTime")
		return nil, err
	}

	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	return &oauth2.Token{
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
		Expiry:      expires.UTC(),
	}, nil
}