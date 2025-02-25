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
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"
)

func TestTagCacheWorks(t *testing.T) {
	ctx := context.Background()

	numberedObjRef := func(i int) *api.ObjectRef {
		return &api.ObjectRef{
			HashAlgo:  api.HashAlgo_SHA256,
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

	ftt.Run("with temp dir", t, func(t *ftt.Test) {
		tempDir, err := ioutil.TempDir("", "instanceche_test")
		assert.Loosely(t, err, should.BeNil)
		defer os.RemoveAll(tempDir)

		fs := fs.NewFileSystem(tempDir, "")

		cannedPin := common.Pin{
			PackageName: "pkg",
			InstanceID:  strings.Repeat("a", 40),
		}

		t.Run("single tag", func(t *ftt.Test) {
			tc := NewTagCache(fs, "service.example.com")
			pin, err := tc.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(common.Pin{}))

			file, err := tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.BeNil)

			// Add new.
			tc.AddTag(ctx, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("a", 40),
			}, "tag:1")
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("a", 40),
			}))
			tc.AddExtractedObjectRef(ctx, cannedPin, "filename", numberedObjRef(1))
			file, err = tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(1)))

			// Replace existing.
			tc.AddTag(ctx, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("b", 40),
			}, "tag:1")
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("b", 40),
			}))
			tc.AddExtractedObjectRef(ctx, cannedPin, "filename", numberedObjRef(2))
			file, err = tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(2)))

			// Save.
			assert.Loosely(t, tc.Save(ctx), should.BeNil)

			// Load.
			another := NewTagCache(fs, "service.example.com")
			pin, err = another.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("b", 40),
			}))
			file, err = tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(2)))
		})

		t.Run("many tags", func(t *ftt.Test) {
			tc := NewTagCache(fs, "service.example.com")

			// Fill up to capacity.
			for i := 0; i < tagCacheMaxSize; i++ {
				assert.Loosely(t, tc.AddTag(ctx, cannedPin, fmt.Sprintf("tag:%d", i)), should.BeNil)
			}
			for i := 0; i < tagCacheMaxExeSize; i++ {
				assert.Loosely(t, tc.AddExtractedObjectRef(ctx, numberedPin(i), "filename", numberedObjRef(i)), should.BeNil)
			}
			assert.Loosely(t, tc.Save(ctx), should.BeNil)

			// Oldest tag is still resolvable.
			pin, err := tc.ResolveTag(ctx, "pkg", "tag:0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(cannedPin))
			file, err := tc.ResolveExtractedObjectRef(ctx, numberedPin(0), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(0)))

			// Add one more tag. Should evict the oldest one.
			assert.Loosely(t, tc.AddTag(ctx, cannedPin, "one_more_tag:0"), should.BeNil)
			assert.Loosely(t, tc.AddExtractedObjectRef(ctx, numberedPin(tagCacheMaxExeSize), "filename", numberedObjRef(tagCacheMaxExeSize)), should.BeNil)
			assert.Loosely(t, tc.Save(ctx), should.BeNil)

			// Oldest tag is evicted.
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(common.Pin{}))
			file, err = tc.ResolveExtractedObjectRef(ctx, numberedPin(0), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.BeNil)

			// But next one is alive.
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(cannedPin))
			file, err = tc.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(1)))

			// Most recent one is also alive.
			pin, err = tc.ResolveTag(ctx, "pkg", "one_more_tag:0")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(cannedPin))
			file, err = tc.ResolveExtractedObjectRef(ctx, numberedPin(tagCacheMaxExeSize), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(tagCacheMaxExeSize)))
		})

		t.Run("parallel update", func(t *ftt.Test) {
			tc1 := NewTagCache(fs, "service.example.com")
			tc2 := NewTagCache(fs, "service.example.com")

			assert.Loosely(t, tc1.AddTag(ctx, cannedPin, "tag:1"), should.BeNil)
			assert.Loosely(t, tc1.AddExtractedObjectRef(ctx, numberedPin(0), "filename", numberedObjRef(0)), should.BeNil)
			assert.Loosely(t, tc2.AddTag(ctx, cannedPin, "tag:2"), should.BeNil)
			assert.Loosely(t, tc2.AddExtractedObjectRef(ctx, numberedPin(1), "filename", numberedObjRef(1)), should.BeNil)

			assert.Loosely(t, tc1.Save(ctx), should.BeNil)
			assert.Loosely(t, tc2.Save(ctx), should.BeNil)

			tc3 := NewTagCache(fs, "service.example.com")

			// Both tags are resolvable.
			pin, err := tc3.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(cannedPin))
			file, err := tc3.ResolveExtractedObjectRef(ctx, numberedPin(0), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(0)))
			pin, err = tc3.ResolveTag(ctx, "pkg", "tag:2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(cannedPin))
			file, err = tc3.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(1)))
		})

		t.Run("multiple services", func(t *ftt.Test) {
			tc1 := NewTagCache(fs, "service1.example.com")
			tc2 := NewTagCache(fs, "service2.example.com")

			// Add same tags and files, that resolve to different hashes on different
			// servers.
			assert.Loosely(t, tc1.AddTag(ctx, numberedPin(0), "tag:1"), should.BeNil)
			assert.Loosely(t, tc1.AddExtractedObjectRef(ctx, numberedPin(1), "filename", numberedObjRef(10)), should.BeNil)
			assert.Loosely(t, tc2.AddTag(ctx, numberedPin(2), "tag:1"), should.BeNil)
			assert.Loosely(t, tc2.AddExtractedObjectRef(ctx, numberedPin(1), "filename", numberedObjRef(20)), should.BeNil)

			assert.Loosely(t, tc1.Save(ctx), should.BeNil)
			assert.Loosely(t, tc2.Save(ctx), should.BeNil)

			tc1 = NewTagCache(fs, "service1.example.com")
			tc2 = NewTagCache(fs, "service2.example.com")

			// Tags are resolvable. tc2.Save didn't overwrite tc1 data.
			pin, err := tc1.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(numberedPin(0)))
			pin, err = tc2.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(numberedPin(2)))

			// File hashes are cached too.
			file, err := tc1.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(10)))
			file, err = tc2.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.Match(numberedObjRef(20)))

			// No "ghost" records for some different service.
			tc3 := NewTagCache(fs, "service3.example.com")
			pin, err = tc3.ResolveTag(ctx, "pkg", "tag:1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pin, should.Match(common.Pin{}))
			file, err = tc3.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, file, should.BeNil)
		})
	})
}
