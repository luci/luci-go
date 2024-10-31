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
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/types"

	"go.chromium.org/luci/buildbucket/appengine/common/buildcel"
	"go.chromium.org/luci/buildbucket/appengine/model"
	bbmetrics "go.chromium.org/luci/buildbucket/metrics"
	pb "go.chromium.org/luci/buildbucket/proto"
)

var cmKey = "custom_metrics"

var currentCustomMetrics stringset.Set

// lock for currentCustomMetrics.
// This lock is mainly for tests. In production, we expect no concurrent read/write
// on currentCustomMetrics, since we use it once when server starts up then
// subsequently during Flushes which are backendground jobs.
var currentCMsLock sync.RWMutex

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

// GetCustomMetrics returns the CustomMetrics intalled in the current context.
// Panic if not found in the context.
func GetCustomMetrics(ctx context.Context) *CustomMetrics {
	cms := ctx.Value(&cmKey)
	if cms == nil {
		panic("missing custom metrics from context")
	}
	return cms.(*CustomMetrics)
}

// CustomMetrics holds the custom metrics and a dedicated tsmon state to report
// metrics to.
type CustomMetrics struct {
	metrics map[pb.CustomMetricBase]map[string]CustomMetric
	state   *tsmon.State
	buf     chan *Report
	m       sync.RWMutex
}

type Report struct {
	Base     pb.CustomMetricBase
	Name     string
	FieldMap map[string]string
	Value    any
}

func (cms *CustomMetrics) getCustomMetricsByBases(bases map[pb.CustomMetricBase]any) map[pb.CustomMetricBase]map[string]CustomMetric {
	res := make(map[pb.CustomMetricBase]map[string]CustomMetric, len(bases))
	cms.m.RLock()
	defer cms.m.RUnlock()
	for b := range bases {
		res[b] = cms.metrics[b]
	}
	return res
}

func (cms *CustomMetrics) getCustomMetricsByBase(base pb.CustomMetricBase) map[string]CustomMetric {
	cms.m.RLock()
	defer cms.m.RUnlock()
	return cms.metrics[base]
}

