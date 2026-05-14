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

// Package logdog contains logic of interacting with Logdog.
package logdog

import (
	"context"
	"net/http"
	"regexp"
	"time"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/bisection/util"
)

var MockedLogdogClientKey = "mocked logdog client"

var logdogPathRegex = regexp.MustCompile(`^/logs/([^/]+)/`)

// GetLogFromViewUrl gets the log from the log's viewURL
func GetLogFromViewUrl(c context.Context, viewUrl string) (string, error) {
	cl := GetClient(c)
	return cl.GetLog(c, viewUrl)
}

type LogdogClient struct{}

func (cl *LogdogClient) GetLog(c context.Context, viewUrl string) (string, error) {
	req, err := http.NewRequest("GET", viewUrl, nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	q.Add("format", "raw")
	req.URL.RawQuery = q.Encode()

	logging.Infof(c, "Sending request to logdog %s", req.URL.String())

	// Extract project from path /logs/<project>/...
	useAuth := false
	if matches := logdogPathRegex.FindStringSubmatch(req.URL.Path); len(matches) > 1 {
		if matches[1] == "chrome" {
			useAuth = true
		}
	}

	res, err := util.SendHTTPRequest(c, req, 30*time.Second, useAuth)
	if err != nil {
		logging.Errorf(c, "Failed to get log from logdog for URL %s: %v", viewUrl, err)
		return "", err
	}
	logging.Infof(c, "Successfully got log from logdog for URL %s, size: %d", viewUrl, len(res))
	return res, nil
}

// We need the interface for testing purpose
type Client interface {
	GetLog(c context.Context, viewUrl string) (string, error)
}

func GetClient(c context.Context) Client {
	if mockClient, ok := c.Value(MockedLogdogClientKey).(*MockedLogdogClient); ok {
		return mockClient
	}
	return &LogdogClient{}
}
