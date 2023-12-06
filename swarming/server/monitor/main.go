// Copyright 2023 The LUCI Authors.
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
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/dsmapper/dsmapperlite"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
	tsmonsrv "go.chromium.org/luci/server/tsmon"

	"go.chromium.org/luci/swarming/server/model"
)

func main() {
	modules := []module.Module{
		cron.NewModuleFromFlags(),
		gaeemulation.NewModuleFromFlags(),
	}

	server.Main(nil, modules, func(srv *server.Server) error {
		// Build a tsmon state with a global target, to make different processes
		// report to metrics into the same target. Processes need to cooperate with
		// one another to avoid conflicts. We do it by relying on GAE cron overrun
		// protection (it won't launch a cron invocation if the previous one is
		// still running).
		state := tsmon.NewState()
		state.SetStore(store.NewInMemory(&target.Task{
			DataCenter:  "appengine",
			ServiceName: srv.Options.TsMonServiceName,
			JobName:     srv.Options.TsMonJobName,
			HostName:    "global",
		}))
		state.InhibitGlobalCallbacksOnFlush()

		// Figure out where to flush metrics.
		switch {
		case srv.Options.Prod && srv.Options.TsMonAccount != "":
			mon, err := tsmonsrv.NewProdXMonitor(srv.Context, 4096, srv.Options.TsMonAccount)
			if err != nil {
				return err
			}
			state.SetMonitor(mon)
		case !srv.Options.Prod:
			state.SetMonitor(monitor.NewDebugMonitor(""))
		default:
			state.SetMonitor(monitor.NewNilMonitor())
		}

		cron.RegisterHandler("report-bots", func(ctx context.Context) error {
			return reportBots(ctx, state, srv.Options.TsMonServiceName)
		})
		return nil
	})
}

////////////////////////////////////////////////////////////////////////////////

var (
	botsPerState = metric.NewInt("swarming/rbe_migration/bots",
		"Number of Swarming bots per RBE migration state.",
		nil,
		field.String("pool"),  // e.g "luci.infra.ci"
		field.String("state"), // e.g. "RBE", "SWARMING", "HYBRID"
	)
	botsStatus         = metric.NewString("executors/status", "Status of a job executor.", nil)
	botsDimensionsPool = metric.NewString("executors/pool", "Pool name for a given job executor.", nil)
	botsRBEInstance    = metric.NewString("executors/rbe", "RBE instance of a job executor.", nil)
)

//   - android_devices is a side effect of the health of each Android devices
//     connected to the bot.
//   - caches has an unbounded matrix.
//   - server_version is the current server version. It'd be good to have but the
//     current monitoring pipeline is not adapted for this.
//   - id is unique for each bot.
//   - temp_band is android specific.
//
// Keep in sync with luci/appengine/swarming/ts_mon_metrics.py.
var ignoredDimensions = stringset.NewFromSlice(
	"android_devices",
	"caches",
	"id",
	"server_version",
	"temp_band",
)

func reportBots(ctx context.Context, state *tsmon.State, serviceName string) error {
	const shardCount = 128

	startTS := clock.Now(ctx)

	shards := make([]*shardState, shardCount)
	for i := range shards {
		shards[i] = newShardState()
	}

	mctx := tsmon.WithState(ctx, state)
	defer cleanUp(mctx, state)

	err := dsmapperlite.Map(ctx, model.BotInfoQuery(), shardCount, 1000,
		func(ctx context.Context, shardIdx int, bot *model.BotInfo) error {
			// These appear to be phantom GCE provider bots which are either being created
			// or weren't fully deleted. They don't have `state` JSON dict populated, and
			// they aren't really running.
			if !bot.LastSeen.IsSet() || len(bot.State) == 0 {
				return nil
			}
			shards[shardIdx].collect(ctx, bot)
			setExecutorMetrics(mctx, bot, serviceName)
			return nil
		},
	)
	if err != nil {
		return errors.Annotate(err, "when visiting BotInfo").Err()
	}

	// Merge all shards into a single set of counters.
	total := newShardState()
	for _, shard := range shards {
		total.mergeFrom(shard)
	}
	logging.Infof(ctx, "Scan done in %s. Total visited bots: %d", clock.Since(ctx, startTS), total.total)

	// Flush them to tsmon. Do not retain in memory after that.
	flushTS := clock.Now(ctx)
	for key, val := range total.counts {
		botsPerState.Set(mctx, val, key.pool, key.state)
	}

	// Note: use `ctx` here (not `mctx`) to report monitor's gRPC stats into
	// the regular process-global tsmon state.
	if err := state.ParallelFlush(ctx, nil, 32); err != nil {
		return errors.Annotate(err, "failed to flush values to monitoring").Err()
	}
	logging.Infof(ctx, "Flushed to monitoring in %s.", clock.Since(ctx, flushTS))
	return nil
}

