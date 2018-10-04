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
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"go.chromium.org/luci/logdog/server/bundleServicesClient"
	"go.chromium.org/luci/logdog/server/collector"
	"go.chromium.org/luci/logdog/server/collector/coordinator"
	"go.chromium.org/luci/logdog/server/service"

	"cloud.google.com/go/pubsub"
)

var (
	errInvalidConfig = errors.New("invalid configuration")
)

// Metrics.
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

// application is the Collector application state.
type application struct {
	service.Service
}

// run is the main execution function.
func (a *application) runCollector(c context.Context) error {
	cfg := a.ServiceConfig()
	ccfg := cfg.GetCollector()
	if ccfg == nil {
		return errors.New("no collector configuration")
	}

	pscfg := cfg.GetTransport().GetPubsub()
	if pscfg == nil {
		return errors.New("missing Pub/Sub configuration")
	}

	// Our Subscription must be a valid one.
	sub := gcps.NewSubscription(pscfg.Project, pscfg.Subscription)
	if err := sub.Validate(); err != nil {
		return fmt.Errorf("invalid Pub/Sub subscription %q: %v", sub, err)
	}

	// New PubSub instance with the authenticated client.
	psClient, err := a.Service.PubSubSubscriberClient(c, pscfg.Project)
	if err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"subscription": sub,
		}.Errorf(c, "Failed to create Pub/Sub client.")
		return err
	}

	psSub := psClient.Subscription(pscfg.Subscription)
	exists, err := psSub.Exists(c)
	if err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"subscription": sub,
		}.Errorf(c, "Could not confirm Pub/Sub subscription.")
		return errInvalidConfig
	}
	psSub.ReceiveSettings = pubsub.ReceiveSettings{
		MaxExtension:           24 * time.Hour,
		MaxOutstandingMessages: int(ccfg.MaxConcurrentMessages), // If < 1, default.
		MaxOutstandingBytes:    0,                               // Default.
	}

	if !exists {
		log.Fields{
			"subscription": sub,
		}.Errorf(c, "Subscription does not exist.")
		return errInvalidConfig
	}
	log.Fields{
		"subscription": sub,
	}.Infof(c, "Successfully validated Pub/Sub subscription.")

	st, err := a.IntermediateStorage(c, true)
	if err != nil {
		return err
	}
	defer st.Close()

	// Initialize a Coordinator client that bundles requests together.
	coordClient := &bundleServicesClient.Client{
		ServicesClient:       a.Coordinator(),
		DelayThreshold:       time.Second,
		BundleCountThreshold: 100,
	}
	defer coordClient.Flush()

	// Initialize our Collector service object using a caching Coordinator
	// interface.
	coord := coordinator.NewCoordinator(coordClient)
	coord = coordinator.NewCache(coord, int(ccfg.StateCacheSize), google.DurationFromProto(ccfg.StateCacheExpiration))

	coll := collector.Collector{
		Coordinator:       coord,
		Storage:           st,
		MaxMessageWorkers: int(ccfg.MaxMessageWorkers),
	}
	defer coll.Close()

	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	// Application shutdown will now operate by cancelling the Collector's
	// shutdown Context.
	a.SetShutdownFunc(cancelFunc)

	// Execute our main subscription pull loop. It will run until the supplied
	// Context is cancelled.
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

	err = retry.Retry(c, transient.Only(retryForever), func() error {
		return grpcutil.WrapIfTransient(psSub.Receive(c, func(c context.Context, msg *pubsub.Message) {
			c = log.SetField(c, "messageID", msg.ID)
			if a.processMessage(c, &coll, msg) {
				// ACK the message, removing it from Pub/Sub.
				msg.Ack()
			} else {
				// NACK the message. It will be redelivered and processed.
				msg.Nack()
			}
		}))
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Warningf(c, "Transient error during subscription Receive loop; retrying...")
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed during Pub/Sub Receive.")
		return err
	}

	log.Debugf(c, "Collector finished.")
	return nil
}

// processMessage returns true if the message should be ACK'd (deleted from
// Pub/Sub) or false if the message should not be ACK'd.
func (a *application) processMessage(c context.Context, coll *collector.Collector, msg *pubsub.Message) bool {
	log.Fields{
		"size": len(msg.Data),
	}.Infof(c, "Received Pub/Sub Message.")

	startTime := clock.Now(c)
	err := coll.Process(c, msg.Data)
	duration := clock.Now(c).Sub(startTime)

	// We track processing time in milliseconds.
	tsTaskProcessingTime.Add(c, duration.Seconds()*1000)

	switch {
	case transient.Tag.In(err):
		// Do not consume
		log.Fields{
			log.ErrorKey: err,
			"duration":   duration,
		}.Warningf(c, "TRANSIENT error ingesting Pub/Sub message.")
		tsPubsubCount.Add(c, 1, "transient_failure")
		return false

	case err == nil:
		log.Fields{
			"size":     len(msg.Data),
			"duration": duration,
		}.Infof(c, "Message successfully processed; ACKing.")
		tsPubsubCount.Add(c, 1, "success")
		return true

	default:
		log.Fields{
			log.ErrorKey: err,
			"size":       len(msg.Data),
			"duration":   duration,
		}.Errorf(c, "Non-transient error ingesting Pub/Sub message; ACKing.")
		tsPubsubCount.Add(c, 1, "failure")
		return true
	}
}

// Entry point.
func main() {
	mathrand.SeedRandomly()
	a := application{
		Service: service.Service{
			Name:               "collector",
			DefaultAuthOptions: chromeinfra.DefaultAuthOptions(),
		},
	}
	a.Run(context.Background(), a.runCollector)
}
