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
	"fmt"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/types"

	bbmetrics "go.chromium.org/luci/buildbucket/metrics"
	pb "go.chromium.org/luci/buildbucket/proto"
)

var cmKey = "custom_metrics"

var currentCustomMetrics stringset.Set

// CustomMetric is an interface to report custom metrics.
type CustomMetric interface {
	getFields() []string
	report(ctx context.Context, value any, fields []any)
}

// customeMetricInfo holds the information about a registered custom metrics.
type customeMetricInfo struct {
	// Metric fields.
	// Keep it here so that we could report the correct fields with correct order.
	fields []string
}

func (cmi *customeMetricInfo) getFields() []string {
	return cmi.fields
}

type counter struct {
	metric.Counter
	*customeMetricInfo
}

func (c *counter) report(ctx context.Context, value any, fields []any) {
	v, ok := value.(int64)
	if !ok {
		panic(fmt.Sprintf("cannot convert %q to int64", value))
	}
	c.Add(ctx, v, fields...)
}

type cumulativeDistribution struct {
	metric.CumulativeDistribution
	*customeMetricInfo
}

func (cd *cumulativeDistribution) report(ctx context.Context, value any, fields []any) {
	v, ok := value.(float64)
	if !ok {
		panic(fmt.Sprintf("cannot convert %q to float64", value))
	}
	cd.Add(ctx, v, fields...)
}

type float struct {
	metric.Float
	*customeMetricInfo
}

func (f *float) report(ctx context.Context, value any, fields []any) {
	v, ok := value.(float64)
	if !ok {
		panic(fmt.Sprintf("cannot convert %q to float64", value))
	}
	f.Set(ctx, v, fields...)
}

type int struct {
	metric.Int
	*customeMetricInfo
}

func (i *int) report(ctx context.Context, value any, fields []any) {
	v, ok := value.(int64)
	if !ok {
		panic(fmt.Sprintf("cannot convert %q to int64", value))
	}
	i.Set(ctx, v, fields...)
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
	buf     chan *Report
	m       sync.RWMutex
}

type Report struct {
	Base     pb.CustomBuildMetricBase
	Name     string
	FieldMap map[string]string
	Value    any
}

// Report tries to report a metric. If it couldn't do the report directly (e.g.
// a metric state recreation is happening), it will save the report to a buffer.
// Returns a bool to indicate if it's a direct report or not (mostly for testing).
func (cms *CustomMetrics) Report(ctx context.Context, r *Report) bool {
	cms.m.RLock()
	defer cms.m.RUnlock()

	if cms.state != nil {
		cm := cms.metrics[r.Base][r.Name]
		if cm == nil {
			return false
		}
		fields := convertFields(cm.getFields(), r.FieldMap)
		ctx = tsmon.WithState(ctx, cms.state)
		cm.report(ctx, r.Value, fields)
		return true
	}

	// Updating metrics. Save the report in buffer for now.
	select {
	case cms.buf <- r:
	default:
		// This can happen only if Flush is stuck for too long for some reason.
		logging.Errorf(ctx, "Custom metric buffer is full, dropping the report")
	}

	return false
}

func convertFields(fieldKeys []string, fieldMap map[string]string) []any {
	fields := make([]any, len(fieldKeys))
	for i, fk := range fieldKeys {
		fields[i] = fieldMap[fk]
	}
	return fields
}

// Flush flushes all stored metrics.
// But before flush, it also checks if there's any change in service config
// that requires to update cms.metrics and recreate cms.state.
// If yes, it'll reset cms.state - so the upcoming reports can be
// buffered, then perform the updates.
// After cms.metrics are updated, it then adds all the buffered reports back
// to the new cms.state.
//
// Called by a backendground job periodically. Must not be called concurrently.
func (cms *CustomMetrics) Flush(ctx context.Context, globalCfg *pb.SettingsCfg) error {
	cms.m.RLock()
	state := cms.state
	metrics := cms.metrics
	cms.m.RUnlock()

	if state == nil {
		panic("no state found for custom metrics")
	}

	// Updates on custom metrics in service config should be rare, check if
	// an update is needed first.
	newCmsInCfg := convertCustomMetricConfig(globalCfg)
	needUpdate := checkUpdates(newCmsInCfg)
	if !needUpdate {
		return state.Flush(ctx, nil)
	}

	// Need to update cms.metrics and recreate cms.state.
	// Reset cms.state for now so that we could do it safely.
	buf := make(chan *Report, 10000)
	cms.m.Lock()
	if cms.state != state {
		panic("unexpected state")
	}
	cms.state = nil
	cms.buf = buf
	cms.m.Unlock()

	defer func() {
		cms.m.Lock()
		if cms.state != nil {
			panic("unexpected state")
		}
		// Restore cms.state and cms.metrics after metrics update (and perhaps
		// state recreation).
		// We are inside **the exclusive** lock. It means there's no one
		// holding the reader lock, i.e. all `cms.buf <- r` calls are done.
		// Once we pass this section, new reports will be written to
		// `state` directly.
		cms.state = state
		cms.metrics = metrics
		cms.buf = nil
		cms.m.Unlock()

		// Wait for all reports to be added to buffer then report all of them.
		// They will be flushed in the next Flush.
		// By now cms.state has been either recreated or restored, so new Reports
		// will stop using the buffer.
		// Should be safe since Flush is called by a backendground job periodically,
		// so there should not be multiple Flush running currenctly.
		close(buf)
		for rep := range buf {
			cms.Report(ctx, rep)
		}
	}()

	// Flush existing reports in state.
	err := state.Flush(ctx, nil)
	if err != nil {
		return err
	}

	// Recreate metrics and state.
	// It is safe to just replace cms.state, since we flushed all data in it and
	// all new data is currently being accumulated in cms.buf.
	// update cms.state and cms.metrics according to new config.
	newCms, err := newCustomMetrics(globalCfg)
	if err == nil {
		metrics, state = newCms.metrics, newCms.state
		currentCustomMetrics = newCmsInCfg
	}
	return err
}

// checkUpdates checks if there's any update for custom metrics in service config.
func checkUpdates(new stringset.Set) bool {
	return currentCustomMetrics.Difference(new).Len() != 0 || new.Difference(currentCustomMetrics).Len() != 0
}

// convertCustomMetricConfig converts custom metrics from globalCfg to a stringset
// so it'll be easier to check differences of custom metrics from different
// versions of globalCfgs.
func convertCustomMetricConfig(globalCfg *pb.SettingsCfg) stringset.Set {
	cmSet := stringset.New(0)
	for _, cm := range globalCfg.CustomMetrics {
		cmStr := fmt.Sprintf("%s:%s:%q", cm.GetMetricBase(), cm.Name, cm.Fields)
		cmSet.Add(cmStr)
	}
	return cmSet
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
	currentCustomMetrics = convertCustomMetricConfig(globalCfg)
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
		fields: cm.Fields,
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
