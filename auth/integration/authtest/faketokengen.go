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
	"fmt"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
)

// Defaults for FakeTokenGenerator.
const (
	DefaultFakeEmail    = "fake_test@example.com"
	DefaultFakeLifetime = 5 * time.Minute
)

// FakeTokenGenerator implements localauth.TokenGenerator by returning fake
// data.
//
// Useful for integration tests that involve local auth server.
//
// Each GenerateToken call returns a token "fake_token_<N>", where N starts
// from 0 and incremented for each call. If KeepRecord is true, each generated
// token is recorded along with a list of scopes that were used to generate it.
// Use TokenScopes() to see what scopes have been used to generate a particular
// token.
type FakeTokenGenerator struct {
	Email      string        // email of the default account (default "fake_test@example.com")
	Lifetime   time.Duration // lifetime of the returned token (default 5 min)
	KeepRecord bool          // if true, record all generated tokens

	m    sync.Mutex
	n    int
	toks map[string][]string // fake token => list of its scopes
}

// GenerateToken is part of TokenGenerator interface.
func (f *FakeTokenGenerator) GenerateToken(ctx context.Context, scopes []string, lifetime time.Duration) (*oauth2.Token, error) {
	f.m.Lock()
	defer f.m.Unlock()

	token := fmt.Sprintf("fake_token_%d", f.n)
	f.n++
	if f.KeepRecord {
		if f.toks == nil {
			f.toks = make(map[string][]string, 1)
		}
		f.toks[token] = append([]string(nil), scopes...)
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

// TokenScopes returns scopes that were used to generate given fake token.
//
// Returns nil for unknown tokens or if KeepRecord is false and tokens weren't
// recorded.
func (f *FakeTokenGenerator) TokenScopes(token string) []string {
	f.m.Lock()
	defer f.m.Unlock()
	return f.toks[token]
}
