// Copyright 2015 The LUCI Authors.
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

package testsecrets

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/server/secrets"
)

func TestStore(t *testing.T) {
	ctx := context.Background()

	ftt.Run("RandomSecret", t, func(t *ftt.Test) {
		store := Store{}

		// Autogenerate one.
		s1, err := store.RandomSecret(ctx, "key1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s1, should.Resemble(secrets.Secret{
			Active: []byte{0xfa, 0x12, 0xf9, 0x2a, 0xfb, 0xe0, 0xf, 0x85},
		}))

		// Getting same one back.
		s2, err := store.RandomSecret(ctx, "key1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s2, should.Resemble(secrets.Secret{
			Active: []byte{0xfa, 0x12, 0xf9, 0x2a, 0xfb, 0xe0, 0xf, 0x85},
		}))
	})

	ftt.Run("StoredSecret", t, func(t *ftt.Test) {
		store := Store{
			Secrets: map[string]secrets.Secret{
				"key1": {Active: []byte("blah")},
			},
		}

		s, err := store.StoredSecret(ctx, "key1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.Resemble(secrets.Secret{Active: []byte("blah")}))

		_, err = store.StoredSecret(ctx, "key2")
		assert.Loosely(t, err, should.Equal(secrets.ErrNoSuchSecret))
	})
}
