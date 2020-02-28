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
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	montypes "go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/logdog/api/config/svcconfig"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/server/archivist"
	"go.chromium.org/luci/logdog/server/bundleServicesClient"
	"go.chromium.org/luci/logdog/server/service"

	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// loopParams control the outer archivist loop.
type loopParams struct {
	batchSize int64
	leaseTime time.Duration
}

var (
	errInvalidConfig = errors.New("invalid configuration")
	errNoWorkToDo    = errors.New("no work to do")

	// Note: these initial values are mostly bogus, they are replaced by
	// fetchLoopParams before the loop starts.
	loop = loopParams{
		// batchSize is the number of jobs to lease from taskqueue per cycle.
		//
		// TaskQueue has a limit of 10qps for leasing tasks, so the batch size must
		// be set to:
		// batchSize * 10 > (max expected stream creation QPS)
		//
		// In 2020, max stream QPS is approximately 1000 QPS
		batchSize: 500,

		// leaseTime is the amount of time to to lease the batch of tasks for.
		//
		// We need:
		// (time to process avg stream) * batchSize < leaseTime
		//
		// As of 2020, 90th percentile process time per stream is ~5s, 95th
		// percentile of the loop time is 25m.
		leaseTime: 40 * time.Minute,
	}

	// loopM protects 'loop'.
	loopM = sync.Mutex{}

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
}

// runForever runs the archivist loop forever.
func runForever(c context.Context, ar archivist.Archivist) error {
	sleepTime := 1
	client := ar.Service
	// Forever loop.
	for {
		// Only exit out if the context is cancelled (I.E. Ctrl + c).
		if c.Err() != nil {
			return c.Err()
		}
		if err := func() error {
			loop := grabLoopParams()

			// Sizes of archival tasks are pretty uniform. It means the total time it
			// takes to process equally sized batches of tasks is approximately the
			// same across all archivist instances. Once a bunch of them start at the
			// same time, they end up hitting LeaseArchiveTasks in waves at
			// approximately the same time. Randomize the batch size +-25% to remove
			// this synchronization.
			factor := int64(mathrand.Intn(c, 500) + 750) // [750, 1250)
			loop.batchSize = loop.batchSize * factor / 1000
			loop.leaseTime = loop.leaseTime * time.Duration(factor) / 1000

			cycleStartTime := clock.Now(c)
			leaseTimeProto := ptypes.DurationProto(loop.leaseTime)
			nc, cancel := context.WithTimeout(c, loop.leaseTime)
			defer cancel()

			logging.Infof(c, "Leasing max %d tasks for %s", loop.batchSize, loop.leaseTime)
			tasks, err := client.LeaseArchiveTasks(c, &logdog.LeaseRequest{
				MaxTasks:  loop.batchSize,
				LeaseTime: leaseTimeProto,
			})
			if err != nil {
				return err
			}
			logging.Fields{
				"cycleStart": true,
				"numLeased":  len(tasks.Tasks),
				"noWork":     len(tasks.Tasks) == 0,
			}.Infof(c, "Start of cycled, leased %d tasks", len(tasks.Tasks))
			tsLeaseCount.Add(c, int64(len(tasks.Tasks)))
			if len(tasks.Tasks) == 0 {
				return errNoWorkToDo
			}
			merr := errors.NewLazyMultiError(len(tasks.Tasks))
			ackTasks := make([]*logdog.ArchiveTask, 0, len(tasks.Tasks))
			for i, task := range tasks.Tasks {
				deleteTask := false
				startTime := clock.Now(c)
				if err := ar.ArchiveTask(nc, task); err != nil {
					merr.Assign(i, err)
				} else { // err == nil
					ackTasks = append(ackTasks, task)
					deleteTask = true
				}
				duration := clock.Now(c).Sub(startTime)
				tsTaskProcessingTime.Add(c, float64(duration.Nanoseconds())/1000000, deleteTask)
			}
			tsNackCount.Add(c, int64(len(tasks.Tasks)-len(ackTasks)))
			// Ack the successful tasks.  We log the error here since there's nothing we can do.
			if len(ackTasks) > 0 {
				tsAckCount.Add(c, int64(len(ackTasks)))
				c := log.SetFields(c, log.Fields{"Tasks": ackTasks})
				if _, err := client.DeleteArchiveTasks(c, &logdog.DeleteRequest{Tasks: ackTasks}); err != nil {
					log.WithError(err).Errorf(c, "error while acking tasks (%s)", ackTasks)
				}
			}

			duration := clock.Now(c).Sub(cycleStartTime)
			logging.Fields{
				"cycleEnd":     true,
				"numLeased":    len(tasks.Tasks),
				"numCompleted": len(ackTasks),
				"timeSpentSec": duration.Seconds(),
			}.Infof(c, "Done archiving %d items (%d successful) took %.2fs", len(tasks.Tasks), len(ackTasks), duration.Seconds())
			tsLoopCycleTime.Add(c, float64(duration.Nanoseconds()/1000000))
			return merr.Get()
		}(); err != nil {
			// Back off on errors.
			sleepTime *= 2
			if sleepTime > maxSleepTime {
				sleepTime = maxSleepTime
			}
			switch err {
			case errNoWorkToDo:
				logging.Infof(c, "no work to do, sleeping for %d seconds", sleepTime)
			default:
				logging.WithError(err).Errorf(c, "got error in loop, sleeping for %d seconds", sleepTime)
			}
			clock.Sleep(c, time.Duration(sleepTime)*time.Second)
		} else {
			sleepTime = 1
		}
	}
}

// grabLoopParams returns a copy of most recent value of `loop`.
func grabLoopParams() loopParams {
	loopM.Lock()
	defer loopM.Unlock()
	return loop
}

// fetchLoopParams updates `loop` based on settings in datastore (or panics).
func fetchLoopParams(c context.Context) {
	// Note: these are eventually controlled through /admin/portal/archivist.
	set := coordinator.GetSettings(c)
	loopM.Lock()
	defer loopM.Unlock()
	if set.ArchivistBatchSize != 0 {
		loop.batchSize = set.ArchivistBatchSize
	}
	if set.ArchivistLeaseTime != 0 {
		loop.leaseTime = set.ArchivistLeaseTime
	}
	logging.Debugf(c, "loop settings: batchSize:%d, leastTime:%s", loop.batchSize, loop.leaseTime)
}

// loopParamsUpdater updates loopParams based on setting in datastore every once
// in a while.
func loopParamsUpdater(c context.Context) {
	for {
		if clock.Sleep(c, 5*time.Minute).Err != nil {
			break //  the context is canceled, the process is exiting
		}
		fetchLoopParams(c)
	}
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

	return runForever(c, ar)
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
	a.Run(context.Background(), a.runArchivist)
}
