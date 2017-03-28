// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package warmup

import (
	"fmt"
	"testing"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorks(t *testing.T) {
	Convey("Works", t, func() {
		called := []string{}

		Register("1", func(context.Context) error {
			called = append(called, "1")
			return nil
		})

		Register("2", func(context.Context) error {
			called = append(called, "2")
			return fmt.Errorf("OMG 1")
		})

		Register("3", func(context.Context) error {
			called = append(called, "3")
			return fmt.Errorf("OMG 2")
		})

		err := Warmup(context.Background())
		So(err.Error(), ShouldEqual, "OMG 1 (and 1 other error)")
		So(called, ShouldResemble, []string{"1", "2", "3"})
	})
}
