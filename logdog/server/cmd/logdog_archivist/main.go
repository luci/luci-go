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

	cloudlogging "cloud.google.com/go/logging"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/server/archivist"
	"go.chromium.org/luci/logdog/server/service"
)

var (
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

	ackChannelOptions = &dispatcher.Options[*logdog.ArchiveTask]{
		Buffer: buffer.Options{
			MaxLeases:     10,
			BatchItemsMax: 500,
			BatchAgeMax:   10 * time.Minute,
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

	mkJobChannelOptions = func(maxWorkers int) *dispatcher.Options[any] {
		return &dispatcher.Options[any]{
			Buffer: buffer.Options{
				MaxLeases:     maxWorkers,
				BatchItemsMax: 1,
				FullBehavior: &buffer.BlockNewItems{
					// Currently (2020Q2) it takes ~4s on average to process a task, and
					// ~60s to lease a new batch. We never want to starve our job workers
					// here, so we need at least 60s worth of tasks. We add 20% margin,
					// just to be safe.
					MaxItems: int(((60. / 4.) * float64(maxWorkers)) * 1.2),
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
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.Bool("consumed"))

	tsLoopCycleTime = metric.NewCumulativeDistribution("logdog/archivist/loop_cycle_time_ms",
		"The amount of time a single batch of leases takes to process.",
		&types.MetricMetadata{Units: types.Milliseconds},
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

// runForever runs the archivist loop forever.
func runForever(ctx context.Context, ar *archivist.Archivist, flags *CommandLineFlags) {
	type archiveJob struct {
		deadline time.Time
		task     *logdog.ArchiveTask
	}

	ackChan, err := dispatcher.NewChannel[*logdog.ArchiveTask](ctx, ackChannelOptions, func(batch *buffer.Batch[*logdog.ArchiveTask]) error {
		var req *logdog.DeleteRequest
		if batch.Meta != nil {
			req = batch.Meta.(*logdog.DeleteRequest)
		} else {
			tasks := make([]*logdog.ArchiveTask, len(batch.Data))
			for i, datum := range batch.Data {
				tasks[i] = datum.Item
				batch.Data[i].Item = nil
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

	jobChanOpts := mkJobChannelOptions(flags.MaxConcurrentTasks)
	jobChan, err := dispatcher.NewChannel[any](ctx, jobChanOpts, func(data *buffer.Batch[any]) error {
		job := data.Data[0].Item.(*archiveJob)

		nc, cancel := context.WithDeadline(ctx, job.deadline)
		defer cancel()

		nc = logging.SetFields(nc, logging.Fields{
			"project": job.task.Project,
			"id":      job.task.Id,
		})

		startTime := clock.Now(nc)
		err := ar.ArchiveTask(nc, job.task)
		duration := clock.Now(nc).Sub(startTime)

		tsTaskProcessingTime.Add(ctx, float64(duration.Nanoseconds())/1000000, err == nil)

		if err == nil {
			select {
			case ackChan.C <- job.task:
			case <-ctx.Done():
				logging.Errorf(nc, "Failed to ACK task %v due to context: %s", job.task, ctx.Err())
			}
		} else {
			tsNackCount.Add(ctx, 1)
			logging.Errorf(nc, "Failed to archive task %v: %s", job.task, err)
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
			logging.Infof(ctx, "Leasing max %d tasks for %s", flags.LeaseBatchSize, flags.LeaseTime)
			deadline = clock.Now(ctx).Add(flags.LeaseTime)
			tasks, err = ar.Service.LeaseArchiveTasks(ctx, &logdog.LeaseRequest{
				MaxTasks:  int64(flags.LeaseBatchSize),
				LeaseTime: durationpb.New(flags.LeaseTime),
			})
			return
		}, retry.LogCallback(ctx, "LeaseArchiveTasks"))
		if ctx.Err() != nil {
			logging.Infof(ctx, "lease thread got context err in RPC: %s", ctx.Err())
			break
		}
		if err != nil {
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
				logging.Infof(ctx, "lease thread got context err in jobChan push: %s", ctx.Err())
				break
			}
		}
	}
}

// googleStorageClient returns an authenticated Google Storage client instance.
func googleStorageClient(ctx context.Context, luciProject string) (gs.Client, error) {
	// TODO(vadimsh): Switch to AsProject + WithProject(project) once
	// we are ready to roll out project scoped service accounts in Logdog.
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "failed to get the authenticating transport").Err()
	}
	client, err := gs.NewProdClient(ctx, tr)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create Google Storage client").Err()
	}
	return client, nil
}

// cloudLoggingClient returns an authenticated Cloud Logging client instance.
func cloudLoggingClient(ctx context.Context, luciProject, cloudProject string, onError func(err error)) (archivist.CLClient, error) {
	cred, err := auth.GetPerRPCCredentials(
		ctx, auth.AsProject,
		auth.WithScopes(auth.CloudOAuthScopes...),
		auth.WithProject(luciProject),
	)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get per RPC credentials").Err()
	}
	cl, err := cloudlogging.NewClient(
		ctx, cloudProject,
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(cred)),
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor())),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(otelgrpc.StreamClientInterceptor())),
	)
	if err != nil {
		return nil, err
	}
	cl.OnError = onError
	return cl, nil
}

// Entry point.
func main() {
	flags := DefaultCommandLineFlags()
	flags.Register(flag.CommandLine)

	cfg := service.MainCfg{BigTableAppProfile: "archivist"}
	service.Main(cfg, func(srv *server.Server, impl *service.Implementations) error {
		if err := flags.Validate(); err != nil {
			return err
		}

		// Initialize the archivist.
		ar := &archivist.Archivist{
			Service:         impl.Coordinator,
			SettingsLoader:  GetSettingsLoader(srv.Options.CloudProject, &flags),
			Storage:         impl.Storage,
			GSClientFactory: googleStorageClient,
			CLClientFactory: cloudLoggingClient,
		}

		// Run the archivist loop until the server closes.
		srv.RunInBackground("archivist", func(ctx context.Context) {
			runForever(ctx, ar, &flags)
		})
		return nil
	})
}
