// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package gce provides functions useful when running on Google Compute Engine.
*/
package gce

import (
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

// IsRunningOnGCE returns true if the binary is running in GCE. It may block
// for ~500 ms on a first call to check for this fact.
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
	resp, err := QueryGCEMetadata("/computeMetadata/v1/instance/", time.Millisecond*500)
	if resp != nil {
		resp.Body.Close()
	}
	isGCE = err == nil
	isGCECached = &isGCE
	return
}

// QueryGCEMetadata sends a request to GCE metadata server.
func QueryGCEMetadata(path string, timeout time.Duration) (*http.Response, error) {
	// TODO(vadimsh): Retry when metadata server returns 500.

	if build.AppengineBuild {
		return nil, fmt.Errorf("GCE metadata is not available from appengine")
	}

	if path == "" || path[0] != '/' {
		return nil, fmt.Errorf("Not a valid URL path, must start with '/': %s", path)
	}

	// Boilerplate for http.Client with a connection timeout.
	client := http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.DialTimeout(network, addr, timeout)
			},
		},
	}

	// Make the call. See https://cloud.google.com/compute/docs/metadata#querying.
	logging.Debugf("Querying GCE metadata: %s", path)
	req, _ := http.NewRequest("GET", gceMetadataServer+path, nil)
	req.Header["Metadata-Flavor"] = []string{"Google"}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		body, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("Unexpected response from metadata server HTTP %d: %v", resp.StatusCode, body)
	}
	flavor := resp.Header["Metadata-Flavor"]
	if len(flavor) != 1 || flavor[0] != "Google" {
		resp.Body.Close()
		return nil, fmt.Errorf("Not a metadata server")
	}
	return resp, nil
}

// resetIsGCECached clear process wide cache of IsRunningOnGCE().
func resetIsGCECached() {
	isGCELock.Lock()
	isGCECached = nil
	isGCELock.Unlock()
}
