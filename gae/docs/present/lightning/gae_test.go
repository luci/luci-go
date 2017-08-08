// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !native_appengine

package demo

import "testing"
import "golang.org/x/net/context"
import "go.chromium.org/gae/impl/memory"
import . "github.com/smartystreets/goconvey/convey"

// START OMIT

import "go.chromium.org/gae/service/datastore" // HL

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
