// Copyright 2020 The LUCI Authors.
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

package refresher

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/quota/quotapb"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
)

// PM encapsulates Project Manager notified by the ConfigRefresher.
//
// In production, this will be prjmanager.Notifier.
type PM interface {
	Poke(ctx context.Context, luciProject string) error
	UpdateConfig(ctx context.Context, luciProject string) error
}

// QM manages run and tryjob quotas.
type QM interface {
	WritePolicy(ctx context.Context, project string) (*quotapb.PolicyConfigID, error)
}

// Refresher handles RefreshProjectConfigTask.
type Refresher struct {
	pm  PM
	tqd *tq.Dispatcher
	qm  QM
	env *common.Env
}

// NewRefresher creates a new project config Refresher and registers its TQ tasks.
func NewRefresher(tqd *tq.Dispatcher, pm PM, qm QM, env *common.Env) *Refresher {
	pcr := &Refresher{pm, tqd, qm, env}
	pcr.tqd.RegisterTaskClass(tq.TaskClass{
		ID:           "refresh-project-config",
		Prototype:    &RefreshProjectConfigTask{},
		Queue:        "refresh-project-config",
		Kind:         tq.NonTransactional,
		Quiet:        true,
		QuietOnError: true,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*RefreshProjectConfigTask)
			if err := pcr.refreshProject(ctx, task.GetProject(), task.GetDisable()); err != nil {
				// Never retry tasks because the refresh task is submitted every minute
				// by the AppEngine Cron.
				return common.TQIfy{NeverRetry: true}.Error(ctx, err)
			}
			return nil
		},
	})
	return pcr
}

// SubmitRefreshTasks submits tasks that update config for LUCI projects
// or disable projects that do not have CV config in LUCI Config.
//
// It's expected to be called by a cron.
func (r *Refresher) SubmitRefreshTasks(ctx context.Context) error {
	projects, err := projectsWithConfig(ctx, prjcfg.ConfigFileName(ctx))
	if err != nil {
		return err
	}

	tasks := make([]*tq.Task, len(projects))
	for i, p := range projects {
		tasks[i] = &tq.Task{
			Title: "update/" + p,
			Payload: &RefreshProjectConfigTask{
				Project: p,
			},
		}
	}

	curEnabledProjects, err := prjcfg.GetAllProjectIDs(ctx, true)
	if err != nil {
		return err
	}
	projectsInLUCIConfig := stringset.NewFromSlice(projects...)
	for _, p := range curEnabledProjects {
		if !projectsInLUCIConfig.Has(p) {
			tasks = append(tasks, &tq.Task{
				Title: "disable/" + p,
				Payload: &RefreshProjectConfigTask{
					Project: p,
					Disable: true,
				},
			})
		}
	}

	err = parallel.WorkPool(32, func(workCh chan<- func() error) {
		for _, task := range tasks {
			workCh <- func() (err error) {
				if err = r.tqd.AddTask(ctx, task); err != nil {
					logging.Errorf(ctx, "Failed to submit task for %q: %s", task.Title, err)
				}
				return
			}
		}
	})

	if err != nil {
		return err.(errors.MultiError).First()
	}
	return nil
}

func (r *Refresher) refreshProject(ctx context.Context, project string, disable bool) error {
	action, actionFn := "update", UpdateProject
	if disable {
		action, actionFn = "disable", DisableProject
	}
	err := actionFn(ctx, project, func(ctx context.Context) error {
		return r.pm.UpdateConfig(ctx, project)
	})
	if err != nil {
		return errors.Annotate(err, "failed to %s project %q", action, project).Err()
	}

	// Refresh quota policies when the config is refreshed. WritePolicy is idempotent and would return immediately if
	// the policies exist within server/quota.
	// TODO(crbug.com/1466346): Move WritePolicy into UpdateProject.
	if _, err := r.qm.WritePolicy(ctx, project); err != nil {
		return err
	}

	if !disable {
		return r.maybePokePM(ctx, project)
	}
	return nil
}

const pokePMInterval = 10 * time.Minute

func (r *Refresher) maybePokePM(ctx context.Context, project string) error {
	now := clock.Now(ctx).UTC()
	offset := common.DistributeOffset(pokePMInterval, "cron-poke", project)
	nextPokeETA := now.Truncate(pokePMInterval).Add(offset)
	if nextPokeETA.Before(now) {
		nextPokeETA = nextPokeETA.Add(pokePMInterval)
	}

	// Cron runs every minute on average and triggers RefreshProjectConfigTask,
	// which may be delayed, so send iff it's less than 1.5 minutes before next
	// poke. This will sometimes result in 2 pokes sent instead of 1, but pokes
	// are less likely to not be sent at all.
	if nextPokeETA.Sub(now) < 90*time.Second {
		return r.pm.Poke(ctx, project)
	}
	return nil
}
