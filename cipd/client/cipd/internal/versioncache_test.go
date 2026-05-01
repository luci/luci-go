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
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	caspb "go.chromium.org/luci/cipd/api/cipd/v1/caspb"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
)

func TestVersionCacheWorks(t *testing.T) {
	numberedObjRef := func(i int) *caspb.ObjectRef {
		return &caspb.ObjectRef{
			HashAlgo:  caspb.HashAlgo_SHA256,
			HexDigest: strings.Repeat(fmt.Sprintf("%02x", i), 32),
		}
	}
	numberedIID := func(i int) string {
		return common.ObjectRefToInstanceID(numberedObjRef(i))
	}
	numberedPin := func(i int) common.Pin {
		return common.Pin{
			PackageName: "pkg",
			InstanceID:  numberedIID(i),
		}
	}

	// We make sure all basic tests work with both the old and new names.
	//
	// We test upgrade paths separately.
	for _, legacy := range []bool{true, false} {
		ftt.Run(fmt.Sprintf("useLegacyName=%t", legacy), t, func(t *ftt.Test) {
			fs := fs.NewFileSystem(t.TempDir(), "")

			cannedPin := common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("a", 40),
			}

			useLegacyName := LegacyVersionCache(legacy)

			t.Run("single tag", func(t *ftt.Test) {
				tc := NewVersionCache(fs, "service.example.com", useLegacyName)
				pin, err := tc.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(common.Pin{}))

				file, err := tc.ResolveExtractedObjectRef(t.Context(), cannedPin, "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.BeNil)

				// Add new.
				tc.AddTag(t.Context(), common.Pin{
					PackageName: "pkg",
					InstanceID:  strings.Repeat("a", 40),
				}, "tag:1")
				pin, err = tc.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(common.Pin{
					PackageName: "pkg",
					InstanceID:  strings.Repeat("a", 40),
				}))
				tc.AddExtractedObjectRef(t.Context(), cannedPin, "filename", numberedObjRef(1))
				file, err = tc.ResolveExtractedObjectRef(t.Context(), cannedPin, "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(1)))

				// Replace existing.
				tc.AddTag(t.Context(), common.Pin{
					PackageName: "pkg",
					InstanceID:  strings.Repeat("b", 40),
				}, "tag:1")
				pin, err = tc.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(common.Pin{
					PackageName: "pkg",
					InstanceID:  strings.Repeat("b", 40),
				}))
				tc.AddExtractedObjectRef(t.Context(), cannedPin, "filename", numberedObjRef(2))
				file, err = tc.ResolveExtractedObjectRef(t.Context(), cannedPin, "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(2)))

				// Flush.
				assert.Loosely(t, tc.Flush(t.Context()), should.BeNil)

				// Load.
				another := NewVersionCache(fs, "service.example.com", useLegacyName)
				pin, err = another.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(common.Pin{
					PackageName: "pkg",
					InstanceID:  strings.Repeat("b", 40),
				}))
				file, err = tc.ResolveExtractedObjectRef(t.Context(), cannedPin, "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(2)))
			})

			t.Run("many tags", func(t *ftt.Test) {
				tc := NewVersionCache(fs, "service.example.com", useLegacyName)

				// Fill up to capacity.
				for i := range maxCachedTags {
					assert.Loosely(t, tc.AddTag(t.Context(), cannedPin, fmt.Sprintf("tag:%d", i)), should.BeNil)
				}
				for i := range maxCachedExecutables {
					assert.Loosely(t, tc.AddExtractedObjectRef(t.Context(), numberedPin(i), "filename", numberedObjRef(i)), should.BeNil)
				}
				assert.Loosely(t, tc.Flush(t.Context()), should.BeNil)

				// Oldest tag is still resolvable.
				pin, err := tc.ResolveTag(t.Context(), "pkg", "tag:0")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(cannedPin))
				file, err := tc.ResolveExtractedObjectRef(t.Context(), numberedPin(0), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(0)))

				// Add one more tag. Should evict the oldest one.
				assert.Loosely(t, tc.AddTag(t.Context(), cannedPin, "one_more_tag:0"), should.BeNil)
				assert.Loosely(t, tc.AddExtractedObjectRef(t.Context(), numberedPin(maxCachedExecutables), "filename", numberedObjRef(maxCachedExecutables)), should.BeNil)
				assert.Loosely(t, tc.Flush(t.Context()), should.BeNil)

				// Oldest tag is evicted.
				pin, err = tc.ResolveTag(t.Context(), "pkg", "tag:0")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(common.Pin{}))
				file, err = tc.ResolveExtractedObjectRef(t.Context(), numberedPin(0), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.BeNil)

				// But next one is alive.
				pin, err = tc.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(cannedPin))
				file, err = tc.ResolveExtractedObjectRef(t.Context(), numberedPin(1), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(1)))

				// Most recent one is also alive.
				pin, err = tc.ResolveTag(t.Context(), "pkg", "one_more_tag:0")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(cannedPin))
				file, err = tc.ResolveExtractedObjectRef(t.Context(), numberedPin(maxCachedExecutables), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(maxCachedExecutables)))
			})

			t.Run("parallel update", func(t *ftt.Test) {
				tc1 := NewVersionCache(fs, "service.example.com", useLegacyName)
				tc2 := NewVersionCache(fs, "service.example.com", useLegacyName)

				assert.Loosely(t, tc1.AddTag(t.Context(), cannedPin, "tag:1"), should.BeNil)
				assert.Loosely(t, tc1.AddExtractedObjectRef(t.Context(), numberedPin(0), "filename", numberedObjRef(0)), should.BeNil)
				assert.Loosely(t, tc2.AddTag(t.Context(), cannedPin, "tag:2"), should.BeNil)
				assert.Loosely(t, tc2.AddExtractedObjectRef(t.Context(), numberedPin(1), "filename", numberedObjRef(1)), should.BeNil)

				assert.Loosely(t, tc1.Flush(t.Context()), should.BeNil)
				assert.Loosely(t, tc2.Flush(t.Context()), should.BeNil)

				tc3 := NewVersionCache(fs, "service.example.com", useLegacyName)

				// Both tags are resolvable.
				pin, err := tc3.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(cannedPin))
				file, err := tc3.ResolveExtractedObjectRef(t.Context(), numberedPin(0), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(0)))
				pin, err = tc3.ResolveTag(t.Context(), "pkg", "tag:2")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(cannedPin))
				file, err = tc3.ResolveExtractedObjectRef(t.Context(), numberedPin(1), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(1)))
			})

			t.Run("multiple services", func(t *ftt.Test) {
				tc1 := NewVersionCache(fs, "service1.example.com", useLegacyName)
				tc2 := NewVersionCache(fs, "service2.example.com", useLegacyName)

				// Add same tags and files, that resolve to different hashes on different
				// servers.
				assert.Loosely(t, tc1.AddTag(t.Context(), numberedPin(0), "tag:1"), should.BeNil)
				assert.Loosely(t, tc1.AddExtractedObjectRef(t.Context(), numberedPin(1), "filename", numberedObjRef(10)), should.BeNil)
				assert.Loosely(t, tc2.AddTag(t.Context(), numberedPin(2), "tag:1"), should.BeNil)
				assert.Loosely(t, tc2.AddExtractedObjectRef(t.Context(), numberedPin(1), "filename", numberedObjRef(20)), should.BeNil)

				assert.Loosely(t, tc1.Flush(t.Context()), should.BeNil)
				assert.Loosely(t, tc2.Flush(t.Context()), should.BeNil)

				tc1 = NewVersionCache(fs, "service1.example.com", useLegacyName)
				tc2 = NewVersionCache(fs, "service2.example.com", useLegacyName)

				// Tags are resolvable. tc2.Flush didn't overwrite tc1 data.
				pin, err := tc1.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(numberedPin(0)))
				pin, err = tc2.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(numberedPin(2)))

				// File hashes are cached too.
				file, err := tc1.ResolveExtractedObjectRef(t.Context(), numberedPin(1), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(10)))
				file, err = tc2.ResolveExtractedObjectRef(t.Context(), numberedPin(1), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.Match(numberedObjRef(20)))

				// No "ghost" records for some different service.
				tc3 := NewVersionCache(fs, "service3.example.com", useLegacyName)
				pin, err = tc3.ResolveTag(t.Context(), "pkg", "tag:1")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pin, should.Match(common.Pin{}))
				file, err = tc3.ResolveExtractedObjectRef(t.Context(), numberedPin(1), "filename")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, file, should.BeNil)
			})
		})
	}
}

