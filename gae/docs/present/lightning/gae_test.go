// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// +build !native_appengine

package demo

import "testing"
import "golang.org/x/net/context"
import "github.com/luci/gae/impl/memory"
import . "github.com/smartystreets/goconvey/convey"

// START OMIT

import "github.com/luci/gae/service/datastore" // HL

func TestGAE(t *testing.T) {
	type Model struct { // HL
		ID   string `gae:"$id"` // HL
		A, B int    // HL
	} // HL
	Convey("Put/Get w/ gae", t, func() {
		ctx := memory.Use(context.Background())
		So(datastore.Put(ctx, // HL
			&Model{"one thing", 10, 20},                // HL
			&Model{"or another", 20, 30}), ShouldBeNil) // HL
		ms := []*Model{{ID: "one thing"}, {ID: "or another"}}
		So(datastore.Get(ctx, ms), ShouldBeNil) // HL
		So(ms, ShouldResemble, []*Model{{"one thing", 10, 20}, {"or another", 20, 30}})
	})
}

// END OMIT
