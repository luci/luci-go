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

// Package oauthid implements OAuth client ID whitelist check.
package oauthid

import (
	"strings"

	"go.chromium.org/luci/common/data/stringset"
)

// Well-known OAuth client_id of https://apis-explorer.appspot.com/.
const GoogleAPIExplorerClientID = "292824132082.apps.googleusercontent.com"

// Whitelist is OAuth client ID whitelist.
type Whitelist struct {
	stringset.Set
}

// NewWhitelist creates new populated client ID whitelist.
func NewWhitelist(primaryID string, additionalIDs []string) Whitelist {
	wl := stringset.New(2 + len(additionalIDs))
	wl.Add(GoogleAPIExplorerClientID)
	if primaryID != "" {
		wl.Add(primaryID)
	}
	for _, id := range additionalIDs {
		if id != "" {
			wl.Add(id)
		}
	}
	return Whitelist{wl}
}

// IsAllowedOAuthClientID returns true if the given OAuth2 client ID can be used
// to authorize access from the given email.
func (wl Whitelist) IsAllowedOAuthClientID(email, clientID string) bool {
	switch {
	// No need to whitelist client IDs for service accounts, since email address
	// uniquely identifies credentials used. Note: this is Google specific.
	case strings.HasSuffix(email, ".gserviceaccount.com"):
		return true
	// clientID must be set for non service accounts.
	case clientID == "":
		return false
	default:
		return wl.Has(clientID)
	}
}
