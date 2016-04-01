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
	gcps "github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/server/internal/logdog/collector"
	"github.com/luci/luci-go/server/internal/logdog/collector/coordinator"
	"github.com/luci/luci-go/server/internal/logdog/service"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	"google.golang.org/cloud/pubsub"
)

var (
	errInvalidConfig = errors.New("invalid configuration")
)

const (
	pubsubPullErrorDelay = 10 * time.Second
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
	sub := gcps.NewSubscription(pscfg.Project, pscfg.Subscription)
	if err := sub.Validate(); err != nil {
		return fmt.Errorf("invalid Pub/Sub subscription %q: %v", sub, err)
	}

	// New PubSub instance with the authenticated client.
	psAuth, err := a.Authenticator(func(o *auth.Options) {
		o.Scopes = gcps.SubscriberScopes
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub token source.")
		return err
	}

	psClient, err := pubsub.NewClient(c, pscfg.Project, cloud.WithTokenSource(psAuth.TokenSource()))
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

	// Execute our main subscription pull loop. It will run until the supplied
	// Context is cancelled.
	psIterator, err := psSub.Pull(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub iterator.")
		return err
	}
	defer func() {
		log.Debugf(c, "Waiting for Pub/Sub subscription iterator to stop...")
		psIterator.Stop()
		log.Debugf(c, "Pub/Sub subscription iterator has stopped.")
	}()

	parallel.Ignore(parallel.Run(int(ccfg.MaxConcurrentMessages), func(taskC chan<- func() error) {
		// Loop until shut down.
		for shutdownCtx.Err() == nil {
			msg, err := psIterator.Next()
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
					"delay":      pubsubPullErrorDelay,
				}.Errorf(c, "Failed to fetch Pub/Sub message, retry after delay...")
				clock.Sleep(c, pubsubPullErrorDelay)
				continue
			}

			taskC <- func() error {
				c := log.SetField(c, "messageID", msg.ID)
				msg.Done(a.processMessage(c, &coll, msg))
				return nil
			}
		}
	}))

	log.Debugf(c, "Collector finished.")
	return nil
}

// processMessage returns true if the message should be ACK'd (deleted from
// Pub/Sub) or false if the message should not be ACK'd.
func (a *application) processMessage(c context.Context, coll *collector.Collector, msg *pubsub.Message) bool {
	log.Fields{
		"ackID": msg.AckID,
		"size":  len(msg.Data),
	}.Infof(c, "Received Pub/Sub Message.")

	startTime := clock.Now(c)
	err := coll.Process(c, msg.Data)
	duration := clock.Now(c).Sub(startTime)

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
}

// Entry point.
func main() {
	a := application{}
	a.Run(context.Background(), a.runCollector)
}
