// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/cipd/client/cipd/common"
	"github.com/luci/luci-go/cipd/client/cipd/local"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTagCacheWorks(t *testing.T) {
	ctx := context.Background()

	Convey("with temp dir", t, func() {
		tempDir, err := ioutil.TempDir("", "instanceche_test")
		So(err, ShouldBeNil)
		defer os.RemoveAll(tempDir)

		fs := local.NewFileSystem(tempDir, "")

		cannedPin := common.Pin{
			PackageName: "pkg",
			InstanceID:  strings.Repeat("a", 40),
		}

		Convey("single tag", func() {
			tc := NewTagCache(fs)
			pin, err := tc.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{})

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

			// Save.
			So(tc.Save(ctx), ShouldBeNil)

			// Load.
			another := NewTagCache(fs)
			pin, err = another.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{
				PackageName: "pkg",
				InstanceID:  strings.Repeat("b", 40),
			})
		})

		Convey("many tags", func() {
			tc := NewTagCache(fs)

			// Fill up to capacity.
			for i := 0; i < tagCacheMaxSize; i++ {
				So(tc.AddTag(ctx, cannedPin, fmt.Sprintf("tag:%d", i)), ShouldBeNil)
			}
			So(tc.Save(ctx), ShouldBeNil)

			// Oldest tag is still resolvable.
			pin, err := tc.ResolveTag(ctx, "pkg", "tag:0")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)

			// Add one more tag. Should evict the oldest one.
			So(tc.AddTag(ctx, cannedPin, "one_more_tag:0"), ShouldBeNil)
			So(tc.Save(ctx), ShouldBeNil)

			// Oldest tag is evicted.
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:0")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, common.Pin{})

			// But next one is alive.
			pin, err = tc.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)

			// Most recent one is also alive.
			pin, err = tc.ResolveTag(ctx, "pkg", "one_more_tag:0")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
		})

		Convey("parallel update", func() {
			tc1 := NewTagCache(fs)
			tc2 := NewTagCache(fs)

			So(tc1.AddTag(ctx, cannedPin, "tag:1"), ShouldBeNil)
			So(tc2.AddTag(ctx, cannedPin, "tag:2"), ShouldBeNil)

			So(tc1.Save(ctx), ShouldBeNil)
			So(tc2.Save(ctx), ShouldBeNil)

			tc3 := NewTagCache(fs)

			// Both tags are resolvable.
			pin, err := tc3.ResolveTag(ctx, "pkg", "tag:1")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
			pin, err = tc3.ResolveTag(ctx, "pkg", "tag:2")
			So(err, ShouldBeNil)
			So(pin, ShouldResemble, cannedPin)
		})
	})
}
