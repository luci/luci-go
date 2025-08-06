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

func testCacheImpls(t *testing.T, testfunc func(*testing.T, ReAuthResultCache)) {
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
	testCacheImpls(t, func(t *testing.T, c ReAuthResultCache) {
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

func TestDiskResultCache_clears(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	c := NewDiskResultCache(ctx, t.TempDir())
	r1 := &ReAuthCheckResult{Host: "host1", Project: "project1", NeedsRAPT: true}
	err := c.Put(ctx, r1)
	assert.NoErr(t, err)
	got, err := c.GetForProject(ctx, "host1", "project1")
	assert.NoErr(t, err)
	assert.That(t, got.HasValidRAPT, should.BeFalse)
}

func TestDiskResultCache_expiry(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	tc := testclock.New(time.Date(2000, 1, 2, 3, 4, 5, 6, time.UTC))
	ctx = clock.Set(ctx, tc)

	c := NewDiskResultCache(ctx, t.TempDir())
	r1 := &ReAuthCheckResult{Host: "host1", Project: "project1", NeedsRAPT: true}
	err := c.Put(ctx, r1)
	assert.NoErr(t, err)

	// Result should be present initially.
	got, err := c.GetForProject(ctx, "host1", "project1")
	assert.NoErr(t, err)
	assert.That(t, got, should.Match(r1))

	// Advance time past expiry.
	tc.Add(2 * resultCacheLifetime)

	// Result should be gone now.
	_, err = c.GetForProject(ctx, "host1", "project1")
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
