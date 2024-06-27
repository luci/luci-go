// Copyright 2024 The LUCI Authors.
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

package metrics

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/types"

	bbmetrics "go.chromium.org/luci/buildbucket/metrics"
	pb "go.chromium.org/luci/buildbucket/proto"
)

var cmKey = "custom_metrics"

// CustomMetric is an interface to report custom metrics.
type CustomMetric interface {
	isDisabled() bool
	setDisabled(disabled bool)
	getFields() []string
}

// customeMetricInfo holds the information about a registered custom metrics.
type customeMetricInfo struct {
	disabled bool
	// Metric fields.
	// Keep it here so that we could report the correct fields with correct order.
	fields []string
}

func (cmi *customeMetricInfo) isDisabled() bool {
	return cmi.disabled
}

func (cmi *customeMetricInfo) setDisabled(disabled bool) {
	cmi.disabled = disabled
}

func (cmi *customeMetricInfo) getFields() []string {
	return cmi.fields
}

type counter struct {
	metric.Counter
	*customeMetricInfo
}

type cumulativeDistribution struct {
	metric.CumulativeDistribution
	*customeMetricInfo
}

type float struct {
	metric.Float
	*customeMetricInfo
}

type int struct {
	metric.Int
	*customeMetricInfo
}

// withCustomMetrics returns a new context with the given map of custom metrics.
func withCustomMetrics(ctx context.Context, cms *CustomMetrics) context.Context {
	return context.WithValue(ctx, &cmKey, cms)
}

// getCustomMetrics returns the CustomMetrics intalled in the current context.
// Panic if not found in the context.
func getCustomMetrics(ctx context.Context) *CustomMetrics {
	cms := ctx.Value(&cmKey)
	if cms == nil {
		panic("missing custom metrics from context")
	}
	return cms.(*CustomMetrics)
}

// CustomMetrics holds the custom metrics and a dedicated tsmon state to report
// metrics to.
type CustomMetrics struct {
	metrics map[pb.CustomBuildMetricBase]map[string]CustomMetric
	state   *tsmon.State
}

// WithCustomMetrics creates a CustomMetrics with dedicated tsmon.State to
// register custom metrics from service config, and store it in context.
//
// This function should only be called at server start up.
func WithCustomMetrics(ctx context.Context, globalCfg *pb.SettingsCfg) (context.Context, error) {
	cmsInCtx, err := newCustomMetrics(globalCfg)
	if err != nil {
		return ctx, err
	}
	return withCustomMetrics(ctx, cmsInCtx), nil
}

// newCustomMetrics creates a CustomMetrics with dedicated tsmon.State to
// register custom metrics from service config.
func newCustomMetrics(globalCfg *pb.SettingsCfg) (*CustomMetrics, error) {
	state := newState()
	v2Custom, err := createCustomMetricMap(globalCfg, state.Registry())
	if err != nil {
		return nil, err
	}

	return &CustomMetrics{
		metrics: v2Custom,
		state:   state,
	}, nil
}

func newState() *tsmon.State {
	state := tsmon.NewState()
	state.SetRegistry(registry.NewRegistry())
	state.SetStore(store.NewInMemory(&bbmetrics.BuilderTarget{}))
	return state
}

func createCustomMetricMap(globalCfg *pb.SettingsCfg, registry *registry.Registry) (map[pb.CustomBuildMetricBase]map[string]CustomMetric, error) {
	v2Custom := make(map[pb.CustomBuildMetricBase]map[string]CustomMetric)
	for _, cm := range globalCfg.CustomMetrics {
		ms, ok := v2Custom[cm.GetMetricBase()]
		if !ok {
			ms = make(map[string]CustomMetric)
			v2Custom[cm.GetMetricBase()] = ms
		}
		nm, err := newMetric(cm, registry)
		if err != nil {
			return nil, err
		}
		ms[cm.Name] = nm
	}
	return v2Custom, nil
}

func newMetric(cm *pb.CustomMetric, registry *registry.Registry) (CustomMetric, error) {
	fields := stringsToFields(cm.Fields)

	opt := &metric.Options{
		TargetType: (&bbmetrics.BuilderTarget{}).Type(),
		Registry:   registry,
	}

	info := &customeMetricInfo{
		disabled: cm.Disabled,
		fields:   cm.Fields,
	}

	switch base := cm.GetMetricBase(); base {
	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_CREATED,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_STARTED,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_COMPLETED:
		return &counter{
			metric.NewCounterWithOptions(cm.Name, opt, cm.Description, nil, fields...),
			info,
		}, nil

	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_CYCLE_DURATIONS,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_RUN_DURATIONS,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_SCHEDULING_DURATIONS:
		return &cumulativeDistribution{
			metric.NewCumulativeDistributionWithOptions(
				cm.Name, opt, cm.Description,
				&types.MetricMetadata{Units: types.Seconds},
				bucketer,
				fields...), info}, nil

	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_MAX_AGE_SCHEDULED:
		return &float{
			metric.NewFloatWithOptions(
				cm.Name, opt, cm.Description,
				&types.MetricMetadata{Units: types.Seconds}, fields...), info}, nil

	case pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_COUNT,
		pb.CustomBuildMetricBase_CUSTOM_BUILD_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT:
		return &int{
			metric.NewIntWithOptions(cm.Name, opt, cm.Description, nil, fields...), info}, nil

	default:
		return nil, errors.Reason("invalid metric base %s", base).Err()
	}
}
