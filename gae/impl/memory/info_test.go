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
		So(info.AppID(c), ShouldEqual, "app-id")
		So(info.FullyQualifiedAppID(c), ShouldEqual, "dev~app-id")
		So(info.RequestID(c), ShouldEqual, "test-request-id")
		sa, err := info.ServiceAccount(c)
		So(err, ShouldBeNil)
		So(sa, ShouldEqual, "gae_service_account@example.com")

		// Setting to "override" applies to initial context.
		c = info.GetTestable(c).SetRequestID("override")
		So(info.RequestID(c), ShouldEqual, "override")

		// Derive inner context, "override" applies.
		c = info.MustNamespace(c, "valid_namespace_name")
		So(info.RequestID(c), ShouldEqual, "override")
	})
}
