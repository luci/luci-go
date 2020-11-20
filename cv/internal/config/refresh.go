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

package config

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	pb "go.chromium.org/luci/cv/api/config/v2"
)

const configFileName = "commit-queue.cfg"

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "refresh-project-config",
		Prototype: &RefreshProjectConfigTask{},
		Queue:     "refresh-project-config",
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*RefreshProjectConfigTask)
			action, actionFn := "update", updateProject
			if task.GetDisable() {
				action, actionFn = "disable", disableProject
			}
			project := task.GetProject()
			if err := actionFn(ctx, project); err != nil {
				errors.Log(ctx, err)
				// Explicitly not forwarding transient tag so that tq won't retry
				// because the refresh task is submitted every minute by AppEngine
				// Cron.
				return errors.Reason("failed to %s project %q: %s", action, project, err).Err()
			}
			return nil
		},
	})
}

// SubmitRefreshTasks submits tasks that update config for LUCI projects
// or disable projects that do not have CV config in LUCI Config.
func SubmitRefreshTasks(ctx context.Context) error {
	projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
	if err != nil {
		return errors.Annotate(err, "failed to get projects that have %q from LUCI Config", configFileName).Tag(transient.Tag).Err()
	}
	tasks := make([]*tq.Task, len(projects))
	for i, p := range projects {
		tasks[i] = &tq.Task{
			Payload: &RefreshProjectConfigTask{
				Project: p,
			},
		}
	}

	curEnabledProjects, err := getAllProjectIDs(ctx, true)
	if err != nil {
		return err
	}
	projectsInLUCIConfig := stringset.NewFromSlice(projects...)
	for _, p := range curEnabledProjects {
		if !projectsInLUCIConfig.Has(p) {
			tasks = append(tasks, &tq.Task{
				Payload: &RefreshProjectConfigTask{
					Project: p,
					Disable: true,
				},
			})
		}
	}

	err = parallel.WorkPool(32, func(workCh chan<- func() error) {
		for _, task := range tasks {
			workCh <- func() error {
				if err := tq.AddTask(ctx, task); err != nil {
					logging.WithError(err).Errorf(ctx, "Failed to submit refresh task for project %q", task.Payload.(*RefreshProjectConfigTask).GetProject())
					return err
				}
				return nil
			}
		}
	})

	if err != nil {
		return err.(errors.MultiError).First()
	}
	return nil
}

// updateProject imports the latest CV Config for a given LUCI Project
// from LUCI Config if the config in CV is outdated.
func updateProject(ctx context.Context, project string) error {
	externalHash, err := getExternalContentHash(ctx, project)
	if err != nil {
		return err
	}
	existingPC := ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, &existingPC); {
	case err != nil && err != datastore.ErrNoSuchEntity:
		return errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
	case !existingPC.Enabled:
		// Go through update process to ensure all configs are present.
	case existingPC.ExternalHash == externalHash:
		return nil // up-to-date
	}

	cfg, err := fetchCfg(ctx, project, externalHash)
	if err != nil {
		return err
	}

	// Write out ConfigHashInfo if missing and all ConfigGroups.
	localHash := computeHash(cfg)
	cgNames := make([]string, len(cfg.GetConfigGroups()))
	for i, cg := range cfg.GetConfigGroups() {
		cgNames[i] = makeConfigGroupName(cg.GetName(), i)
	}
	targetEVersion := existingPC.EVersion + 1

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		hashInfo := ConfigHashInfo{
			Hash:    localHash,
			Project: datastore.MakeKey(ctx, projectConfigKind, project),
		}
		switch err := datastore.Get(ctx, &hashInfo); {
		case err != nil && err != datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "failed to get ConfigHashInfo(Hash=%q)", localHash).Tag(transient.Tag).Err()
		case err == nil && hashInfo.ProjectEVersion >= targetEVersion:
			return nil // Do not go backwards.
		default:
			hashInfo.ProjectEVersion = targetEVersion
			hashInfo.UpdateTime = datastore.RoundTime(clock.Now(ctx)).UTC()
			hashInfo.ConfigGroupNames = cgNames
			return errors.Annotate(datastore.Put(ctx, &hashInfo), "failed to put ConfigHashInfo(Hash=%q)", localHash).Tag(transient.Tag).Err()
		}
	}, nil)
	if err != nil {
		return errors.Annotate(err, "failed to run transaction to update ConfigHashInfo").Tag(transient.Tag).Err()
	}

	if err := putConfigGroups(ctx, cfg, project, localHash); err != nil {
		return err
	}

	updated := false
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		updated = false
		pc := ProjectConfig{Project: project}
		switch err := datastore.Get(ctx, &pc); {
		case err != nil && err != datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
		case pc.EVersion != existingPC.EVersion:
			return nil // Already updated by concurrent updateProject.
		default:
			pc = ProjectConfig{
				Project:          project,
				Enabled:          true,
				UpdateTime:       datastore.RoundTime(clock.Now(ctx)).UTC(),
				EVersion:         targetEVersion,
				Hash:             localHash,
				ExternalHash:     externalHash,
				ConfigGroupNames: cgNames,
			}
			updated = true
			return errors.Annotate(datastore.Put(ctx, &pc), "failed to put ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
			// TODO(yiwzhang): Notify ProjectManager
		}
	}, nil)

	switch {
	case err != nil:
		return errors.Annotate(err, "failed to run transaction to update ProjectConfig").Tag(transient.Tag).Err()
	case updated:
		logging.Infof(ctx, "updated project %q to rev %s hash %s ", project, "TODO", localHash)
	}
	return nil
}

