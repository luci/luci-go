// Copyright 2022 The LUCI Authors.
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

// Package util contains utility functions
package util

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.chromium.org/luci/server/auth"
)

func SendHTTPRequest(c context.Context, req *http.Request, timeout time.Duration) (string, error) {
	c, cancel := context.WithTimeout(c, timeout)
	defer cancel()

	transport, err := auth.GetRPCTransport(c, auth.NoAuth)
	if err != nil {
		return "", err
	}

	client := &http.Client{
		Transport: transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()
	status := resp.StatusCode
	if status != http.StatusOK {
		return "", fmt.Errorf("Bad response code: %v", status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("Cannot get response body %w", err)
	}
	return string(body), nil
}
