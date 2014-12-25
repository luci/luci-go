// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package gce provides functions useful when running on Google Compute Engine.
*/
package gce

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"sync"
	"time"

	"infra/libs/build"
	"infra/libs/logging"
)

// Guards isGCECached.
var isGCELock sync.Mutex

// Cached result of IsRunningOnGCE or nil if not yet known.
var isGCECached *bool

// URL of a metadata server, to be replaced in tests.
var gceMetadataServer = "http://metadata.google.internal"

// Client to talk to metadata server.
var metaClient = &http.Client{
	Transport: &http.Transport{
		Dial: (&net.Dialer{
			Timeout:   750 * time.Millisecond,
			KeepAlive: 30 * time.Second,
		}).Dial,
		ResponseHeaderTimeout: 750 * time.Millisecond,
	},
}

// ErrNotGCE is returned by functions in this package when they are called outside of GCE.
var ErrNotGCE = errors.New("Not running in GCE")

// MetadataError can be returned as error by QueryGCEMetadata.
type MetadataError struct {
	StatusCode int
	Response   string
}

// AccessToken represents an OAuth2 access_token fetched from metadata server.
type AccessToken struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

// ServiceAccount represents a service account available to GCE instance.
type ServiceAccount struct {
	Email  string   `json:"email"`
	Scopes []string `json:"scopes"`
}

// IsRunningOnGCE returns true if the binary is running in GCE.
func IsRunningOnGCE() (isGCE bool) {
	if build.AppengineBuild {
		return false
	}

	isGCELock.Lock()
	defer isGCELock.Unlock()

	if isGCECached != nil {
		isGCE = *isGCECached
		return
	}

	// See https://cloud.google.com/compute/docs/metadata#runninggce.
	// Basically try to ping the metadata server to see whether it's there.
	_, err := QueryGCEMetadata("/")
	isGCE = err == nil
	isGCECached = &isGCE
	return
}

// GetAccessToken fetches OAuth token for given account from metadata server.
func GetAccessToken(account string) (*AccessToken, error) {
	if !IsRunningOnGCE() {
		return nil, ErrNotGCE
	}
	body, err := QueryGCEMetadata("/instance/service-accounts/" + account + "/token")
	if err != nil {
		return nil, err
	}
	tok := &AccessToken{}
	err = json.Unmarshal(body, tok)
	if err != nil {
		return nil, err
	}
	return tok, nil
}

// GetServiceAccount fetches information about given service account.
func GetServiceAccount(account string) (*ServiceAccount, error) {
	if !IsRunningOnGCE() {
		return nil, ErrNotGCE
	}
	body, err := QueryGCEMetadata("/instance/service-accounts/" + account + "/?recursive=true")
	if err != nil {
		return nil, err
	}
	acc := &ServiceAccount{}
	err = json.Unmarshal(body, acc)
	if err != nil {
		return nil, err
	}
	return acc, nil
}

// GetInstanceAttribute returns a value of custom instance attribute or nil if missing.
func GetInstanceAttribute(key string) ([]byte, error) {
	if !IsRunningOnGCE() {
		return nil, ErrNotGCE
	}
	body, err := QueryGCEMetadata("/instance/attributes/" + key)
	if err != nil {
		metaErr, ok := err.(*MetadataError)
		if ok && metaErr.StatusCode == 404 {
			return nil, nil
		}
		return nil, err
	}
	return body, nil
}

// QueryGCEMetadata sends a generic request to GCE metadata server.
func QueryGCEMetadata(path string) ([]byte, error) {
	if build.AppengineBuild {
		return nil, ErrNotGCE
	}
	if path == "" || path[0] != '/' {
		return nil, fmt.Errorf("Not a valid URL path, must start with '/': %s", path)
	}
	url := gceMetadataServer + "/computeMetadata/v1" + path

	// See https://cloud.google.com/compute/docs/metadata#querying.
	var lastError error
	for i := 0; i < 5; i++ {
		if i != 0 {
			logging.Debugf("Retrying GCE query in 1 sec...")
			time.Sleep(1 * time.Second)
		}
		logging.Debugf("Querying GCE metadata: %s", url)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header["Metadata-Flavor"] = []string{"Google"}
		resp, err := metaClient.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 299 {
			return body, nil
		}
		lastError = &MetadataError{
			StatusCode: resp.StatusCode,
			Response:   string(body),
		}
		if resp.StatusCode < 500 {
			break
		}
	}
	return nil, lastError
}

func (e *MetadataError) Error() string {
	return fmt.Sprintf("Metadata server returned HTTP %d: %s", e.StatusCode, e.Response)
}
