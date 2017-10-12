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
	"fmt"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/auth/localauth"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/lucictx"
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

	srv    *localauth.Server
	done   chan struct{}
	cancel func()
}

// Start launches the local auth server and adds it to LUCI_CONTEXT, returning
// the resulting derived context.Context.
//
// If you are going to call subprocesses, make sure to export LUCI_CONTEXT into
// their environment via lucictx.Export(...).SetInCmd(...).
//
// Make sure to call Stop to stop the server.
func (f *FakeContext) Start(ctx context.Context) (context.Context, error) {
	if f.srv != nil {
		return nil, fmt.Errorf("already running")
	}

	// Fill in defaults.
	if f.Email == "" {
		f.Email = DefaultFakeEmail
	}
	if f.Token == "" {
		f.Token = DefaultFakeToken
	}
	if f.Lifetime == 0 {
		f.Lifetime = DefaultFakeLifetime
	}

	// Bind to the port, prepare "local_auth" section of new LUCI_CONTEXT.
	srv := &localauth.Server{
		TokenGenerators: map[string]localauth.TokenGenerator{
			"authtest": fakeGenerator{f},
		},
		DefaultAccountID: "authtest",
	}
	localAuth, err := srv.Initialize(ctx)
	if err != nil {
		return nil, err
	}

	// Launch the auth server in a background goroutine.
	if err := srv.Start(); err != nil {
		srv.Close(ctx) // close the listening socket
		return nil, err
	}

	// Make it seen to the world.
	f.srv = srv

	logging.Debugf(ctx, "The fake local auth server is at http://127.0.0.1:%d", localAuth.RPCPort)
	return lucictx.SetLocalAuth(ctx, localAuth), nil
}

// Stop stops the local auth server if it is running.
func (f *FakeContext) Stop(ctx context.Context) {
	if f.srv != nil {
		f.srv.Close(ctx)
		f.srv = nil
	}
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
