// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"encoding/json"
	"io"

	"go.chromium.org/luci/auth/reauth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/ssh"
	"go.chromium.org/luci/common/webauthn"
)

// sshPluginMain reads a plugin request from `r`, sends the request
// over an SSH agent dialed by `dialer`, obtains and writes a plugin
// response to `w`.
//
// If an error occurred, this function will write an error plugin
// response if possible.
func sshPluginMain(ctx context.Context, dialer ssh.AgentDialer, r io.Reader, w io.Writer) error {
	req, err := reauth.PluginReadFrame(r)

	if err != nil {
		logging.Errorf(ctx, "Failed to read plugin request: %v", err)
		writePluginError(w, errors.WrapIf(err, "ssh plugin local failure"))
		return err
	}

	conn, err := dialer.Dial(ctx)
	if err != nil {
		logging.Errorf(ctx, "Failed to dial upstream agent: %v", err)
		writePluginError(w, errors.WrapIf(err, "ssh plugin local failure"))
		return err
	}
	c := ssh.AgentClient{AgentConn: conn}
	defer c.Close()

	resp, err := c.SendExtensionRequest(
		ssh.AgentExtensionRequest{
			ExtensionType: reauth.SSHExtensionForwardedChallenge,
			ExtensionData: req,
		},
	)
	if err != nil {
		logging.Errorf(ctx, "Failed to complete SSH agent request: %v", err)
		writePluginError(w, errors.WrapIf(err, "ssh plugin upstream failure"))
		return err
	}

	if err := reauth.PluginWriteFrame(w, resp); err != nil {
		logging.Errorf(ctx, "Failed to write plugin response: %v", err)
		return errors.WrapIf(err, "ssh plugin local failure")
	}

	return nil
}

// Creates a webauthn.GetAssertionResponse that indicates an error.
func newPluginErrorResponse(err error) ([]byte, error) {
	return json.Marshal(webauthn.GetAssertionResponse{
		Type:  "getResponse",
		Error: err.Error(),
	})
}

func writePluginError(w io.Writer, err error) error {
	body, err := newPluginErrorResponse(err)

	if err != nil {
		return err
	}

	return reauth.PluginWriteFrame(w, body)
}
