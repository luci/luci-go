// Copyright 2016 The LUCI Authors.
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

package iam

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/logging"
)

var (
	// DefaultIamBaseURL is IAM's core API endpoint.
	DefaultIamBaseURL = &url.URL{
		Scheme: "https",
		Host:   "iam.googleapis.com",
	}

	// DefaultAccountCredentialsBaseURL is IAM's account credentials API endpoint.
	DefaultAccountCredentialsBaseURL = &url.URL{
		Scheme: "https",
		Host:   "iamcredentials.googleapis.com",
	}
)

// ClaimSet contains information about the JWT signature including the
// permissions being requested (scopes), the target of the token, the issuer,
// the time the token was issued, and the lifetime of the token.
//
// See RFC 7515.
type ClaimSet struct {
	Iss   string `json:"iss"`             // email address of the client_id of the application making the access token request
	Scope string `json:"scope,omitempty"` // space-delimited list of the permissions the application requests
	Aud   string `json:"aud"`             // descriptor of the intended target of the assertion (Optional).
	Exp   int64  `json:"exp"`             // the expiration time of the assertion (seconds since Unix epoch)
	Iat   int64  `json:"iat"`             // the time the assertion was issued (seconds since Unix epoch)
	Typ   string `json:"typ,omitempty"`   // token type (Optional).

	// Email for which the application is requesting delegated access (Optional).
	Sub string `json:"sub,omitempty"`
}

// Client knows how to perform IAM API v1 calls.
type Client struct {
	Client   *http.Client // client to use to make calls
	BasePath string       // replaceable in tests, DefaultIamBaseURL / DefaultAccountCredentials by default.
}

// SignBlob signs a blob using a service account's system-managed key.
//
// The caller must have "roles/iam.serviceAccountTokenCreator" role in the
// service account's IAM policy and caller's OAuth token must have one of the
// scopes:
//  * https://www.googleapis.com/auth/iam
//  * https://www.googleapis.com/auth/cloud-platform
//
// Returns ID of the signing key and the signature on success.
//
// On API-level errors (e.g. insufficient permissions) returns *googleapi.Error.
func (cl *Client) SignBlob(ctx context.Context, serviceAccount string, blob []byte) (keyName string, signature []byte, err error) {
	var request struct {
		BytesToSign []byte `json:"bytesToSign"`
	}
	request.BytesToSign = blob
	var response struct {
		KeyID     string `json:"keyId"`
		Signature []byte `json:"signature"`
	}
	if err = cl.iamAPIRequest(ctx, "projects/-/serviceAccounts/"+serviceAccount, "signBlob", &request, &response); err != nil {
		return "", nil, err
	}
	return response.KeyID, response.Signature, nil
}

// SignJWT signs a claim set using a service account's system-managed key.
//
// It injects the key ID into the JWT header before singing. As a result, JWTs
// produced by SignJWT are slightly faster to verify, because we know what
// public key to use exactly and don't need to enumerate all active keys.
//
// It also checks the expiration time and refuses to sign claim sets with
// 'exp' set to more than 1h from now. Otherwise it is similar to SignBlob.
//
// The caller must have "roles/iam.serviceAccountTokenCreator" role in the
// service account's IAM policy and caller's OAuth token must have one of the
// scopes:
//  * https://www.googleapis.com/auth/iam
//  * https://www.googleapis.com/auth/cloud-platform
//
// Returns ID of the signing key and the signed JWT on success.
//
// On API-level errors (e.g. insufficient permissions) returns *googleapi.Error.
func (cl *Client) SignJWT(ctx context.Context, serviceAccount string, cs *ClaimSet) (keyName, signedJwt string, err error) {
	blob, err := json.Marshal(cs)
	if err != nil {
		return "", "", err
	}
	var request struct {
		Payload string `json:"payload"`
	}
	request.Payload = string(blob) // yep, this is JSON inside JSON
	var response struct {
		KeyID     string `json:"keyId"`
		SignedJwt string `json:"signedJwt"`
	}
	if err = cl.iamAPIRequest(ctx, "projects/-/serviceAccounts/"+serviceAccount, "signJwt", &request, &response); err != nil {
		return "", "", err
	}
	return response.KeyID, response.SignedJwt, nil
}

