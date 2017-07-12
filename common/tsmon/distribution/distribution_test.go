// Copyright 2015 The LUCI Authors.
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

package distribution

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNew(t *testing.T) {
	Convey("Passing nil uses the default bucketer", t, func() {
		d := New(nil)
		So(d.Bucketer(), ShouldEqual, DefaultBucketer)
	})
}

func TestAdd(t *testing.T) {
	Convey("Add", t, func() {
		d := New(FixedWidthBucketer(10, 2))
		So(d.Sum(), ShouldEqual, 0)
		So(d.Count(), ShouldEqual, 0)

		d.Add(1)
		So(d.Buckets(), ShouldResemble, []int64{0, 1})
		d.Add(10)
		So(d.Buckets(), ShouldResemble, []int64{0, 1, 1})
		d.Add(20)
		So(d.Buckets(), ShouldResemble, []int64{0, 1, 1, 1})
		d.Add(30)
		So(d.Buckets(), ShouldResemble, []int64{0, 1, 1, 2})
		So(d.Sum(), ShouldEqual, 61)
		So(d.Count(), ShouldEqual, 4)
	})
}
