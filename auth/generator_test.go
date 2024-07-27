// Copyright 2021 The LUCI Authors.
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

package auth

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/auth/internal"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTokenGenerator(t *testing.T) {
	t.Parallel()

	ftt.Run("With TokenGenerator", t, func(t *ftt.Test) {
		ctx := context.Background()
		provider := &fakeTokenProvider{
			interactive: false,
		}

		gen := NewTokenGenerator(ctx, Options{
			Method:                   ServiceAccountMethod, // to allow arbitrary scopes and audience
			testingCache:             &internal.MemoryTokenCache{},
			testingBaseTokenProvider: provider,
		})

		t.Run("GenerateOAuthToken", func(t *ftt.Test) {
			tok, err := gen.GenerateOAuthToken(ctx, []string{"b", "a"}, time.Minute)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.NotBeNil)
			assert.Loosely(t, tok.AccessToken, should.Equal("some minted access token"))

			tok, err = gen.GenerateOAuthToken(ctx, []string{"a", "b"}, time.Minute)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.NotBeNil)

			email, err := gen.GetEmail()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, email, should.Equal("some-email-minttoken@example.com"))

			// Created only one authenticator.
			assert.Loosely(t, gen.authenticators, should.HaveLength(1))

			tok, err = gen.GenerateOAuthToken(ctx, []string{"a"}, time.Minute)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.NotBeNil)

			// Created one more.
			assert.Loosely(t, gen.authenticators, should.HaveLength(2))
		})

		t.Run("GenerateIDToken", func(t *ftt.Test) {
			provider.useIDTokens = true

			tok, err := gen.GenerateIDToken(ctx, "aud_1", time.Minute)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.NotBeNil)
			assert.Loosely(t, tok.AccessToken, should.Equal("some minted ID token"))

			tok, err = gen.GenerateIDToken(ctx, "aud_1", time.Minute)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.NotBeNil)

			email, err := gen.GetEmail()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, email, should.Equal("some-email-minttoken@example.com"))

			// Created only one authenticator.
			assert.Loosely(t, gen.authenticators, should.HaveLength(1))

			tok, err = gen.GenerateIDToken(ctx, "aud_2", time.Minute)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tok, should.NotBeNil)

			// Created one more.
			assert.Loosely(t, gen.authenticators, should.HaveLength(2))
		})

		t.Run("GetEmail", func(t *ftt.Test) {
			email, err := gen.GetEmail()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, email, should.Equal("some-email-minttoken@example.com"))
		})
	})
}
