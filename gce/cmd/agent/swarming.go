// Copyright 2018 The LUCI Authors.
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

package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// SwarmingClient is a Swarming server client.
type SwarmingClient struct {
	// Client is the *http.Client to use to communicate with the Swarming server.
	*http.Client
	// PlatformStrategy is the platform-specific strategy to use.
	PlatformStrategy
	// server is the Swarming server URL.
	server string
}

// swrKey is the key to a *SwarmingClient in the context.
var swrKey = "swr"

// withSwarming returns a new context with the given *SwarmingClient installed.
func withSwarming(c context.Context, cli *SwarmingClient) context.Context {
	return context.WithValue(c, &swrKey, cli)
}

// getSwarming returns the *SwarmingClient installed in the current context.
func getSwarming(c context.Context) *SwarmingClient {
	return c.Value(&swrKey).(*SwarmingClient)
}

// fetch fetches the Swarming bot code.
func (s *SwarmingClient) fetch(c context.Context, path, user string) error {
	botCode := s.server + "/bot_code"
	logging.Infof(c, "downloading: %s", botCode)
	rsp, err := s.Get(botCode)
	if err != nil {
		return errors.Fmt("failed to fetch bot code: %w", err)
	}
	defer rsp.Body.Close()
	if rsp.StatusCode != http.StatusOK {
		return errors.Fmt("server returned %q", rsp.Status)
	}

	logging.Infof(c, "installing: %s", path)
	// 0644 allows the bot code to be read by all users.
	// Useful when SSHing to the instance.
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return errors.Fmt("failed to open: %s: %w", path, err)
	}
	defer out.Close()
	_, err = io.Copy(out, rsp.Body)
	if err != nil {
		return errors.Fmt("failed to write: %s: %w", path, err)
	}
	if err := s.chown(c, path, user); err != nil {
		return errors.Fmt("failed to chown: %s: %w", path, err)
	}
	return nil
}

// Configure fetches the Swarming bot code and configures it to run on startup.
func (s *SwarmingClient) Configure(c context.Context, dir, user string, python string) error {
	// 0755 allows the directory structure to be read and listed by all users.
	// Useful when SSHing fo the instance.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return errors.Fmt("failed to create: %s: %w", dir, err)
	}
	if err := s.chown(c, dir, user); err != nil {
		return errors.Fmt("failed to chown: %s: %w", dir, err)
	}
	zip := filepath.Join(dir, "swarming_bot.zip")
	switch _, err := os.Stat(zip); {
	case os.IsNotExist(err):
	case err != nil:
		return errors.Fmt("failed to stat: %s: %w", zip, err)
	default:
		logging.Infof(c, "already installed: %s", zip)
		return nil
	}
	if err := s.fetch(c, zip, user); err != nil {
		return err
	}
	return s.autostart(c, zip, user, python)
}
