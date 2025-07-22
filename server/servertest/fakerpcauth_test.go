// Copyright 2024 The LUCI Authors.
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

package servertest

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestFakeRPCAuthTokens(t *testing.T) {
	t.Parallel()

	gen := FakeRPCTokenGenerator{
		Seed:  123,
		Email: "abc@example.com",
	}
	check := FakeRPCAuth{
		Seed:             123,
		IDTokensAudience: stringset.NewFromSlice("good-aud"),
	}

	now := time.Date(2044, time.April, 4, 4, 4, 4, 0, time.UTC)
	ctx, _ := testclock.UseTime(context.Background(), now)

	call := func(header string) (*auth.User, error) {
		md := authtest.NewFakeRequestMetadata()
		if header != "" {
			md.FakeHeader.Set("Authorization", header)
		}
		user, _, err := check.Authenticate(ctx, md)
		return user, err
	}

	t.Run("GenerateOAuthToken", func(t *testing.T) {
		tok, err := gen.GenerateOAuthToken(ctx, []string{"a", "b"}, 0)
		assert.NoErr(t, err)

		user, err := call("Bearer " + tok.AccessToken)
		assert.NoErr(t, err)
		assert.That(t, user, should.Match(&auth.User{
			Identity: "user:abc@example.com",
			Email:    "abc@example.com",
			Extra: &FakeToken{
				Kind:   "oauth",
				Email:  "abc@example.com",
				Expiry: now.Unix() + 15*60,
				Seed:   123,
				Scopes: []string{"a", "b"},
			},
		}))
	})

	t.Run("GenerateIDToken", func(t *testing.T) {
		tok, err := gen.GenerateIDToken(ctx, "good-aud", 0)
		assert.NoErr(t, err)

		user, err := call("Bearer " + tok.AccessToken)
		assert.NoErr(t, err)
		assert.That(t, user, should.Match(&auth.User{
			Identity: "user:abc@example.com",
			Email:    "abc@example.com",
			Extra: &FakeToken{
				Kind:   "id",
				Email:  "abc@example.com",
				Expiry: now.Unix() + 15*60,
				Seed:   123,
				Aud:    "good-aud",
			},
		}))
	})

	t.Run("No header", func(t *testing.T) {
		user, err := call("")
		assert.NoErr(t, err)
		assert.Loosely(t, user, should.BeNil)
	})

	t.Run("Malformed header", func(t *testing.T) {
		badHeaders := []string{
			"not-bearer zzz",
			"bearer",
			"bearer AAAA AAAA",
			"bearer notbase64",
			"bearer AAAA", // not JSON
		}
		for _, hdr := range badHeaders {
			_, err := call(hdr)
			assert.That(t, grpcutil.Code(err), should.Equal(codes.Unauthenticated))
		}
	})

	t.Run("Wrong seed", func(t *testing.T) {
		gen := FakeRPCTokenGenerator{
			Seed:  111,
			Email: "abc@example.com",
		}

		tok, err := gen.GenerateOAuthToken(ctx, []string{"a", "b"}, 0)
		assert.NoErr(t, err)

		_, err = call("Bearer " + tok.AccessToken)
		assert.That(t, err, should.ErrLike("unexpected seed"))
	})

	t.Run("Wrong audience", func(t *testing.T) {
		tok, err := gen.GenerateIDToken(ctx, "bad-aud", 0)
		assert.NoErr(t, err)

		_, err = call("Bearer " + tok.AccessToken)
		assert.That(t, err, should.ErrLike("unexpected ID token audience"))
	})

	t.Run("Expiry", func(t *testing.T) {
		ctx, _ := testclock.UseTime(context.Background(), now.Add(-20*time.Minute))
		tok, err := gen.GenerateOAuthToken(ctx, []string{"a", "b"}, 0)
		assert.NoErr(t, err)

		_, err = call("Bearer " + tok.AccessToken)
		assert.That(t, err, should.ErrLike("fake token expired 5m0s ago"))
	})
}
