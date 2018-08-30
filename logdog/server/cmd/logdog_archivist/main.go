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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	montypes "go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/server/archivist"
	"go.chromium.org/luci/logdog/server/bundleServicesClient"
	"go.chromium.org/luci/logdog/server/service"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"

	"go.chromium.org/luci/hardcoded/chromeinfra"

	"net/http"
	_ "net/http/pprof"
)

var (
	errInvalidConfig = errors.New("invalid configuration")

	// tsTaskProcessingTime measures the amount of time spent processing a single
	// task.
	//
	// The "consumed" field is true if the underlying task was consumed and
	// false if it was not.
	tsTaskProcessingTime = metric.NewCumulativeDistribution("logdog/archivist/task_processing_time_ms",
		"The amount of time (in milliseconds) that a single task takes to process.",
		&montypes.MetricMetadata{Units: montypes.Milliseconds},
		distribution.DefaultBucketer,
		field.Bool("consumed"))
)

// application is the Archivist application state.
type application struct {
	service.Service
}

// run is the main execution function.
func (a *application) runArchivist(c context.Context) error {
	cfg := a.ServiceConfig()

	// Starting a webserver for pprof.
	// TODO(hinoka): Checking for memory leaks, Remove me when 795156 is fixed.
	go func() {
		log.WithError(http.ListenAndServe("localhost:6060", nil)).Errorf(c, "failed to start webserver")
	}()

	coordCfg, acfg := cfg.GetCoordinator(), cfg.GetArchivist()
	switch {
	case coordCfg == nil:
		fallthrough

	case acfg == nil:
		return errors.New("missing required config: archivist")
	case acfg.GsStagingBucket == "":
		return errors.New("missing required config: archivist.gs_staging_bucket")
	}

	// Initialize Pub/Sub client.
	//
	// We will initialize both an authenticated Client instance and an
	// authenticated Context, since we need the latter for raw ACK deadline
	// updates.
	taskSub := gcps.Subscription(acfg.Subscription)
	if err := taskSub.Validate(); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"value":      taskSub,
		}.Errorf(c, "Task subscription did not validate.")
		return errors.New("invalid task subscription name")
	}
	psProject, psSubscriptionName := taskSub.Split()

	// New PubSub instance with the authenticated client.
	psClient, err := a.Service.PubSubSubscriberClient(c, psProject)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to create Pub/Sub client.")
		return err
	}
	sub := psClient.Subscription(psSubscriptionName)
	sub.ReceiveSettings = pubsub.ReceiveSettings{
		// These must be -1 (unlimited), otherwise the flow controller will saturate.
		// PubSub performs poorly as a Task Queue otherwise.
		// https://github.com/GoogleCloudPlatform/google-cloud-go/issues/919#issuecomment-372403175
		MaxExtension:        -1,
		MaxOutstandingBytes: -1,

		MaxOutstandingMessages: 16,
		NumGoroutines:          8,
	}

	// Initialize our Storage.
	//
	// NOTE: We're requesting read/write access even though we only need read-only
	// access because GKE doesn't understand the read-only scope:
	// https://www.googleapis.com/auth/bigtable.readonly
	st, err := a.IntermediateStorage(c, true)
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

	// Initialize a Coordinator client that bundles requests together.
	coordClient := &bundleServicesClient.Client{
		ServicesClient:       a.Coordinator(),
		DelayThreshold:       time.Second,
		BundleCountThreshold: 100,
	}
	defer coordClient.Flush()

	ar := archivist.Archivist{
		Service:        coordClient,
		SettingsLoader: a.GetSettingsLoader(acfg),
		Storage:        st,
		GSClient:       gsClient,
	}

	// Application shutdown will now operate by stopping the Iterator.
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	// Application shutdown will now operate by cancelling the Collector's
	// shutdown Context.
	a.SetShutdownFunc(cancelFunc)

	// Execute our main subscription pull loop. It will run until the supplied
	// Context is cancelled.
	log.Fields{
		"subscription": taskSub,
	}.Infof(c, "Pulling tasks from Pub/Sub subscription.")

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
		return grpcutil.WrapIfTransient(sub.Receive(c, func(c context.Context, msg *pubsub.Message) {
			c = log.SetFields(c, log.Fields{
				"messageID": msg.ID,
			})

			// ACK or NACK the message based on whether our task was consumed.
			deleteTask := false
			defer func() {
				// ACK the message if it is completed. If not, NACK it.
				if deleteTask {
					msg.Ack()
				} else {
					msg.Nack()
				}
			}()

			// Time how long task processing takes for metrics.
			startTime := clock.Now(c)
			defer func() {
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

				// Add to our processing time metric.
				tsTaskProcessingTime.Add(c, duration.Seconds()*1000, deleteTask)
			}()

			task, err := makePubSubArchivistTask(c, psSubscriptionName, msg)
			c = log.SetFields(c, log.Fields{
				"consumed":         task.consumed,
				"subscriptionName": task.subscriptionName,
				"taskTimestamp":    task.timestamp.Format(time.RFC3339Nano),
				"archiveTask":      task.at,
			})
			if task.msg != nil {
				// Log all fields except data.
				c = log.SetFields(c, log.Fields{
					"message": map[string]interface{}{
						"id":          task.msg.ID,
						"attributes":  task.msg.Attributes,
						"publishTime": task.msg.PublishTime.Format(time.RFC3339Nano),
					},
				})
			}
			if err != nil {
				log.WithError(err).Errorf(c, "Failed to unmarshal archive task from message.")
				deleteTask = true
				return
			}

			ar.ArchiveTask(c, task)
			deleteTask = task.consumed
		}))
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Warningf(c, "Transient error during subscription Receive loop; retrying...")
	})

	if err := errors.Unwrap(err); err != nil && err != context.Canceled {
		log.WithError(err).Errorf(c, "Failed during Pub/Sub Receive.")
		return err
	}

	log.Debugf(c, "Archivist finished.")
	return nil
}

