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

package internal

import (
	"fmt"
	"os"
	"slices"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cipd/client/cipd/fs"
)

func TestVersionCacheWorks(t *testing.T) {
	t.Parallel()

	// We make sure all basic tests work with both the old and new names.
	//
	// We test upgrade paths separately.
	for _, legacy := range []LegacyVersionCache{UseLegacyVCName, UseModernVCName} {
		ftt.Run(fmt.Sprintf("useLegacyName=%t", legacy), t, func(t *ftt.Test) {
			fs := fs.NewFileSystem(t.TempDir(), "")

			t.Run("single tag", func(t *ftt.Test) {
				ctx := t.Context()

				tc := &VersionCache{FS: fs, SaveName: legacy}
				assert.That(t, tc, shouldNotHaveTag(ctx, "service.example.com", "pkg", "tag:1"))
				assert.That(t, tc, shouldNotHaveFile(ctx, "service.example.com", "pkg", IID1("a"), "filename"))

				// Add new.
				assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))
				assert.That(t, tc, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))

				assert.That(t, tc, shouldAddFile(ctx, "service.example.com", "pkg", IID1("a"), "filename", IID1("1")))
				assert.That(t, tc, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("a"), "filename", IID1("1")))

				// Replace existing.
				assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "tag:1", IID1("b")))
				assert.That(t, tc, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:1", IID1("b")))

				assert.That(t, tc, shouldAddFile(ctx, "service.example.com", "pkg", IID1("a"), "filename", IID2("2")))
				assert.That(t, tc, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("a"), "filename", IID2("2")))

				// Flush.
				assert.Loosely(t, tc.Flush(t.Context()), should.BeNil)

				// Load.
				another := &VersionCache{FS: fs, SaveName: legacy}
				assert.That(t, another, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:1", IID1("b")))
				assert.That(t, another, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("a"), "filename", IID2("2")))
			})

			t.Run("many tags", func(t *ftt.Test) {
				ctx := t.Context()
				tc := &VersionCache{FS: fs, SaveName: legacy}

				// Fill up to capacity.
				for i := range defaultMaxTags {
					assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", fmt.Sprintf("tag:%d", i), IID1("a")))
				}
				pkgIIDs := make([]string, defaultMaxExtractedObjectRefs)
				for i := range defaultMaxExtractedObjectRefs {
					pkgIIDs[i] = IID2(i)
				}
				slices.Sort(pkgIIDs)
				for _, iid := range pkgIIDs {
					assert.That(t, tc, shouldAddFile(ctx, "service.example.com", "pkg", iid, "filename", iid))
				}
				assert.NoErr(t, tc.Flush(ctx))

				// Oldest tag is still resolvable.
				assert.That(t, tc, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:0", IID1("a")))
				assert.That(t, tc, shouldHaveFile(ctx, "service.example.com", "pkg", pkgIIDs[0], "filename", pkgIIDs[0]))

				// Add one more tag. Should evict the oldest one.
				assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "one_more_tag:0", IID1("a")))
				assert.That(t, tc, shouldAddFile(ctx, "service.example.com", "pkg", IID2(defaultMaxExtractedObjectRefs), "filename", IID2(defaultMaxExtractedObjectRefs)))
				assert.NoErr(t, tc.Flush(ctx))

				// Oldest tag is evicted.
				assert.That(t, tc, shouldNotHaveTag(ctx, "service.example.com", "pkg", "tag:0"))
				assert.That(t, tc, shouldNotHaveFile(ctx, "service.example.com", "pkg", pkgIIDs[0], "filename"))

				// But next one is alive.
				assert.That(t, tc, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))
				assert.That(t, tc, shouldHaveFile(ctx, "service.example.com", "pkg", IID2(1), "filename", IID2(1)))

				// Most recent one is also alive.
				assert.That(t, tc, shouldHaveTag(ctx, "service.example.com", "pkg", "one_more_tag:0", IID1("a")))
				assert.That(t, tc, shouldHaveFile(ctx, "service.example.com", "pkg", IID2(defaultMaxExtractedObjectRefs), "filename", IID2(defaultMaxExtractedObjectRefs)))
			})

			t.Run("parallel update", func(t *ftt.Test) {
				ctx := t.Context()
				tc1 := &VersionCache{FS: fs, SaveName: legacy}
				tc2 := &VersionCache{FS: fs, SaveName: legacy}

				assert.That(t, tc1, shouldAddTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))
				assert.That(t, tc1, shouldAddFile(ctx, "service.example.com", "pkg", IID2(0), "filename", IID2(0)))
				assert.That(t, tc2, shouldAddTag(ctx, "service.example.com", "pkg", "tag:2", IID1("a")))
				assert.That(t, tc2, shouldAddFile(ctx, "service.example.com", "pkg", IID2(1), "filename", IID2(1)))

				assert.NoErr(t, tc1.Flush(ctx))
				assert.NoErr(t, tc2.Flush(ctx))

				tc3 := &VersionCache{FS: fs, SaveName: legacy}

				// Both tags are resolvable.
				assert.That(t, tc3, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))
				assert.That(t, tc3, shouldHaveFile(ctx, "service.example.com", "pkg", IID2(0), "filename", IID2(0)))
				assert.That(t, tc3, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:2", IID1("a")))
				assert.That(t, tc3, shouldHaveFile(ctx, "service.example.com", "pkg", IID2(1), "filename", IID2(1)))
			})

			t.Run("multiple services", func(t *ftt.Test) {
				ctx := t.Context()
				tc1 := &VersionCache{FS: fs, SaveName: legacy}
				tc2 := &VersionCache{FS: fs, SaveName: legacy}

				// Add same tags and files, that resolve to different hashes on different
				// servers.
				assert.That(t, tc1, shouldAddTag(ctx, "service1.example.com", "pkg", "tag:1", IID2(0)))
				assert.That(t, tc1, shouldAddFile(ctx, "service1.example.com", "pkg", IID2(1), "filename", IID2(10)))
				assert.That(t, tc2, shouldAddTag(ctx, "service2.example.com", "pkg", "tag:1", IID2(2)))
				assert.That(t, tc2, shouldAddFile(ctx, "service2.example.com", "pkg", IID2(1), "filename", IID2(20)))

				assert.NoErr(t, tc1.Flush(ctx))
				assert.NoErr(t, tc2.Flush(ctx))

				tc1 = &VersionCache{FS: fs, SaveName: legacy}
				tc2 = &VersionCache{FS: fs, SaveName: legacy}

				// Tags are resolvable. tc2.Flush didn't overwrite tc1 data.
				assert.That(t, tc1, shouldHaveTag(ctx, "service1.example.com", "pkg", "tag:1", IID2(0)))
				assert.That(t, tc2, shouldHaveTag(ctx, "service2.example.com", "pkg", "tag:1", IID2(2)))

				// File hashes are cached too.
				assert.That(t, tc1, shouldHaveFile(ctx, "service1.example.com", "pkg", IID2(1), "filename", IID2(10)))
				assert.That(t, tc2, shouldHaveFile(ctx, "service2.example.com", "pkg", IID2(1), "filename", IID2(20)))

				// No "ghost" records for some different service.
				tc3 := &VersionCache{FS: fs, SaveName: legacy}
				assert.That(t, tc3, shouldNotHaveTag(ctx, "service3.example.com", "pkg", "tag:1"))
				assert.That(t, tc3, shouldNotHaveFile(ctx, "service3.example.com", "pkg", IID2(1), "filename"))
			})
		})
	}
}

