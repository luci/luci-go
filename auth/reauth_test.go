// Copyright 2025 The LUCI Authors.
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
	"net/http"
	"testing"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/internal"
	"go.chromium.org/luci/auth/reauth"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestRAPT(t *testing.T) {
	t.Parallel()

	ftt.Run("Test refresh and get", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
		}
		raptProvider := &stubRAPTProvider{
			rapt: &reauth.RAPT{
				Token:  "nahinahi",
				Expiry: future,
			},
		}
		auth, ctx := newAuth(InteractiveLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      future,
			},
		})
		ra := ReAuthenticator{
			Authenticator: auth,
			provider:      raptProvider.Get,
		}

		_, err := ra.GetRAPT(ctx)
		assert.Loosely(t, err, should.NotBeNil)

		err = ra.RenewRAPT(ctx)
		assert.Loosely(t, err, should.BeNil)

		got, err := ra.GetRAPT(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, got, should.Equal("nahinahi"))
	})

	ftt.Run("Test get expired", t, func(t *ftt.Test) {
		tokenProvider := &fakeTokenProvider{
			interactive: false,
		}
		raptProvider := &stubRAPTProvider{
			rapt: &reauth.RAPT{
				Token:  "nahinahi",
				Expiry: past,
			},
		}
		auth, ctx := newAuth(InteractiveLogin, tokenProvider, nil, "")
		cacheToken(auth, tokenProvider, &internal.Token{
			Token: oauth2.Token{
				AccessToken: "cached",
				Expiry:      future,
			},
		})
		ra := ReAuthenticator{
			Authenticator: auth,
			provider:      raptProvider.Get,
		}

		_, err := ra.GetRAPT(ctx)
		assert.Loosely(t, err, should.NotBeNil)

		err = ra.RenewRAPT(ctx)
		assert.Loosely(t, err, should.BeNil)

		_, err = ra.GetRAPT(ctx)
		assert.Loosely(t, err, should.NotBeNil)
	})
}

type stubRAPTProvider struct {
	rapt *reauth.RAPT
	err  error
}

func (p *stubRAPTProvider) Get(ctx context.Context, c *http.Client) (*reauth.RAPT, error) {
	return p.rapt, p.err
}
