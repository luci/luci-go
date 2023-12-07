// Copyright 2020 The LUCI Authors.
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

package limiter

import (
	"context"

	"go.chromium.org/luci/auth/identity"

	"go.chromium.org/luci/server/auth"
)

// PeerLabelFromAuthState looks at the auth.State in the context and derives
// a peer label from it.
//
// Currently returns one of "unknown", "anonymous", "authenticated".
//
// TODO(vadimsh): Have a small group with an allowlist of identities that are OK
// to use as peer label directly.
func PeerLabelFromAuthState(ctx context.Context) string {
	if s := auth.GetState(ctx); s != nil {
		if s.PeerIdentity() == identity.AnonymousIdentity {
			return "anonymous"
		}
		return "authenticated"
	}
	return "unknown"
}
