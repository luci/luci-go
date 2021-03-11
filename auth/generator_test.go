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

	. "github.com/smartystreets/goconvey/convey"
)

func TestTokenGenerator(t *testing.T) {
	t.Parallel()

	Convey("With TokenGenerator", t, func() {
		ctx := context.Background()
		provider := &fakeTokenProvider{
			interactive: false,
		}

		gen := NewTokenGenerator(ctx, Options{
			Method:                   ServiceAccountMethod, // to allow arbitrary scopes and audience
			testingCache:             &internal.MemoryTokenCache{},
			testingBaseTokenProvider: provider,
		})

		Convey("GenerateOAuthToken", func() {
			tok, err := gen.GenerateOAuthToken(ctx, []string{"b", "a"}, time.Minute)
			So(err, ShouldBeNil)
			So(tok, ShouldNotBeNil)
			So(tok.AccessToken, ShouldEqual, "some minted access token")

			tok, err = gen.GenerateOAuthToken(ctx, []string{"a", "b"}, time.Minute)
			So(err, ShouldBeNil)
			So(tok, ShouldNotBeNil)

			email, err := gen.GetEmail()
			So(err, ShouldBeNil)
			So(email, ShouldEqual, "some-email-minttoken@example.com")

			// Created only one authenticator.
			So(gen.authenticators, ShouldHaveLength, 1)

			tok, err = gen.GenerateOAuthToken(ctx, []string{"a"}, time.Minute)
			So(err, ShouldBeNil)
			So(tok, ShouldNotBeNil)

			// Created one more.
			So(gen.authenticators, ShouldHaveLength, 2)
		})

		Convey("GenerateIDToken", func() {
			provider.useIDTokens = true

			tok, err := gen.GenerateIDToken(ctx, "aud_1", time.Minute)
			So(err, ShouldBeNil)
			So(tok, ShouldNotBeNil)
			So(tok.AccessToken, ShouldEqual, "some minted ID token")

			tok, err = gen.GenerateIDToken(ctx, "aud_1", time.Minute)
			So(err, ShouldBeNil)
			So(tok, ShouldNotBeNil)

			email, err := gen.GetEmail()
			So(err, ShouldBeNil)
			So(email, ShouldEqual, "some-email-minttoken@example.com")

			// Created only one authenticator.
			So(gen.authenticators, ShouldHaveLength, 1)

			tok, err = gen.GenerateIDToken(ctx, "aud_2", time.Minute)
			So(err, ShouldBeNil)
			So(tok, ShouldNotBeNil)

			// Created one more.
			So(gen.authenticators, ShouldHaveLength, 2)
		})

		Convey("GetEmail", func() {
			email, err := gen.GetEmail()
			So(err, ShouldBeNil)
			So(email, ShouldEqual, "some-email-minttoken@example.com")
		})
	})
}
