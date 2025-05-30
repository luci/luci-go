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
	"os"
	"strings"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/server/cmd/statsd-to-tsmon/config"
)

// NameComponentIndex is an index of a statsd metric name components.
//
// E.g. in a metric "envoy.clusters.upstream.membership_healthy" the component
// "clusters" has index 1.
type NameComponentIndex int

// Config holds rules for converting statsd metrics into tsmon metrics.
//
// Each rule tells how to transform statsd metric name into a tsmon metric
// and its fields.
type Config struct {
	metrics   map[string]types.Metric // registered tsmon metrics
	perSuffix map[string]*Rule        // statsd metric suffix -> its conversion rule
}

// Rule describes how to send a tsmon metric given a matching statsd metric.
type Rule struct {
	// Metric is some concrete tsmon metric.
	//
	// Its set of fields matches `Fields`.
	Metric types.Metric

	// Fields describes how to assemble metric fields.
	//
	// Each item either a string for a preset field value or a NameComponentIndex
	// to grab field's value from parsed statsd metric name.
	Fields []any

	// Statsd metric name pattern, as taken from the config.
	pattern *pattern
}

// pattern is a parsed conversion rule pattern.
//
// Parsed pattern "*.cluster.${upstream}.membership_healthy" results in
//
//	pattern{
//	  str: "*.cluster.${upstream}.membership_healthy",
//	  len: 4,
//	  vars: {"upstream": 2},
//	  static: [{1, "cluster"}, {3, "membership_healthy"}]
//	  suffix: "membership_healthy",
//	}
type pattern struct {
	str    string
	len    int
	vars   map[string]int
	static []staticNameComponent
	suffix string
}

// staticNameComponent is static component of a pattern.
type staticNameComponent struct {
	index int
	value string
}

// LoadConfig parses and interprets the configuration file.
//
// It should be a config.Config proto encoded using jsonpb.
func LoadConfig(path string) (*Config, error) {
	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	msg, err := parseConfig(blob)
	if err != nil {
		return nil, err
	}
	return loadConfig(msg)
}

// FindMatchingRule finds a conversion rule that matches the statsd metric name.
//
// The metric name is given as a list of its components, e.g. "a.b.c" is
// represented by [][]byte{{'a'}, {'b'}, {'c'}}.
func (cfg *Config) FindMatchingRule(name [][]byte) *Rule {
	// Find the rule matching the suffix.
	if len(name) == 0 {
		return nil
	}
	rule := cfg.perSuffix[string(name[len(name)-1])]
	if rule == nil {
		return nil
	}

	// Skip if `name` doesn't match the rest of the pattern.
	if len(name) != rule.pattern.len {
		return nil
	}
	for _, s := range rule.pattern.static {
		if string(name[s.index]) != s.value {
			return nil
		}
	}

	return rule
}

// parseConfig converts config jsonpb into a proto.
func parseConfig(blob []byte) (*config.Config, error) {
	cfg := &config.Config{}
	if err := proto.UnmarshalText(string(blob), cfg); err != nil {
		return nil, errors.Fmt("bad config format: %w", err)
	}
	return cfg, nil
}

// loadConfig interprets the config proto.
func loadConfig(cfg *config.Config) (*Config, error) {
	metrics, err := loadMetrics(cfg.Metrics)
	if err != nil {
		return nil, err
	}

	perSuffix := map[string]*Rule{}

	for _, metricSpec := range cfg.Metrics {
		metric := metrics[metricSpec.Metric]
		for idx, ruleSpec := range metricSpec.Rules {
			if ruleSpec.Pattern == "" {
				return nil, errors.Fmt("metric %q: rule #%d: a pattern is required", metricSpec.Metric, idx+1)
			}

			rule, err := loadRule(metric, ruleSpec)
			if err != nil {
				return nil, errors.Fmt("metric %q: rule %q: %w", metricSpec.Metric, ruleSpec.Pattern, err)
			}

			if perSuffix[rule.pattern.suffix] != nil {
				return nil, errors.Fmt("metric %q: rule %q: there's already another rule with this suffix", metricSpec.Metric, ruleSpec.Pattern)
			}
			perSuffix[rule.pattern.suffix] = rule
		}
	}

	return &Config{
		metrics:   metrics,
		perSuffix: perSuffix,
	}, nil
}

