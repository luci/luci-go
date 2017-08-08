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

package tumble

import (
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock/clockflag"
	"go.chromium.org/luci/server/settings"
	"golang.org/x/net/context"
)

const (
	// baseName is the base tumble name. It is also the name of the tumble task
	// queue and settings dict.
	baseName = "tumble"
)

// Config is the set of tweakable things for tumble. If you use something other
// than the defaults (e.g. unset values), you must ensure that all aspects of
// your application use the same config.
//
// The JSON annotations are for settings module storage (see settings.go).
type Config struct {
	// NumShards is the number of tumble shards that will process concurrently.
	// It defaults to 32.
	NumShards uint64 `json:"numShards,omitempty"`

	// NumGoroutines is the number of gorountines that will process in parallel
	// in a single shard. Each goroutine will process exactly one root entity.
	// It defaults to 16.
	NumGoroutines int `json:"numGoroutines,omitempty"`

	// TemporalMinDelay is the minimum number of seconds to wait before the
	// task queue entry for a given shard will run. It defaults to 1 second.
	TemporalMinDelay clockflag.Duration `json:"temporalMinDelay,omitempty"`

	// TemporalRoundFactor is the number of seconds to batch together in task
	// queue tasks. It defaults to 4 seconds.
	TemporalRoundFactor clockflag.Duration `json:"temporalRoundFactor,omitempty"`

	// ProcessLoopDuration is the maximum lifetime of a process loop. A process
	// batch will refrain from re-entering its loop after this much time has
	// elapsed.
	//
	// This is not a hard termination boundary. If a loop round starts before
	// ProcessLoopDuration has been reached, it will be permitted to continue past
	// the duration.
	//
	// If this is <= 0, the process will loop at most once.
	ProcessLoopDuration clockflag.Duration `json:"processLoopDuration,omitempty"`

	// DustSettleTimeout is the amount of time to wait in between mutation
	// processing iterations.
	//
	// This should be chosen as a compromise between higher expectations of the
	// eventually-consistent datastore and task processing latency.
	DustSettleTimeout clockflag.Duration `json:"dustSettleTimeout,omitempty"`

	// MaxNoWorkDelay is the maximum amount of time to wait in between mutation
	// processing iterations when there was no work the previous iteration.
	//
	// When no work has been done, each round will begin by waiting
	// DustSettleTimeout seconds (minimum of 1 second). If no work was done that
	// round, this will continue to exponentially grow each successive no-work
	// round until capped at MaxNoWorkDelay. If work is encountered during any
	// round, the delay is reset.
	//
	// If MaxNoWorkDelay is <= 0, the delay will continue exponentially growing
	// until the shard terminates.
	//
	// This should be chosen as a compromise between higher expectations of the
	// eventually-consistent datastore and task processing latency.
	MaxNoWorkDelay clockflag.Duration `json:"MaxNoWorkDelay,omitempty"`

	// NoWorkDelayGrowth is the exponential growth factor for the delay in
	// between processing loop rounds when no work was done.
	//
	// If NoWorkDelayGrowth is <= 1, a growth factor of 1 will be used.
	NoWorkDelayGrowth int `json:"NoWorkDelayGrowth,omitempty"`

	// ProcessMaxBatchSize is the number of mutations that each processor
	// goroutine will attempt to include in each commit.
	//
	// It defaults to 128. A negative value means no limit.
	ProcessMaxBatchSize int `json:"processMaxBatchSize,omitempty"`

	// DelayedMutations enables the 'DelayedMutation' mutation subtype.
	//
	// If you set this to true, you MUST also add the second index mentioned
	// in the package docs.
	DelayedMutations bool `json:"delayedMutations,omitempty"`
}

// defaultConfig returns the default configuration settings.
var defaultConfig = Config{
	NumShards:           32,
	TemporalMinDelay:    clockflag.Duration(time.Second),
	TemporalRoundFactor: clockflag.Duration(4 * time.Second),
	ProcessLoopDuration: clockflag.Duration(9*time.Minute + 30*time.Second),
	DustSettleTimeout:   clockflag.Duration(2 * time.Second),
	MaxNoWorkDelay:      clockflag.Duration(2 * time.Second), // == DustSettleTimeout
	NoWorkDelayGrowth:   3,
	NumGoroutines:       16,
	ProcessMaxBatchSize: 128,
}

// getConfig returns the current configuration.
//
// It first tries to load it from settings. If no settings is installed, or if
// there is no configuration in settings, defaultConfig is returned.
func getConfig(c context.Context) *Config {
	cfg := Config{}
	switch err := settings.Get(c, baseName, &cfg); err {
	case nil:
		break
	case settings.ErrNoSettings:
		// Defaults.
		cfg = defaultConfig
	default:
		panic(fmt.Errorf("could not fetch Tumble settings - %s", err))
	}
	return &cfg
}
