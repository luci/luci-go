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
	"context"
	"time"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/server/router"
)

var (
	// gcPrefixDeletions is the number of attempts to garbage collect prefixes.
	gcPrefixAttempts = metric.NewCounter(
		"logdog/stats/gc_prefix_attempts",
		"Number of prefixes deleted due to garbage collection",
		nil,
		field.String("project"),
	)

	// gcPrefixDeletions is the number of successful garbage collected prefixes.
	gcPrefixDeletions = metric.NewCounter(
		"logdog/stats/gc_prefix_deletions",
		"Number of prefixes deleted due to garbage collection",
		nil,
		field.String("project"),
	)

	// gcStreamDeletions is the number of attempts to garbage collect streams.
	gcStreamAttempts = metric.NewCounter(
		"logdog/stats/gc_stream_attempts",
		"Number of stream deleted due to garbage collection",
		nil,
		field.String("project"),
	)

	// gcStreamDeletions is the number of successful garbage collected streams.
	gcStreamDeletions = metric.NewCounter(
		"logdog/stats/gc_stream_deletions",
		"Number of streams deleted due to garbage collection",
		nil,
		field.String("project"),
	)
)

func runGC(ctx context.Context, deleteBefore time.Time, shard, shardCount int32) error {
	// TODO(iannucci): how do we surface multitudinous errors?
	// TODO(iannucci): how do we surface authentication errors?
	// TODO(iannucci): how do we avoid racking up $$$ stackdriver logging?

	// Have a LogStream deleter workpool to delete LogStreams (and their GCS
	// entities). This will be doing most of the work for the shard.
	//
	// For each LogPrefix in the range:
	//   Fire a goroutine which pushes all LogStreams for this LogPrefix into
	//     the LogStream deleter workpool.
	//     - After they all succeed, delete the LogPrefix.

	return nil
}

// doGCCronNSShard garbage collects LogPrefixes, LogStreams and LogStreamStates.
func gcCronHandlerNSShard(ctx *router.Context) {
	// Namespace
	// RecursionNumber = 0
	// FromAge
	// ToAge
	// TaskTTL

	// Pick to iterate backwards or forwards using
	//   `RecursionNumber%2 ^ RetryNumber%2`
	// To help avoid index inconsistency.

	// Set a deadline on the context.

	// fromAge, toAge, done := runGC(...)

	// If done, return 200
	// Else if the deadline was hit and we still have TaskTTL, reschedule the task
	// into taskqueue with tighter upper/lower bounds, and increased
	// RecursionNumber. Return 200.
}

// doGCCronNS kicks off the garbage collection process for a given namespace.
//
// It will guess the number of entities in the namespace and either:
//   * Directly try to clean up the namespace OR
//   * Fire off some number of shards to doGCCronNSShard.
func gcCronHandlerNS(ctx *router.Context) {
	// Ask datastore __Stat_Ns_Kind__ for ($namespace, LogStream) to estimate how
	//   many LogStreams there are in the namespace.
	//
	// Get Query("LogPrefix").Order("Expiration").Limit(1) and observe Created
	//   property. This will be the oldest LogPrefix and will act as a lower bound
	//   on LogStream entities to delete.
	//
	// Estimate how many LogStream entities there are to process.

	// If there's a small estimated number, process them all inline.

	// If there's a large estimated number, fire off up to $maxShards tasks which
	// will tail recurse themselves up to, say, 24 hours. The algorithm should
	// pick a number of shards appropriate for the estimated number of entities to
	// process
}

// doGCCron kicks off the garbage collection process.
//
// Runs bi-weekly. Fires off one taskqueue task per namespace in the datastore
// to doGCCronNS.
func gcCronHandler(ctx *router.Context) {
}