func TestVersionCacheUpgrade(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	fs := fs.NewFileSystem(t.TempDir(), "")

	// Make a legacy cache with a tag and flush it.
	tc := &VersionCache{FS: fs, SaveName: UseLegacyVCName}
	assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))
	assert.NoErr(t, tc.Flush(ctx))

	// Legacy filename is there.
	legacyPath, err := fs.RootRelToAbs(legacyVersionCacheName)
	assert.NoErr(t, err)
	_, err = fs.Stat(ctx, legacyPath)
	assert.NoErr(t, err)

	// Load.
	another := &VersionCache{FS: fs}

	// Legacy filename is still there.
	_, err = fs.Stat(ctx, legacyPath)
	assert.NoErr(t, err)

	// Make sure our tag is there.
	assert.That(t, another, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))

	// Make a change so that our cache is updated.
	assert.That(t, another, shouldAddTag(ctx, "service.example.com", "pkg", "tag:2", IID1("b")))

	// Flush the new cache.
	assert.NoErr(t, another.Flush(ctx))

	// New name is there.
	currentPath, err := fs.RootRelToAbs(versionCacheName)
	assert.NoErr(t, err)
	_, err = fs.Stat(ctx, currentPath)
	assert.NoErr(t, err)

	// Old name is still there, too.
	_, err = fs.Stat(t.Context(), legacyPath)
	assert.NoErr(t, err)

	// Delete the new name so that the cache will load the old one; make sure old
	// one does not have our `tag:2` from above; at this point new clients have
	// effectively abandoned the old cache name. In the event that the cache is
	// ONLY used by new clients, this will waste one or two KB. In the event that
	// the cache has both new and old clients sharing it, the old clients will
	// continue to use the old cache, and the new clients will continue to use
	// the new one.
	assert.NoErr(t, os.Remove(currentPath))
	tc = &VersionCache{FS: fs, SaveName: UseLegacyVCName}
	// Has the old tag.
	assert.That(t, tc, shouldHaveTag(t.Context(), "service.example.com", "pkg", "tag:1", IID1("a")))

	// But not the new one.
	assert.That(t, tc, shouldNotHaveTag(t.Context(), "service.example.com", "pkg", "tag:2"))
}

