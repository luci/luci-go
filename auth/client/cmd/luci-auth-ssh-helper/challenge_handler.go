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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"go.chromium.org/luci/auth/reauth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/ssh"
)

// A forwardingHandler forwards challenges to a subprocess handler.
type forwardingHandler struct {
}

func (forwardingHandler) pluginCmd() string {
	return os.Getenv("GOOGLE_AUTH_WEBAUTHN_PLUGIN")
}

func (h forwardingHandler) available() bool {
	return h.pluginCmd() != ""
}

func (h forwardingHandler) handle(ctx context.Context, payload []byte) ssh.AgentMessage {
	var input bytes.Buffer
	err := reauth.PluginWriteFrame(&input, payload)
	if err != nil {
		// Crash if writing to buffer fails (likely due to
		// insanely huge payload)
		panic(err)
	}
	output, err := h.send(ctx, &input)
	if err != nil {
		return ssh.AgentMessage{
			Code:    ssh.AgentExtensionFailure,
			Payload: fmt.Appendf(nil, "agent handler failure: %v", err),
		}
	}
	outPayload, err := reauth.PluginReadFrame(bytes.NewBuffer(output))
	if err != nil {
		return ssh.AgentMessage{
			Code:    ssh.AgentExtensionFailure,
			Payload: fmt.Appendf(nil, "agent handler read frame failure: %v", err),
		}
	}
	return ssh.AgentMessage{
		Code:    ssh.AgentSuccess,
		Payload: outPayload,
	}
}

// send sends bytes to the plugin command and returns the output.
func (h forwardingHandler) send(ctx context.Context, r io.Reader) ([]byte, error) {
	cmd := exec.CommandContext(ctx, h.pluginCmd())
	cmd.Stdin = r
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	logging.Debugf(ctx, "forwarded challenge handler stderr: %q", stderr.Bytes())
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			logging.Errorf(ctx, "forwarded challenge handler exit code: %v", exitErr.ExitCode())
			logging.Errorf(ctx, "forwarded challenge handler stderr: %q", stderr.Bytes())
		}
	}
	return out, err
}
