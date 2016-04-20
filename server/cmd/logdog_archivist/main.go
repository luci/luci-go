// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"io"
	"time"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/server/internal/logdog/archivist"
	"github.com/luci/luci-go/server/internal/logdog/service"
	"golang.org/x/net/context"
	"google.golang.org/cloud"
	gcps "google.golang.org/cloud/pubsub"
)

var (
	errInvalidConfig = errors.New("invalid configuration")
)

const (
	// subscriptionErrorDelay is the amount of time to sleep after a subscription
	// iterator returns a non-terminal error.
	subscriptionErrorDelay = 10 * time.Second
)

// application is the Archivist application state.
type application struct {
	service.Service
}

// run is the main execution function.
func (a *application) runArchivist(c context.Context) error {
	cfg := a.Config()

	coordCfg, acfg := cfg.GetCoordinator(), cfg.GetArchivist()
	switch {
	case coordCfg == nil:
		fallthrough

	case acfg == nil:
		return errors.New("missing Archivist configuration")
	case acfg.GsBase == "":
		return errors.New("missing archive GS bucket")
	case acfg.GsStagingBase == "":
		return errors.New("missing archive staging GS bucket")
	}

	// Construct and validate our GS bases.
	gsBase := gs.Path(acfg.GsBase)
	if gsBase.Bucket() == "" {
		log.Fields{
			"value": gsBase,
		}.Errorf(c, "Google Storage base does not include a bucket name.")
		return errors.New("invalid Google Storage base")
	}

	gsStagingBase := gs.Path(acfg.GsStagingBase)
	if gsStagingBase.Bucket() == "" {
		log.Fields{
			"value": gsStagingBase,
		}.Errorf(c, "Google Storage staging base does not include a bucket name.")
		return errors.New("invalid Google Storage staging base")
	}

	// Initialize Pub/Sub client.
	//
	// We will initialize both an authenticated Client instance and an
	// authenticated Context, since we need the latter for raw ACK deadline
	// updates.
	taskSub := pubsub.Subscription(acfg.Subscription)
	if err := taskSub.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"value":      taskSub,
		}.Errorf(c, "Task subscription did not validate.")
		return errors.New("invalid task subscription name")
	}
	psProject, psSubscriptionName := taskSub.Split()

	psAuth, err := a.Authenticator(func(o *auth.Options) {
		o.Scopes = pubsub.SubscriberScopes
	})
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get Pub/Sub authenticator.")
		return err
	}

	// Pub/Sub: HTTP Client => Context
	psHTTPClient, err := psAuth.Client()
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create authenticated Pub/Sub transport.")
		return err
	}
	psContext := cloud.WithContext(c, psProject, psHTTPClient)

	// Pub/Sub: TokenSource => Client
	psClient, err := gcps.NewClient(c, psProject, cloud.WithTokenSource(psAuth.TokenSource()))
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub client.")
		return err
	}
	sub := psClient.Subscription(psSubscriptionName)

	// Initialize our Storage.
	st, err := a.IntermediateStorage(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get storage instance.")
		return err
	}
	defer st.Close()

	// Initialize our Google Storage client.
	gsClient, err := a.GSClient(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get Google Storage client.")
		return err
	}
	defer gsClient.Close()

	ar := archivist.Archivist{
		Service:  a.Coordinator(),
		Storage:  st,
		GSClient: gsClient,

		GSBase:           gsBase,
		GSStagingBase:    gsStagingBase,
		StreamIndexRange: int(acfg.StreamIndexRange),
		PrefixIndexRange: int(acfg.PrefixIndexRange),
		ByteRange:        int(acfg.ByteRange),
	}

	tasks := int(acfg.Tasks)
	if tasks <= 0 {
		tasks = 1
	}

	log.Fields{
		"subscription": taskSub,
		"tasks":        tasks,
	}.Infof(c, "Pulling tasks from Pub/Sub subscription.")
	it, err := sub.Pull(c, gcps.MaxExtension(pubsub.MaxACKDeadline), gcps.MaxPrefetch(tasks))
	if err != nil {
		log.Fields{
			log.ErrorKey:   err,
			"subscription": taskSub,
		}.Errorf(c, "Failed to create Pub/Sub subscription iterator.")
	}
	defer it.Stop()

	// Application shutdown will now operate by stopping the Iterator.
	a.SetShutdownFunc(it.Stop)

	// Loop, pulling messages from our iterator and dispatching them.
	parallel.Ignore(parallel.Run(tasks, func(taskC chan<- func() error) {
		for {
			msg, err := it.Next()
			switch err {
			case nil:
				c := log.SetFields(c, log.Fields{
					"messageID": msg.ID,
					"ackID":     msg.AckID,
				})

				// Dispatch an archive handler for this message.
				taskC <- func() error {
					deleteTask := false
					defer func() {
						msg.Done(deleteTask)
					}()

					task, err := makePubSubArchivistTask(psContext, psSubscriptionName, msg)
					if err != nil {
						log.WithError(err).Errorf(c, "Failed to unmarshal archive task from message.")
						deleteTask = true
						return nil
					}

					startTime := clock.Now(c)
					deleteTask = ar.ArchiveTask(c, task)
					duration := clock.Now(c).Sub(startTime)

					if deleteTask {
						log.Fields{
							"duration": duration,
						}.Infof(c, "Task successfully processed; deleting.")
					} else {
						log.Fields{
							"duration": duration,
						}.Infof(c, "Task processing incomplete. Not deleting.")
					}
					return nil
				}

			case io.EOF, context.Canceled, context.DeadlineExceeded:
				log.Infof(c, "Subscription iterator is finished.")
				return

			default:
				log.WithError(err).Warningf(c, "Subscription iterator returned error. Sleeping...")
				clock.Sleep(c, subscriptionErrorDelay)
				continue
			}
		}
	}))

	log.Debugf(c, "Archivist finished.")
	return nil
}

// Entry point.
func main() {
	a := application{}
	a.Run(context.Background(), a.runArchivist)
}
