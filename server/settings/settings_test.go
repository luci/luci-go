// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package settings

import (
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type exampleSettings struct {
	Greetings string `json:"greetings"`
}

type anotherSettings struct{}

func TestSettings(t *testing.T) {
	Convey("Works", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), time.Unix(1444945245, 0))
		settings := New(&MemoryStorage{Expiration: time.Second})
		s := exampleSettings{}

		// Nothing is set yet.
		So(settings.Get(ctx, "key", &s), ShouldEqual, ErrNoSettings)

		// Set something.
		So(settings.Set(ctx, "key", &exampleSettings{"hi"}, "who", "why"), ShouldBeNil)

		// Old value (the lack of there of) is still cached.
		So(settings.Get(ctx, "key", &s), ShouldEqual, ErrNoSettings)

		// Non-caching version works.
		So(settings.GetUncached(ctx, "key", &s), ShouldBeNil)
		So(s, ShouldResemble, exampleSettings{"hi"})

		// Advance time to make old value expired.
		tc.Add(2 * time.Second)
		So(settings.Get(ctx, "key", &s), ShouldBeNil)
		So(s, ShouldResemble, exampleSettings{"hi"})

		// Not a pointer.
		So(settings.Get(ctx, "key", s), ShouldEqual, ErrBadType)

		// Not *exampleSettings.
		So(settings.Get(ctx, "key", &anotherSettings{}), ShouldEqual, ErrBadType)
	})
}

func TestContext(t *testing.T) {
	Convey("Works", t, func() {
		ctx := context.Background()
		s := exampleSettings{}

		So(Get(ctx, "key", &exampleSettings{}), ShouldEqual, ErrNoSettings)
		So(GetUncached(ctx, "key", &exampleSettings{}), ShouldEqual, ErrNoSettings)
		So(Set(ctx, "key", &exampleSettings{}, "who", "why"), ShouldEqual, ErrNoSettings)

		ctx = Use(ctx, New(&MemoryStorage{}))
		So(Set(ctx, "key", &exampleSettings{"hi"}, "who", "why"), ShouldBeNil)

		So(Get(ctx, "key", &s), ShouldBeNil)
		So(s, ShouldResemble, exampleSettings{"hi"})

		So(GetUncached(ctx, "key", &s), ShouldBeNil)
		So(s, ShouldResemble, exampleSettings{"hi"})
	})
}