func TestVersionCacheOfflineRefs(t *testing.T) {
	t.Parallel()

	fs := fs.NewFileSystem(t.TempDir(), "")

	ctx := t.Context()
	ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeLocal)

	// Make a legacy cache with a tag and save it.
	tc := &VersionCache{FS: fs, SaveRefs: true}
	assert.That(t, tc, shouldAddRef(ctx, "service.example.com", "pkg", "latest", IID1("a")))
	assert.NoErr(t, tc.Flush(ctx))

	tc = &VersionCache{FS: fs, LoadRefs: true}
	assert.That(t, tc, shouldHaveRef(ctx, "service.example.com", "pkg", "latest", IID1("a")))

	// Verify that by default we don't load refs, but we do pass them through.
	tc = &VersionCache{FS: fs}
	assert.That(t, tc, shouldNotHaveRef(ctx, "service.example.com", "pkg", "latest"))
	assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "version:123", IID1("b")))
	assert.NoErr(t, tc.Flush(ctx))

	// Still there.
	tc = &VersionCache{FS: fs, LoadRefs: true}
	assert.That(t, tc, shouldHaveRef(ctx, "service.example.com", "pkg", "latest", IID1("a")))
	assert.That(t, tc, shouldHaveTag(ctx, "service.example.com", "pkg", "version:123", IID1("b")))
}

func TestVersionCacheMultiService(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	fsInst := fs.NewFileSystem(t.TempDir(), "")

	tc := &VersionCache{FS: fsInst}
	assert.That(t, tc, shouldAddTag(ctx, "service1.example.com", "pkg", "some:tag", IID1("a")))
	assert.That(t, tc, shouldAddTag(ctx, "service2.example.com", "pkg", "some:tag", IID1("b")))
	assert.NoErr(t, tc.Flush(ctx))

	tc = &VersionCache{FS: fsInst}
	assert.That(t, tc, shouldHaveTag(ctx, "service1.example.com", "pkg", "some:tag", IID1("a")))
	assert.That(t, tc, shouldHaveTag(ctx, "service2.example.com", "pkg", "some:tag", IID1("b")))
}

