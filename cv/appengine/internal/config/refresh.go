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
		RoutingPrefix: "internal/tasks/config/refresh-project-config",
		Handler: func(ctx context.Context, payload proto.Message) error {
			project := payload.(*RefreshProjectConfigTask).ProjectId
			return RefreshConfig(ctx, project)
		},
	})
}

// SubmitRefreshTasks submits task that refreshes config for each LUCI project.
//
// This includes all Projects that has `configFileName` file in LUCI Config and
// all currently enabled Projects in CV.
func SubmitRefreshTasks(ctx context.Context) error {
	projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
	if err != nil {
		return errors.Annotate(err, "failed to get projects have file %q from LUCI Config", configFileName).Tag(transient.Tag).Err()
	}
	projectsToRefresh := stringset.NewFromSlice(projects...)
	projects, err = getAllProjectIDs(ctx, true)
	if err != nil {
		return errors.Annotate(err, "failed to get existing enabled projects").Tag(transient.Tag).Err()
	}
	projectsToRefresh.AddAll(projects)

	var failedProjects []string
	projectsToRefresh.Iter(func(project string) bool {
		tq.AddTask(ctx, &tq.Task{
			Payload: &RefreshProjectConfigTask{
				ProjectId: project,
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

// RefreshConfig imports the latest Change Verifier config for a given LUCI
// Project from LUCI Config.
//
// Upon successful import, ProjectManager will be notified if this Project is
// newly-discovered of a new config revision is imported. If this Project can't
// be found in LUCI Config, Project will be set to disabled state and
// ProjectManager will also be notified.
func RefreshConfig(ctx context.Context, project string) error {
	var (
		cfg  pb.Config
		meta config.Meta
	)
	getConfigErr := cfgclient.Get(ctx, config.ProjectSet(project), configFileName, cfgclient.ProtoText(&cfg), &meta)

	// TODO(crbug/710619): Remove the isStale check once bug is fixed. Checking
	// whether the `getConfigErr` is ErrNoConfig will be sufficient.
	isStaleConfig, err := isStaleConfig(ctx, project, getConfigErr)
	if err != nil {
		return err
	}

	switch revision, gitSHA := meta.ContentHash, meta.Revision; {
	case getConfigErr != nil && !isStaleConfig:
		err := datastore.RunInTransaction(ctx, func(c context.Context) error {
			return conditionallyUpdateConfig(c, project, revision, gitSHA, &cfg)
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

func conditionallyUpdateConfig(ctx context.Context, project, revision, gitSHA string, cfg *pb.Config) error {
	pc := ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, &pc); {
	case err == nil && pc.LatestRevision == revision && pc.SchemaVersion == currentSchemaVersion:
		return nil
	case err == nil || datastore.IsErrNoSuchEntity(err):
		if err := putConfigGroups(ctx, project, revision, cfg); err != nil {
			return errors.Annotate(err, "failed to put ProjectConfig for project %q", project).Err()
		}
		if err := putProjectConfig(ctx, project, revision, gitSHA, true, cfg); err != nil {
			return err
		}
		// TODO(yiwzhang): Notify ProjectManager
		return nil
	default:
		return errors.Annotate(err, "failed to get ProjectConfig for project %q", project).Err()
	}
}

func disableProject(ctx context.Context, project string) error {
	pc := ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, &pc); {
	case datastore.IsErrNoSuchEntity(err):
		// No-op when disable non-existing Project
		return nil
	case err != nil:
		return errors.Annotate(err, "failed to get ProjectConfig for project %q", project).Err()
	}
	if err := putProjectConfig(ctx, project, pc.LatestRevision, pc.GitSHA, false, nil); err != nil {
		return errors.Annotate(err, "failed to put ProjectConfig for project %q", project).Err()
	}
	// TODO(yiwzhang): Notify ProjectManager
	return nil
}
