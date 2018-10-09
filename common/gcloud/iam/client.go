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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"net/url"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/logging"
)

const (
	// DefaultBasePath is root of IAM API.
	DefaultBasePath = "https://iam.googleapis.com/"

	// DefaultAccountCredentialsPath is root of IAM Account Credentials API.
	DefaultAccountCredentialsBasePath = "https://iamcredentials.googleapis.com/"

	// OAuthScope is an OAuth scope required by IAM API.
	OAuthScope = "https://www.googleapis.com/auth/iam"

	// Token type for generated OAauth2 tokens
	tokenType = "Bearer"
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
	BasePath string       // replaceable in tests, DefaultBasePath by default.
}

// SignBlob signs a blob using a service account's system-managed key.
//
// The caller must have "roles/iam.serviceAccountActor" role in the service
// account's IAM policy and caller's OAuth token must have one of the scopes:
//  * https://www.googleapis.com/auth/iam
//  * https://www.googleapis.com/auth/cloud-platform
//
// Returns ID of the signing key and the signature on success.
//
// On API-level errors (e.g. insufficient permissions) returns *googleapi.Error.
func (cl *Client) SignBlob(c context.Context, serviceAccount string, blob []byte) (keyName string, signature []byte, err error) {
	var request struct {
		BytesToSign []byte `json:"bytesToSign"`
	}
	request.BytesToSign = blob
	var response struct {
		KeyId     string `json:"keyId"`
		Signature []byte `json:"signature"`
	}
	if err = cl.iamApiRequest(c, "projects/-/serviceAccounts/"+serviceAccount, "signBlob", &request, &response); err != nil {
		return "", nil, err
	}
	return response.KeyId, response.Signature, nil
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
// The caller must have "roles/iam.serviceAccountActor" role in the service
// account's IAM policy and caller's OAuth token must have one of the scopes:
//  * https://www.googleapis.com/auth/iam
//  * https://www.googleapis.com/auth/cloud-platform
//
// Returns ID of the signing key and the signed JWT on success.
//
// On API-level errors (e.g. insufficient permissions) returns *googleapi.Error.
func (cl *Client) SignJWT(c context.Context, serviceAccount string, cs *ClaimSet) (keyName, signedJwt string, err error) {
	blob, err := json.Marshal(cs)
	if err != nil {
		return "", "", err
	}
	var request struct {
		Payload string `json:"payload"`
	}
	request.Payload = string(blob) // yep, this is JSON inside JSON
	var response struct {
		KeyId     string `json:"keyId"`
		SignedJwt string `json:"signedJwt"`
	}
	if err = cl.iamApiRequest(c, "projects/-/serviceAccounts/"+serviceAccount, "signJwt", &request, &response); err != nil {
		return "", "", err
	}
	return response.KeyId, response.SignedJwt, nil
}

// GetIAMPolicy fetches an IAM policy of a resource.
//
// On non-success HTTP status codes returns googleapi.Error.
func (cl *Client) GetIAMPolicy(c context.Context, resource string) (*Policy, error) {
	response := &Policy{}
	if err := cl.iamApiRequest(c, resource, "getIamPolicy", nil, response); err != nil {
		return nil, err
	}
	return response, nil
}

// SetIAMPolicy replaces an IAM policy of a resource.
//
// Returns a new policy (with Etag field updated).
func (cl *Client) SetIAMPolicy(c context.Context, resource string, p Policy) (*Policy, error) {
	var request struct {
		Policy *Policy `json:"policy"`
	}
	request.Policy = &p
	response := &Policy{}
	if err := cl.iamApiRequest(c, resource, "setIamPolicy", &request, response); err != nil {
		return nil, err
	}
	return response, nil
}

// ModifyIAMPolicy reads IAM policy, calls callback to modify it, and then
// puts it back (if callback really changed it).
//
// Cast error to *googleapi.Error and compare http status to http.StatusConflict
// to detect update race conditions. It is usually safe to retry in case of
// a conflict.
func (cl *Client) ModifyIAMPolicy(c context.Context, resource string, cb func(*Policy) error) error {
	policy, err := cl.GetIAMPolicy(c, resource)
	if err != nil {
		return err
	}
	// Make a copy to be mutated in the callback. Need to keep the original to
	// be able to detect changes.
	clone := policy.Clone()
	if err := cb(&clone); err != nil {
		return err
	}
	if clone.Equals(*policy) {
		return nil
	}
	_, err = cl.SetIAMPolicy(c, resource, clone)
	return err
}

// GenerateAccessToken creates a service account OAuth token using IAM's
// :generateAccessToken API.
//
// On non-success HTTP status codes returns googleapi.Error.
func (cl *Client) GenerateAccessToken(c context.Context, serviceAccount string, scopes []string, delegates *[]string, lifetime *time.Duration) (*oauth2.Token, error) {
	var body struct {
		Delegates []string `json:"delegates"`
		Scope     []string `json:"scope"`
		Lifetime  string   `json:"lifetime"`
	}

	body.Scope = scopes
	body.Lifetime = "3600s"

	// Default lifetime is 3600 seconds according to API documentation.
	// Requesting longer lifetime will cause an API error.
	if lifetime != nil {
		body.Lifetime = lifetime.String()
	}
	if delegates != nil {
		body.Delegates = *delegates
	}

	var resp struct {
		AccessToken string `json:"accessToken"`
		ExpireTime  string `json:"expireTime"`
		TokenType   string
	}
	if err := cl.credentialsApiRequest(c, fmt.Sprintf("projects/-/serviceAccounts/%s", url.QueryEscape(serviceAccount)), "generateAccessToken", &body, &resp); err != nil {
		return nil, err
	}

	expires, err := time.Parse(time.RFC3339, resp.ExpireTime)
	if err != nil {
		err = fmt.Errorf("Unable to parse 'expireTime': %s", resp.ExpireTime)
		logging.WithError(err).Errorf(c, "Bad token endpoint response, unable to parse expireTime")
		return nil, err
	}

	if resp.TokenType == "" {
		resp.TokenType = tokenType
	}
	return &oauth2.Token{
		AccessToken: resp.AccessToken,
		TokenType:   resp.TokenType,
		Expiry:      expires.UTC(),
	}, nil

}

func (cl *Client) iamApiRequest(c context.Context, resource, action string, body, resp interface{}) error {
	if cl.BasePath != "" {
		return cl.genericApiRequest(c, cl.BasePath, resource, action, body, resp)
	}
	return cl.genericApiRequest(c, DefaultBasePath, resource, action, body, resp)
}

func (cl *Client) credentialsApiRequest(c context.Context, resource, action string, body, resp interface{}) error {
	if cl.BasePath != "" {
		return cl.genericApiRequest(c, cl.BasePath, resource, action, body, resp)
	}
	return cl.genericApiRequest(c, DefaultAccountCredentialsBasePath, resource, action, body, resp)
}

// apiRequest performs HTTP POST to an IAM API endpoint.
func (cl *Client) genericApiRequest(c context.Context, base, resource, action string, body, resp interface{}) error {
	// Construct the URL from endpoint and query components
	endpoint, err := url.Parse(base)
	if err != nil {
		return err
	}
	query, err := url.Parse(fmt.Sprintf("v1/%s:%s?alt=json", resource, action))
	if err != nil {
		return err
	}
	endpoint = endpoint.ResolveReference(query)

	// Serialize the body.
	var reader io.Reader
	if body != nil {
		blob, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(blob)
	}

	// Issue the request
	req, err := http.NewRequest("POST", endpoint.String(), reader)
	if err != nil {
		return err
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send and handle errors. This is roughly how google-api-go-client calls
	// methods. CheckResponse returns *googleapi.Error.
	logging.Debugf(c, "POST %s", endpoint)
	res, err := ctxhttp.Do(c, cl.Client, req)
	if err != nil {
		return err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		logging.WithError(err).Errorf(c, "POST %s failed", endpoint)
		return err
	}
	return json.NewDecoder(res.Body).Decode(resp)
}
