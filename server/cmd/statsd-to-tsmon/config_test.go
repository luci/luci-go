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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/cmd/statsd-to-tsmon/config"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

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

		assert.Loosely(t, rule("xxx.foo.val.xxx.bar.sfx1"), should.Equal("*.foo.${var}.*.bar.sfx1"))
		assert.Loosely(t, rule("yyy.foo.val.yyy.bar.sfx1"), should.Equal("*.foo.${var}.*.bar.sfx1"))
		assert.Loosely(t, rule("foo.bar.sfx2"), should.Equal("foo.bar.sfx2"))

		// Wrong length.
		assert.Loosely(t, rule("foo.val.xxx.bar.sfx1"), should.BeEmpty)
		// Wrong static component.
		assert.Loosely(t, rule("xxx.foo.val.xxx.baz.sfx1"), should.BeEmpty)
		// Unknown suffix.
		assert.Loosely(t, rule("foo.bar.sfx3"), should.BeEmpty)
		// Empty.
		assert.Loosely(t, rule(""), should.BeEmpty)
	})

	ftt.Run("Errors", t, func(t *ftt.Test) {
		call := func(cfg *config.Config) error {
			_, err := loadConfig(cfg)
			return err
		}

		t.Run("Bad pattern", func(t *ftt.Test) {
			assert.Loosely(t, call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m3",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "."},
						},
					},
				},
			}), should.ErrLike(`bad pattern`))
		})

		t.Run("Not enough fields", func(t *ftt.Test) {
			assert.Loosely(t, call(&config.Config{
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
			}), should.ErrLike(`value of field "f2" is not provided`))
		})

		t.Run("Extra field", func(t *ftt.Test) {
			assert.Loosely(t, call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m5",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "foo", Fields: map[string]string{"f1": "value"}},
						},
					},
				},
			}), should.ErrLike(`has too many fields`))
		})

		t.Run("Bad field value", func(t *ftt.Test) {
			assert.Loosely(t, call(&config.Config{
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
			}), should.ErrLike(`field "f1" has bad value`))
		})

		t.Run("Unknown var ref", func(t *ftt.Test) {
			assert.Loosely(t, call(&config.Config{
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
			}), should.ErrLike(`field "f1" references undefined var "bar"`))
		})

		t.Run("Not a static suffix", func(t *ftt.Test) {
			assert.Loosely(t, call(&config.Config{
				Metrics: []*config.Metric{
					{
						Metric: "m8",
						Kind:   config.Kind_COUNTER,
						Rules: []*config.Rule{
							{Pattern: "foo.*"},
						},
					},
				},
			}), should.ErrLike(`must end with a static suffix`))
		})

		t.Run("Dup suffix", func(t *ftt.Test) {
			assert.Loosely(t, call(&config.Config{
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
			}), should.ErrLike(`there's already another rule with this suffix`))
		})
	})
}

func TestLoadMetrics(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, m, should.HaveLength(3))

		g, ok := m["gauge"].(metric.Int)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, g.Metadata().Units, should.Equal(types.Milliseconds))
		assert.Loosely(t, g.Info().Fields, should.Resemble([]field.Field{
			{Name: "f1", Type: field.StringType},
			{Name: "f2", Type: field.StringType},
		}))

		c, ok := m["counter"].(metric.Counter)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, c.Metadata().Units, should.Equal(types.Bytes))
		assert.Loosely(t, c.Info().Fields, should.Resemble([]field.Field{
			{Name: "f3", Type: field.StringType},
			{Name: "f4", Type: field.StringType},
		}))

		d, ok := m["distribution"].(metric.CumulativeDistribution)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, d.Metadata().Units, should.Equal(types.Milliseconds))
		assert.Loosely(t, d.Info().Fields, should.Resemble([]field.Field{
			{Name: "f5", Type: field.StringType},
			{Name: "f6", Type: field.StringType},
		}))
	})
}

func TestPattern(t *testing.T) {
	t.Parallel()

	ftt.Run("Parse success", t, func(t *ftt.Test) {
		p, err := parsePattern("abc.${foo}.*.${bar}.zzz")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, p, should.Resemble(&pattern{
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
		}))
	})

	ftt.Run("All static", t, func(t *ftt.Test) {
		p, err := parsePattern("abc.def")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, p, should.Resemble(&pattern{
			str: "abc.def",
			len: 2,
			static: []staticNameComponent{
				{0, "abc"},
				{1, "def"},
			},
			suffix: "def",
		}))
	})

	ftt.Run("Empty component", t, func(t *ftt.Test) {
		_, err := parsePattern("abc..zzz")
		assert.Loosely(t, err, should.ErrLike("empty name component"))
	})

	ftt.Run("Bad var", t, func(t *ftt.Test) {
		_, err := parsePattern("${}")
		assert.Loosely(t, err, should.ErrLike("var name is required"))

		_, err = parsePattern("foo-${bar}")
		assert.Loosely(t, err, should.ErrLike("is not allowed"))
	})

	ftt.Run("Dup var", t, func(t *ftt.Test) {
		_, err := parsePattern("${abc}.${abc}")
		assert.Loosely(t, err, should.ErrLike("duplicate var"))
	})

	ftt.Run("Var suffix", t, func(t *ftt.Test) {
		_, err := parsePattern("${abc}.${def}")
		assert.Loosely(t, err, should.ErrLike("must end with a static suffix"))
	})

	ftt.Run("Star suffix", t, func(t *ftt.Test) {
		_, err := parsePattern("abc.*")
		assert.Loosely(t, err, should.ErrLike("must end with a static suffix"))
	})
}
