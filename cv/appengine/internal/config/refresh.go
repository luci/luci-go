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
	"sort"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/proto"

	pb "go.chromium.org/luci/cv/api/config/v2"
)

const configFileName = "commit-queue.cfg"

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:            "refresh-project-config",
		Prototype:     &RefreshProjectConfigTask{},
		Queue:         "refresh-project-config",
		RoutingPrefix: "/internal/tasks/config/refresh-project-config",
		Handler: func(ctx context.Context, payload proto.Message) error {
			project := payload.(*RefreshProjectConfigTask).Project
			return RefreshConfig(ctx, project)
		},
	})
}

// SubmitRefreshTasks submits tasks that refreshes config for LUCI projects.
//
// This includes all Projects that has CV config in LUCI Config and all
// currently enabled Projects in CV.
func SubmitRefreshTasks(ctx context.Context) error {
	projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
	if err != nil {
		return errors.Annotate(err, "failed to get projects have %q from LUCI Config", configFileName).Tag(transient.Tag).Err()
	}
	projectsToRefresh := stringset.NewFromSlice(projects...)
	projects, err = getAllProjectIDs(ctx, true)
	if err != nil {
		return errors.Annotate(err, "failed to get currently enabled projects in CV").Tag(transient.Tag).Err()
	}
	projectsToRefresh.AddAll(projects)

	var failedProjects []string
	projectsToRefresh.Iter(func(project string) bool {
		tq.AddTask(ctx, &tq.Task{
			Payload: &RefreshProjectConfigTask{
				Project: project,
			},
			DeduplicationKey: project,
		})
		if err != nil {
			logging.WithError(err).Errorf(ctx, "Failed to submit refresh task for project %q", project)
			failedProjects = append(failedProjects, project)
		}
		return true
	})
	if len(failedProjects) != 0 {
		sort.Strings(failedProjects)
		return errors.Reason("failed to submit refresh task for projects: %q", failedProjects).Err()
	}
	return nil
}

// RefreshConfig imports the latest CV config for a given LUCI Project from
// LUCI Config.
//
// If the given Project can't be found in LUCI Config or don't have CV config,
// this Project will be disabled in CV.
func RefreshConfig(ctx context.Context, project string) error {
	var (
		cfg  pb.Config
		meta config.Meta
	)
	getConfigErr := cfgclient.Get(ctx, config.ProjectSet(project), configFileName, cfgclient.ProtoText(&cfg), &meta)

	// TODO(crbug/710619): Remove the isStale check once the bug is fixed.
	// Checking whether `getConfigErr` is ErrNoConfig would be sufficient.
	isStaleConfig, err := isStaleConfig(ctx, project, getConfigErr)
	if err != nil {
		return err
	}

	switch {
	case getConfigErr == nil && !isStaleConfig:
		if meta.ContentHash == "" {
			return errors.Reason("LUCI Config returns empty hash when getting CV config of %q", project).Tag(transient.Tag).Err()
		}
		err := datastore.RunInTransaction(ctx, func(c context.Context) error {
			return updateConfig(c, project, meta.ContentHash, &cfg)
		}, nil)
		if err != nil {
			err = errors.Annotate(err, "failed to update config for project %q", project).Tag(transient.Tag).Err()
		}
		return err
	case getConfigErr == config.ErrNoConfig || isStaleConfig:
		err := datastore.RunInTransaction(ctx, func(c context.Context) error {
			return disableProject(c, project)
		}, nil)
		if err != nil {
			err = errors.Annotate(err, "failed to disable project %q", project).Tag(transient.Tag).Err()
		}
		return err
	default:
		return getConfigErr
	}
}

func isStaleConfig(ctx context.Context, project string, getConfigErr error) (bool, error) {
	if getConfigErr != nil {
		return false, nil
	}
	projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
	if err != nil {
		return false, errors.Annotate(err, "failed to get projects have file %q from LUCI Config", configFileName).Err()
	}
	for _, p := range projects {
		if p == project {
			return false, nil
		}
	}
	return true, nil
}

// updateConfig updates `ProjectConfig` and writes new `ConfigGroup`s if
// the given `configHash` is different from the current one.
func updateConfig(ctx context.Context, project, configHash string, cfg *pb.Config) error {
	pc := ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, &pc); {
	case err == nil && pc.ConfigHash == configHash:
		return nil
	case err == nil || datastore.IsErrNoSuchEntity(err):
		ids, err := writeConfigGroups(ctx, cfg, project, configHash, pc.Version+1)
		if err != nil {
			return errors.Annotate(err, "failed to write ConfigGroups for project %q", project).Err()
		}
		pc = ProjectConfig{
			Project:        project,
			Enabled:        true,
			ConfigHash:     configHash,
			Version:        pc.Version + 1,
			UpdatedTime:    clock.Now(ctx),
			ConfigGroupIDs: ids,
		}
		if err := datastore.Put(ctx, &pc); err != nil {
			return errors.Annotate(err, "failed to write ProjectConfig for project %q", project).Err()
		}
		// TODO(yiwzhang): Notify ProjectManager
		return nil
	default:
		return errors.Annotate(err, "failed to get existing ProjectConfig for project %q", project).Err()
	}
}

func disableProject(ctx context.Context, project string) error {
	pc := ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, &pc); {
	case datastore.IsErrNoSuchEntity(err):
		return nil // No-op when disable non-existent Project
	case err != nil:
		return errors.Annotate(err, "failed to existing get ProjectConfig for project %q", project).Err()
	}
	pc = ProjectConfig{
		Project:     project,
		Enabled:     false,
		Version:     pc.Version + 1,
		UpdatedTime: clock.Now(ctx),
	}
	if err := datastore.Put(ctx, &pc); err != nil {
		return errors.Annotate(err, "failed to write ProjectConfig for project %q", project).Err()
	}
	// TODO(yiwzhang): Notify ProjectManager
	return nil
}
