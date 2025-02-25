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
	"reflect"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
)

func makeContext() context.Context {
	ret, _ := tsmon.WithDummyInMemory(context.Background())
	return ret
}

func TestMetrics(t *testing.T) {
	t.Parallel()
	tt := target.TaskType
	opt := &Options{
		TargetType: tt,
	}

	ftt.Run("Int", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewIntWithOptions("int", opt, "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewIntWithOptions("int", opt, "description", nil) },
			should.Panic)

		assert.Loosely(t, m.Get(c), should.BeZero)
		m.Set(c, 42)
		assert.Loosely(t, m.Get(c), should.Equal(42))

		assert.Loosely(t, func() { m.Set(c, 42, "field") }, should.Panic)
		assert.Loosely(t, func() { m.Get(c, "field") }, should.Panic)
	})

	ftt.Run("Counter", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewCounterWithOptions("counter", opt, "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewCounterWithOptions("counter", opt, "description", nil) },
			should.Panic)

		assert.Loosely(t, m.Get(c), should.BeZero)

		m.Add(c, 3)
		assert.Loosely(t, m.Get(c), should.Equal(3))

		m.Add(c, 2)
		assert.Loosely(t, m.Get(c), should.Equal(5))
	})

	ftt.Run("Float", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewFloatWithOptions("float", opt, "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewFloatWithOptions("float", opt, "description", nil) },
			should.Panic)

		assert.Loosely(t, m.Get(c), should.AlmostEqual(0.0))

		m.Set(c, 42.3)
		assert.Loosely(t, m.Get(c), should.AlmostEqual(42.3))

		assert.Loosely(t, func() { m.Set(c, 42.3, "field") }, should.Panic)
		assert.Loosely(t, func() { m.Get(c, "field") }, should.Panic)
	})

	ftt.Run("FloatCounter", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewFloatCounterWithOptions("float_counter", opt, "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewFloatCounterWithOptions("float_counter", opt, "description", nil) },
			should.Panic)

		assert.Loosely(t, m.Get(c), should.AlmostEqual(0.0))

		m.Add(c, 3.1)
		assert.Loosely(t, m.Get(c), should.AlmostEqual(3.1))

		m.Add(c, 2.2)
		assert.Loosely(t, m.Get(c), should.AlmostEqual(5.3, 0.00000000000001))
	})

	ftt.Run("String", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewStringWithOptions("string", opt, "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t, func() { NewStringWithOptions("string", opt, "description", nil) }, should.Panic)

		assert.Loosely(t, m.Get(c), should.BeEmpty)

		m.Set(c, "hello")
		assert.Loosely(t, m.Get(c), should.Equal("hello"))

		assert.Loosely(t, func() { m.Set(c, "hello", "field") }, should.Panic)
		assert.Loosely(t, func() { m.Get(c, "field") }, should.Panic)
	})

	ftt.Run("Bool", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewBoolWithOptions("bool", opt, "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewBoolWithOptions("bool", opt, "description", nil) },
			should.Panic)

		assert.Loosely(t, m.Get(c), should.Equal(false))

		m.Set(c, true)
		assert.Loosely(t, m.Get(c), should.BeTrue)

		assert.Loosely(t, func() { m.Set(c, true, "field") }, should.Panic)
		assert.Loosely(t, func() { m.Get(c, "field") }, should.Panic)
	})

	ftt.Run("CumulativeDistribution", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewCumulativeDistributionWithOptions("cumul_dist", opt, "description", nil, distribution.FixedWidthBucketer(10, 20))
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t, func() { NewCumulativeDistributionWithOptions("cumul_dist", opt, "description", nil, m.Bucketer()) }, should.Panic)

		assert.Loosely(t, m.Bucketer().GrowthFactor(), should.BeZero)
		assert.That(t, m.Bucketer().Width(), should.Equal(10.0))
		assert.Loosely(t, m.Bucketer().NumFiniteBuckets(), should.Equal(20))

		assert.Loosely(t, m.Get(c), should.BeNil)

		m.Add(c, 5)

		v := m.Get(c)
		assert.Loosely(t, v.Bucketer().GrowthFactor(), should.BeZero)
		assert.That(t, v.Bucketer().Width(), should.Equal(10.0))
		assert.Loosely(t, v.Bucketer().NumFiniteBuckets(), should.Equal(20))
		assert.That(t, v.Sum(), should.Equal(5.0))
		assert.Loosely(t, v.Count(), should.Equal(1))

		assert.Loosely(t, func() { m.Add(c, 5, "field") }, should.Panic)
		assert.Loosely(t, func() { m.Get(c, "field") }, should.Panic)
	})

	ftt.Run("NonCumulativeDistribution", t, func(t *ftt.Test) {
		c := makeContext()
		m := NewNonCumulativeDistributionWithOptions("noncumul_dist", opt, "description", nil, distribution.FixedWidthBucketer(10, 20))
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t, func() {
			NewNonCumulativeDistributionWithOptions("noncumul_dist", opt, "description", nil, m.Bucketer())
		}, should.Panic)

		assert.Loosely(t, m.Bucketer().GrowthFactor(), should.BeZero)
		assert.That(t, m.Bucketer().Width(), should.Equal(10.0))
		assert.Loosely(t, m.Bucketer().NumFiniteBuckets(), should.Equal(20))

		assert.Loosely(t, m.Get(c), should.BeNil)

		d := distribution.New(m.Bucketer())
		d.Add(15)
		m.Set(c, d)

		v := m.Get(c)
		assert.Loosely(t, v.Bucketer().GrowthFactor(), should.BeZero)
		assert.That(t, v.Bucketer().Width(), should.Equal(10.0))
		assert.Loosely(t, v.Bucketer().NumFiniteBuckets(), should.Equal(20))
		assert.That(t, v.Sum(), should.Equal(15.0))
		assert.Loosely(t, v.Count(), should.Equal(1))

		assert.Loosely(t, func() { m.Set(c, d, "field") }, should.Panic)
		assert.Loosely(t, func() { m.Get(c, "field") }, should.Panic)
	})
}

