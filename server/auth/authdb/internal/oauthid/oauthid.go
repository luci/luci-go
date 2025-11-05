// Copyright 2019 The LUCI Authors.
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

// Package oauthid implements OAuth client ID allowlist check.
package oauthid

import (
	"go.chromium.org/luci/common/data/stringset"
)

// GoogleAPIExplorerClientID is the well-known OAuth client_id of
// https://apis-explorer.appspot.com/.
const GoogleAPIExplorerClientID = "292824132082.apps.googleusercontent.com"

// Allowlist is OAuth client ID allowlist.
type Allowlist struct {
	stringset.Set
}

// NewAllowlist creates new populated client ID allowlist.
func NewAllowlist(primaryID string, additionalIDs []string) Allowlist {
	l := stringset.New(2 + len(additionalIDs))
	l.Add(GoogleAPIExplorerClientID)
	if primaryID != "" {
		l.Add(primaryID)
	}
	for _, id := range additionalIDs {
		if id != "" {
			l.Add(id)
		}
	}
	return Allowlist{l}
}

// IsAllowedOAuthClientID returns true if the given OAuth2 client ID can be used
// to authorize access from the given email.
func (l Allowlist) IsAllowedOAuthClientID(email, clientID string) bool {
	return l.Has(clientID)
}
