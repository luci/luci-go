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

	"go.chromium.org/luci/auth/reauth"
	"go.chromium.org/luci/common/ssh"
)

// A forwardingHandler forwards challenges to subprocess plugin for signing.
type forwardingHandler struct {
	reauth.PluginIO
}

func (h forwardingHandler) handle(ctx context.Context, payload []byte) ssh.AgentMessage {
	var input bytes.Buffer
	err := reauth.PluginWriteFrame(&input, payload)
	if err != nil {
		// Crash if writing to buffer fails (likely due to
		// insanely huge payload)
		panic(err)
	}
	output, err := h.Send(ctx, &input)
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