type counterKey struct {
	pool  string // e.g. "luci.infra.ci"
	state string // e.g. "SWARMING"
}

type shardState struct {
	counts map[counterKey]int64
	total  int64
}

func newShardState() *shardState {
	return &shardState{
		counts: map[counterKey]int64{},
	}
}

func (s *shardState) collect(ctx context.Context, bot *model.BotInfo) {
	migrationState := "UNKNOWN"

	if bot.Quarantined {
		migrationState = "QUARANTINED"
	} else if bot.IsInMaintenance() {
		migrationState = "MAINTENANCE"
	} else {
		var botState struct {
			Handshaking   bool   `json:"handshaking,omitempty"`
			RBEInstance   string `json:"rbe_instance,omitempty"`
			RBEHybridMode bool   `json:"rbe_hybrid_mode,omitempty"`
		}
		if err := json.Unmarshal(bot.State, &botState); err == nil {
			switch {
			case botState.Handshaking:
				// This is not a fully connected bot.
				return
			case botState.RBEInstance == "":
				migrationState = "SWARMING"
			case botState.RBEHybridMode:
				migrationState = "HYBRID"
			case !botState.RBEHybridMode:
				migrationState = "RBE"
			}
		} else {
			logging.Warningf(ctx, "Bot %s: bad state:\n:%s", bot.BotID(), bot.State)
		}
	}

	if bot.IsDead() {
		migrationState = "DEAD_" + migrationState
	}

	pools := bot.DimenionsByKey("pool")
	if len(pools) == 0 {
		pools = []string{"unknown"}
	}
	for _, pool := range pools {
		s.counts[counterKey{pool, migrationState}] += 1
	}
	s.total += 1
}

func (s *shardState) mergeFrom(another *shardState) {
	for key, count := range another.counts {
		s.counts[key] += count
	}
	s.total += another.total
}

// setExecutorMetrics sets the executors metrics.
func setExecutorMetrics(mctx context.Context, bot *model.BotInfo, serviceName string) {
	// HostName needs to be set per bot. Cannot use global target.
	tctx := target.Set(mctx, &target.Task{
		DataCenter:  "appengine",
		ServiceName: serviceName,
		HostName:    fmt.Sprintf("autogen:%s", bot.BotID()),
	})
	// Status.
	status := bot.GetStatus()
	botsStatus.Set(tctx, status)
	// DimensionsPool.
	dims := poolFromDimensions(bot.Dimensions)
	botsDimensionsPool.Set(tctx, dims)
	// RBEInstance.
	rbeState := "none"
	var botState struct {
		RBEInstance string `json:"rbe_instance,omitempty"`
	}
	if err := json.Unmarshal(bot.State, &botState); err == nil {
		if botState.RBEInstance != "" {
			rbeState = botState.RBEInstance
		}
	} else {
		logging.Warningf(mctx, "Bot %s: bad state:\n:%s", bot.BotID(), bot.State)
	}
	botsRBEInstance.Set(tctx, rbeState)
}

// poolFromDimensions serializes the bot's dimensions and trims out redundant prefixes.
// i.e. ["cpu:x86-64", "cpu:x86-64-Broadwell_GCE"] returns "cpu:x86-64-Broadwell_GCE".
func poolFromDimensions(dimensions []string) string {
	// Assuming dimensions are sorted.
	var pairs []string

	for current := 0; current < len(dimensions); current++ {
		key := strings.SplitN(dimensions[current], ":", 2)[0]
		if ignoredDimensions.Has(key) {
			continue
		}
		next := current + 1
		// Set `current` to the longest (and last) prefix of the chain.
		// i.e. if chain is ["os:Ubuntu", "os:Ubuntu-22", "os:Ubuntu-22.04"]
		// dimensions[current] is "os:Ubuntu-22.04"
		for next < len(dimensions) && strings.HasPrefix(dimensions[next], dimensions[current]) {
			current++
			next++
		}
		pairs = append(pairs, dimensions[current])
	}
	return strings.Join(pairs, "|")
}

func cleanUp(mctx context.Context, state *tsmon.State) {
	state.Store().Reset(mctx, botsPerState)
	state.Store().Reset(mctx, botsStatus)
	state.Store().Reset(mctx, botsDimensionsPool)
	state.Store().Reset(mctx, botsRBEInstance)
}
