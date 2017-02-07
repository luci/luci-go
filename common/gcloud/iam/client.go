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
)

const (
	// DefaultBasePath is root of IAM API.
	DefaultBasePath = "https://iam.googleapis.com/"
)

// Client knows how to perform IAM API v1 calls.
type Client struct {
	Client   *http.Client // client to use to make calls
	BasePath string       // replaceable in tests, DefaultBasePath by default.
}

// SignBlob signs a blob using a service account's system-managed private key.
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
	res, err := ctxhttp.Do(c, cl.Client, req)
	if err != nil {
		return err
	}
	defer googleapi.CloseBody(res)
	if err := googleapi.CheckResponse(res); err != nil {
		return err
	}
	return json.NewDecoder(res.Body).Decode(resp)
}
