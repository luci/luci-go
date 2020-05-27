// Copyright 2020 The LUCI Authors.
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

package main

import (
	"strings"
	"testing"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/cmd/statsd-to-tsmon/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		cfg, err := loadConfig(&config.Config{
			Metrics: []*config.Metric{
				{
					Metric: "m1",
					Kind:   config.Kind_COUNTER,
					Fields: []string{"f1", "f2"},
					Rules: []*config.Rule{
						{
							Pattern: "*.foo.${var}.*.bar.sfx1",
							Fields: map[string]string{
								"f1": "static",
								"f2": "${var}",
							},
						},
					},
				},
				{
					Metric: "m2",
					Kind:   config.Kind_COUNTER,
					Rules: []*config.Rule{
						{
							Pattern: "foo.bar.sfx2",
						},
					},
				},
			},
		})
		So(err, ShouldBeNil)

		rule := func(m string) string {
			chunks := strings.Split(m, ".")
			bytes := make([][]byte, len(chunks))
			for i, c := range chunks {
				bytes[i] = []byte(c)
			}
			if r := cfg.FindMatchingRule(bytes); r != nil {
				return r.pattern.str
			}
			return ""
		}

		So(rule("xxx.foo.val.xxx.bar.sfx1"), ShouldEqual, "*.foo.${var}.*.bar.sfx1")
		So(rule("yyy.foo.val.yyy.bar.sfx1"), ShouldEqual, "*.foo.${var}.*.bar.sfx1")
		So(rule("foo.bar.sfx2"), ShouldEqual, "foo.bar.sfx2")

		// Wrong length.
		So(rule("foo.val.xxx.bar.sfx1"), ShouldEqual, "")
		// Wrong static component.
		So(rule("xxx.foo.val.xxx.baz.sfx1"), ShouldEqual, "")
		// Unknown suffix.
		So(rule("foo.bar.sfx3"), ShouldEqual, "")
		// Empty.
		So(rule(""), ShouldEqual, "")
	})

	Convey("Errors", t, func() {
		call := func(cfg *config.Config) error {
			_, err := loadConfig(cfg)
			return err
		}

		Convey("Bad pattern", func() {
			So(call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m3",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "."},
						},
					},
				},
			}), ShouldErrLike, `bad pattern`)
		})

		Convey("Not enough fields", func() {
			So(call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m4",
						Kind:   config.Kind_COUNTER,
						Fields: []string{"f1", "f2"},
						Rules: []*config.Rule{
							{Pattern: "foo", Fields: map[string]string{"f1": "value"}},
						},
					},
				},
			}), ShouldErrLike, `value of field "f2" is not provided`)
		})

		Convey("Extra field", func() {
			So(call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m5",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "foo", Fields: map[string]string{"f1": "value"}},
						},
					},
				},
			}), ShouldErrLike, `has too many fields`)
		})

		Convey("Bad field value", func() {
			So(call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m6",
						Kind:   config.Kind_COUNTER,
						Fields: []string{"f1"},
						Rules: []*config.Rule{
							{Pattern: "foo", Fields: map[string]string{"f1": "foo-${bar}"}},
						},
					},
				},
			}), ShouldErrLike, `field "f1" has bad value`)
		})

		Convey("Unknown var ref", func() {
			So(call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m7",
						Kind:   config.Kind_COUNTER,
						Fields: []string{"f1"},
						Rules: []*config.Rule{
							{Pattern: "foo", Fields: map[string]string{"f1": "${bar}"}},
						},
					},
				},
			}), ShouldErrLike, `field "f1" references undefined var "bar"`)
		})

		Convey("Not a static suffix", func() {
			So(call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m8",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "foo.*"},
						},
					},
				},
			}), ShouldErrLike, `must end with a static suffix`)
		})

		Convey("Dup suffix", func() {
			So(call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m9",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "foo1.bar"},
						},
					},
					{
						Metric: "m10",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "foo2.bar"},
						},
					},
				},
			}), ShouldErrLike, `there's already another rule with this suffix`)
		})
	})
}

func TestLoadMetrics(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		m, err := loadMetrics([]*config.Metric{
			{
				Metric: "gauge",
				Kind:   config.Kind_GAUGE,
				Units:  config.Unit_MILLISECONDS,
				Fields: []string{"f1", "f2"},
			},
			{
				Metric: "counter",
				Kind:   config.Kind_COUNTER,
				Units:  config.Unit_BYTES,
				Fields: []string{"f3", "f4"},
			},
			{
				Metric: "distribution",
				Kind:   config.Kind_CUMULATIVE_DISTRIBUTION,
				Fields: []string{"f5", "f6"},
			},
		})
		So(err, ShouldBeNil)
		So(m, ShouldHaveLength, 3)

		g, ok := m["gauge"].(metric.Int)
		So(ok, ShouldBeTrue)
		So(g.Metadata().Units, ShouldEqual, types.Milliseconds)
		So(g.Info().Fields, ShouldResemble, []field.Field{
			{Name: "f1", Type: field.StringType},
			{Name: "f2", Type: field.StringType},
		})

		c, ok := m["counter"].(metric.Counter)
		So(ok, ShouldBeTrue)
		So(c.Metadata().Units, ShouldEqual, types.Bytes)
		So(c.Info().Fields, ShouldResemble, []field.Field{
			{Name: "f3", Type: field.StringType},
			{Name: "f4", Type: field.StringType},
		})

		d, ok := m["distribution"].(metric.CumulativeDistribution)
		So(ok, ShouldBeTrue)
		So(d.Metadata().Units, ShouldEqual, types.Milliseconds)
		So(d.Info().Fields, ShouldResemble, []field.Field{
			{Name: "f5", Type: field.StringType},
			{Name: "f6", Type: field.StringType},
		})
	})
}

func TestPattern(t *testing.T) {
	t.Parallel()

	Convey("Parse success", t, func() {
		p, err := parsePattern("abc.${foo}.*.${bar}.zzz")
		So(err, ShouldBeNil)
		So(p, ShouldResemble, &pattern{
			str: "abc.${foo}.*.${bar}.zzz",
			len: 5,
			vars: map[string]int{
				"foo": 1,
				"bar": 3,
			},
			static: []staticNameComponent{
				{0, "abc"},
				{4, "zzz"},
			},
			suffix: "zzz",
		})
	})

	Convey("All static", t, func() {
		p, err := parsePattern("abc.def")
		So(err, ShouldBeNil)
		So(p, ShouldResemble, &pattern{
			str: "abc.def",
			len: 2,
			static: []staticNameComponent{
				{0, "abc"},
				{1, "def"},
			},
			suffix: "def",
		})
	})

	Convey("Empty component", t, func() {
		_, err := parsePattern("abc..zzz")
		So(err, ShouldErrLike, "empty name component")
	})

	Convey("Bad var", t, func() {
		_, err := parsePattern("${}")
		So(err, ShouldErrLike, "var name is required")

		_, err = parsePattern("foo-${bar}")
		So(err, ShouldErrLike, "is not allowed")
	})

	Convey("Dup var", t, func() {
		_, err := parsePattern("${abc}.${abc}")
		So(err, ShouldErrLike, "duplicate var")
	})

	Convey("Var suffix", t, func() {
		_, err := parsePattern("${abc}.${def}")
		So(err, ShouldErrLike, "must end with a static suffix")
	})

	Convey("Star suffix", t, func() {
		_, err := parsePattern("abc.*")
		So(err, ShouldErrLike, "must end with a static suffix")
	})
}
