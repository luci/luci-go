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
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	montypes "go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/server/archivist"
	"go.chromium.org/luci/logdog/server/bundleServicesClient"
	"go.chromium.org/luci/logdog/server/service"

	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// loopParams control the outer archivist loop.
type loopParams struct {
	batchSize int64
	deadline  time.Duration
}

// Geneates a randomized LeaseRequest from this loopParams.
//
// Sizes of archival task batches are pretty uniform.  This means the total time
// it takes to process equally sized batches of tasks is approximately the same
// across all archivist instances. Once a bunch of them start at the same time,
// they end up hitting LeaseArchiveTasks in waves at approximately the same
// time.
//
// This method randomizes the batch size +-25% to remove this synchronization.
func (l loopParams) mkRequest(ctx context.Context) *logdog.LeaseRequest {
	factor := int64(mathrand.Intn(ctx, 500) + 750) // [750, 1250)

	return &logdog.LeaseRequest{
		MaxTasks:  l.batchSize * factor / 1000,
		LeaseTime: ptypes.DurationProto(l.deadline),
	}
}

var (
	errInvalidConfig = errors.New("invalid configuration")
	errNoWorkToDo    = errors.New("no work to do")

	leaseRetryParams = func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   time.Second,
				Retries: -1,
			},
			Multiplier: 1.25,
			MaxDelay:   time.Minute * 10,
		}
	}

	ackRetryParams = func() retry.Iterator {
		return &retry.ExponentialBackoff{
			Limited: retry.Limited{
				Delay:   time.Second,
				Retries: 5,
			},
			Multiplier: 1.25,
		}
	}

	maxAckSize = 500

	// maxSleepTime is the max amount of time to sleep in-between errors, in seconds.
	maxSleepTime = 32

	// tsTaskProcessingTime measures the amount of time spent processing a single
	// task.
	//
	// The "consumed" field is true if the underlying task was consumed and
	// false if it was not.
	tsTaskProcessingTime = metric.NewCumulativeDistribution("logdog/archivist/task_processing_time_ms_ng",
		"The amount of time (in milliseconds) that a single task takes to process in the new pipeline.",
		&montypes.MetricMetadata{Units: montypes.Milliseconds},
		distribution.DefaultBucketer,
		field.Bool("consumed"))

	tsLoopCycleTime = metric.NewCumulativeDistribution("logdog/archivist/loop_cycle_time_ms",
		"The amount of time a single batch of leases takes to process.",
		&montypes.MetricMetadata{Units: montypes.Milliseconds},
		distribution.DefaultBucketer)

	tsLeaseCount = metric.NewCounter("logdog/archivist/tasks_leased",
		"Number of tasks leased.",
		nil)

	tsNackCount = metric.NewCounter("logdog/archivist/tasks_not_acked",
		"Number of tasks leased but failed.",
		nil)

	tsAckCount = metric.NewCounter("logdog/archivist/tasks_acked",
		"Number of tasks successfully completed and acked.",
		nil)
)

// application is the Archivist application state.
type application struct {
	service.Service

	maxConcurrentTasks int
}

type archiveJob struct {
	deadline time.Time
	task     *logdog.ArchiveTask
}

func taskLeaser(ctx context.Context, client logdog.ServicesClient, jobChan chan<- *archiveJob) {
	defer close(jobChan)

	sleepTime := 1

	var previousCycle time.Time
	for ctx.Err() != nil {
		loopParams := grabLoopParams()
		req := loopParams.mkRequest(ctx)

		var tasks *logdog.LeaseResponse
		var deadline time.Time

		logging.Infof(ctx, "Leasing max %d tasks for %s", loopParams.batchSize, loopParams.deadline)
		err := retry.Retry(ctx, leaseRetryParams, func() (err error) {
			deadline = clock.Now(ctx).Add(loop.deadline)
			tasks, err = client.LeaseArchiveTasks(ctx, req)
			return
		}, retry.LogCallback(ctx, "LeaseArchiveTasks"))
		if ctx.Err() != nil && err != nil {
			panic("impossible: infinite retry stopped: " + err.Error())
		}

		if !previousCycle.IsZero() {
			now := clock.Now(ctx)
			tsLoopCycleTime.Add(ctx, float64(now.Sub(previousCycle).Nanoseconds()/1000000))
			previousCycle = now
		}

		if len(tasks.Tasks) == 0 {
			sleepTime *= 2
			if sleepTime > maxSleepTime {
				sleepTime = maxSleepTime
			}
			logging.Infof(ctx, "no work to do, sleeping for %d seconds", sleepTime)
			clock.Sleep(ctx, time.Duration(sleepTime)*time.Second)
			previousCycle = time.Time{}
			continue
		} else {
			sleepTime = 1
		}

		for _, task := range tasks.Tasks {
			select {
			case jobChan <- &archiveJob{deadline, task}:
			case <-ctx.Done():
				logging.Infof(ctx, "lease thread got context err: %s", ctx.Err())
				break
			}
		}
	}

	logging.Infof(ctx, "lease thread quitting")
}

