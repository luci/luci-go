// Copyright 2017 The LUCI Authors.
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
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/auth/localauth"
	"go.chromium.org/luci/common/clock"
)

// Defaults for FakeContext.
const (
	DefaultFakeEmail    = "fake_test@example.com"
	DefaultFakeToken    = "fake_test_token"
	DefaultFakeLifetime = 5 * time.Minute
)

// FakeContext can be used to setup local auth server that serves fake tokens.
//
// Useful to simulate auth context in integration tests.
type FakeContext struct {
	Email    string        // email of the default account (default "fake_test@example.com")
	Token    string        // access token to return (default "fake_test_token")
	Lifetime time.Duration // lifetime of the returned token (default 5 min)
}

// Use launches the local auth server, adds it to LUCI_CONTEXT and calls the
// callback.
//
// If you going to call subprocesses, make sure to export LUCI_CONTEXT into
// their environment via lucictx.Export(...).SetInCmd(...).
func (f *FakeContext) Use(ctx context.Context, cb func(context.Context) error) error {
	if f.Email == "" {
		f.Email = DefaultFakeEmail
	}
	if f.Token == "" {
		f.Token = DefaultFakeToken
	}
	if f.Lifetime == 0 {
		f.Lifetime = DefaultFakeLifetime
	}
	srv := &localauth.Server{
		TokenGenerators: map[string]localauth.TokenGenerator{
			"authtest": fakeGenerator{f},
		},
		DefaultAccountID: "authtest",
	}
	return localauth.WithLocalAuth(ctx, srv, cb)
}

type fakeGenerator struct {
	*FakeContext
}

func (f fakeGenerator) GenerateToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	return &oauth2.Token{
		AccessToken: f.Token,
		Expiry:      clock.Now(ctx).Add(f.Lifetime),
	}, nil
}

func (f fakeGenerator) GetEmail() (string, error) {
	return f.Email, nil
}
