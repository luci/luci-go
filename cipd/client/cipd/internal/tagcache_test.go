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
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/context"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/common"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("with temp dir", t, func() {
		tempDir, err := ioutil.TempDir("", "instanceche_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		fs := fs.NewFileSystem(tempDir, "")

		cannedPin := common.Pin{
			PackageName: "pkg",
			InstanceID:  strings.Repeat("a", 40),
		}

		Convey("single tag", func() {
			tc := NewTagCache(fs, "service.example.com")
			pin, err := tc.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{})

			file, err := tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			So(err, ShouldBeNil)
			So(file, ShouldBeNil)

			// Add new.
			tc.AddTag(ctx, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("a", 40),
			}, "tag:1")
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("a", 40),
			})
			tc.AddExtractedObjectRef(ctx, cannedPin, "filename", numberedObjRef(1))
			file, err = tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(1))

			// Replace existing.
			tc.AddTag(ctx, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("b", 40),
			}, "tag:1")
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("b", 40),
			})
			tc.AddExtractedObjectRef(ctx, cannedPin, "filename", numberedObjRef(2))
			file, err = tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(2))

			// Save.
			So(tc.Save(ctx), ShouldBeNil)

			// Load.
			another := NewTagCache(fs, "service.example.com")
			pin, err = another.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("b", 40),
			})
			file, err = tc.ResolveExtractedObjectRef(ctx, cannedPin, "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(2))
		})

		Convey("many tags", func() {
			tc := NewTagCache(fs, "service.example.com")

			// Fill up to capacity.
			for i := 0; i < tagCacheMaxSize; i++ {
				So(tc.AddTag(ctx, cannedPin, fmt.Sprintf("tag:%d", i)), ShouldBeNil)
			}
			for i := 0; i < tagCacheMaxExeSize; i++ {
				So(tc.AddExtractedObjectRef(ctx, numberedPin(i), "filename", numberedObjRef(i)), ShouldBeNil)
			}
			So(tc.Save(ctx), ShouldBeNil)

			// Oldest tag is still resolvable.
			pin, err := tc.ResolveTag(ctx, "pkg", "tag:0")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
			file, err := tc.ResolveExtractedObjectRef(ctx, numberedPin(0), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(0))

			// Add one more tag. Should evict the oldest one.
			So(tc.AddTag(ctx, cannedPin, "one_more_tag:0"), ShouldBeNil)
			So(tc.AddExtractedObjectRef(ctx, numberedPin(tagCacheMaxExeSize), "filename", numberedObjRef(tagCacheMaxExeSize)), ShouldBeNil)
			So(tc.Save(ctx), ShouldBeNil)

			// Oldest tag is evicted.
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:0")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{})
			file, err = tc.ResolveExtractedObjectRef(ctx, numberedPin(0), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldBeNil)

			// But next one is alive.
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
			file, err = tc.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(1))

			// Most recent one is also alive.
			pin, err = tc.ResolveTag(ctx, "pkg", "one_more_tag:0")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
			file, err = tc.ResolveExtractedObjectRef(ctx, numberedPin(tagCacheMaxExeSize), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(tagCacheMaxExeSize))
		})

		Convey("parallel update", func() {
			tc1 := NewTagCache(fs, "service.example.com")
			tc2 := NewTagCache(fs, "service.example.com")

			So(tc1.AddTag(ctx, cannedPin, "tag:1"), ShouldBeNil)
			So(tc1.AddExtractedObjectRef(ctx, numberedPin(0), "filename", numberedObjRef(0)), ShouldBeNil)
			So(tc2.AddTag(ctx, cannedPin, "tag:2"), ShouldBeNil)
			So(tc2.AddExtractedObjectRef(ctx, numberedPin(1), "filename", numberedObjRef(1)), ShouldBeNil)

			So(tc1.Save(ctx), ShouldBeNil)
			So(tc2.Save(ctx), ShouldBeNil)

			tc3 := NewTagCache(fs, "service.example.com")

			// Both tags are resolvable.
			pin, err := tc3.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
			file, err := tc3.ResolveExtractedObjectRef(ctx, numberedPin(0), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(0))
			pin, err = tc3.ResolveTag(ctx, "pkg", "tag:2")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
			file, err = tc3.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(1))
		})

		Convey("multiple services", func() {
			tc1 := NewTagCache(fs, "service1.example.com")
			tc2 := NewTagCache(fs, "service2.example.com")

			// Add same tags and files, that resolve to different hashes on different
			// servers.
			So(tc1.AddTag(ctx, numberedPin(0), "tag:1"), ShouldBeNil)
			So(tc1.AddExtractedObjectRef(ctx, numberedPin(1), "filename", numberedObjRef(10)), ShouldBeNil)
			So(tc2.AddTag(ctx, numberedPin(2), "tag:1"), ShouldBeNil)
			So(tc2.AddExtractedObjectRef(ctx, numberedPin(1), "filename", numberedObjRef(20)), ShouldBeNil)

			So(tc1.Save(ctx), ShouldBeNil)
			So(tc2.Save(ctx), ShouldBeNil)

			tc1 = NewTagCache(fs, "service1.example.com")
			tc2 = NewTagCache(fs, "service2.example.com")

			// Tags are resolvable. tc2.Save didn't overwrite tc1 data.
			pin, err := tc1.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, numberedPin(0))
			pin, err = tc2.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, numberedPin(2))

			// File hashes are cached too.
			file, err := tc1.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(10))
			file, err = tc2.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldResemble, numberedObjRef(20))

			// No "ghost" records for some different service.
			tc3 := NewTagCache(fs, "service3.example.com")
			pin, err = tc3.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{})
			file, err = tc3.ResolveExtractedObjectRef(ctx, numberedPin(1), "filename")
			So(err, ShouldBeNil)
			So(file, ShouldBeNil)
		})
	})
}
