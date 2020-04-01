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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	montypes "go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	"golang.org/x/time/rate"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/server/archivist"
	"go.chromium.org/luci/logdog/server/bundleServicesClient"
	"go.chromium.org/luci/logdog/server/service"
)

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

	ackChannelOptions = &dispatcher.Options{
		QPSLimit: rate.NewLimiter(1, 1), // 1 QPS max rate
		Buffer: buffer.Options{
			MaxLeases:     10,
			BatchSize:     500,
			BatchDuration: 10 * time.Minute,
			FullBehavior: &buffer.BlockNewItems{
				MaxItems: 10 * 500,
			},
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   time.Second,
						Retries: 10,
					},
					Multiplier: 1.25,
					MaxDelay:   time.Minute * 10,
				}
			},
		},
	}

	mkJobChannelOptions = func(maxWorkers int) *dispatcher.Options {
		return &dispatcher.Options{
			Buffer: buffer.Options{
				MaxLeases: maxWorkers,
				BatchSize: 1,
				FullBehavior: &buffer.BlockNewItems{
					MaxItems: 2 * maxWorkers,
				},
			},
		}
	}

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

// runForever runs the archivist loop forever.
func runForever(ctx context.Context, taskConcurrency int, ar archivist.Archivist) {
	type archiveJob struct {
		deadline time.Time
		task     *logdog.ArchiveTask
	}

	ackChan, err := dispatcher.NewChannel(ctx, ackChannelOptions, func(batch *buffer.Batch) error {
		var req *logdog.DeleteRequest
		if batch.Meta != nil {
			req = batch.Meta.(*logdog.DeleteRequest)
		} else {
			tasks := make([]*logdog.ArchiveTask, len(batch.Data))
			for i, datum := range batch.Data {
				tasks[i] = datum.(*logdog.ArchiveTask)
				batch.Data[i] = nil
			}
			req = &logdog.DeleteRequest{Tasks: tasks}
			batch.Meta = req
		}
		_, err := ar.Service.DeleteArchiveTasks(ctx, req)
		if err == nil {
			tsAckCount.Add(ctx, int64(len(req.Tasks)))
		}
		return transient.Tag.Apply(err)
	})
	if err != nil {
		panic(err) // only occurs if Options is invalid
	}
	defer func() {
		logging.Infof(ctx, "draining ACK channel")
		ackChan.CloseAndDrain(ctx)
		logging.Infof(ctx, "ACK channel drained")
	}()

	jobChanOpts := mkJobChannelOptions(taskConcurrency)
	jobChan, err := dispatcher.NewChannel(ctx, jobChanOpts, func(data *buffer.Batch) error {
		job := data.Data[0].(*archiveJob)

		nc, cancel := context.WithDeadline(ctx, job.deadline)
		defer cancel()

		startTime := clock.Now(ctx)
		err := ar.ArchiveTask(nc, job.task)
		duration := clock.Now(ctx).Sub(startTime)
		tsTaskProcessingTime.Add(ctx, float64(duration.Nanoseconds())/1000000, err == nil)

		if err == nil {
			select {
			case ackChan.C <- job.task:
			case <-ctx.Done():
				logging.Errorf(ctx, "Failed to ACK task %v due to context: %s", job.task, ctx.Err())
			}
		} else {
			tsNackCount.Add(ctx, 1)
			logging.Errorf(ctx, "Failed to archive task %v: %s", job.task, err)
		}

		return nil
	})
	if err != nil {
		panic(err) // only occurs if Options is invalid
	}
	defer func() {
		logging.Infof(ctx, "Job channel draining")
		jobChan.CloseAndDrain(ctx)
		logging.Infof(ctx, "Job channel drained")
	}()

	// now we spin forever, pushing items into jobChan.
	sleepTime := 1
	var previousCycle time.Time
	for ctx.Err() == nil {
		var tasks *logdog.LeaseResponse
		var deadline time.Time

		err := retry.Retry(ctx, leaseRetryParams, func() (err error) {
			// we grab here to avoid getting stuck with some bad loopParams.
			loopParams := grabLoopParams()
			req := loopParams.mkRequest(ctx)
			logging.Infof(ctx, "Leasing max %d tasks for %s", loopParams.batchSize, loopParams.deadline)
			deadline = clock.Now(ctx).Add(loop.deadline)
			tasks, err = ar.Service.LeaseArchiveTasks(ctx, req)
			return
		}, retry.LogCallback(ctx, "LeaseArchiveTasks"))
		if ctx.Err() == nil && err != nil {
			panic("impossible: infinite retry stopped: " + err.Error())
		}

		now := clock.Now(ctx)
		if !previousCycle.IsZero() {
			tsLoopCycleTime.Add(ctx, float64(now.Sub(previousCycle).Nanoseconds()/1000000))
		}
		previousCycle = now

		if len(tasks.Tasks) == 0 {
			sleepTime *= 2
			if sleepTime > maxSleepTime {
				sleepTime = maxSleepTime
			}
			logging.Infof(ctx, "no work to do, sleeping for %d seconds", sleepTime)
			clock.Sleep(ctx, time.Duration(sleepTime)*time.Second)
			continue
		} else {
			tsLeaseCount.Add(ctx, int64(len(tasks.Tasks)))
			sleepTime = 1
		}

		for _, task := range tasks.Tasks {
			select {
			case jobChan.C <- &archiveJob{deadline, task}:
			case <-ctx.Done():
				logging.Infof(ctx, "lease thread got context err: %s", ctx.Err())
				break
			}
		}
	}

	logging.Infof(ctx, "runForever no longer running forever: ctx.Err() == %s", ctx.Err())
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
