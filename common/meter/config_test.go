// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meter

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
	Convey(`An zero (unmetered) config instance`, t, func() {
		cfg := new(Config)

		Convey(`It will register as unconstained.`, func() {
			So(cfg.HasFlushConstraints(), ShouldBeFalse)
		})
	})

	Convey(`A configured config instance`, t, func() {
		cfg := &Config{
			Count: 10,
			Delay: 1 * time.Second,
		}

		Convey(`Will register as metered.`, func() {
			So(cfg.HasFlushConstraints(), ShouldBeTrue)
		})
	})
}