// GenerateAccessToken creates a service account OAuth token using IAM's
// :generateAccessToken API.
//
// On non-success HTTP status codes returns googleapi.Error.
func (cl *Client) GenerateAccessToken(ctx context.Context, serviceAccount string, scopes []string, delegates []string, lifetime time.Duration) (*oauth2.Token, error) {
	var body struct {
		Delegates []string `json:"delegates,omitempty"`
		Scope     []string `json:"scope"`
		Lifetime  string   `json:"lifetime,omitempty"`
	}
	body.Delegates = delegates
	body.Scope = scopes

	// Default lifetime is 3600 seconds according to API documentation.
	// Requesting longer lifetime will cause an API error which is
	// forwarded to the caller.
	if lifetime > 0 {
		body.Lifetime = lifetime.String()
	}

	var resp struct {
		AccessToken string `json:"accessToken"`
		ExpireTime  string `json:"expireTime"`
	}
	if err := cl.credentialsAPIRequest(ctx, serviceAccount, "generateAccessToken", &body, &resp); err != nil {
		return nil, err
	}

	expires, err := time.Parse(time.RFC3339, resp.ExpireTime)
	if err != nil {
		return nil, fmt.Errorf("unable to parse 'expireTime': %s", resp.ExpireTime)
	}

	return &oauth2.Token{
		AccessToken: resp.AccessToken,
		TokenType:   "Bearer",
		Expiry:      expires.UTC(),
	}, nil
}

// GenerateIDToken creates a service account OpenID Connect ID token using IAM's
// :generateIdToken API.
//
// On non-success HTTP status codes returns googleapi.Error.
func (cl *Client) GenerateIDToken(ctx context.Context, serviceAccount string, audience string, includeEmail bool, delegates []string) (string, error) {
	var body struct {
		Delegates    []string `json:"delegates,omitempty"`
		Audience     string   `json:"audience"`
		IncludeEmail bool     `json:"includeEmail,omitempty"`
	}
	body.Delegates = delegates
	body.Audience = audience
	body.IncludeEmail = includeEmail

	var resp struct {
		Token string `json:"token"`
	}
	if err := cl.credentialsAPIRequest(ctx, serviceAccount, "generateIdToken", &body, &resp); err != nil {
		return "", err
	}
	return resp.Token, nil
}

// iamAPIRequest performs HTTP POST to the core IAM API endpoint.
func (cl *Client) iamAPIRequest(ctx context.Context, resource, action string, body, resp interface{}) error {
	if cl.BasePath != "" {
		// We are in "testing"
		base, err := url.Parse(cl.BasePath)
		if err != nil {
			return err
		}
		return cl.genericAPIRequest(ctx, base, resource, action, body, resp)
	}
	return cl.genericAPIRequest(ctx, DefaultIamBaseURL, resource, action, body, resp)
}

// credentialsAPIRequest performs HTTP POST to the IAM credentials API endpoint.
func (cl *Client) credentialsAPIRequest(ctx context.Context, serviceAccount, action string, body, resp interface{}) error {
	resource := fmt.Sprintf("projects/-/serviceAccounts/%s", url.QueryEscape(serviceAccount))
	if cl.BasePath != "" {
		// We are in "testing"
		base, err := url.Parse(cl.BasePath)
		if err != nil {
			return err
		}
		return cl.genericAPIRequest(ctx, base, resource, action, body, resp)
	}
	return cl.genericAPIRequest(ctx, DefaultAccountCredentialsBaseURL, resource, action, body, resp)
}

// genericAPIRequest performs HTTP POST to an IAM API endpoint.
func (cl *Client) genericAPIRequest(ctx context.Context, base *url.URL, resource, action string, body, resp interface{}) error {
	query, err := url.Parse(fmt.Sprintf("v1/%s:%s?alt=json", resource, action))
	if err != nil {
		return err
	}
	endpoint := base.ResolveReference(query)

	// Serialize the body.
	var reader io.Reader
	if body != nil {
		blob, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(blob)
	}

	// Issue the request.
	req, err := http.NewRequest("POST", endpoint.String(), reader)
	if err != nil {
		return err
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send and handle errors. This is roughly how google-api-go-client calls
	// methods. CheckResponse returns *googleapi.Error.
	logging.Debugf(ctx, "POST %s", endpoint)
	res, err := ctxhttp.Do(ctx, cl.Client, req)
	if err != nil {
		return err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		logging.WithError(err).Errorf(ctx, "POST %s failed", endpoint)
		return err
	}
	return json.NewDecoder(res.Body).Decode(resp)
}