func TestVersionCacheChaining(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	offlineFS := fs.NewFileSystem(t.TempDir(), "")
	tcOffline := &VersionCache{FS: offlineFS, SaveRefs: true, LoadRefs: true}
	assert.That(t, tcOffline, shouldAddTag(ctx, "service.example.com", "pkg", "some:tag", IID1("a")))
	assert.That(t, tcOffline, shouldAddFile(ctx, "service.example.com", "pkg", IID1("a"), "aFile", IID1("a")))
	assert.That(t, tcOffline, shouldAddRef(ctx, "service.example.com", "pkg", "some/ref", IID1("a")))
	assert.NoErr(t, tcOffline.Flush(ctx))

	onlineFS := fs.NewFileSystem(t.TempDir(), "")
	tcOnline := &VersionCache{
		FS:       onlineFS,
		ChainTo:  tcOffline,
		LoadRefs: true,
		SaveRefs: true,
	}

	// We can read the offline tag via tcOnline.
	assert.That(t, tcOnline, shouldHaveTag(ctx, "service.example.com", "pkg", "some:tag", IID1("a")))
	assert.That(t, tcOnline, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("a"), "aFile", IID1("a")))
	assert.That(t, tcOnline, shouldHaveRef(ctx, "service.example.com", "pkg", "some/ref", IID1("a")))

	// Directly overwrite all three.
	assert.That(t, tcOnline, shouldAddTag(ctx, "service.example.com", "pkg", "some:tag", IID1("b")))
	assert.That(t, tcOnline, shouldAddFile(ctx, "service.example.com", "pkg", IID1("a"), "aFile", IID1("b")))
	assert.That(t, tcOnline, shouldAddRef(ctx, "service.example.com", "pkg", "some/ref", IID1("b")))
	// Add three new ones.
	assert.That(t, tcOnline, shouldAddTag(ctx, "service.example.com", "pkg", "other:tag", IID1("c")))
	assert.That(t, tcOnline, shouldAddFile(ctx, "service.example.com", "pkg", IID1("b"), "aFile", IID1("c")))
	assert.That(t, tcOnline, shouldAddRef(ctx, "service.example.com", "pkg", "other/ref", IID1("c")))
	assert.NoErr(t, tcOnline.Flush(ctx))

	// We should observe all 6 in the online cache.
	tcOnline2 := &VersionCache{FS: onlineFS, LoadRefs: true}
	assert.That(t, tcOnline2, shouldHaveTag(ctx, "service.example.com", "pkg", "some:tag", IID1("b")))
	assert.That(t, tcOnline2, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("a"), "aFile", IID1("b")))
	assert.That(t, tcOnline2, shouldHaveRef(ctx, "service.example.com", "pkg", "some/ref", IID1("b")))
	assert.That(t, tcOnline2, shouldHaveTag(ctx, "service.example.com", "pkg", "other:tag", IID1("c")))
	assert.That(t, tcOnline2, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("b"), "aFile", IID1("c")))
	assert.That(t, tcOnline2, shouldHaveRef(ctx, "service.example.com", "pkg", "other/ref", IID1("c")))

	// And the original cache is untouched.
	tcOffline2 := &VersionCache{FS: offlineFS, SaveRefs: true, LoadRefs: true}
	assert.That(t, tcOffline2, shouldAddTag(ctx, "service.example.com", "pkg", "some:tag", IID1("a")))
	assert.That(t, tcOffline2, shouldAddFile(ctx, "service.example.com", "pkg", IID1("a"), "aFile", IID1("a")))
	assert.That(t, tcOffline2, shouldAddRef(ctx, "service.example.com", "pkg", "some/ref", IID1("a")))
	assert.That(t, tcOffline2, shouldNotHaveTag(ctx, "service.example.com", "pkg", "other:tag"))
	assert.That(t, tcOffline2, shouldNotHaveFile(ctx, "service.example.com", "pkg", IID1("b"), "aFile"))
	assert.That(t, tcOffline2, shouldNotHaveRef(ctx, "service.example.com", "pkg", "other/ref"))
}

