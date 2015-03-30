// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// GetJSON does a simple HTTP GET on a JSON endpoint.
//
// Returns the status code and the error, if any.
func GetJSON(c *http.Client, url string, v interface{}) (int, error) {
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Get(url)
	if err != nil {
		return 0, fmt.Errorf("couldn't resolve %s: %s", url, err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return resp.StatusCode, fmt.Errorf("bad response %s: %s", url, err)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	expected := "application/json; charset=utf-8"
	if ct != expected {
		return resp.StatusCode, fmt.Errorf("unexpected Content-Type, expected \"%s\", got \"%s\"", expected, ct)
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// ReadJSONFile reads a file and decode it as JSON.
func ReadJSONFile(filePath string, object interface{}) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %s", filePath, err)
	}
	defer f.Close()
	if err = json.NewDecoder(f).Decode(object); err != nil {
		return fmt.Errorf("failed to decode %s: %s", filePath, err)
	}
	return nil
}

// WriteJSONFile writes object as json encoded into filePath with 2 spaces
// indentation. File permission is set to user only.
func WriteJSONFile(filePath string, object interface{}) error {
	d, err := json.MarshalIndent(object, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode %s: %s", filePath, err)
	}

	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open %s: %s", filePath, err)
	}
	defer f.Close()
	if _, err := f.Write(d); err != nil {
		return fmt.Errorf("failed to write %s: %s", filePath, err)
	}
	return nil
}
