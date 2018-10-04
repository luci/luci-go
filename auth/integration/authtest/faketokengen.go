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
	"context"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
)

// Defaults for FakeTokenGenerator.
const (
	DefaultFakeEmail    = "fake_test@example.com"
	DefaultFakeToken    = "fake_test_token"
	DefaultFakeLifetime = 5 * time.Minute
)

// FakeTokenGenerator implements localauth.TokenGenerator by returning fake
// data.
//
// Useful for integration tests that involve local auth server.
type FakeTokenGenerator struct {
	Email    string        // email of the default account (default "fake_test@example.com")
	Token    string        // access token to return (default "fake_test_token")
	Lifetime time.Duration // lifetime of the returned token (default 5 min)
}

// GenerateToken is part of TokenGenerator interface.
func (f *FakeTokenGenerator) GenerateToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	token := f.Token
	if token == "" {
		token = DefaultFakeToken
	}
	tokenLifetime := f.Lifetime
	if tokenLifetime == 0 {
		tokenLifetime = DefaultFakeLifetime
	}
	return &oauth2.Token{
		AccessToken: token,
		Expiry:      clock.Now(ctx).Add(tokenLifetime),
	}, nil
}

// GetEmail is part of TokenGenerator interface.
func (f *FakeTokenGenerator) GetEmail() (string, error) {
	if f.Email == "" {
		return DefaultFakeEmail, nil
	}
	return f.Email, nil
}
