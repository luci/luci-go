// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bigtable

import (
	"fmt"
	"testing"

	"github.com/luci/luci-go/common/errors"
	. "github.com/smartystreets/goconvey/convey"
)

func TestBigTable(t *testing.T) {
	t.Parallel()

	Convey(`A nil error is not marked transient.`, t, func() {
		So(wrapTransient(nil), ShouldBeNil)
	})

	Convey(`A regular error is not marked transient.`, t, func() {
		So(errors.IsTransient(wrapTransient(fmt.Errorf("boring error"))), ShouldBeFalse)
	})

	Convey(`An error containing "Internal error encountered" is marked transient.`, t, func() {
		So(errors.IsTransient(wrapTransient(fmt.Errorf("Hey, Internal error encountered!"))), ShouldBeTrue)
	})
}