func ackProcessor(ctx context.Context, client logdog.ServicesClient, ackChan <-chan *archiveJob) {
	var batch []*logdog.ArchiveTask
	var batchDeadline time.Time
	batchTimer := clock.NewTimer(ctx)
	defer batchTimer.Stop()

	// sendIt takes the current batch and fires an async routine to do the RPC.
	// It then clears the batch state.
	sendIt := func() {
		if len(batch) > 0 {
			req := &logdog.DeleteRequest{Tasks: batch}
			go func() {
				err := retry.Retry(ctx, ackRetryParams, func() error {
					_, err := client.DeleteArchiveTasks(ctx, req)
					return err
				}, retry.LogCallback(ctx, "DeleteArchiveTasks"))
				if err != ctx.Err() {
					// just log the error; if we end up re-leasing these tasks to something
					// else either the collector
					logging.Errorf(ctx, "Failed to delete %d tasks: %s", len(batch), err)
				}
			}()
		}

		batch = nil
		batchDeadline = time.Time{}
		batchTimer.Stop()
	}

	for {
		var batchDeadlineCh <-chan clock.TimerResult
		if batchDeadline.After(clock.Now(ctx)) {
			batchDeadlineCh = batchTimer.GetC()
		}

		var job *archiveJob
		select {
		case <-batchDeadlineCh:
			sendIt()
		case job = <-ackChan:
		case <-ctx.Done():
			logging.Infof(ctx, "ackProcessor context canceled: %s", ctx.Err())
			return
		}

		batch = append(batch, job.task)
		if len(batch) >= maxAckSize {
			sendIt()
		} else {
			jobDeadline := job.deadline.Add(-time.Second * 30)
			if batchDeadline.IsZero() || jobDeadline.Before(batchDeadline) {
				batchDeadline = jobDeadline
				batchTimer.Reset(clock.Until(ctx, batchDeadline))
			}
		}
	}
}

// runForever runs the archivist loop forever.
func runForever(ctx context.Context, taskConcurrency int, ar archivist.Archivist) {
	// TODO(iannucci): I would have used channel.Dispatcher here but we're still
	// vetting its correctness. (2020Q1)

	jobChan := make(chan *archiveJob)

	ackChan := make(chan *archiveJob)
	defer close(ackChan)

	// goroutine to lease batches and fill jobChan
	go taskLeaser(ctx, ar.Service, jobChan)

	// goroutine to drain ackChan and do ACKs in batches.
	go ackProcessor(ctx, ar.Service, ackChan)

	// finally: work pool to process jobChan and fill ackChan
	parallel.WorkPool(taskConcurrency, func(ch chan<- func() error) {
		for {
			var job *archiveJob
			select {
			case job = <-jobChan:
			case <-ctx.Done():
				logging.Infof(ctx, "runForever context canceled: %s", ctx.Err())
				return
			}

			runArchive := func() error {
				nc, cancel := context.WithDeadline(ctx, job.deadline)
				defer cancel()

				startTime := clock.Now(ctx)
				err := ar.ArchiveTask(nc, job.task)
				duration := clock.Now(ctx).Sub(startTime)
				tsTaskProcessingTime.Add(ctx, float64(duration.Nanoseconds())/1000000, err == nil)

				if err == nil {
					select {
					case ackChan <- job:
					case <-ctx.Done():
						logging.Errorf(ctx, "Failed to ACK task %v due to context: %s", job.task, ctx.Err())
					}
				} else {
					logging.Errorf(ctx, "Failed to archive task %v: %s", job.task, err)
				}

				return nil
			}

			select {
			case <-ctx.Done():
				return
			case ch <- runArchive:
			}
		}
	})
}

// run is the main execution function.
func (a *application) runArchivist(c context.Context) error {
	cfg := a.ServiceConfig()

	coordCfg, acfg := cfg.GetCoordinator(), cfg.GetArchivist()
	switch {
	case coordCfg == nil:
		fallthrough

	case acfg == nil:
		return errors.New("missing required config: archivist")
	case acfg.GsStagingBucket == "":
		return errors.New("missing required config: archivist.gs_staging_bucket")
	}

	// Initialize our Storage.
	st, err := a.IntermediateStorage(c, true)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get storage instance.")
		return err
	}
	defer st.Close()

	// Defines our Google Storage client project scoped factory.
	gsClientFactory := func(ctx context.Context, project string) (gs.Client, error) {
		gsClient, err := a.GSClient(ctx, project)
		if err != nil {
			log.WithError(err).Errorf(c, "Failed to get Google Storage client.")
			return nil, err
		}
		return gsClient, nil
	}

	// Initialize a Coordinator client that bundles requests together.
	coordClient := &bundleServicesClient.Client{
		ServicesClient:       a.Coordinator(),
		DelayThreshold:       time.Second,
		BundleCountThreshold: 100,
	}
	defer coordClient.Flush()

	ar := archivist.Archivist{
		Service:         coordClient,
		SettingsLoader:  a.GetSettingsLoader(acfg),
		Storage:         st,
		GSClientFactory: gsClientFactory,
	}

	// Application shutdown will now operate by stopping the Iterator.
	c, cancelFunc := context.WithCancel(c)
	defer cancelFunc()

	// Application shutdown will now operate by cancelling the Archivist's
	// shutdown Context.
	a.SetShutdownFunc(cancelFunc)

	// Load our settings and update them periodically.
	fetchLoopParams(c)
	go loopParamsUpdater(c)

	runForever(c, a.maxConcurrentTasks, ar)

	return nil
}

// GetSettingsLoader is an archivist.SettingsLoader implementation that merges
// global and project-specific settings.
//
// The resulting settings object will be verified by the Archivist.
func (a *application) GetSettingsLoader(acfg *svcconfig.Archivist) archivist.SettingsLoader {
	serviceID := a.ServiceID()

	return func(c context.Context, project string) (*archivist.Settings, error) {
		// Fold in our project-specific configuration, if valid.
		pcfg, err := a.ProjectConfig(c, project)
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
				"project":    project,
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
	a.Flags.IntVar(&a.maxConcurrentTasks, "max-concurrent-tasks", 1,
		"Maximum number of archive tasks to process concurrently. "+
			"Pass 0 to set infinite limit.")
	a.Run(context.Background(), a.runArchivist)
}
