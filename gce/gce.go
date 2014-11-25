// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package infra/libs/gce provides functions useful when running on Google Compute
Engine.
*/
package gce

import (
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Sirupsen/logrus"
)

// Guards isGCECached.
var isGCELock = sync.Mutex{}

// Cached result of IsRunningOnGCE or nil if not yet known.
var isGCECached *bool

// URL of a metadata server, to be replaced in tests.
var gceMetadataServer = "http://metadata.google.internal"

// IsRunningOnGCE returns true if the binary is running in GCE. It may block
// for ~500 ms on a first call to check for this fact.
func IsRunningOnGCE() (isGCE bool) {
	isGCELock.Lock()
	defer isGCELock.Unlock()

	if isGCECached != nil {
		isGCE = *isGCECached
		return
	}

	// Already checked in some previous process?
	cachedCheckFile := cachedCheckFilename()
	isGCE, err := readBoolFromFile(cachedCheckFile)
	if err == nil {
		logrus.Debugf("Reusing cached GCE check: IsRunningOnGCE == %v", isGCE)
		isGCECached = &isGCE
		return
	}

	// See https://cloud.google.com/compute/docs/metadata#runninggce.
	// Basically try to ping the metadata server to see whether it's there.
	_, err = QueryGCEMetadata("/", time.Millisecond*500)
	isGCE = err == nil
	isGCECached = &isGCE

	// Store the result of the check in the file, to avoid repeating it.
	err = writeBoolToFile(cachedCheckFile, isGCE)
	if err != nil {
		logrus.Warningf("Failed to cached GCE check result: %s", err.Error())
	}
	return
}

// QueryGCEMetadata sends a request to GCE metadata server.
func QueryGCEMetadata(path string, timeout time.Duration) (*http.Response, error) {
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
	logrus.Debugf("Querying GCE metadata: %s", path)
	req, _ := http.NewRequest("GET", gceMetadataServer+path, nil)
	req.Header["Metadata-Flavor"] = []string{"Google"}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Unexpected HTTP status from metadata server: %d", resp.StatusCode)
	}
	flavor := resp.Header["Metadata-Flavor"]
	if len(flavor) != 1 || flavor[0] != "Google" {
		return nil, fmt.Errorf("Not a metadata server")
	}
	return resp, nil
}

////////////////////////////////////////////////////////////////////////////////
// Utilities.

// Root of a temp directory. To be mocked in tests.
var gceUtilTempDir = os.TempDir()

// cachedCheckFilename returns a path to a file to store cached value of
// IsRunningOnGCE check.
func cachedCheckFilename() string {
	// Use hostname in the filename to handle disk reuse among VMs (i.e. retry
	// the check when VM image is booted on a different host).
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return filepath.Join(gceUtilTempDir, "_cr_gce_check_"+hostname+".tmp")
}

// readBoolFromFile reads a boolean value from a file on disk.
func readBoolFromFile(path string) (bool, error) {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return false, err
	}
	asStr := string(data)
	if asStr == "true" {
		return true, nil
	} else if asStr == "false" {
		return false, nil
	}
	return false, fmt.Errorf("Unexpected value: %s", asStr)
}

// writeBoolToFile writes a boolean value to a file on disk.
func writeBoolToFile(path string, value bool) error {
	var toStore string
	if value {
		toStore = "true"
	} else {
		toStore = "false"
	}
	return ioutil.WriteFile(path, []byte(toStore), 0666)
}

// resetIsGCECached clear process wide cache of IsRunningOnGCE().
func resetIsGCECached() {
	isGCELock.Lock()
	isGCECached = nil
	isGCELock.Unlock()
}
