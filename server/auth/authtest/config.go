// Copyright 2016 The LUCI Authors.
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

package authtest

import (
	"net/http"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/auth"
)

// MockAuthConfig configures auth library for unit tests environment.
//
// If modifies the configure stored in the context. See auth.SetConfig for more
// info.
func MockAuthConfig(c context.Context) context.Context {
	return auth.ModifyConfig(c, func(cfg auth.Config) auth.Config {
		cfg.AnonymousTransport = func(context.Context) http.RoundTripper {
			return http.DefaultTransport
		}
		cfg.AccessTokenProvider = func(ic context.Context, scopes []string) (*oauth2.Token, error) {
			return &oauth2.Token{
				AccessToken: "fake_token",
				TokenType:   "Bearer",
				Expiry:      clock.Now(ic).Add(time.Hour).UTC(),
			}, nil
		}
		return cfg
	})
}
