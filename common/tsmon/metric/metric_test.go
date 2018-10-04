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

package metric

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"

	. "github.com/smartystreets/goconvey/convey"
)

func makeContext() context.Context {
	ret, _ := tsmon.WithDummyInMemory(context.Background())
	return ret
}

func TestMetrics(t *testing.T) {
	t.Parallel()

	Convey("Int", t, func() {
		c := makeContext()
		m := NewInt("int", "description", nil)
		So(func() { NewInt("int", "description", nil) }, ShouldPanic)

		So(m.Get(c), ShouldEqual, 0)
		m.Set(c, 42)
		So(m.Get(c), ShouldEqual, 42)

		So(func() { m.Set(c, 42, "field") }, ShouldPanic)
		So(func() { m.Get(c, "field") }, ShouldPanic)
	})

	Convey("Counter", t, func() {
		c := makeContext()
		m := NewCounter("counter", "description", nil)
		So(func() { NewCounter("counter", "description", nil) }, ShouldPanic)

		So(m.Get(c), ShouldEqual, 0)

		m.Add(c, 3)
		So(m.Get(c), ShouldEqual, 3)

		m.Add(c, 2)
		So(m.Get(c), ShouldEqual, 5)
	})

	Convey("Float", t, func() {
		c := makeContext()
		m := NewFloat("float", "description", nil)
		So(func() { NewFloat("float", "description", nil) }, ShouldPanic)

		So(m.Get(c), ShouldAlmostEqual, 0.0)

		m.Set(c, 42.3)
		So(m.Get(c), ShouldAlmostEqual, 42.3)

		So(func() { m.Set(c, 42.3, "field") }, ShouldPanic)
		So(func() { m.Get(c, "field") }, ShouldPanic)
	})

	Convey("FloatCounter", t, func() {
		c := makeContext()
		m := NewFloatCounter("float_counter", "description", nil)
		So(func() { NewFloatCounter("float_counter", "description", nil) }, ShouldPanic)

		So(m.Get(c), ShouldAlmostEqual, 0.0)

		m.Add(c, 3.1)
		So(m.Get(c), ShouldAlmostEqual, 3.1)

		m.Add(c, 2.2)
		So(m.Get(c), ShouldAlmostEqual, 5.3)
	})

	Convey("String", t, func() {
		c := makeContext()
		m := NewString("string", "description", nil)
		So(func() { NewString("string", "description", nil) }, ShouldPanic)

		So(m.Get(c), ShouldEqual, "")

		m.Set(c, "hello")
		So(m.Get(c), ShouldEqual, "hello")

		So(func() { m.Set(c, "hello", "field") }, ShouldPanic)
		So(func() { m.Get(c, "field") }, ShouldPanic)
	})

	Convey("Bool", t, func() {
		c := makeContext()
		m := NewBool("bool", "description", nil)
		So(func() { NewBool("bool", "description", nil) }, ShouldPanic)

		So(m.Get(c), ShouldEqual, false)

		m.Set(c, true)
		So(m.Get(c), ShouldBeTrue)

		So(func() { m.Set(c, true, "field") }, ShouldPanic)
		So(func() { m.Get(c, "field") }, ShouldPanic)
	})

	Convey("CumulativeDistribution", t, func() {
		c := makeContext()
		m := NewCumulativeDistribution("cumul_dist", "description", nil, distribution.FixedWidthBucketer(10, 20))
		So(func() { NewCumulativeDistribution("cumul_dist", "description", nil, m.Bucketer()) }, ShouldPanic)

		So(m.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(m.Bucketer().Width(), ShouldEqual, 10)
		So(m.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)

		So(m.Get(c), ShouldBeNil)

		m.Add(c, 5)

		v := m.Get(c)
		So(v.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(v.Bucketer().Width(), ShouldEqual, 10)
		So(v.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)
		So(v.Sum(), ShouldEqual, 5)
		So(v.Count(), ShouldEqual, 1)

		So(func() { m.Add(c, 5, "field") }, ShouldPanic)
		So(func() { m.Get(c, "field") }, ShouldPanic)
	})

	Convey("NonCumulativeDistribution", t, func() {
		c := makeContext()
		m := NewNonCumulativeDistribution("noncumul_dist", "description", nil, distribution.FixedWidthBucketer(10, 20))
		So(func() { NewNonCumulativeDistribution("noncumul_dist", "description", nil, m.Bucketer()) }, ShouldPanic)

		So(m.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(m.Bucketer().Width(), ShouldEqual, 10)
		So(m.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)

		So(m.Get(c), ShouldBeNil)

		d := distribution.New(m.Bucketer())
		d.Add(15)
		m.Set(c, d)

		v := m.Get(c)
		So(v.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(v.Bucketer().Width(), ShouldEqual, 10)
		So(v.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)
		So(v.Sum(), ShouldEqual, 15)
		So(v.Count(), ShouldEqual, 1)

		So(func() { m.Set(c, d, "field") }, ShouldPanic)
		So(func() { m.Get(c, "field") }, ShouldPanic)
	})
}
