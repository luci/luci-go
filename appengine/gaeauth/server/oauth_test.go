// Copyright 2018 The LUCI Authors.
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

package server

import (
	"context"
	"testing"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestGetUserCredentials(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	m := OAuth2Method{}

	call := func(hdr string) (*oauth2.Token, error) {
		req := authtest.NewFakeRequestMetadata()
		req.FakeHeader.Add("Authorization", hdr)
		return m.GetUserCredentials(ctx, req)
	}

	ftt.Run("Works", t, func(t *ftt.Test) {
		tok, err := call("Bearer abc.def")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(&oauth2.Token{
			AccessToken: "abc.def",
			TokenType:   "Bearer",
		}))
	})

	ftt.Run("Bad headers", t, func(t *ftt.Test) {
		_, err := call("")
		assert.Loosely(t, err, should.Equal(errBadAuthHeader))
		_, err = call("abc.def")
		assert.Loosely(t, err, should.Equal(errBadAuthHeader))
		_, err = call("Basic abc.def")
		assert.Loosely(t, err, should.Equal(errBadAuthHeader))
	})
}
