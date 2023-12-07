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
	"context"
	"net/http"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
)

// MockAuthConfig configures the auth library for unit tests environment.
//
// You need this *only* if your tests call auth.Authenticate(...) or
// auth.GetRPCTransport(...). If your tests only check groups or permissions
// (for example when testing bodies of request handlers), use FakeState instead.
// See its docs for some examples.
func MockAuthConfig(ctx context.Context, mocks ...MockedDatum) context.Context {
	return auth.ModifyConfig(ctx, func(cfg auth.Config) auth.Config {
		fakeDB := NewFakeDB(mocks...)
		cfg.DBProvider = func(context.Context) (authdb.DB, error) {
			return fakeDB, nil
		}
		cfg.AnonymousTransport = func(context.Context) http.RoundTripper {
			return http.DefaultTransport
		}
		cfg.AccessTokenProvider = func(ctx context.Context, scopes []string) (*oauth2.Token, error) {
			return &oauth2.Token{
				AccessToken: "fake_token",
				TokenType:   "Bearer",
				Expiry:      clock.Now(ctx).Add(time.Hour).UTC(),
			}, nil
		}
		return cfg
	})
}
