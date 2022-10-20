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

package internal

import (
	"context"
	"net/http"
)

// Features here can be disabled during the compilation by excluding
// corresponding *.go files (e.g. via `-tags copybara`). Can be used to avoid
// undesirable compile-time dependencies.

var (
	// NewLUCITSTokenProvider returns TokenProvider that uses a LUCI Token Server
	// to grab tokens belonging to some service account.
	//
	// Implemented in luci_ts.go.
	NewLUCITSTokenProvider func(ctx context.Context, host, actAs, realm string, scopes []string, audience string, transport http.RoundTripper) (TokenProvider, error)

	// NewLoginSessionTokenProvider returns TokenProvider that can perform
	// a user-interacting login flow that involves a LoginSessions service.
	NewLoginSessionTokenProvider func(ctx context.Context, loginSessionsHost, clientID, clientSecret string, scopes []string, transport http.RoundTripper) (TokenProvider, error)
)