func TestVersionCacheLimits(t *testing.T) {
	t.Parallel()

	t.Run("custom limit", func(t *testing.T) {
		ctx := t.Context()

		fsInst := fs.NewFileSystem(t.TempDir(), "")
		tc := &VersionCache{
			FS:                     fsInst,
			MaxTags:                2,
			MaxExtractedObjectRefs: 2,
			MaxRefs:                2,
			SaveRefs:               true,
		}

		// Add 3 tags/files/refs.
		assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "tag:1", IID1("a")))
		assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "tag:2", IID1("b")))
		assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", "tag:3", IID1("c")))
		assert.That(t, tc, shouldAddFile(ctx, "service.example.com", "pkg", IID1("a"), "aFile", IID1("a")))
		assert.That(t, tc, shouldAddFile(ctx, "service.example.com", "pkg", IID1("b"), "bFile", IID1("b")))
		assert.That(t, tc, shouldAddFile(ctx, "service.example.com", "pkg", IID1("c"), "cFile", IID1("c")))
		assert.That(t, tc, shouldAddRef(ctx, "service.example.com", "pkg", "ref/1", IID1("a")))
		assert.That(t, tc, shouldAddRef(ctx, "service.example.com", "pkg", "ref/2", IID1("b")))
		assert.That(t, tc, shouldAddRef(ctx, "service.example.com", "pkg", "ref/3", IID1("c")))

		assert.NoErr(t, tc.Flush(ctx))

		tc2 := &VersionCache{FS: fsInst, LoadRefs: true}
		// The first tag/file/ref should be evicted because MaxTags is 2.
		assert.That(t, tc2, shouldNotHaveTag(ctx, "service.example.com", "pkg", "tag:1"))
		assert.That(t, tc2, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:2", IID1("b")))
		assert.That(t, tc2, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:3", IID1("c")))
		assert.That(t, tc2, shouldNotHaveFile(ctx, "service.example.com", "pkg", IID1("a"), "aFile"))
		assert.That(t, tc2, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("b"), "bFile", IID1("b")))
		assert.That(t, tc2, shouldHaveFile(ctx, "service.example.com", "pkg", IID1("c"), "cFile", IID1("c")))
		assert.That(t, tc2, shouldNotHaveRef(ctx, "service.example.com", "pkg", "ref/1"))
		assert.That(t, tc2, shouldHaveRef(ctx, "service.example.com", "pkg", "ref/2", IID1("b")))
		assert.That(t, tc2, shouldHaveRef(ctx, "service.example.com", "pkg", "ref/3", IID1("c")))
	})

	t.Run("no limit", func(t *testing.T) {
		ctx := t.Context()

		fsInst := fs.NewFileSystem(t.TempDir(), "")
		tc := &VersionCache{
			FS:      fsInst,
			MaxTags: -1, // No limit.
		}

		// Add more tags than default maxCachedTags.
		for i := range defaultMaxTags + 5 {
			assert.That(t, tc, shouldAddTag(ctx, "service.example.com", "pkg", fmt.Sprintf("tag:%d", i), IID1("a")))
		}
		assert.NoErr(t, tc.Flush(ctx))

		tc2 := &VersionCache{FS: fsInst}
		// Since there's no limit, even tag:0 should be preserved.
		assert.That(t, tc2, shouldHaveTag(ctx, "service.example.com", "pkg", "tag:0", IID1("a")))
		assert.That(t, tc2, shouldHaveTag(ctx, "service.example.com", "pkg", fmt.Sprintf("tag:%d", defaultMaxTags+4), IID1("a")))
	})
}
