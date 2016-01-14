// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package coordinator is a user-friendly interface to the Coordinator's logs
// endpoint.
package coordinator

import (
	"fmt"
	"net/http"

	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/auth"
)

var (
	// Scopes is the set of scopes needed for the Coordinator user endpoints.
	Scopes = []string{
		auth.OAuthScopeEmail,
	}
)

// Config is the set of configurable parameters for a Coordinator client.
type Config struct {
	// The authenticated HTTP client to use.
	Client *http.Client

	// UserAgent, if not nil, replaces the user agent string used by the client.
	UserAgent string

	// The API base path for the Logs API. If empty, the default path will be
	// used.
	LogsAPIBasePath string
}

func (c *Config) logs() (*logs.Service, error) {
	svc, err := logs.New(c.Client)
	if err != nil {
		return nil, err
	}
	if c.UserAgent != "" {
		svc.UserAgent = c.UserAgent
	}
	if c.LogsAPIBasePath != "" {
		svc.BasePath = fmt.Sprintf("%slogs/v1/", c.LogsAPIBasePath)
	}
	return svc, nil
}
