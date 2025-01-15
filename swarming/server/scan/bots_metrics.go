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

package scan

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/target"

	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
)

// BotsMetricsReporter is BotVisitor that reports stats about bots to the
// monitoring.
type BotsMetricsReporter struct {
	// ServiceName is a service name to put into metrics' target.
	ServiceName string
	// JobName is a job name to put into metrics' target.
	JobName string
	// Monitor to use to flush metrics.
	Monitor monitor.Monitor

	state  *tsmon.State
	shards []*metricsReporterShardState
}

var _ BotVisitor = (*BotsMetricsReporter)(nil)

// Prepare prepares the visitor to use `shards` parallel queries.
//
// Part of BotVisitor interface.
func (r *BotsMetricsReporter) Prepare(ctx context.Context, shards int) {
	r.state = newTSMonState(r.ServiceName, r.JobName, r.Monitor)
	r.shards = make([]*metricsReporterShardState, shards)
	for i := range r.shards {
		r.shards[i] = newMetricsReporterShardState()
	}
}

// Visit is called for every bot.
//
// Part of BotVisitor interface.
func (r *BotsMetricsReporter) Visit(ctx context.Context, shard int, bot *model.BotInfo) {
	r.shards[shard].collect(bot)
	setExecutorMetrics(tsmon.WithState(ctx, r.state), bot, r.ServiceName)
}

// Finalize is called once the scan is done.
//
// Part of BotVisitor interface.
func (r *BotsMetricsReporter) Finalize(ctx context.Context, scanErr error) error {
	// Report final counts only if scan completed successfully to avoid bogus
	// values (better no values at all). Flush whatever was reported by
	// setExecutorMetrics(...) even if the scan failed midway: these values are
	// valid (they are per-bot, doesn't matter if not all bots were visited).
	if scanErr == nil {
		total := newMetricsReporterShardState()
		for _, shard := range r.shards {
			total.mergeFrom(shard)
		}
		mctx := tsmon.WithState(ctx, r.state)
		for key, val := range total.counts {
			metrics.BotsPerState.Set(mctx, val, key.pool, key.state)
		}
	}
	return flushTSMonState(ctx, r.state)
}

////////////////////////////////////////////////////////////////////////////////

// ignoredDimensions is dimensions to exclude from the BotsDimensionsPool metric
// value.
//
// Ignoring these values significantly reduces total cordiality of the set of
// metric values (speeding up precalculations based on it) or discards
// information not actually relevant for monitoring.
//
// Keep in sync with luci/appengine/swarming/ts_mon_metrics.py.
var ignoredDimensions = stringset.NewFromSlice(
	// Side effect of the health of each Android devices connected to the bot.
	"android_devices",
	// Unbounded set of values.
	"caches",
	// Unique for each bot, already part of the metric target.
	"id",
	// Server-assigned, not relevant to the bot at all.
	"server_version",
	// Android specific.
	"temp_band",
)

type metricsReporterShardState struct {
	counts map[counterKey]int64
	total  int64
}

type counterKey struct {
	pool  string // e.g. "luci.infra.ci"
	state string // e.g. "SWARMING"
}

func newMetricsReporterShardState() *metricsReporterShardState {
	return &metricsReporterShardState{
		counts: map[counterKey]int64{},
	}
}

func (s *metricsReporterShardState) collect(bot *model.BotInfo) {
	var migrationState string

	if bot.Quarantined {
		migrationState = "QUARANTINED"
	} else if bot.IsInMaintenance() {
		migrationState = "MAINTENANCE"
	} else {
		if bot.State.MustReadBool("handshaking") {
			// This is not a fully connected bot.
			return
		}
		switch {
		case bot.State.Err() != nil:
			migrationState = "UNKNOWN"
		case bot.State.MustReadString("rbe_instance") == "":
			migrationState = "SWARMING"
		case bot.State.MustReadBool("rbe_hybrid_mode"):
			migrationState = "HYBRID"
		default:
			migrationState = "RBE"
		}
	}

	if bot.IsDead() {
		migrationState = "DEAD_" + migrationState
	}

	pools := bot.DimensionsByKey("pool")
	if len(pools) == 0 {
		pools = []string{"unknown"}
	}
	for _, pool := range pools {
		s.counts[counterKey{pool, migrationState}] += 1
	}
	s.total += 1
}

func (s *metricsReporterShardState) mergeFrom(another *metricsReporterShardState) {
	for key, count := range another.counts {
		s.counts[key] += count
	}
	s.total += another.total
}

// setExecutorMetrics sets metrics reported to the per-bot target.
func setExecutorMetrics(ctx context.Context, bot *model.BotInfo, serviceName string) {
	rbeState := "none"
	if rbeInstance := bot.State.MustReadString("rbe_instance"); rbeInstance != "" {
		rbeState = rbeInstance
	}

	// Each bot has its own target.
	ctx = target.Set(ctx, &target.Task{
		DataCenter:  "appengine",
		ServiceName: serviceName,
		HostName:    fmt.Sprintf("autogen:%s", bot.BotID()),
	})
	metrics.BotsStatus.Set(ctx, bot.GetStatus())
	metrics.BotsDimensionsPool.Set(ctx, poolFromDimensions(bot.Dimensions))
	metrics.BotsRBEInstance.Set(ctx, rbeState)
	metrics.BotsVersion.Set(ctx, bot.Version)
}

// poolFromDimensions serializes the bot's dimensions and trims out redundant
// prefixes, i.e. ["cpu:x86-64", "cpu:x86-64-Broadwell_GCE"] returns
// "cpu:x86-64-Broadwell_GCE".
func poolFromDimensions(dims []string) string {
	// Assuming dimensions are sorted.
	var pairs []string

	for current := 0; current < len(dims); current++ {
		key := strings.SplitN(dims[current], ":", 2)[0]
		if ignoredDimensions.Has(key) {
			continue
		}
		next := current + 1
		// Set `current` to the longest (and last) prefix of the chain.
		// i.e. if chain is ["os:Ubuntu", "os:Ubuntu-22", "os:Ubuntu-22.04"]
		// dimensions[current] is "os:Ubuntu-22.04"
		for next < len(dims) && strings.HasPrefix(dims[next], dims[current]) {
			current++
			next++
		}
		pairs = append(pairs, dims[current])
	}
	return strings.Join(pairs, "|")
}
