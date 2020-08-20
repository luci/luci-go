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

// +build native_appengine

package demo

import "testing"
import "google.golang.org/appengine/aetest"
import . "github.com/smartystreets/goconvey/convey"

// START OMIT

import "google.golang.org/appengine/datastore" // HL

func TestNative(t *testing.T) {
	type Model struct{ A, B int } // HL

	Convey("Put/Get", t, func() {
		ctx, shtap, err := aetest.NewContext()
		So(err, ShouldBeNil)
		defer shtap()

		keys := []*datastore.Key{
			datastore.NewKey(ctx, "Model", "one thing", 0, nil),  // HL
			datastore.NewKey(ctx, "Model", "or another", 0, nil), // HL
		}
		_, err = datastore.PutMulti(ctx, keys, []*Model{{10, 20}, {20, 30}}) // HL
		So(err, ShouldBeNil)

		ms := make([]*Model, 2)
		So(datastore.GetMulti(ctx, keys, ms), ShouldBeNil) // HL
		So(ms, ShouldResemble, []*Model{{10, 20}, {20, 30}})
	})
}

// END OMIT
