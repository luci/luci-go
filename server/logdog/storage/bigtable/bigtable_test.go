// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"testing"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/grpcutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBigTable(t *testing.T) {
	t.Parallel()

	Convey(`A nil error is not marked transient.`, t, func() {
		So(wrapTransient(nil), ShouldBeNil)
	})

	Convey(`A regular error is not marked transient.`, t, func() {
		So(grpcutil.IsTransient(grpcutil.Canceled), ShouldBeFalse)
		So(errors.IsTransient(wrapTransient(grpcutil.Canceled)), ShouldBeFalse)
	})

	Convey(`An gRPC transient error is marked transient.`, t, func() {
		So(grpcutil.IsTransient(grpcutil.Internal), ShouldBeTrue)
		So(errors.IsTransient(wrapTransient(grpcutil.Internal)), ShouldBeTrue)
	})
}
