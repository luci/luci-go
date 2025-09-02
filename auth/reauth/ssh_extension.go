// Copyright 2024 The LUCI Authors.
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

package reauth

// Name of SSH_AUTH_SOCK environment variable.
const EnvSSHAuthSock = "SSH_AUTH_SOCK"

// SSH extension names.
const (
	// Message is for checking if the receiving agent is a LUCI agent.
	SSHExtensionPing = "ping@reauth.luci.go.chromium.org"

	// Message is a forwarded WebAuthn challenge.
	SSHExtensionForwardedChallenge = "forwarded-challenge@reauth.luci.go.chromium.org"
)
