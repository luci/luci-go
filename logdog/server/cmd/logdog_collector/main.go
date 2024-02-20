// Copyright 2016 The LUCI Authors.
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
	"flag"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/logdog/server/collector"
	"go.chromium.org/luci/logdog/server/collector/coordinator"
	"go.chromium.org/luci/logdog/server/service"
)

var (
	// tsPubsubCount counts the number of Pub/Sub messages processed by the
	// Archivist.
	//
	// Result tracks the outcome of each message, either "success", "failure", or
	// "transient_failure".
	tsPubsubCount = metric.NewCounter("logdog/collector/subscription/count",
		"The number of Pub/Sub messages pulled.",
		nil,
		field.String("result"))

	// tsTaskProcessingTime tracks the amount of time a single subscription
	// message takes to process, in milliseconds.
	tsTaskProcessingTime = metric.NewCumulativeDistribution("logdog/collector/subscription/processing_time_ms",
		"Amount of time in milliseconds that a single Pub/Sub message takes to process.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer)
)

// runForever runs the collector loop until the context closes.
func runForever(ctx context.Context, coll *collector.Collector, sub *pubsub.Subscription) {
	retryForever := func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   200 * time.Millisecond,
				Retries: -1, // Unlimited.
			},
			MaxDelay:   10 * time.Second,
			Multiplier: 2,
		}
	}

	retry.Retry(ctx, retryForever, func() error {
		return sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
			ctx = logging.SetField(ctx, "messageID", msg.ID)
			if processMessage(ctx, coll, msg) {
				// ACK the message, removing it from Pub/Sub.
				msg.Ack()
			} else {
				// NACK the message. It will be redelivered and processed.
				msg.Nack()
			}
		})
	}, func(err error, d time.Duration) {
		logging.Fields{
			"error": err,
			"delay": d,
		}.Errorf(ctx, "Error during subscription Receive loop; retrying...")
	})
}

// processMessage returns true if the message should be ACK'd (deleted from
// Pub/Sub) or false if the message should not be ACK'd.
func processMessage(ctx context.Context, coll *collector.Collector, msg *pubsub.Message) bool {
	startTime := clock.Now(ctx)
	err := coll.Process(ctx, msg.Data)
	duration := clock.Now(ctx).Sub(startTime)

	// We track processing time in milliseconds.
	tsTaskProcessingTime.Add(ctx, duration.Seconds()*1000)

	switch {
	case transient.Tag.In(err) || errors.Contains(err, context.Canceled):
		// Do not consume
		logging.Fields{
			"error":    err,
			"duration": duration,
		}.Warningf(ctx, "TRANSIENT error ingesting Pub/Sub message.")
		tsPubsubCount.Add(ctx, 1, "transient_failure")
		return false

	case err == nil:
		tsPubsubCount.Add(ctx, 1, "success")
		return true

	default:
		logging.Fields{
			"error":    err,
			"size":     len(msg.Data),
			"duration": duration,
		}.Errorf(ctx, "Non-transient error ingesting Pub/Sub message; ACKing.")
		tsPubsubCount.Add(ctx, 1, "failure")
		return true
	}
}

// pubSubClient returns an authenticated Google PubSub client instance.
func pubSubClient(ctx context.Context, cloudProject string) (*pubsub.Client, error) {
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the token source").Err()
	}
	client, err := pubsub.NewClient(ctx, cloudProject, option.WithTokenSource(ts))
	if err != nil {
		return nil, errors.Annotate(err, "failed to create the PubSub client").Err()
	}
	return client, nil
}

// Entry point.
func main() {
	flags := CommandLineFlags{}
	flags.Register(flag.CommandLine)

	cfg := service.MainCfg{BigTableAppProfile: "collector"}
	service.Main(cfg, func(srv *server.Server, impl *service.Implementations) error {
		if err := flags.Validate(); err != nil {
			return err
		}

		// Initialize our Collector service object using a caching Coordinator
		// interface.
		coll := &collector.Collector{
			Coordinator: coordinator.NewCache(
				coordinator.NewCoordinator(impl.Coordinator),
				flags.StateCacheSize,
				flags.StateCacheExpiration,
			),
			Storage:           impl.Storage,
			MaxMessageWorkers: flags.MaxMessageWorkers,
		}
		srv.RegisterCleanup(func(context.Context) { coll.Close() })

		// Initialize a Subscription object ready to pull messages.
		psClient, err := pubSubClient(srv.Context, flags.PubSubProject)
		if err != nil {
			return err
		}
		psSub := psClient.Subscription(flags.PubSubSubscription)
		psSub.ReceiveSettings = pubsub.ReceiveSettings{
			MaxExtension:           24 * time.Hour,
			MaxOutstandingMessages: flags.MaxConcurrentMessages, // If < 1, default.
			MaxOutstandingBytes:    0,                           // Default.
		}

		// Run the collector loop until the server closes.
		srv.RunInBackground("collector", func(ctx context.Context) {
			runForever(ctx, coll, psSub)
		})
		return nil
	})
}
