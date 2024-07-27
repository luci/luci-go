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

package internal

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestMemoryTokenCache(t *testing.T) {
	t.Parallel()

	ftt.Run("MemoryTokenCache works", t, func(t *ftt.Test) {
		testCacheSemantics(context.Background(), &MemoryTokenCache{}, t)
	})

	ftt.Run("MemoryTokenCache works (parallel)", t, func(t *ftt.Test) {
		testCacheInParallel(context.Background(), &MemoryTokenCache{}, t)
	})
}

func testCacheSemantics(ctx context.Context, cache TokenCache, t testing.TB) {
	// Missing is fine.
	tok, err := cache.GetToken(&CacheKey{Key: "missing"})
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, tok, should.BeNil)

	// Deleting missing is fine.
	assert.Loosely(t, cache.DeleteToken(&CacheKey{Key: "missing"}), should.BeNil)

	// Put -> Get.
	tok1 := &Token{
		Token: oauth2.Token{
			AccessToken: "abc",
			Expiry:      clock.Now(ctx).Add(time.Hour).UTC(),
		},
		Email: "some@example.com",
	}
	assert.Loosely(t, cache.PutToken(&CacheKey{Key: "k"}, tok1), should.BeNil)
	tok, err = cache.GetToken(&CacheKey{Key: "k"})
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, tok, should.Resemble(tok1))

	// Put a bunch more.
	for i := 0; i < 5; i++ {
		assert.Loosely(t, cache.PutToken(&CacheKey{Key: fmt.Sprintf("k%d", i)}, tok1), should.BeNil)
	}

	// Overwrite -> Get.
	tok2 := &Token{
		Token: oauth2.Token{
			AccessToken: "def",
			Expiry:      clock.Now(ctx).Add(2 * time.Hour).UTC(),
		},
		Email: "another@example.com",
	}
	assert.Loosely(t, cache.PutToken(&CacheKey{Key: "k"}, tok2), should.BeNil)
	tok, err = cache.GetToken(&CacheKey{Key: "k"})
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, tok, should.Resemble(tok2))

	// Delete -> Get.
	assert.Loosely(t, cache.DeleteToken(&CacheKey{Key: "k"}), should.BeNil)
	tok, err = cache.GetToken(&CacheKey{Key: "k"})
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, tok, should.BeNil)

	// The rest is still there. Remove it.
	for i := 0; i < 5; i++ {
		k := &CacheKey{Key: fmt.Sprintf("k%d", i)}
		tok, err = cache.GetToken(k)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(tok1))
		assert.Loosely(t, cache.DeleteToken(k), should.BeNil)
	}
}

func testCacheInParallel(ctx context.Context, cache TokenCache, t testing.TB) {
	tok := &Token{
		Token: oauth2.Token{
			AccessToken: "zzz",
			Expiry:      clock.Now(ctx).Add(365 * 24 * time.Hour),
		},
	}

	const (
		threads  = 30
		attempts = 15
	)

	// There are no guarantees in general that parallel writes to the cache will
	// not lose some data (see DiskTokenCache comments). But each op should be
	// successful, even if its results are later silently discarded.
	wg := sync.WaitGroup{}
	errs := make(chan error, threads*attempts)
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < attempts; j++ {
				err := cache.PutToken(&CacheKey{Key: fmt.Sprintf("%d", j)}, tok)
				if err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()

	// The channel with errors should be empty.
	select {
	case err := <-errs:
		assert.Loosely(t, err, should.BeNil)
	default:
	}
}
