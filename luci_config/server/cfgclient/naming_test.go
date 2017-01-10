// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package cfgclient

import (
	"testing"

	"github.com/luci/gae/impl/memory"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestServiceConfigSet(t *testing.T) {
	t.Parallel()

	Convey(`With a testing AppEngine context`, t, func() {
		c := memory.Use(context.Background())

		Convey(`Can get the current service config set.`, func() {
			So(CurrentServiceConfigSet(c), ShouldEqual, "services/app")
		})
	})
}
