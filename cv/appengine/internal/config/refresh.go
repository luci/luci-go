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
	"sync/atomic"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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
			return RefreshConfig(ctx, project)
		},
	})
}

// SubmitRefreshTasks submits tasks that refresh config for LUCI projects.
//
// This includes all Projects that has CV config in LUCI Config and all
// currently enabled Projects in CV.
func SubmitRefreshTasks(ctx context.Context) error {
	projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
	if err != nil {
		return errors.Annotate(err, "get projects that have %q from LUCI Config", configFileName).Err()
	}
	projectsToRefresh := stringset.NewFromSlice(projects...)
	projects, err = getAllProjectIDs(ctx, true)
	if err != nil {
		return errors.Annotate(err, "get currently enabled projects").Err()
	}
	projectsToRefresh.AddAll(projects)

	var failedCount int32
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
					atomic.AddInt32(&failedCount, 1)
				}
				return err
			}
			return true
		})
	})

	if err != nil {
		return errors.Annotate(err, "failed to submit refresh tasks for %d out of %d projects", failedCount, len(projectsToRefresh)).Err()
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
		return errors.Annotate(disableProject(ctx, project), "disable project %q", project).Err()
	case err != nil:
		return err
	case !isUpToDate(ctx, project, externalHash):
		return errors.Annotate(updateProject(ctx, project, externalHash), "update config for project %q", project).Err()
	default:
		return nil
	}
}

func getContentHashFromLUCIConfig(ctx context.Context, project string) (string, error) {
	var meta config.Meta
	switch err := cfgclient.Get(ctx, config.ProjectSet(project), configFileName, nil, &meta); {
	case err == config.ErrNoConfig:
		return "", err
	case err != nil:
		return "", errors.Annotate(err, "fetch meta from LUCI Config").Err()
	case meta.ContentHash == "":
		return "", errors.Reason("LUCI Config returns empty content hash for project %q", project).Err()
	default:
		// TODO(crbug/710619): Remove this additional check once the bug is fixed.
		// Checking whether `err` is ErrNoConfig would be sufficient.
		projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
		if err != nil {
			return "", errors.Annotate(err, "get projects that have file %q from LUCI Config", configFileName).Err()
		}
		for _, p := range projects {
			if p == project {
				return meta.ContentHash, nil
			}
		}
		return "", config.ErrNoConfig
	}
}

func isUpToDate(ctx context.Context, project, externalHash string) bool {
	pc := ProjectConfig{Project: project}
	if err := datastore.Get(ctx, &pc); err != nil {
		// Treat error as not up-to-date. We will check again while updating.
		return false
	}
	return pc.ExternalHash == externalHash
}

func updateProject(ctx context.Context, project, externalHash string) error {
	cfg := &pb.Config{}
	if err := fetchCfg(ctx, project, externalHash, cfg); err != nil {
		return err
	}
	localHash := computeHash(cfg)
	if err := putConfigGroups(ctx, cfg, project, localHash); err != nil {
		return err
	}
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		pc := ProjectConfig{Project: project}
		// Get again instead of reusing previous get result because ProjectConfig
		// may be updated by a concurrent request while writing the ConfigGroups.
		switch err := datastore.Get(ctx, &pc); {
		case err == nil && pc.ExternalHash == externalHash:
			return nil // up-to-date
		case err == nil || err == datastore.ErrNoSuchEntity:
			eVersion := pc.EVersion + 1
			configGroups := cfg.GetConfigGroups()
			now := datastore.RoundTime(clock.Now(ctx))
			pc = ProjectConfig{
				Project:          project,
				Enabled:          true,
				UpdatedTime:      now,
				EVersion:         eVersion,
				Hash:             localHash,
				ExternalHash:     externalHash,
				ConfigGroupNames: make([]string, len(configGroups)),
			}
			for i, cg := range configGroups {
				pc.ConfigGroupNames[i] = makeConfigGroupName(cg.GetName(), i)
			}
			hashInfo := ConfigHashInfo{
				Hash:       localHash,
				Project:    datastore.MakeKey(ctx, projectConfigKind, project),
				EVersion:   eVersion,
				ImportTime: now,
			}
			if err := datastore.Put(ctx, &pc, &hashInfo); err != nil {
				return errors.Annotate(err, "put ProjectConfig").Err()
			}
			// TODO(yiwzhang): Notify ProjectManager
			return nil
		default:
			return errors.Annotate(err, "get existing ProjectConfig").Err()
		}
	}, nil)
}

func fetchCfg(ctx context.Context, project string, contentHash string, cfg *pb.Config) error {
	content, err := cfgclient.Client(ctx).GetConfigByHash(ctx, contentHash)
	if err != nil {
		return errors.Annotate(err, "get config by content hash").Err()
	}
	if err := cfgclient.ProtoText(cfg)(content); err != nil {
		return errors.Annotate(err, "deserialize config content").Err()
	}
	return nil
}

func disableProject(ctx context.Context, project string) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		pc := ProjectConfig{Project: project}
		switch err := datastore.Get(ctx, &pc); {
		case datastore.IsErrNoSuchEntity(err):
			return nil // No-op when disable non-existent Project
		case err != nil:
			return errors.Annotate(err, "get existing ProjectConfig").Err()
		}
		pc.Enabled = false
		pc.UpdatedTime = datastore.RoundTime(clock.Now(ctx))
		pc.EVersion++
		if err := datastore.Put(ctx, &pc); err != nil {
			return errors.Annotate(err, "put ProjectConfig").Err()
		}
		// TODO(yiwzhang): Notify ProjectManager
		return nil
	}, nil)
}
