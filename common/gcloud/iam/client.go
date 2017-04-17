// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package iam

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"
	"google.golang.org/api/googleapi"

	"github.com/luci/luci-go/common/logging"
)

const (
	// DefaultBasePath is root of IAM API.
	DefaultBasePath = "https://iam.googleapis.com/"
	// OAuthScope is an OAuth scope required by IAM API.
	OAuthScope = "https://www.googleapis.com/auth/iam"
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
	if err = cl.apiRequest(c, "projects/-/serviceAccounts/"+serviceAccount, "signBlob", &request, &response); err != nil {
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
	if err = cl.apiRequest(c, "projects/-/serviceAccounts/"+serviceAccount, "signJwt", &request, &response); err != nil {
		return "", "", err
	}
	return response.KeyId, response.SignedJwt, nil
}

// GetIAMPolicy fetches an IAM policy of a resource.
//
// On non-success HTTP status codes returns googleapi.Error.
func (cl *Client) GetIAMPolicy(c context.Context, resource string) (*Policy, error) {
	response := &Policy{}
	if err := cl.apiRequest(c, resource, "getIamPolicy", nil, response); err != nil {
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
	if err := cl.apiRequest(c, resource, "setIamPolicy", &request, response); err != nil {
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

// apiRequest performs HTTP POST to an IAM API endpoint.
func (cl *Client) apiRequest(c context.Context, resource, action string, body, resp interface{}) error {
	// Serialize the body.
	var reader io.Reader
	if body != nil {
		blob, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(blob)
	}

	// Prepare the request.
	base := cl.BasePath
	if base == "" {
		base = DefaultBasePath
	}
	if base[len(base)-1] != '/' {
		base += "/"
	}
	url := fmt.Sprintf("%sv1/%s:%s?alt=json", base, resource, action)
	req, err := http.NewRequest("POST", url, reader)
	if err != nil {
		return err
	}
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Send and handle errors. This is roughly how google-api-go-client calls
	// methods. CheckResponse returns *googleapi.Error.
	logging.Debugf(c, "POST %s", url)
	res, err := ctxhttp.Do(c, cl.Client, req)
	if err != nil {
		return err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		logging.WithError(err).Errorf(c, "POST %s failed", url)
		return err
	}
	return json.NewDecoder(res.Body).Decode(resp)
}
