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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/iam"
	"go.chromium.org/luci/common/logging"
)

var (
	googleTokenEndpoint = "https://www.googleapis.com/oauth2/v4/token"
	jwtGrantType        = "urn:ietf:params:oauth:grant-type:jwt-bearer"
)

// Signer knows how to sign JWTs with a private key owned by a service account.
type Signer interface {
	// SignJWT signs the claim set with some active private key to produce JWT.
	SignJWT(c context.Context, serviceAccount string, cs *iam.ClaimSet) (keyName, signedJwt string, err error)
}

// JwtFlowParams describes how to perform GetAccessToken call.
type JwtFlowParams struct {
	// ServiceAccount is a service account name to get an access token for.
	ServiceAccount string

	// Signer signs JWTs with a private key owned by the service account.
	Signer Signer

	// Scopes is a list of OAuth2 scopes to claim.
	Scopes []string

	// Client is a non-authenticating client to use for the exchange.
	//
	// If not set, http.DefaultClient will be used.
	Client *http.Client

	// tokenEndpoint is used in tests to mock www.googleapis.com.
	tokenEndpoint string
}

// GetAccessToken grabs an access token using a JWT as an authorization grant.
//
// It performs same kind of a flow as when using a regular service account
// private key, except it allows any signer implementation (not necessarily
// based on local crypto). This is particularly helpful when using 'signBlob'
// IAM API to sign JWTs, since it allows to mint an access token for accounts we
// don't have private keys for (but have "roles/iam.serviceAccountActor" role).
//
// The returned token usually have 1 hour lifetime.
//
// Does not retry transient errors. Returns signing and HTTP connection errors
// as is. Unsuccessful HTTP requests result in *googleapi.Error.
func GetAccessToken(c context.Context, params JwtFlowParams) (*oauth2.Token, error) {
	// See https://developers.google.com/identity/protocols/OAuth2ServiceAccount#authorizingrequests
	// Also https://github.com/golang/oauth2/blob/master/jwt/jwt.go.

	if params.Client == nil {
		params.Client = http.DefaultClient
	}
	if params.tokenEndpoint == "" {
		params.tokenEndpoint = googleTokenEndpoint
	}

	// Prepare a claim set to be signed by the service account key. Note that
	// Google backends seem to ignore Exp field and always give one-hour long
	// tokens, so we just always request 1h long token too.
	//
	// Also revert time back a bit, for the sake of machines whose time is not
	// perfectly in sync with global time. If client machine's time is in the
	// future according to Google server clock, the access token request will be
	// denied. It doesn't complain about slightly late clock though.
	now := clock.Now(c).Add(-15 * time.Second)
	claimSet := &iam.ClaimSet{
		Iat:   now.Unix(),
		Exp:   now.Add(time.Hour).Unix(),
		Iss:   params.ServiceAccount,
		Scope: strings.Join(params.Scopes, " "),
		Aud:   params.tokenEndpoint,
	}

	// Sign it, thus obtaining so called 'assertion'. Note that with Google gRPC
	// endpoints, an assertion by itself can be used as an access token (for an
	// URL specified in Aud field). It doesn't work for GAE backends though.
	_, assertion, err := params.Signer.SignJWT(c, params.ServiceAccount, claimSet)
	if err != nil {
		return nil, err
	}

	// Exchange the assertion for the access token.
	v := url.Values{}
	v.Set("grant_type", jwtGrantType)
	v.Set("assertion", assertion)
	logging.Debugf(c, "POST %s", params.tokenEndpoint)
	resp, err := ctxhttp.PostForm(c, params.Client, params.tokenEndpoint, v)
	if err != nil {
		logging.WithError(err).Errorf(c, "POST %s failed", params.tokenEndpoint)
		return nil, err
	}
	defer googleapi.CloseBody(resp)
	if err := googleapi.CheckResponse(resp); err != nil {
		logging.WithError(err).Errorf(c, "POST %s failed", params.tokenEndpoint)
		return nil, err
	}
	var token struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int64  `json:"expires_in"` // relative seconds from now
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		logging.WithError(err).Errorf(c, "Bad token endpoint response")
		return nil, err
	}

	// The Google endpoint always returns positive 'expires_in'.
	if token.ExpiresIn <= 0 {
		err = fmt.Errorf("bad 'expires_in': %d", token.ExpiresIn)
		logging.WithError(err).Errorf(c, "Bad token endpoint response")
		return nil, err
	}
	if token.TokenType == "" {
		token.TokenType = "Bearer"
	}
	return &oauth2.Token{
		AccessToken: token.AccessToken,
		TokenType:   token.TokenType,
		Expiry:      clock.Now(c).Add(time.Duration(token.ExpiresIn) * time.Second).UTC(),
	}, nil
}
