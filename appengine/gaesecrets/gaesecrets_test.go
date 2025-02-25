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

package gaesecrets

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
)

func TestWorks(t *testing.T) {
	ftt.Run("gaesecrets.Store works", t, func(t *ftt.Test) {
		c := Use(memory.Use(context.Background()), nil)
		c = caching.WithEmptyProcessCache(c)

		// Autogenerates one.
		s1, err := secrets.RandomSecret(c, "key1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(s1.Active), should.Equal(32))

		// Returns same one.
		s2, err := secrets.RandomSecret(c, "key1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s2, should.Match(s1))

		// Can also be fetched as stored.
		s3, err := secrets.StoredSecret(c, "key1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s3, should.Match(s1))

		// Missing stored secrets are not auto-generated though.
		_, err = secrets.StoredSecret(c, "key2")
		assert.Loosely(t, err, should.Equal(secrets.ErrNoSuchSecret))
	})
}