func TestMetricsDefaultTargetType(t *testing.T) {
	t.Parallel()

	// These tests ensure that metrics are given target.NilType, if created
	// without a target type specified.
	tt := target.NilType
	opt := &Options{
		TargetType: tt,
	}

	ftt.Run("Int", t, func(t *ftt.Test) {
		m := NewInt("int", "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewIntWithOptions("int", opt, "description", nil) },
			should.Panic)
	})

	ftt.Run("Counter", t, func(t *ftt.Test) {
		m := NewCounter("counter", "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewCounterWithOptions("counter", opt, "description", nil) },
			should.Panic)
	})

	ftt.Run("Float", t, func(t *ftt.Test) {
		m := NewFloat("float", "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewFloatWithOptions("float", opt, "description", nil) },
			should.Panic)
	})

	ftt.Run("FloatCounter", t, func(t *ftt.Test) {
		m := NewFloatCounter("float_counter", "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewFloatCounterWithOptions("float_counter", opt, "description", nil) },
			should.Panic)
	})

	ftt.Run("String", t, func(t *ftt.Test) {
		m := NewString("string", "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t, func() { NewStringWithOptions("string", opt, "description", nil) }, should.Panic)
	})

	ftt.Run("Bool", t, func(t *ftt.Test) {
		m := NewBool("bool", "description", nil)
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t,
			func() { NewBoolWithOptions("bool", opt, "description", nil) },
			should.Panic)
	})

	ftt.Run("CumulativeDistribution", t, func(t *ftt.Test) {
		m := NewCumulativeDistribution("cumul_dist", "description", nil, distribution.FixedWidthBucketer(10, 20))
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t, func() { NewCumulativeDistributionWithOptions("cumul_dist", opt, "description", nil, m.Bucketer()) }, should.Panic)

	})

	ftt.Run("NonCumulativeDistribution", t, func(t *ftt.Test) {
		m := NewNonCumulativeDistribution("noncumul_dist", "description", nil, distribution.FixedWidthBucketer(10, 20))
		assert.Loosely(t, m.Info().TargetType, should.Match(tt))
		assert.Loosely(t, func() {
			NewNonCumulativeDistributionWithOptions("noncumul_dist", opt, "description", nil, m.Bucketer())
		}, should.Panic)
	})
}