// loadMetrics instantiates tsmon metrics based on the configuration.
func loadMetrics(cfg []*config.Metric) (map[string]types.Metric, error) {
	metrics := make(map[string]types.Metric, len(cfg))

	for idx, spec := range cfg {
		name := spec.Metric
		if name == "" {
			return nil, errors.Fmt("metric #%d: a name is required", idx+1)
		}
		if metrics[name] != nil {
			return nil, errors.Fmt("duplicate metric %q", name)
		}

		if len(spec.Fields) != stringset.NewFromSlice(spec.Fields...).Len() {
			return nil, errors.Fmt("metric %q: has duplicate fields", name)
		}
		tsmonFields := make([]field.Field, len(spec.Fields))
		for idx, fieldName := range spec.Fields {
			tsmonFields[idx] = field.String(fieldName)
		}

		var metadata types.MetricMetadata
		switch spec.Units {
		case config.Unit_MILLISECONDS:
			metadata.Units = types.Milliseconds
		case config.Unit_BYTES:
			metadata.Units = types.Bytes
		case config.Unit_UNIT_UNSPECIFIED:
			// no units, this is fine
		default:
			return nil, errors.Fmt("metric %q: unrecognized units %s", name, spec.Units)
		}

		var m types.Metric
		switch spec.Kind {
		case config.Kind_GAUGE:
			m = metric.NewInt(name, spec.Desc, &metadata, tsmonFields...)
		case config.Kind_COUNTER:
			m = metric.NewCounter(name, spec.Desc, &metadata, tsmonFields...)
		case config.Kind_CUMULATIVE_DISTRIBUTION:
			// Distributions are used for StatsdMetricTimer metrics, they are always
			// in milliseconds per statsd protocol.
			m = metric.NewCumulativeDistribution(
				name,
				spec.Desc,
				&types.MetricMetadata{Units: types.Milliseconds},
				distribution.DefaultBucketer,
				tsmonFields...)
		default:
			return nil, errors.Fmt("metric %q: unrecognized type %s", name, spec.Kind)
		}

		metrics[name] = m
	}

	return metrics, nil
}

// loadRule interprets single rule{...} config stanza.
func loadRule(metric types.Metric, spec *config.Rule) (*Rule, error) {
	pat, err := parsePattern(spec.Pattern)
	if err != nil {
		return nil, errors.Fmt("bad pattern: %w", err)
	}

	// Make sure the rule specifies all required fields and only them.
	tsmonFields := metric.Info().Fields
	fields := make([]any, len(tsmonFields))
	for idx, f := range tsmonFields {
		val, ok := spec.Fields[f.Name]
		if !ok {
			return nil, errors.Fmt("value of field %q is not provided", f.Name)
		}
		// Here `val` may be a variable (e.g. "${var}"), referring to a position
		// in the parsed pattern, or just some static string.
		vr, err := parseVar(val)
		if err != nil {
			return nil, errors.Fmt("field %q has bad value %q: %w", f.Name, val, err)
		}
		switch {
		case vr != "":
			componentIdx, ok := pat.vars[vr]
			if !ok {
				return nil, errors.Fmt("field %q references undefined var %q", f.Name, vr)
			}
			fields[idx] = NameComponentIndex(componentIdx)
		case val != "":
			fields[idx] = val // just a static string
		default:
			return nil, errors.Fmt("field %q has empty value, this is not allowed", f.Name)
		}
	}

	// We checked metricsDesc.fields is a subset of spec.Fields. Now check there
	// are in fact equal.
	if len(spec.Fields) != len(tsmonFields) {
		return nil, errors.New("has too many fields")
	}

	return &Rule{
		Metric:  metric,
		Fields:  fields,
		pattern: pat,
	}, nil
}

// parsePattern parses a string like "*.cluster.${upstream}.membership_healthy"
// into its components.
//
// Each var appearance must be unique and the pattern should end with a static
// suffix (i.e. not ".*" and not ".${var}").
func parsePattern(pat string) (*pattern, error) {
	chunks := strings.Split(pat, ".")
	p := &pattern{
		str: pat,
		len: len(chunks),
	}
	for idx, chunk := range chunks {
		if chunk == "" {
			return nil, errors.New("empty name component")
		}
		if chunk == "*" {
			continue // an index not otherwise mentioned in *pattern is a wildcard
		}
		switch vr, err := parseVar(chunk); {
		case err != nil:
			return nil, errors.Fmt("in name component %q: %w", chunk, err)
		case vr != "":
			if _, hasIt := p.vars[vr]; hasIt {
				return nil, errors.Fmt("duplicate var %q", vr)
			}
			if p.vars == nil {
				p.vars = make(map[string]int, 1)
			}
			p.vars[vr] = idx
		default:
			p.static = append(p.static, staticNameComponent{
				index: idx,
				value: chunk,
			})
		}
	}

	// We require suffixes to be static to simplify FindMatchingRule.
	if len(p.static) != 0 {
		if last := p.static[len(p.static)-1]; last.index == p.len-1 {
			p.suffix = last.value
		}
	}
	if p.suffix == "" {
		return nil, errors.New("must end with a static suffix")
	}

	return p, nil
}

// parseVar takes "${<something>}" and returns "<something>".
//
// If the input doesn't look like "${...}" and doesn't have "${" in it at all,
// returns an empty string and no error.
//
// If the input doesn't look like "${...}" but has "${" somewhere in it, returns
// an error: var usage such as "foo-${bar}" is not supported.
func parseVar(p string) (string, error) {
	if strings.HasPrefix(p, "${") && strings.HasSuffix(p, "}") {
		if len(p) == 3 {
			return "", errors.New("var name is required")
		}
		return p[2 : len(p)-1], nil
	}
	if strings.Contains(p, "${") {
		return "", errors.New("var usage such as `foo-${bar}` is not allowed")
	}
	return "", nil
}
