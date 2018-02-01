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
	"testing"

	"golang.org/x/net/context"

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

		v, err := m.Get(c)
		So(v, ShouldEqual, 0)
		So(err, ShouldBeNil)

		v, err = m.Get(c, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(c, 42)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldEqual, 42)
		So(err, ShouldBeNil)

		err = m.Set(c, 42, "field")
		So(err, ShouldNotBeNil)

		So(func() { NewInt("int", "description", nil) }, ShouldPanic)
	})

	Convey("Counter", t, func() {
		c := makeContext()
		m := NewCounter("counter", "description", nil)

		v, err := m.Get(c)
		So(v, ShouldEqual, 0)
		So(err, ShouldBeNil)

		err = m.Add(c, 3)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldEqual, 3)
		So(err, ShouldBeNil)

		err = m.Add(c, 2)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldEqual, 5)
		So(err, ShouldBeNil)

		So(func() { NewCounter("counter", "description", nil) }, ShouldPanic)
	})

	Convey("Float", t, func() {
		c := makeContext()
		m := NewFloat("float", "description", nil)

		v, err := m.Get(c)
		So(v, ShouldAlmostEqual, 0.0)
		So(err, ShouldBeNil)

		v, err = m.Get(c, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(c, 42.3)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldAlmostEqual, 42.3)
		So(err, ShouldBeNil)

		err = m.Set(c, 42.3, "field")
		So(err, ShouldNotBeNil)

		So(func() { NewFloat("float", "description", nil) }, ShouldPanic)
	})

	Convey("FloatCounter", t, func() {
		c := makeContext()
		m := NewFloatCounter("float_counter", "description", nil)

		v, err := m.Get(c)
		So(v, ShouldAlmostEqual, 0.0)
		So(err, ShouldBeNil)

		err = m.Add(c, 3.1)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldAlmostEqual, 3.1)
		So(err, ShouldBeNil)

		err = m.Add(c, 2.2)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldAlmostEqual, 5.3)
		So(err, ShouldBeNil)

		So(func() { NewFloatCounter("float_counter", "description", nil) }, ShouldPanic)
	})

	Convey("String", t, func() {
		c := makeContext()
		m := NewString("string", "description", nil)

		v, err := m.Get(c)
		So(v, ShouldEqual, "")
		So(err, ShouldBeNil)

		v, err = m.Get(c, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(c, "hello")
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldEqual, "hello")
		So(err, ShouldBeNil)

		err = m.Set(c, "hello", "field")
		So(err, ShouldNotBeNil)

		So(func() { NewString("string", "description", nil) }, ShouldPanic)
	})

	Convey("Bool", t, func() {
		c := makeContext()
		m := NewBool("bool", "description", nil)

		v, err := m.Get(c)
		So(v, ShouldEqual, false)
		So(err, ShouldBeNil)

		v, err = m.Get(c, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(c, true)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v, ShouldEqual, true)
		So(err, ShouldBeNil)

		err = m.Set(c, true, "field")
		So(err, ShouldNotBeNil)

		So(func() { NewBool("bool", "description", nil) }, ShouldPanic)
	})

	Convey("CumulativeDistribution", t, func() {
		c := makeContext()
		m := NewCumulativeDistribution("cumul_dist", "description", nil, distribution.FixedWidthBucketer(10, 20))

		v, err := m.Get(c)
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)

		So(m.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(m.Bucketer().Width(), ShouldEqual, 10)
		So(m.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)

		v, err = m.Get(c, "field")
		So(err, ShouldNotBeNil)

		err = m.Add(c, 5)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(v.Bucketer().Width(), ShouldEqual, 10)
		So(v.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)
		So(v.Sum(), ShouldEqual, 5)
		So(v.Count(), ShouldEqual, 1)
		So(err, ShouldBeNil)

		So(func() { NewCumulativeDistribution("cumul_dist", "description", nil, m.Bucketer()) }, ShouldPanic)
	})

	Convey("NonCumulativeDistribution", t, func() {
		c := makeContext()
		m := NewNonCumulativeDistribution("noncumul_dist", "description", nil, distribution.FixedWidthBucketer(10, 20))

		v, err := m.Get(c)
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)

		So(m.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(m.Bucketer().Width(), ShouldEqual, 10)
		So(m.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)

		v, err = m.Get(c, "field")
		So(err, ShouldNotBeNil)

		d := distribution.New(m.Bucketer())
		d.Add(15)
		err = m.Set(c, d)
		So(err, ShouldBeNil)

		v, err = m.Get(c)
		So(v.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(v.Bucketer().Width(), ShouldEqual, 10)
		So(v.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)
		So(v.Sum(), ShouldEqual, 15)
		So(v.Count(), ShouldEqual, 1)
		So(err, ShouldBeNil)

		So(func() { NewNonCumulativeDistribution("noncumul_dist", "description", nil, m.Bucketer()) }, ShouldPanic)
	})
}