func TestMetricsWithMultipleTargets(t *testing.T) {
	t.Parallel()
	testTaskTargets := []target.Task{{TaskNum: 0}, {TaskNum: 1}}
	testDeviceTargets := []target.NetworkDevice{{Hostname: "a"}}

	ftt.Run("with a single TargetType", t, func(t *ftt.Test) {
		c := makeContext()
		opt := &Options{TargetType: target.TaskType}

		t.Run("with a single target in context", func(t *ftt.Test) {
			m := NewIntWithOptions("m_with_s_s", opt, "desc", nil)
			tctx := target.Set(c, &testTaskTargets[0])
			assert.Loosely(t, m.Get(tctx), should.BeZero)
			m.Set(tctx, 42)
			assert.Loosely(t, m.Get(tctx), should.Equal(42))
		})

		t.Run("with multiple targets in context", func(t *ftt.Test) {
			m := NewIntWithOptions("m_with_s_m", opt, "desc", nil)
			tctx0 := target.Set(c, &testTaskTargets[0])
			tctx1 := target.Set(tctx0, &testTaskTargets[1])
			assert.Loosely(t, m.Get(tctx0), should.BeZero)
			assert.Loosely(t, m.Get(tctx1), should.BeZero)
			m.Set(tctx0, 41)
			m.Set(tctx1, 42)
			assert.Loosely(t, m.Get(tctx0), should.Equal(41))
			assert.Loosely(t, m.Get(tctx1), should.Equal(42))
		})
	})

	ftt.Run("with multiple TargetTypes", t, func(t *ftt.Test) {
		c := makeContext()

		t.Run("with a single target in context for each type", func(t *ftt.Test) {
			tctx := target.Set(
				target.Set(c, &testTaskTargets[0]), &testDeviceTargets[0],
			)

			// two metrics with the same name, but different types.
			mDevice := NewIntWithOptions("m_with_m_s", &Options{TargetType: target.DeviceType}, "desc", nil)
			mTask := NewIntWithOptions("m_with_m_s", &Options{TargetType: target.TaskType}, "desc", nil)

			assert.Loosely(t, mTask.Get(tctx), should.BeZero)
			assert.Loosely(t, mDevice.Get(tctx), should.BeZero)
			mTask.Set(tctx, 41)
			mDevice.Set(tctx, 42)
			assert.Loosely(t, mTask.Get(tctx), should.Equal(41))
			assert.Loosely(t, mDevice.Get(tctx), should.Equal(42))
		})
	})
}

// To avoid import cycle, unit tests for Registry with metrics are implemented
// here.
func TestMetricWithRegistry(t *testing.T) {
	t.Parallel()

	ftt.Run("A single metric", t, func(t *ftt.Test) {
		t.Run("with TargetType", func(t *ftt.Test) {
			metric := NewIntWithOptions("registry/test/1", &Options{TargetType: target.TaskType}, "desc", nil)
			var registered types.Metric
			registry.Global.Iter(func(m types.Metric) {
				if reflect.DeepEqual(m.Info(), metric.Info()) {
					registered = m
				}
			})
			assert.Loosely(t, registered, should.NotBeNil)
		})
		t.Run("without TargetType", func(t *ftt.Test) {
			metric := NewInt("registry/test/1", "desc", nil)
			var registered types.Metric
			registry.Global.Iter(func(m types.Metric) {
				if reflect.DeepEqual(m.Info(), metric.Info()) {
					registered = m
				}
			})
			assert.Loosely(t, registered, should.NotBeNil)
		})
	})

	ftt.Run("Multiple metrics", t, func(t *ftt.Test) {
		opt := &Options{TargetType: target.TaskType}
		t.Run("with the same metric name and targe type", func(t *ftt.Test) {
			NewIntWithOptions("registry/test/2", opt, "desc", nil)
			assert.Loosely(t, func() {
				NewIntWithOptions("registry/test/2", opt, "desc", nil)
			}, should.Panic)
		})

		t.Run("with the same metric name, but different target type", func(t *ftt.Test) {
			mTask := NewIntWithOptions("registry/test/3", opt, "desc", nil)
			mDevice := NewIntWithOptions("registry/test/3", &Options{TargetType: target.DeviceType}, "desc", nil)
			mNil := NewInt("registry/test/3", "desc", nil)

			var rTask, rDevice, rNil types.Metric
			registry.Global.Iter(func(m types.Metric) {
				if reflect.DeepEqual(m.Info(), mTask.Info()) {
					rTask = m
				} else if reflect.DeepEqual(m.Info(), mDevice.Info()) {
					rDevice = m
				} else if reflect.DeepEqual(m.Info(), mNil.Info()) {
					rNil = m
				}
			})

			assert.Loosely(t, rTask, should.NotBeNil)
			assert.Loosely(t, rDevice, should.NotBeNil)
			assert.Loosely(t, rNil, should.NotBeNil)
		})
	})
}

func TestMetricsWithTimeUnit(t *testing.T) {
	t.Parallel()
	anno := &types.MetricMetadata{Units: types.Seconds}

	ftt.Run("check", t, func(t *ftt.Test) {
		t.Run("with types that time unit can be given to", func(t *ftt.Test) {
			NewInt("time/int", "desc", anno)
			NewCounter("time/counter", "desc", anno)
			NewFloat("time/float", "desc", anno)
			NewFloatCounter("time/float_counter", "desc", anno)

			bkt := distribution.FixedWidthBucketer(10, 20)
			NewCumulativeDistribution("time/cdist", "desc", anno, bkt)
			NewNonCumulativeDistribution("time/ncdist", "desc", anno, bkt)
		})

		t.Run("with types that should panic if a time unit is given", func(t *ftt.Test) {
			assert.Loosely(t, func() { NewString("time/string", "desc", anno) }, should.Panic)
			assert.Loosely(t, func() { NewBool("time/bool", "desc", anno) }, should.Panic)
		})
	})
}