func TestVersionCacheUpgrade(t *testing.T) {
	fs := fs.NewFileSystem(t.TempDir(), "")

	// Make a legacy cache with a tag and flush it.
	tc := NewVersionCache(fs, "service.example.com", WriteLegacyVersionCacheName)
	tc.AddTag(t.Context(), common.Pin{
		PackageName: "pkg",
		InstanceID:  strings.Repeat("a", 40),
	}, "tag:1")
	assert.NoErr(t, tc.Flush(t.Context()))

	// Legacy filename is there.
	legacyPath, err := fs.RootRelToAbs(legacyVersionCacheName)
	assert.NoErr(t, err)
	_, err = fs.Stat(t.Context(), legacyPath)
	assert.NoErr(t, err)

	// Load.
	another := NewVersionCache(fs, "service.example.com", WriteVersionCacheName)

	// Legacy filename is still there.
	_, err = fs.Stat(t.Context(), legacyPath)
	assert.NoErr(t, err)

	// Make sure our tag is there.
	pin, err := another.ResolveTag(t.Context(), "pkg", "tag:1")
	assert.Loosely(t, err, should.BeNil)
	assert.Loosely(t, pin, should.Match(common.Pin{
		PackageName: "pkg",
		InstanceID:  strings.Repeat("a", 40),
	}))

	// Make a change so that our cache is updated.
	another.AddTag(t.Context(), common.Pin{
		PackageName: "pkg",
		InstanceID:  strings.Repeat("b", 40),
	}, "tag:2")

	// Flush the new cache.
	assert.NoErr(t, another.Flush(t.Context()))

	// New name is there.
	currentPath, err := fs.RootRelToAbs(versionCacheName)
	assert.NoErr(t, err)
	_, err = fs.Stat(t.Context(), currentPath)
	assert.NoErr(t, err)

	// Old name is not.
	_, err = fs.Stat(t.Context(), legacyPath)
	assert.ErrIsLike(t, err, os.ErrNotExist)
}
