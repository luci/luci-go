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

package prod

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sync"

	"golang.org/x/net/context"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/urlfetch"
)

var devAccountCache struct {
	once    sync.Once
	account string
	err     error
}

// developerAccount is used on dev server to get account name matching
// the OAuth token produced by AccessToken.
//
// On dev server ServiceAccount returns empty string, but AccessToken(...) works
// and returns developer's token (the one configured with "gcloud auth"). We can
// use it to get the matching account name.
func developerAccount(gaeCtx context.Context) (string, error) {
	if !appengine.IsDevAppServer() {
		panic("developerAccount must not be used outside of devserver")
	}
	devAccountCache.once.Do(func() {
		devAccountCache.account, devAccountCache.err = fetchDevAccount(gaeCtx)
		if devAccountCache.err == nil {
			log.Debugf(gaeCtx, "Devserver is running as %q", devAccountCache.account)
		} else {
			log.Errorf(gaeCtx, "Failed to fetch account name associated with AccessToken - %s", devAccountCache.err)
		}
	})
	return devAccountCache.account, devAccountCache.err
}

// fetchDevAccount grabs an access token and calls Google API to get associated
// email.
func fetchDevAccount(gaeCtx context.Context) (string, error) {
	// Grab the developer's token from devserver.
	tok, _, err := appengine.AccessToken(gaeCtx, "https://www.googleapis.com/auth/userinfo.email")
	if err != nil {
		return "", err
	}

	// Fetch the info dict associated with the token.
	client := http.Client{
		Transport: &urlfetch.Transport{Context: gaeCtx},
	}
	resp, err := client.Get("https://www.googleapis.com/oauth2/v3/tokeninfo?access_token=" + url.QueryEscape(tok))
	if err != nil {
		return "", err
	}
	defer func() {
		ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 500 {
		return "", fmt.Errorf("devserver: transient error when validating token (HTTP %d)", resp.StatusCode)
	}

	// There's more stuff in the reply, we don't need it.
	var tokenInfo struct {
		Email string `json:"email"`
		Error string `json:"error_description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenInfo); err != nil {
		return "", fmt.Errorf("devserver: failed to deserialize token info JSON - %s", err)
	}
	switch {
	case tokenInfo.Error != "":
		return "", fmt.Errorf("devserver: bad token - %s", tokenInfo.Error)
	case tokenInfo.Email == "":
		return "", fmt.Errorf("devserver: token is not associated with an email")
	}
	return tokenInfo.Email, nil
}
