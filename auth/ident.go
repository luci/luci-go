// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"infra/libs/build"
)

// FetchIdentity performs an HTTP request (using provided transport) to an
// endpoint that knows how to resolve access token to an account name. It then
// returns this account name. It's assumed that transport adds authentication
// headers to requests (like a transport returned by Authenticator does).
func FetchIdentity(transport http.RoundTripper) (string, error) {
	client := http.Client{Transport: transport}
	resp, err := client.Get(tokenCheckURL())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	bytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("Unexpected response HTTP %d: %s", resp.StatusCode, string(bytes))
	}

	var response struct {
		Identity  string `json:"identity"`
		ErrorText string `json:"text"`
	}
	err = json.Unmarshal(bytes, &response)
	if err != nil {
		return "", err
	}
	if response.ErrorText != "" {
		return "", fmt.Errorf("Call failed: %s", response.ErrorText)
	}
	return response.Identity, nil
}

func tokenCheckURL() (url string) {
	if build.ReleaseBuild {
		url = "https://chrome-infra-auth.appspot.com"
	} else {
		url = "https://chrome-infra-auth-dev.appspot.com"
	}
	url += "/auth/api/v1/accounts/self"
	return
}
