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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
)

func TestBootstrapToken(t *testing.T) {
	t.Parallel()

	const testIdent = identity.Identity("user:someone@example.com")

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx = caching.WithEmptyProcessCache(ctx)

		err := datastore.Put(ctx, &LegacyBootstrapSecret{
			Key:    LegacyBootstrapSecretKey(ctx),
			Values: [][]byte{{0, 1, 2, 3, 4}},
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run("Legacy OK", func(t *ftt.Test) {
			tok, err := GenerateBootstrapToken(ctx, testIdent)
			assert.Loosely(t, err, should.BeNil)

			ident, err := ValidateBootstrapToken(ctx, tok)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, ident, should.Equal(testIdent))
		})

		t.Run("Legacy err", func(t *ftt.Test) {
			tok, err := GenerateBootstrapToken(ctx, testIdent)
			assert.Loosely(t, err, should.BeNil)

			modified := "AAAA" + tok[4:]

			_, err = ValidateBootstrapToken(ctx, modified)
			assert.Loosely(t, err, should.NotBeNil)
		})
	})
}