func getExternalContentHash(ctx context.Context, project string) (string, error) {
	var meta config.Meta
	switch err := cfgclient.Get(ctx, config.ProjectSet(project), configFileName, nil, &meta); {
	case err != nil:
		return "", errors.Annotate(err, "failed to fetch meta from LUCI Config").Tag(transient.Tag).Err()
	case meta.ContentHash == "":
		return "", errors.Reason("LUCI Config returns empty content hash for project %q", project).Err()
	default:
		return meta.ContentHash, nil
	}
}

func fetchCfg(ctx context.Context, project string, contentHash string) (*pb.Config, error) {
	content, err := cfgclient.Client(ctx).GetConfigByHash(ctx, contentHash)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get config by content hash").Tag(transient.Tag).Err()
	}
	ret := &pb.Config{}
	if err := cfgclient.ProtoText(ret)(content); err != nil {
		return nil, errors.Annotate(err, "failed to deserialize config content").Err()
	}
	// TODO(yiwzhang): validate the config here again to prevent ingesting a bad
	// version of config that accidentally slips into LUCI Config.
	// See: go.chromium.org/luci/cq/appengine/config
	return ret, nil
}

// disableProject disables the given LUCI Project if it is currently enabled.
func disableProject(ctx context.Context, project string) error {
	disabled := false

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		disabled = false
		pc := ProjectConfig{Project: project}
		switch err := datastore.Get(ctx, &pc); {
		case datastore.IsErrNoSuchEntity(err):
			return nil // No-op when disabling non-existent Project
		case err != nil:
			return errors.Annotate(err, "failed to get existing ProjectConfig").Tag(transient.Tag).Err()
		case !pc.Enabled:
			return nil // Already disabled
		}
		pc.Enabled = false
		pc.UpdateTime = datastore.RoundTime(clock.Now(ctx)).UTC()
		pc.EVersion++
		if err := datastore.Put(ctx, &pc); err != nil {
			return errors.Annotate(err, "failed to put ProjectConfig").Tag(transient.Tag).Err()
		}
		disabled = true
		// TODO(yiwzhang): Notify ProjectManager
		return nil
	}, nil)

	switch {
	case err != nil:
		return errors.Annotate(err, "failed to run transaction to disable project %q", project).Tag(transient.Tag).Err()
	case disabled:
		logging.Infof(ctx, "disabled project %q", project)
	}
	return nil
}