// GetSettingsLoader is an archivist.SettingsLoader implementation that merges
// global and project-specific settings.
//
// The resulting settings object will be verified by the Archivist.
func (a *application) GetSettingsLoader(acfg *svcconfig.Archivist) archivist.SettingsLoader {
	serviceID := a.ServiceID()

	return func(c context.Context, proj types.ProjectName) (*archivist.Settings, error) {
		// Fold in our project-specific configuration, if valid.
		pcfg, err := a.ProjectConfig(c, proj)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"project":    proj,
			}.Errorf(c, "Failed to fetch project configuration.")
			return nil, err
		}

		indexParam := func(get func(ic *svcconfig.ArchiveIndexConfig) int32) int {
			if ic := pcfg.ArchiveIndexConfig; ic != nil {
				if v := get(ic); v > 0 {
					return int(v)
				}
			}

			if ic := acfg.ArchiveIndexConfig; ic != nil {
				if v := get(ic); v > 0 {
					return int(v)
				}
			}

			return 0
		}

		// Load our base settings.
		//
		// Archival bases are:
		// Staging: gs://<services:gs_staging_bucket>/<project-id>/...
		// Archive: gs://<project:archive_gs_bucket>/<project-id>/...
		st := archivist.Settings{
			GSBase:        gs.MakePath(pcfg.ArchiveGsBucket, "").Concat(serviceID),
			GSStagingBase: gs.MakePath(acfg.GsStagingBucket, "").Concat(serviceID),

			IndexStreamRange: indexParam(func(ic *svcconfig.ArchiveIndexConfig) int32 { return ic.StreamRange }),
			IndexPrefixRange: indexParam(func(ic *svcconfig.ArchiveIndexConfig) int32 { return ic.PrefixRange }),
			IndexByteRange:   indexParam(func(ic *svcconfig.ArchiveIndexConfig) int32 { return ic.ByteRange }),
			AlwaysRender:     (acfg.RenderAllStreams || pcfg.RenderAllStreams),
		}

		// Fold project settings into loaded ones.
		return &st, nil
	}
}

// Entry point.
func main() {
	mathrand.SeedRandomly()
	a := application{
		Service: service.Service{
			Name:               "archivist",
			DefaultAuthOptions: chromeinfra.DefaultAuthOptions(),
		},
	}
	a.Run(context.Background(), a.runArchivist)
}
