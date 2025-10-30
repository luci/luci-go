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

package gerrit

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestReverse(t *testing.T) {
	t.Parallel()
	s := []int{1, 2, 3, 4, 5}
	var got []int
	for _, v := range reverse(s) {
		got = append(got, v)
	}
	assert.That(t, got, should.Match([]int{5, 4, 3, 2, 1}))
}

func testAllCacheImpls(t *testing.T, testfunc func(*testing.T, ReAuthResultCache)) {
	t.Run("MemResultCache", func(t *testing.T) {
		t.Parallel()
		c := &MemResultCache{}
		testfunc(t, c)
	})
	t.Run("DiskResultCache", func(t *testing.T) {
		t.Parallel()
		c := NewDiskResultCache(context.Background(), t.TempDir())
		testfunc(t, c)
	})
}

func TestReAuthResultCache(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testAllCacheImpls(t, func(t *testing.T, c ReAuthResultCache) {
		r1 := &ReAuthCheckResult{Host: "host1", Project: "project1", NeedsRAPT: true}
		r2 := &ReAuthCheckResult{Host: "host1", Project: "project2", NeedsRAPT: false}
		r3 := &ReAuthCheckResult{Host: "host2", Project: "project1", NeedsRAPT: true}

		err := c.Put(ctx, r1)
		assert.NoErr(t, err)
		err = c.Put(ctx, r2)
		assert.NoErr(t, err)
		err = c.Put(ctx, r3)
		assert.NoErr(t, err)

		got, err := c.GetForProject(ctx, "host1", "project1")
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(r1))

		got, err = c.GetForProject(ctx, "host1", "project2")
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(r2))

		got, err = c.GetForProject(ctx, "host2", "project1")
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(r3))

		_, err = c.GetForProject(ctx, "host3", "project1")
		assert.That(t, err, should.Equal(ErrResultMissing))

		_, err = c.GetForProject(ctx, "host2", "project3")
		assert.That(t, err, should.Equal(ErrResultMissing))

		// Test overwriting existing entry
		r4 := &ReAuthCheckResult{Host: "host1", Project: "project1", NeedsRAPT: false}
		err = c.Put(ctx, r4)
		assert.NoErr(t, err)
		got, err = c.GetForProject(ctx, "host1", "project1")
		assert.NoErr(t, err)
		assert.That(t, got, should.Match(r4))
	})
}

func TestDiskResultCache_Clear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewDiskResultCache(ctx, t.TempDir())
	r1 := &ReAuthCheckResult{Host: "host1", Project: "project1", NeedsRAPT: true}
	err := c.Put(ctx, r1)
	assert.NoErr(t, err)
	err = c.Clear()
	assert.NoErr(t, err)
	_, err = c.GetForProject(ctx, "host1", "project1")
	assert.ErrIsLike(t, err, ErrResultMissing)
}

func TestDiskResultCache_expiry_Default(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := testclock.New(time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC))
	ctx = clock.Set(ctx, tc)

	c := NewDiskResultCache(ctx, t.TempDir())
	r := &ReAuthCheckResult{Host: "host", Project: "project", NeedsRAPT: true}
	err := c.Put(ctx, r)
	assert.NoErr(t, err)

	// Result should be present initially.
	got, err := c.GetForProject(ctx, "host", "project")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(r))

	// Advance time past the shorter expiry, it should still be there.
	tc.Add(2 * resultCacheLifetimeRAPTNotNeeded)
	got, err = c.GetForProject(ctx, "host", "project")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(r))

	// Advance time past the longer expiry.
	tc.Add(2 * resultCacheLifetimeDefault)

	// Result should be gone now.
	_, err = c.GetForProject(ctx, "host", "project")
	assert.That(t, err, should.Equal(ErrResultMissing))
}

