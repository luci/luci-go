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
	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := srv.Serve(); err != nil {
			logging.WithError(err).Errorf(ctx, "Unexpected error in the fake local auth server loop")
		}
	}()

	// Make it seen to the world.
	f.srv = srv
	f.done = done

	logging.Debugf(ctx, "The fake local auth server is at http://127.0.0.1:%d", localAuth.RPCPort)
	return lucictx.SetLocalAuth(ctx, localAuth), nil
}

// Stop stops the local auth server if it is running.
func (f *FakeContext) Stop(ctx context.Context) {
	if f.srv == nil {
		return
	}

	// Gracefully stop the server.
	logging.Debugf(ctx, "Stopping the fake local auth server...")
	if err := f.srv.Close(); err != nil {
		logging.WithError(err).Warningf(ctx, "Failed to close the fake local auth server")
	}

	// Wait for it to really die. Should be fast. Limit by timeout just in case.
	ctx, cancel := clock.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	select {
	case <-f.done:
		logging.Debugf(ctx, "The fake local auth server stopped")
	case <-ctx.Done():
		logging.WithError(ctx.Err()).Warningf(ctx, "Giving up waiting for the fake local auth server to stop")
	}

	// Cleanup the state no matter what.
	f.srv = nil
	f.done = nil
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
