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
	"encoding/json"
	"fmt"

	"go.chromium.org/luci/common/ssh"
	"go.chromium.org/luci/common/webauthn"
)

func forwardedChallengeHandler(payload []byte) ssh.AgentMessage {
	// TODO(b/415121564): Implement this and send payload to a local security
	// key handler.

	resp, err := json.Marshal(
		webauthn.GetAssertionResponse{
			Type:  "getResponse",
			Error: "not implemented",
		})

	if err != nil {
		return ssh.AgentMessage{
			Code:    ssh.AgentExtensionFailure,
			Payload: fmt.Appendf(nil, "agent handler failure: %v", err),
		}
	}

	return ssh.AgentMessage{
		Code:    ssh.AgentSuccess,
		Payload: resp,
	}
}
