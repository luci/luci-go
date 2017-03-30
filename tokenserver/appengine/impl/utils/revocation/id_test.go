// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package revocation

import (
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/gae/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateTokenID(t *testing.T) {
	Convey("Works", t, func() {
		ctx := memory.Use(context.Background())

		id, err := GenerateTokenID(ctx, "zzz")
		So(err, ShouldBeNil)
		So(id, ShouldEqual, 1)

		id, err = GenerateTokenID(ctx, "zzz")
		So(err, ShouldBeNil)
		So(id, ShouldEqual, 2)
	})
}
