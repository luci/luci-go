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

package prjcfg

import (
	"context"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/cv/api/config/v2"
)

const ConfigFileName = "commit-queue.cfg"

// ProjectsWithConfig returns all LUCI projects which have CV config.
func ProjectsWithConfig(ctx context.Context) ([]string, error) {
	projects, err := cfgclient.ProjectsWithConfig(ctx, ConfigFileName)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get projects with %q from LUCI Config",
			ConfigFileName).Tag(transient.Tag).Err()
	}
	return projects, nil
}

// NotifyCallback is called in a transaction context from UpdateProject and
// DisableProject. Used by configcron package.
type NotifyCallback func(context.Context) error

// UpdateProject imports the latest CV Config for a given LUCI Project
// from LUCI Config if the config in CV is outdated.
func UpdateProject(ctx context.Context, project string, notify NotifyCallback) error {
	meta, err := getConfigMeta(ctx, project)
	if err != nil {
		return err
	}
	existingPC := ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, &existingPC); {
	case err != nil && err != datastore.ErrNoSuchEntity:
		return errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
	case !existingPC.Enabled:
		// Go through update process to ensure all configs are present.
	case existingPC.ExternalHash == meta.ContentHash:
		return nil // Already up-to-date.
	}

	cfg, meta, err := fetchCfg(ctx, project)
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
			hashInfo.GitRevision = meta.Revision
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
				ExternalHash:     meta.ContentHash,
				ConfigGroupNames: cgNames,
			}
			updated = true
			if err := datastore.Put(ctx, &pc); err != nil {
				return errors.Annotate(err, "failed to put ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
			}
			return notify(ctx)
		}
	}, nil)

	switch {
	case err != nil:
		return errors.Annotate(err, "failed to run transaction to update ProjectConfig").Tag(transient.Tag).Err()
	case updated:
		logging.Infof(ctx, "updated project %q to rev %s hash %s ", project, meta.Revision, localHash)
	}
	return nil
}

// getConfigMeta fetches the Meta for a project config.
//
// Returns an error in the case of an empty content hash or fetch failure.
func getConfigMeta(ctx context.Context, project string) (*config.Meta, error) {
	var meta config.Meta
	switch err := cfgclient.Get(ctx, config.ProjectSet(project), ConfigFileName, nil, &meta); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to fetch meta from LUCI Config").Tag(transient.Tag).Err()
	case meta.ContentHash == "":
		return nil, errors.Reason("LUCI Config returns empty content hash for project %q", project).Err()
	default:
		return &meta, nil
	}
}

// fetchCfg a project config contents from luci-config.
func fetchCfg(ctx context.Context, project string) (*pb.Config, *config.Meta, error) {
	meta := &config.Meta{}
	ret := &pb.Config{}
	err := cfgclient.Get(
		ctx, config.ProjectSet(project),
		ConfigFileName,
		cfgclient.ProtoText(ret),
		meta,
	)
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to get the project config").Err()
	}
	// TODO(yiwzhang): validate the config here again to prevent ingesting a
	// bad version of config that accidentally slips into LUCI Config.
	// See: go.chromium.org/luci/cq/appengine/config
	return ret, meta, nil
}

// DisableProject disables the given LUCI Project if it is currently enabled.
func DisableProject(ctx context.Context, project string, notify NotifyCallback) error {
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
		return notify(ctx)
	}, nil)

	switch {
	case err != nil:
		return errors.Annotate(err, "failed to run transaction to disable project %q", project).Tag(transient.Tag).Err()
	case disabled:
		logging.Infof(ctx, "disabled project %q", project)
	}
	return nil
}
