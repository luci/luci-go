// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"testing"

	"github.com/luci/gae/service/info"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMustNamespace(t *testing.T) {
	Convey("Testable interface works", t, func() {
		c := UseWithAppID(context.Background(), "dev~app-id")

		// Default value.
		So(info.Get(c).AppID(), ShouldEqual, "app-id")
		So(info.Get(c).FullyQualifiedAppID(), ShouldEqual, "dev~app-id")
		So(info.Get(c).RequestID(), ShouldEqual, "test-request-id")

		// Setting to "override" applies to initial context.
		c = info.Get(c).Testable().SetRequestID("override")
		So(info.Get(c).RequestID(), ShouldEqual, "override")

		// Derive inner context, "override" applies.
		c = info.Get(c).MustNamespace("valid_namespace_name")
		So(info.Get(c).RequestID(), ShouldEqual, "override")
	})
}
