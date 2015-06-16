// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package clock

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSystemTimer(t *testing.T) {
	t.Parallel()

	Convey(`A systemTimer instance`, t, func() {
		t := new(systemTimer)

		Convey(`Should start with a nil channel.`, func() {
			So(t.GetC(), ShouldBeNil)
		})

		Convey(`When stopped, should return inactive.`, func() {
			So(t.Stop(), ShouldBeFalse)
		})

		Convey(`When reset`, func() {
			active := t.Reset(1 * time.Hour)
			So(active, ShouldBeFalse)

			Convey(`When reset to a short duration`, func() {
				// Upper bound of supported platform resolution. Windows is 15ms, so
				// make sure we exceed that.
				active := t.Reset(100 * time.Millisecond)

				Convey(`Should return active.`, func() {
					So(active, ShouldBeTrue)
				})

				Convey(`Should trigger shortly.`, func() {
					tm := <-t.GetC()
					So(tm, ShouldNotResemble, time.Time{})
				})
			})

			Convey(`When stopped, should return active and have a non-nil C.`, func() {
				active := t.Stop()
				So(active, ShouldBeTrue)
				So(t.GetC(), ShouldNotBeNil)
			})

			Convey(`When stopped, should return active.`, func() {
				active := t.Stop()
				So(active, ShouldBeTrue)
			})

			Convey(`Should have a non-nil channel.`, func() {
				So(t.GetC(), ShouldNotBeNil)
			})

			Reset(func() {
				t.Stop()
			})
		})
	})
}
