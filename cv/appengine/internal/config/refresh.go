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
	pb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
)

const configFileName = "commit-queue.cfg"

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "refresh-project-config",
		Prototype: &RefreshProjectConfigTask{},
		Queue:     "refresh-project-config",
		Handler: func(ctx context.Context, payload proto.Message) error {
			project := payload.(*RefreshProjectConfigTask).Project
			if err := RefreshConfig(ctx, project); err != nil {
				errors.Log(ctx, err)
				// Explicitly not forwarding transient tag here so that tq won't
				// retry because the refresh task are submitted every minute by
				// AppEngine Cron.
				return errors.Reason("failed to refresh config for project %q: %s", project, err).Err()
			}

			return nil
		},
	})
}

// SubmitRefreshTasks submits tasks that refresh config for LUCI projects.
//
// All projects that are enabled in CV or currently have CV config will
// be refreshed.
func SubmitRefreshTasks(ctx context.Context) error {
	projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
	if err != nil {
		return errors.Annotate(err, "failed to get projects that have %q from LUCI Config", configFileName).Tag(transient.Tag).Err()
	}
	projectsToRefresh := stringset.NewFromSlice(projects...)
	projects, err = getAllProjectIDs(ctx, true)
	if err != nil {
		return err
	}
	projectsToRefresh.AddAll(projects)

	err = parallel.WorkPool(32, func(tasks chan<- func() error) {
		projectsToRefresh.Iter(func(project string) bool {
			tasks <- func() error {
				err := tq.AddTask(ctx, &tq.Task{
					Payload: &RefreshProjectConfigTask{
						Project: project,
					},
				})
				if err != nil {
					logging.WithError(err).Errorf(ctx, "Failed to submit refresh task for project %q", project)
				}
				return err
			}
			return true
		})
	})

	if err != nil {
		return err.(errors.MultiError).First()
	}
	return nil
}

// RefreshConfig imports the latest CV config for a given LUCI Project from
// LUCI Config.
//
// If the given Project can't be found in LUCI Config or doesn't have CV config,
// this Project will be moved to disabled state in CV.
func RefreshConfig(ctx context.Context, project string) error {
	switch externalHash, err := getContentHashFromLUCIConfig(ctx, project); {
	case err == config.ErrNoConfig:
		return disableProject(ctx, project)
	case err != nil:
		return err
	default:
		return updateProject(ctx, project, externalHash)
	}
}

func getContentHashFromLUCIConfig(ctx context.Context, project string) (string, error) {
	var meta config.Meta
	switch err := cfgclient.Get(ctx, config.ProjectSet(project), configFileName, nil, &meta); {
	case err == config.ErrNoConfig:
		return "", err
	case err != nil:
		return "", errors.Annotate(err, "failed to fetch meta from LUCI Config").Tag(transient.Tag).Err()
	case meta.ContentHash == "":
		return "", errors.Reason("LUCI Config returns empty content hash for project %q", project).Err()
	default:
		// TODO(crbug/710619): Remove this additional check once the bug is fixed.
		// Checking whether `err` is ErrNoConfig would be sufficient.
		projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
		if err != nil {
			return "", errors.Annotate(err, "failed to get projects that have file %q from LUCI Config", configFileName).Tag(transient.Tag).Err()
		}
		for _, p := range projects {
			if p == project {
				return meta.ContentHash, nil
			}
		}
		return "", config.ErrNoConfig
	}
}

func updateProject(ctx context.Context, project, externalHash string) error {
	existingPC := ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, &existingPC); {
	case err != nil && err != datastore.ErrNoSuchEntity:
		return errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
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
	hashInfo := ConfigHashInfo{
		Hash:    localHash,
		Project: datastore.MakeKey(ctx, projectConfigKind, project),
	}
	switch err := datastore.Get(ctx, &hashInfo); {
	case err == datastore.ErrNoSuchEntity:
		hashInfo.UpdateTime = datastore.RoundTime(clock.Now(ctx))
		hashInfo.ConfigGroupNames = cgNames
		if err := datastore.Put(ctx, &hashInfo); err != nil {
			return errors.Annotate(err, "failed to put ConfigHashInfo(Hash=%q)", localHash).Tag(transient.Tag).Err()
		}
	case err != nil:
		return errors.Annotate(err, "failed to get ConfigHashInfo(Hash=%q)", localHash).Tag(transient.Tag).Err()
	}
	if err := putConfigGroups(ctx, cfg, project, localHash); err != nil {
		return err
	}

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		pc := ProjectConfig{Project: project}
		targetEVersion := existingPC.EVersion + 1
		switch err := datastore.Get(ctx, &pc); {
		case err != nil && err != datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
		case pc.EVersion != existingPC.EVersion:
			// Concurrent updateProject was faster. Leave it as is.
			return nil
		default:
			now := datastore.RoundTime(clock.Now(ctx))
			pc = ProjectConfig{
				Project:          project,
				Enabled:          true,
				UpdateTime:      now,
				EVersion:         targetEVersion,
				Hash:             localHash,
				ExternalHash:     externalHash,
				ConfigGroupNames: cgNames,
			}

			hashInfo.ProjectEVersion = targetEVersion
			hashInfo.UpdateTime = now
			if err := datastore.Put(ctx, &pc, &hashInfo); err != nil {
				return err
			}
			// TODO(yiwzhang): Notify ProjectManager
			return nil
		}
	}, nil)
	return errors.Annotate(err, "failed to run transaction to update ProjectConfig(project=%q)", project).Err()
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
	return ret, nil
}

func disableProject(ctx context.Context, project string) error {
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		pc := ProjectConfig{Project: project}
		switch err := datastore.Get(ctx, &pc); {
		case datastore.IsErrNoSuchEntity(err):
			return nil // No-op when disabling non-existent Project
		case err != nil:
			return errors.Annotate(err, "failed to get existing ProjectConfig").Tag(transient.Tag).Err()
		}
		pc.Enabled = false
		pc.UpdateTime = datastore.RoundTime(clock.Now(ctx))
		pc.EVersion++
		if err := datastore.Put(ctx, &pc); err != nil {
			return errors.Annotate(err, "failed to put ProjectConfig").Tag(transient.Tag).Err()
		}
		// TODO(yiwzhang): Notify ProjectManager
		return nil
	}, nil)
	return errors.Annotate(err, "failed to run transaction to disable project %q", project).Err()
}