func TestDiskResultCache_expiry_RAPTNotNeeded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := testclock.New(time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC))
	ctx = clock.Set(ctx, tc)

	c := NewDiskResultCache(ctx, t.TempDir())
	r := &ReAuthCheckResult{Host: "host", Project: "project", NeedsRAPT: false}
	err := c.Put(ctx, r)
	assert.NoErr(t, err)

	// Result should be present initially.
	got, err := c.GetForProject(ctx, "host", "project")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(r))

	// Advance time past expiry.
	tc.Add(2 * resultCacheLifetimeRAPTNotNeeded)

	// Result should be gone now.
	_, err = c.GetForProject(ctx, "host", "project")
	assert.That(t, err, should.Equal(ErrResultMissing))
}

func TestNormalizeGerritHost(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		input    string
		expected string
	}{
		{"chromium.googlesource.com", "chromium-review.googlesource.com"},
		{"chromium-review.googlesource.com", "chromium-review.googlesource.com"},
	}
	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			assert.That(t, normalizeGerritHost(tc.input), should.Equal(tc.expected))
		})
	}
}

func TestIsSupportedURLHost(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		host     string
		expected bool
	}{
		{"chromium.googlesource.com", true},
		{"chromium-review.googlesource.com", true},
		{"foo.googlesource.com", false},
		{"chromium.example.com", false},
		{"", false},
	}
	for _, tc := range testCases {
		assert.That(t, isSupportedHost(tc.host), should.Equal(tc.expected))
	}
}

func TestDiskResultCache_VersionMismatch(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := testclock.New(time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC))
	ctx = clock.Set(ctx, tc)

	c := NewDiskResultCache(ctx, t.TempDir())

	// Create a cache with an old version.
	testTimestamp := time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC)
	oldStore := newResultStore()
	oldStore.Version = resultStoreCurrentVersion - 1
	oldStore.Entries = append(oldStore.Entries, &cachedResult{
		ReAuthCheckResult: ReAuthCheckResult{
			Host:         "host",
			Project:      "project",
			NeedsRAPT:    true,
			HasValidRAPT: false,
		},
		Timestamp: testTimestamp,
		Expiry:    testTimestamp.Add(1 * time.Hour),
	})
	err := c.write(ctx, oldStore)
	assert.NoErr(t, err)

	// The persisted entry isn't expired.
	assert.That(t, c.expired(ctx, oldStore.Entries[0]), should.BeFalse)

	// The cache should be empty due to version mismatch.
	_, err = c.GetForProject(ctx, "host", "project")
	assert.That(t, err, should.Equal(ErrResultMissing))

	// A new entry should be successfully added.
	r := &ReAuthCheckResult{Host: "host", Project: "project", NeedsRAPT: true}
	err = c.Put(ctx, r)
	assert.NoErr(t, err)

	// The new entry should be retrievable.
	got, err := c.GetForProject(ctx, "host", "project")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(r))

	// The store's version should be set to the current one.
	s, err := c.read(ctx)
	assert.NoErr(t, err)
	assert.That(t, s.Version, should.Equal(resultStoreCurrentVersion))
}

func TestDiskResultCache_PersistedFileHasNoVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := testclock.New(time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC))
	ctx = clock.Set(ctx, tc)

	c := NewDiskResultCache(ctx, t.TempDir())

	// Hard-code a JSON without version in the cache file.
	oldCacheContent := `{"entries":[{"host":"host","project":"project","needsRapt":false,"timestamp":"2000-01-02T03:04:05.600Z","expiry":"2001-01-02T03:04:05.600Z"}]}`
	assert.NoErr(t, os.WriteFile(c.cacheFile(), []byte(oldCacheContent), 0644))

	// Check the hardcoded string can be parsed correctly.
	var oldStore resultStore
	assert.NoErr(t, json.Unmarshal([]byte(oldCacheContent), &oldStore))
	assert.Loosely(t, oldStore.Entries, should.HaveLength(1))

	// Check we reads back an empty cache.
	s, err := c.read(ctx)
	assert.NoErr(t, err)
	assert.Loosely(t, s.Entries, should.BeEmpty)
	assert.That(t, s.Version, should.Equal(resultStoreCurrentVersion))

	// The cache isn't used.
	_, err = c.GetForProject(ctx, "host", "project")
	assert.That(t, err, should.Equal(ErrResultMissing))
}
