// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"time"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"github.com/luci/luci-go/common/gcloud/pubsub/ackbuffer"
	"github.com/luci/luci-go/common/gcloud/pubsub/subscriber"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/internal/logdog/collector"
	"github.com/luci/luci-go/server/internal/logdog/collector/coordinator"
	"github.com/luci/luci-go/server/internal/logdog/service"
	"golang.org/x/net/context"
)

var (
	errInvalidConfig = errors.New("invalid configuration")
)

// application is the Collector application state.
type application struct {
	service.Service
}

// run is the main execution function.
func (a *application) runCollector(c context.Context) error {
	cfg := a.Config()
	ccfg := cfg.GetCollector()
	if ccfg == nil {
		return errors.New("no collector configuration")
	}

	pscfg := cfg.GetTransport().GetPubsub()
	if pscfg == nil {
		return errors.New("missing Pub/Sub configuration")
	}

	// Our Subscription must be a valid one.
	sub := pubsub.NewSubscription(pscfg.Project, pscfg.Subscription)
	if err := sub.Validate(); err != nil {
		return fmt.Errorf("invalid Pub/Sub subscription %q: %v", sub, err)
	}

	// New PubSub instance with the authenticated client.
	psClient, err := a.AuthenticatedClient(func(o *auth.Options) {
		o.Scopes = pubsub.SubscriberScopes
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub client.")
		return err
	}

	// Create a retrying Pub/Sub client.
	ps := &pubsub.Retry{
		Connection: pubsub.NewConnection(psClient),
		Callback: func(err error, d time.Duration) {
			log.Fields{
				log.ErrorKey: err,
				"delay":      d,
			}.Warningf(c, "Transient error encountered; retrying...")
		},
	}

	exists, err := ps.SubExists(c, sub)
	if err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"subscription": sub,
		}.Errorf(c, "Could not confirm Pub/Sub subscription.")
		return errInvalidConfig
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

	st, err := a.IntermediateStorage(c)
	if err != nil {
		return err
	}
	defer st.Close()

	// Application shutdown will now operate by cancelling the Collector's
	// shutdown Context.
	shutdownCtx, shutdownFunc := context.WithCancel(c)
	a.SetShutdownFunc(shutdownFunc)

	// Start an ACK buffer so that we can batch ACKs. Note that we do NOT use the
	// shutdown context here, as we want clean shutdowns to continue to ack any
	// buffered messages.
	ab := ackbuffer.New(c, ackbuffer.Config{
		Ack: ackbuffer.NewACK(ps, sub, 0),
	})
	defer ab.CloseAndFlush()

	// Initialize our Collector service object using a caching Coordinator
	// interface.
	coord := coordinator.NewCoordinator(a.Coordinator())
	coord = coordinator.NewCache(coord, int(ccfg.StateCacheSize), ccfg.StateCacheExpiration.Duration())

	coll := collector.Collector{
		Coordinator:       coord,
		Storage:           st,
		MaxMessageWorkers: int(ccfg.MaxMessageWorkers),
	}
	defer coll.Close()

	// Execute our main Subscriber loop. It will run until the supplied Context
	// is cancelled.
	clk := clock.Get(c)
	engine := subscriber.Subscriber{
		S:       subscriber.NewSource(ps, sub),
		A:       ab,
		Workers: int(ccfg.MaxConcurrentMessages),
	}
	engine.Run(shutdownCtx, func(msg *pubsub.Message) bool {
		c := log.SetField(c, "messageID", msg.ID)
		log.Fields{
			"ackID": msg.AckID,
			"size":  len(msg.Data),
		}.Infof(c, "Received Pub/Sub Message.")

		startTime := clk.Now()
		err := coll.Process(c, msg.Data)
		duration := clk.Now().Sub(startTime)

		switch {
		case errors.IsTransient(err):
			// Do not consume
			log.Fields{
				log.ErrorKey: err,
				"duration":   duration,
			}.Warningf(c, "TRANSIENT error ingesting Pub/Sub message.")
			return false

		case err == nil:
			log.Fields{
				"ackID":    msg.AckID,
				"size":     len(msg.Data),
				"duration": duration,
			}.Infof(c, "Message successfully processed; ACKing.")
			return true

		default:
			log.Fields{
				log.ErrorKey: err,
				"ackID":      msg.AckID,
				"size":       len(msg.Data),
				"duration":   duration,
			}.Errorf(c, "Non-transient error ingesting Pub/Sub message; ACKing.")
			return true
		}
	})

	log.Debugf(c, "Collector finished.")
	return nil
}

// Entry point.
func main() {
	a := application{}
	a.Run(context.Background(), a.runCollector)
}
