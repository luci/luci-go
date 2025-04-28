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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDiskTokenCache(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	ctx := context.Background()
	ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC.Local())

	ftt.Run("DiskTokenCache works", t, func(t *ftt.Test) {
		// testCacheSemantics is in proc_cache_test.go.
		testCacheSemantics(ctx, &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		}, t)
	})

	ftt.Run("Retains unknown cacheFileEntry fields", t, func(t *ftt.Test) {
		cacheFile := filepath.Join(tmp, "tokens.json")
		testData := `
		{
			"cache": [
				{
					"key": {"key": "a"},
					"token": {
						"access_token": "abc"
					},
					"email": "a@example.com",
					"random_stuff": {"abc": {"def": "zzz"}},
					"abc": "def"
				}
			],
			"last_update": "2021-02-08T23:18:00.463912Z"
		}
		`
		assert.Loosely(t, os.WriteFile(cacheFile, []byte(testData), 0600), should.BeNil)

		cache := &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		}

		tok, err := cache.GetToken(&CacheKey{Key: "a"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.Resemble(&Token{
			Token: oauth2.Token{AccessToken: "abc"},
			Email: "a@example.com",
		}))
		assert.Loosely(t, cache.PutToken(&CacheKey{Key: "a"}, &Token{
			Token: oauth2.Token{
				AccessToken: "def",
				Expiry:      clock.Now(ctx).Add(time.Hour).UTC(),
			},
			Email: "a@example.com",
		}), should.BeNil)

		// Check "random_stuff" and "abc" were preserved by the update.
		blob, err := os.ReadFile(cacheFile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(blob), should.Equal(`{
  "cache": [
    {
      "key": {
        "key": "a"
      },
      "token": {
        "access_token": "def",
        "expiry": "2016-02-03T05:05:06.000000007Z"
      },
      "id_token": "",
      "email": "a@example.com",
      "last_update": "2016-02-03T04:05:06.000000007Z",
      "abc": "def",
      "random_stuff": {
        "abc": {
          "def": "zzz"
        }
      }
    }
  ],
  "last_update": "2016-02-03T04:05:06.000000007Z"
}`))
	})

	ftt.Run("Merges creds.json and tokens.json", t, func(t *ftt.Test) {
		oldCacheFile := filepath.Join(tmp, "creds.json")
		oldCacheFileData := `
		{
			"cache": [
				{
					"key": {"key": "a"},
					"token": {
						"access_token": "abc",
						"expiry": "2016-02-03T07:00:00Z"
					},
					"email": "a@example.com",
					"last_update": "2016-02-03T05:00:00Z"
				},
				{
					"key": {"key": "b"},
					"token": {
						"access_token": "def",
						"expiry": "2016-02-03T07:00:00Z"
					},
					"email": "a@example.com",
					"last_update": "2016-02-03T05:00:00Z"
				}
			],
			"last_update": "2016-02-03T05:00:00Z"
		}
		`

		newCacheFile := filepath.Join(tmp, "tokens.json")
		newCacheFileData := `
		{
			"cache": [
				{
					"key": {"key": "a"},
					"token": {
						"access_token": "better-abc",
						"expiry": "2016-02-03T07:00:00Z"
					},
					"email": "a@example.com",
					"last_update": "2016-02-03T06:00:00Z",
					"extra": "zzz"
				},
				{
					"key": {"key": "c"},
					"token": {
						"access_token": "zzz",
						"expiry": "2016-02-03T07:00:00Z"
					},
					"email": "a@example.com",
					"last_update": "2016-02-03T06:00:00Z"
				}
			],
			"last_update": "2016-02-03T06:00:00Z"
		}
		`

		assert.Loosely(t, os.WriteFile(oldCacheFile, []byte(oldCacheFileData), 0600), should.BeNil)
		assert.Loosely(t, os.WriteFile(newCacheFile, []byte(newCacheFileData), 0600), should.BeNil)

		cache := &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		}

		tok, err := cache.GetToken(&CacheKey{Key: "a"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.Token.AccessToken, should.Equal("better-abc"))
		assert.Loosely(t, cache.PutToken(&CacheKey{Key: "a"}, &Token{
			Token: oauth2.Token{
				AccessToken: "xyz",
				Expiry:      clock.Now(ctx).Add(time.Hour).UTC(),
			},
			Email: "a@example.com",
		}), should.BeNil)

		updatedOld, err := os.ReadFile(oldCacheFile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(updatedOld), should.Equal(`{
  "cache": [
    {
      "key": {
        "key": "a"
      },
      "token": {
        "access_token": "xyz",
        "expiry": "2016-02-03T05:05:06.000000007Z"
      },
      "id_token": "",
      "email": "a@example.com",
      "last_update": "2016-02-03T04:05:06.000000007Z",
      "extra": "zzz"
    },
    {
      "key": {
        "key": "c"
      },
      "token": {
        "access_token": "zzz",
        "expiry": "2016-02-03T07:00:00Z"
      },
      "id_token": "",
      "email": "a@example.com",
      "last_update": "2016-02-03T06:00:00Z"
    },
    {
      "key": {
        "key": "b"
      },
      "token": {
        "access_token": "def",
        "expiry": "2016-02-03T07:00:00Z"
      },
      "id_token": "",
      "email": "a@example.com",
      "last_update": "2016-02-03T05:00:00Z"
    }
  ],
  "last_update": "2016-02-03T05:00:00Z"
}`))

		// tokens.json is almost identical, except last_update is newer.
		updatedNew, err := os.ReadFile(newCacheFile)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, string(updatedNew), should.Equal(`{
  "cache": [
    {
      "key": {
        "key": "a"
      },
      "token": {
        "access_token": "xyz",
        "expiry": "2016-02-03T05:05:06.000000007Z"
      },
      "id_token": "",
      "email": "a@example.com",
      "last_update": "2016-02-03T04:05:06.000000007Z",
      "extra": "zzz"
    },
    {
      "key": {
        "key": "c"
      },
      "token": {
        "access_token": "zzz",
        "expiry": "2016-02-03T07:00:00Z"
      },
      "id_token": "",
      "email": "a@example.com",
      "last_update": "2016-02-03T06:00:00Z"
    },
    {
      "key": {
        "key": "b"
      },
      "token": {
        "access_token": "def",
        "expiry": "2016-02-03T07:00:00Z"
      },
      "id_token": "",
      "email": "a@example.com",
      "last_update": "2016-02-03T05:00:00Z"
    }
  ],
  "last_update": "2016-02-03T04:05:06.000000007Z"
}`))
	})

	// TODO(vadimsh): This test is flaky on Windows, there's non zero probability
	// that all 15 attempts (see testCacheInParallel) will hit "Access is denied"
	// error. This can be "fixed" by increasing number of attempts or sleeping
	// more between attempts. Both increase test runtime.
	ftt.Run("DiskTokenCache works (parallel)", t, func(t *ftt.Test) {
		t.Skip("flaky on Windows")
		// testCacheInParallel is in proc_cache_test.go.
		//
		// Use real clock here to test real-world interaction when retrying disk
		// writes.
		ctx := context.Background()
		testCacheInParallel(ctx, &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		}, t)
	})

	ftt.Run("Cleans up old tokens", t, func(t *ftt.Test) {
		cache := &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		}

		cache.PutToken(&CacheKey{Key: "a"}, &Token{
			Token: oauth2.Token{
				AccessToken: "abc",
				Expiry:      clock.Now(ctx),
			},
		})
		cache.PutToken(&CacheKey{Key: "b"}, &Token{
			Token: oauth2.Token{
				AccessToken:  "abc",
				RefreshToken: "def",
				Expiry:       clock.Now(ctx),
			},
		})

		// GCAccessTokenMaxAge later, "a" is gone while the cache is updated.
		tc.Add(GCAccessTokenMaxAge)
		unused := &Token{
			Token: oauth2.Token{
				AccessToken: "zzz",
				Expiry:      clock.Now(ctx).Add(365 * 24 * time.Hour),
			},
		}
		cache.PutToken(&CacheKey{Key: "unused"}, unused)

		tok, err := cache.GetToken(&CacheKey{Key: "a"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.BeNil)

		// "b" is still there.
		tok, err = cache.GetToken(&CacheKey{Key: "b"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.RefreshToken, should.Equal("def"))

		// Some time later "b" is also removed.
		tc.Add(GCRefreshTokenMaxAge)
		cache.PutToken(&CacheKey{Key: "unused"}, unused)

		tok, err = cache.GetToken(&CacheKey{Key: "b"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok, should.BeNil)
	})

	ftt.Run("metadata round trip", t, func(t *ftt.Test) {
		cache := &DiskTokenCache{
			Context:    ctx,
			SecretsDir: tmp,
		}

		err := cache.PutToken(&CacheKey{Key: "a"}, &Token{
			Token: oauth2.Token{
				AccessToken: "abc",
				Expiry:      clock.Now(ctx),
			},
			Metadata: map[string]json.RawMessage{
				"nahi": json.RawMessage(`"teri"`),
			},
		})
		assert.Loosely(t, err, should.BeNil)
		tok, err := cache.GetToken(&CacheKey{Key: "a"})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, tok.Metadata, should.Match(map[string]json.RawMessage{
			"nahi": json.RawMessage(`"teri"`),
		}))
	})
}
