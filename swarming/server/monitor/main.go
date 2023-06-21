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

	"go.chromium.org/luci/common/clock"
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
			mon, err := tsmonsrv.NewProdXMonitor(srv.Context, 1024, srv.Options.TsMonAccount)
			if err != nil {
				return err
			}
			state.SetMonitor(mon)
		case !srv.Options.Prod:
			state.SetMonitor(monitor.NewDebugMonitor(""))
		default:
			state.SetMonitor(monitor.NewNilMonitor())
		}

		cron.RegisterHandler("report-rbe-bots", func(ctx context.Context) error {
			return reportRBEBots(ctx, state)
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
)

func reportRBEBots(ctx context.Context, state *tsmon.State) error {
	const shardCount = 128

	startTS := clock.Now(ctx)

	shards := make([]*shardState, shardCount)
	for i := range shards {
		shards[i] = newShardState()
	}

	err := dsmapperlite.Map(ctx, model.BotInfoQuery(), shardCount, 1000,
		func(ctx context.Context, shardIdx int, bot *model.BotInfo) error {
			shards[shardIdx].collect(ctx, bot)
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
	mctx := tsmon.WithState(ctx, state)
	defer state.Store().Reset(mctx, botsPerState)
	for key, val := range total.counts {
		botsPerState.Set(mctx, val, key.pool, key.state)
	}

	// Note: use `ctx` here (not `mctx`) to report monitor's gRPC stats into
	// the regular process-global tsmon state.
	if err := state.ParallelFlush(ctx, nil, 8); err != nil {
		return errors.Annotate(err, "failed to flush values to monitoring").Err()
	}
	logging.Infof(ctx, "Flushed to monitoring in %s.", clock.Since(ctx, flushTS))
	return nil
}

type shardState struct {
	counts map[counterKey]int64
	total  int64
}

type counterKey struct {
	pool  string // e.g. "luci.infra.ci"
	state string // e.g. "SWARMING"
}

func newShardState() *shardState {
	return &shardState{
		counts: map[counterKey]int64{},
	}
}

func (s *shardState) collect(ctx context.Context, bot *model.BotInfo) {
	// These appear to be phantom GCE provider bots which are either being created
	// or weren't fully deleted. They don't have `state` JSON dict populated, and
	// they aren't really running.
	if bot.LastSeen.IsZero() || len(bot.State) == 0 {
		return
	}

	migrationState := "UNKNOWN"

	if bot.Quarantined {
		migrationState = "QUARANTINED"
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
			logging.Warningf(ctx, "Bot %s: bad state:\n:%s", bot.Parent.StringID(), bot.State)
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
