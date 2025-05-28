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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	lucivalidation "go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/validation"
)

// projectsWithConfig returns all LUCI projects which have CV config.
func projectsWithConfig(ctx context.Context, configFileName string) ([]string, error) {
	projects, err := cfgclient.ProjectsWithConfig(ctx, configFileName)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get projects with %q from LUCI Config",
			configFileName).Tag(transient.Tag).Err()
	}
	return projects, nil
}

// NotifyCallback is called in a transaction context from UpdateProject and
// DisableProject. Used by configcron package.
type NotifyCallback func(context.Context) error

// UpdateProject imports the latest CV Config for a given LUCI Project
// from LUCI Config if the config in CV is outdated.
func UpdateProject(ctx context.Context, project string, notify NotifyCallback) error {
	configFileName := prjcfg.ConfigFileName(ctx)
	need, existingPC, err := needsUpdate(ctx, project, configFileName)
	switch {
	case err != nil:
		return err
	case !need:
		return nil
	}

	cfg, meta, err := fetchCfg(ctx, project, configFileName)
	if err != nil {
		return err
	}
	vctx := &lucivalidation.Context{Context: ctx}
	if err := validation.ValidateProject(vctx, cfg, project); err != nil {
		return errors.Annotate(err, "ValidateProject").Err()
	}
	if verr := vctx.Finalize(); verr != nil {
		logging.Errorf(ctx, "UpdateProject %q on invalid config: %s", project, verr)
	}

	// Write out ConfigHashInfo if missing and all ConfigGroups.
	localHash, err := prjcfg.ComputeHash(cfg)
	if err != nil {
		return err
	}
	cgNames := make([]string, len(cfg.GetConfigGroups()))
	for i, cg := range cfg.GetConfigGroups() {
		cgNames[i] = cg.GetName()
	}
	targetEVersion := existingPC.EVersion + 1

	err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		hashInfo := prjcfg.ConfigHashInfo{
			Hash:    localHash,
			Project: prjcfg.ProjectConfigKey(ctx, project),
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
			hashInfo.SchemaVersion = prjcfg.SchemaVersion
			return transient.Tag.Apply(errors.WrapIf(datastore.Put(ctx, &hashInfo), "failed to put ConfigHashInfo(Hash=%q)", localHash))
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
		pc := prjcfg.ProjectConfig{Project: project}
		switch err := datastore.Get(ctx, &pc); {
		case err != nil && err != datastore.ErrNoSuchEntity:
			return errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
		case pc.EVersion != existingPC.EVersion:
			return nil // Already updated by concurrent updateProject.
		default:
			pc = prjcfg.ProjectConfig{
				Project:          project,
				Enabled:          true,
				UpdateTime:       datastore.RoundTime(clock.Now(ctx)).UTC(),
				EVersion:         targetEVersion,
				Hash:             localHash,
				ExternalHash:     meta.ContentHash,
				ConfigGroupNames: cgNames,
				SchemaVersion:    prjcfg.SchemaVersion,
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

// needsUpdate checks if there is a new config version.
//
// Loads and returns the ProjectConfig stored in Datastore.
func needsUpdate(ctx context.Context, project string, configFileName string) (bool, prjcfg.ProjectConfig, error) {
	pc := prjcfg.ProjectConfig{Project: project}
	var meta config.Meta
	// NOTE: config metadata fetched here can't be used later to fetch actual
	// contents (see https://crrev.com/c/3050832), so it is only used
	// to check if fetching config contents is even necessary.
	ps, err := config.ProjectSet(project)
	if err != nil {
		return false, pc, err
	}
	switch err := cfgclient.Get(ctx, ps, configFileName, nil, &meta); {
	case err != nil:
		return false, pc, errors.Annotate(err, "failed to fetch meta from LUCI Config").Tag(transient.Tag).Err()
	case meta.ContentHash == "":
		return false, pc, errors.Reason("LUCI Config returns empty content hash for project %q", project).Err()
	}

	switch err := datastore.Get(ctx, &pc); {
	case err == datastore.ErrNoSuchEntity:
		// ProjectConfig's zero value is a good sentinel for non yet saved case.
		return true, pc, nil
	case err != nil:
		return false, pc, errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
	case !pc.Enabled:
		// Go through update process to ensure all configs are present.
		return true, pc, nil
	case pc.ExternalHash != meta.ContentHash:
		return true, pc, nil
	case pc.SchemaVersion != prjcfg.SchemaVersion:
		// Intentionally using != here s.t. rollbacks result in downgrading of the
		// schema. Given that project configs are checked and potentially updated
		// every ~1 minute, this if OK.
		return true, pc, nil
	default:
		// Already up-to-date.
		return false, pc, nil
	}
}

// fetchCfg a project config contents from luci-config.
func fetchCfg(ctx context.Context, project string, configFileName string) (*cfgpb.Config, *config.Meta, error) {
	meta := &config.Meta{}
	ret := &cfgpb.Config{}
	ps, err := config.ProjectSet(project)
	if err != nil {
		return nil, nil, err
	}
	err = cfgclient.Get(
		ctx,
		ps,
		configFileName,
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
		pc := prjcfg.ProjectConfig{Project: project}
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

// putConfigGroups puts the ConfigGroups in the given CV config to datastore.
//
// It checks for existence of each ConfigGroup first to avoid unnecessary puts.
// It is also idempotent so it is safe to retry and can be called out of a
// transactional context.
func putConfigGroups(ctx context.Context, cfg *cfgpb.Config, project, hash string) error {
	cgLen := len(cfg.GetConfigGroups())
	if cgLen == 0 {
		return nil
	}

	// Check if there are any existing entities with the current schema version
	// such that we can skip updating them.
	projKey := prjcfg.ProjectConfigKey(ctx, project)
	entities := make([]*prjcfg.ConfigGroup, cgLen)
	for i, cg := range cfg.GetConfigGroups() {
		entities[i] = &prjcfg.ConfigGroup{
			ID:      prjcfg.MakeConfigGroupID(hash, cg.GetName()),
			Project: projKey,
		}
	}
	err := datastore.Get(ctx, entities)
	errs, ok := err.(errors.MultiError)
	switch {
	case err != nil && !ok:
		return errors.Annotate(err, "failed to check the existence of ConfigGroups").Tag(transient.Tag).Err()
	case err == nil:
		errs = make(errors.MultiError, cgLen)
	}
	toPut := entities[:0] // re-use the slice
	for i, err := range errs {
		ent := entities[i]
		switch {
		case err == datastore.ErrNoSuchEntity:
			// proceed to put below.
		case err != nil:
			return errors.Annotate(err, "failed to check the existence of one of ConfigGroups").Tag(transient.Tag).Err()
		case ent.SchemaVersion != prjcfg.SchemaVersion:
			// Intentionally using != here s.t. rollbacks result in downgrading
			// of the schema. Given that project configs are checked and
			// potentially updated every ~1 minute, this if OK.
		default:
			continue // up to date
		}
		ent.SchemaVersion = prjcfg.SchemaVersion
		ent.DrainingStartTime = cfg.GetDrainingStartTime()
		ent.SubmitOptions = cfg.GetSubmitOptions()
		ent.Content = cfg.GetConfigGroups()[i]
		ent.HonorGerritLinkedAccounts = cfg.GetHonorGerritLinkedAccounts()
		ent.CQStatusHost = cfg.GetCqStatusHost()
		toPut = append(toPut, ent)
	}

	if err := datastore.Put(ctx, toPut); err != nil {
		return errors.Annotate(err, "failed to put ConfigGroups").Tag(transient.Tag).Err()
	}
	return nil
}