// Report tries to report one or more metric. If it couldn't do the report
// directly (e.g. a metric state recreation is happening), it will save the
// reports to a buffer.
// Returns a bool to indicate if it's a direct report or not (mostly for testing).
func (cms *CustomMetrics) Report(ctx context.Context, rs ...*Report) bool {
	cms.m.RLock()
	defer cms.m.RUnlock()

	if cms.state != nil {
		for _, r := range rs {
			cm := cms.metrics[r.Base][r.Name]
			if cm == nil {
				return false
			}
			fields := convertFields(cm.getFields(), r.FieldMap)
			ctx = tsmon.WithState(ctx, cms.state)
			cm.report(ctx, r.Value, fields)
		}
		return true
	}

	// Updating metrics. Save the report in buffer for now.
	for _, r := range rs {
		select {
		case cms.buf <- r:
		default:
			// This can happen only if Flush is stuck for too long for some reason.
			logging.Errorf(ctx, "Custom metric buffer is full, dropping the report")
		}
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
func (cms *CustomMetrics) Flush(ctx context.Context, globalCfg *pb.SettingsCfg, mon monitor.Monitor) error {
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
		return state.ParallelFlush(ctx, mon, 4)
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
	err := state.ParallelFlush(ctx, mon, 4)
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
		currentCMsLock.Lock()
		currentCustomMetrics = newCmsInCfg
		currentCMsLock.Unlock()
	}
	return err
}

// checkUpdates checks if there's any update for custom metrics in service config.
func checkUpdates(new stringset.Set) bool {
	currentCMsLock.RLock()
	defer currentCMsLock.RUnlock()
	return currentCustomMetrics.Difference(new).Len() != 0 || new.Difference(currentCustomMetrics).Len() != 0
}

// convertCustomMetricConfig converts custom metrics from globalCfg to a stringset
// so it'll be easier to check differences of custom metrics from different
// versions of globalCfgs.
func convertCustomMetricConfig(globalCfg *pb.SettingsCfg) stringset.Set {
	cmSet := stringset.New(0)
	for _, cm := range globalCfg.CustomMetrics {
		cmStr := fmt.Sprintf("%s:%s:%q", cm.GetMetricBase(), cm.Name, cm.ExtraFields)
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
	currentCMsLock.Lock()
	currentCustomMetrics = convertCustomMetricConfig(globalCfg)
	currentCMsLock.Unlock()
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

func createCustomMetricMap(globalCfg *pb.SettingsCfg, registry *registry.Registry) (map[pb.CustomMetricBase]map[string]CustomMetric, error) {
	v2Custom := make(map[pb.CustomMetricBase]map[string]CustomMetric)
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
	// Add the default metric fields.
	base := cm.GetMetricBase()
	commonFields, err := GetCommonBaseFields(base)
	if err != nil {
		return nil, err
	}
	fieldStrs := make([]string, 0, len(commonFields)+len(cm.ExtraFields))
	fieldStrs = append(fieldStrs, commonFields...)
	fieldStrs = append(fieldStrs, cm.ExtraFields...)
	fields := stringsToFields(fieldStrs)

	opt := &metric.Options{
		TargetType: (&bbmetrics.BuilderTarget{}).Type(),
		Registry:   registry,
	}

	info := &customeMetricInfo{
		fields: fieldStrs,
	}

	switch base {
	case pb.CustomMetricBase_CUSTOM_METRIC_BASE_CREATED,
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED,
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED:
		return &counter{
			metric.NewCounterWithOptions(cm.Name, opt, cm.Description, nil, fields...),
			info,
		}, nil

	case pb.CustomMetricBase_CUSTOM_METRIC_BASE_CYCLE_DURATIONS,
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_RUN_DURATIONS,
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_SCHEDULING_DURATIONS:
		return &cumulativeDistribution{
			metric.NewCumulativeDistributionWithOptions(
				cm.Name, opt, cm.Description,
				&types.MetricMetadata{Units: types.Seconds},
				bucketer,
				fields...), info}, nil

	case pb.CustomMetricBase_CUSTOM_METRIC_BASE_MAX_AGE_SCHEDULED:
		return &float{
			metric.NewFloatWithOptions(
				cm.Name, opt, cm.Description,
				&types.MetricMetadata{Units: types.Seconds}, fields...), info}, nil

	case pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
		pb.CustomMetricBase_CUSTOM_METRIC_BASE_CONSECUTIVE_FAILURE_COUNT:
		return &int{
			metric.NewIntWithOptions(cm.Name, opt, cm.Description, nil, fields...), info}, nil

	default:
		return nil, errors.Reason("invalid metric base %s", base).Err()
	}
}

func buildCustomMetricsToReport(b *model.Build, bases map[pb.CustomMetricBase]any) map[pb.CustomMetricBase][]*pb.CustomMetricDefinition {
	toReport := make(map[pb.CustomMetricBase][]*pb.CustomMetricDefinition)
	for _, cm := range b.CustomMetrics {
		if _, ok := bases[cm.Base]; ok {
			toReport[cm.Base] = append(toReport[cm.Base], cm.Metric)
		}
	}
	return toReport
}

// getBuildDetails gets all fields for a build so it could be evaluated by
// the cel expressions later.
// TODO(b/338071541): we could also examine cel expressions to generate a
// read mask for all needed fields, instead of getting everything.
func getBuildDetails(ctx context.Context, b *model.Build) *pb.Build {
	m, err := model.NewBuildMask("", nil, &pb.BuildMask{AllFields: true})
	if err != nil {
		logging.Errorf(ctx, "failed to create build mask for all fields: %s", err)
		return nil
	}
	build, err := b.ToProto(ctx, m, func(b *pb.Build) error { return nil })
	if err != nil {
		logging.Errorf(ctx, "failed to get details for build %d: %s", b.ID, err)
		return nil
	}
	return build
}

// Report to custom metrics.
func reportToCustomMetrics(ctx context.Context, build *model.Build, cmValues map[pb.CustomMetricBase]any) {
	// Get custom metrics saved with the build.
	toReport := buildCustomMetricsToReport(build, cmValues)
	if len(toReport) == 0 {
		return
	}

	// Get current active custom metrics.
	cms := GetCustomMetrics(ctx)
	metrics := cms.getCustomMetricsByBases(cmValues)

	// Filter out the inactive custom metrics from the build.
	hasCMsUpdated := false
	toReportUpdated := make(map[pb.CustomMetricBase][]*pb.CustomMetricDefinition, len(toReport))
	for b, ms := range toReport {
		acms := metrics[b]
		if len(acms) == 0 {
			logging.Infof(ctx, "Skipping reporting custom metrics with base %s for build %d, likely due to changes to registered custom metrics", b, build.ID)
			continue
		}
		hasCMsUpdated = true
		toReportUpdated[b] = make([]*pb.CustomMetricDefinition, 0, len(ms))
		for _, m := range ms {
			if _, ok := acms[m.Name]; ok {
				toReportUpdated[b] = append(toReportUpdated[b], m)
			}
		}
	}
	if !hasCMsUpdated {
		return
	}

	// The build has things to report, get its details.
	bp := getBuildDetails(ctx, build)
	if bp == nil {
		return
	}

	var reports []*Report
	for b, v := range cmValues {
		mfs := getBuildCustomMetrics(ctx, bp, b, toReportUpdated[b])
		for name, fieldMap := range mfs {
			r := &Report{
				Base:     b,
				Name:     name,
				FieldMap: fieldMap,
				Value:    v,
			}
			reports = append(reports, r)
		}
	}
	cms.Report(ctx, reports...)
}

// getBuildCustomMetrics returns the custom metrics with base that b should report to.
//
// It returns a map of strings with the keys being the custom metric names, and
// values being each metric's fields.
func getBuildCustomMetrics(ctx context.Context, build *pb.Build, base pb.CustomMetricBase, toReport []*pb.CustomMetricDefinition) map[string]map[string]string {
	res := make(map[string]map[string]string)
	defaultFields, err := getDefaultMetricFieldValues(build, base)
	if err != nil {
		logging.Errorf(ctx, "failed to get default metric fields for build %d: %s", build.Id, err)
	}
	for _, cmp := range toReport {
		// Check if b fulfills the predicates.
		matched, err := buildcel.BoolEval(build, cmp.Predicates)
		if err != nil {
			logging.Errorf(ctx, "failed to evaluate build %d with predicates: %s", build.Id, err)
			continue
		}
		if !matched {
			continue
		}

		// Get metric fields.
		var fieldMap map[string]string
		if len(cmp.ExtraFields) > 0 {
			fieldMap, err = buildcel.StringMapEval(build, cmp.ExtraFields)
			if err != nil {
				logging.Errorf(ctx, "failed to evaluate build %d with fields: %s", build.Id, err)
				continue
			}
		}

		// Add default metric fields.
		if len(fieldMap) == 0 {
			fieldMap = defaultFields
		} else {
			for k, v := range defaultFields {
				fieldMap[k] = v
			}
		}
		res[cmp.Name] = fieldMap
	}
	return res
}

func getDefaultMetricFieldValues(b *pb.Build, base pb.CustomMetricBase) (map[string]string, error) {
	baseFields, err := GetCommonBaseFields(base)
	if err != nil {
		return nil, err
	}

	baseFieldValues := make(map[string]string, len(baseFields))
	for _, f := range baseFields {
		switch f {
		case "status":
			baseFieldValues[f] = pb.Status_name[int32(b.Status)]
		default:
			return nil, errors.Reason("invalid base field %s", f).Err()
		}
	}
	return baseFieldValues, nil
}

// ValidateExtraFieldsWithBase performs metric base related validations on extraFields.
func ValidateExtraFieldsWithBase(base pb.CustomMetricBase, extraFields []string) error {
	// No extra fields for builder metrics
	if IsBuilderMetric(base) && len(extraFields) > 0 {
		return errors.Reason("custom builder metric cannot have extra_fields").Err()
	}

	// no base fields in extra fields
	baseFields, err := GetCommonBaseFields(base)
	if err != nil {
		return errors.Reason("base %s is invalid", base).Err()
	}
	if len(baseFields) == 0 {
		return nil
	}
	bfSet := stringset.NewFromSlice(baseFields...)
	fSet := stringset.NewFromSlice(extraFields...)
	if fSet.Contains(bfSet) {
		return errors.Reason("cannot contain base fields %q in extra_fields", baseFields).Err()
	}
	return nil
}
