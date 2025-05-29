// Copyright 2022 The LUCI Authors.
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
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/caching"

	configpb "go.chromium.org/luci/analysis/proto/config"
)

// LRU cache, of which only one slot is used (config for all projects
// is stored in the same slot). We use LRU cache instead of cache slot
// as we sometimes want to refresh config before it has expired.
// Only the LRU Cache has the methods to do this.
var projectsCache = caching.RegisterLRUCache[string, ProjectConfigs](1)

const projectConfigKind = "luci.analysis.ProjectConfig"

// ProjectCacheExpiry defines how often project configuration stored
// in the in-process cache is refreshed from datastore.
const ProjectCacheExpiry = 1 * time.Minute

// StartingEpoch is the earliest valid config version for a project.
// It is deliberately different from the timestamp zero value to be
// discernible from "timestamp not populated" programming errors.
var StartingEpoch = time.Date(1900, time.January, 1, 0, 0, 0, 0, time.UTC)

var (
	importAttemptCounter = metric.NewCounter(
		"analysis/project_config/import_attempt",
		"The number of import attempts of project config",
		nil,
		// status can be "success" or "failure".
		field.String("project"), field.String("status"))
)

type cachedProjectConfig struct {
	_extra datastore.PropertyMap `gae:"-,extra"`
	_kind  string                `gae:"$kind,luci.analysis.ProjectConfig"`

	ID     string      `gae:"$id"` // The name of the project for which the config is.
	Config []byte      `gae:",noindex"`
	Meta   config.Meta `gae:",noindex"`
}

func init() {
	// Registers validation of the given configuration paths with cfgmodule.
	validation.Rules.Add("regex:projects/.*", "${appid}.cfg", func(ctx *validation.Context, configSet, path string, content []byte) error {
		project := config.Set(configSet).Project()
		// Discard the returned deserialized message.
		validateProjectConfigRaw(ctx, project, string(content))
		return nil
	})
}

// updateProjects fetches fresh project-level configuration from LUCI Config
// service and stores it in datastore.
func updateProjects(ctx context.Context) error {
	// Fetch freshest configs from the LUCI Config.
	fetchedConfigs, err := fetchLatestProjectConfigs(ctx)
	if err != nil {
		return err
	}

	var errs []error
	parsedConfigs := make(map[string]*fetchedProjectConfig)
	for project, fetch := range fetchedConfigs {
		valCtx := validation.Context{Context: ctx}
		valCtx.SetFile(fetch.Path)
		msg := validateProjectConfigRaw(&valCtx, project, fetch.Content)
		if err := valCtx.Finalize(); err != nil {
			blocking := err.(*validation.Error).WithSeverity(validation.Blocking)
			if blocking != nil {
				// Continue through validation errors to ensure a validation
				// error in one project does not affect other projects.
				errs = append(errs, errors.Fmt("validation errors for %q: %w", project, blocking))
				msg = nil
			}
		}
		// We create an entry even for invalid config (where msg == nil),
		// because we want to signal that config for this project still exists
		// and existing config should be retained instead of being deleted.
		parsedConfigs[project] = &fetchedProjectConfig{
			Config: msg,
			Meta:   fetch.Meta,
		}
	}
	forceUpdate := false
	success := true
	if err := updateStoredConfig(ctx, parsedConfigs, forceUpdate); err != nil {
		errs = append(errs, err)
		success = false
	}
	// Report success for all projects that passed validation, assuming the
	// update succeeded.
	for project, config := range parsedConfigs {
		status := "success"
		if !success || config.Config == nil {
			status = "failure"
		}
		importAttemptCounter.Add(ctx, 1, project, status)
	}

	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

type fetchedProjectConfig struct {
	// config is the project-level configuration, if it has passed validation,
	// and nil otherwise.
	Config *configpb.ProjectConfig
	// meta is populated with config metadata.
	Meta config.Meta
}

// updateStoredConfig updates the config stored in datastore. fetchedConfigs
// contains the new configs to store, setForTesting forces overwrite of existing
// configuration (ignoring whether the config revision is newer).
func updateStoredConfig(ctx context.Context, fetchedConfigs map[string]*fetchedProjectConfig, setForTesting bool) error {
	// Drop out of any existing datastore transactions.
	ctx = cleanContext(ctx)

	currentConfigs, err := fetchProjectConfigEntities(ctx)
	if err != nil {
		return err
	}

	var errs []error
	var toPut []*cachedProjectConfig
	for project, fetch := range fetchedConfigs {
		if fetch.Config == nil {
			// Config did not pass validation.
			continue
		}
		cur, ok := currentConfigs[project]
		if !ok {
			cur = &cachedProjectConfig{
				ID: project,
			}
		}
		if !setForTesting && cur.Meta.Revision == fetch.Meta.Revision {
			logging.Infof(ctx, "Cached config %s is up-to-date at rev %q", cur.ID, cur.Meta.Revision)
			continue
		}
		configToSave := proto.Clone(fetch.Config).(*configpb.ProjectConfig)
		if !setForTesting {
			var lastUpdated time.Time
			if cur.Config != nil {
				cfg := &configpb.ProjectConfig{}
				if err := proto.Unmarshal(cur.Config, cfg); err != nil {
					// Continue through errors to ensure bad config for one project
					// does not affect others.
					errs = append(errs, errors.Fmt("unmarshal current config: %w", err))
					continue
				}
				lastUpdated = cfg.LastUpdated.AsTime()
			}
			// ContentHash updated implies Revision updated, but Revision updated
			// does not imply ContentHash updated. To avoid unnecessarily
			// incrementing the last updated time (which triggers re-clustering),
			// only update it if content has changed.
			if cur.Meta.ContentHash != fetch.Meta.ContentHash {
				// Content updated. Update version.
				now := clock.Now(ctx)
				if !now.After(lastUpdated) {
					errs = append(errs, errors.New("old config version is after current time"))
					continue
				}
				lastUpdated = now
			}
			configToSave.LastUpdated = timestamppb.New(lastUpdated)
		}
		// else: use LastUpdated time provided in SetTestProjectConfig call.

		blob, err := proto.Marshal(configToSave)
		if err != nil {
			// Continue through errors to ensure bad config for one project
			// does not affect others.
			errs = append(errs, errors.Fmt("marshal fetched config: %w", err))
			continue
		}
		logging.Infof(ctx, "Updating cached config %s: %q -> %q", cur.ID, cur.Meta.Revision, fetch.Meta.Revision)
		toPut = append(toPut, &cachedProjectConfig{
			ID:     cur.ID,
			Config: blob,
			Meta:   fetch.Meta,
		})
	}

	var toDelete []*datastore.Key
	for project, cur := range currentConfigs {
		if _, ok := fetchedConfigs[project]; ok {
			continue
		}
		logging.Infof(ctx, "Mark config deleted %s", cur.ID)
		if setForTesting {
			toDelete = append(toDelete, datastore.KeyForObj(ctx, cur))
			continue
		}
		// Config already marked as deleted in datastore.
		if cur.Meta.ContentHash == "" {
			continue
		}
		// For config that is deleted from LUCI config, we keep an entry in datastore with
		// an empty config except having lastUpdatedTime as the time of deletion.
		// This is to avoid a timestamp rollback situation.
		configToSave := &configpb.ProjectConfig{LastUpdated: timestamppb.New(clock.Now(ctx))}
		blob, err := proto.Marshal(configToSave)
		if err != nil {
			// Continue through errors to ensure bad config for one project
			// does not affect others.
			errs = append(errs, errors.Fmt("marshal deleted config: %w", err))
			continue
		}
		toPut = append(toPut, &cachedProjectConfig{
			ID:     cur.ID,
			Config: blob,
			Meta:   config.Meta{},
		})
	}
	if err := datastore.Delete(ctx, toDelete); err != nil {
		errs = append(errs, errors.Fmt("deleting stale project configs: %w", err))
	}
	if err := datastore.Put(ctx, toPut); err != nil {
		errs = append(errs, errors.Fmt("updating project configs: %w", err))
	}

	if len(errs) > 0 {
		return errors.NewMultiError(errs...)
	}
	return nil
}

func fetchLatestProjectConfigs(ctx context.Context) (map[string]config.Config, error) {
	configs, err := cfgclient.Client(ctx).GetProjectConfigs(ctx, "${appid}.cfg", false)
	if err != nil {
		return nil, err
	}
	result := make(map[string]config.Config)
	for _, cfg := range configs {
		project := cfg.ConfigSet.Project()
		if project != "" {
			result[project] = cfg
		}
	}
	return result, nil
}

// fetchProjectConfigEntities retrieves project configuration entities
// from datastore, including metadata.
func fetchProjectConfigEntities(ctx context.Context) (map[string]*cachedProjectConfig, error) {
	var configs []*cachedProjectConfig
	err := datastore.GetAll(ctx, datastore.NewQuery(projectConfigKind), &configs)
	if err != nil {
		return nil, errors.Fmt("fetching project configs from datastore: %w", err)
	}
	result := make(map[string]*cachedProjectConfig)
	for _, cfg := range configs {
		result[cfg.ID] = cfg
	}
	return result, nil
}

// ProjectConfigs represents the physcial to logical content translation of project configs.
type ProjectConfigs struct {
	// Configs physically present in datastore.
	contents map[string]*configpb.ProjectConfig
}

// Project returns the logical project config.
func (p *ProjectConfigs) Project(project string) *configpb.ProjectConfig {
	if cfg, ok := p.contents[project]; ok {
		return cfg
	}
	// Return project if it exists, otherwise the fake empty one with the LastUpdated = StartEpoch time set.
	return NewEmptyProject()
}

// Keys returns all projects that have a record in datastore.
// It can be a project that currently have a config or had a config before but then deleted.
func (p *ProjectConfigs) Keys() []string {
	keys := make([]string, 0, len(p.contents))
	for k := range p.contents {
		keys = append(keys, k)
	}
	return keys
}

// projectsWithMinimumVersion retrieves projects configurations, with
// the specified project at at least the specified minimumVersion.
// If no particular minimum version is desired, specify a project of ""
// or a minimumVersion of time.Time{}.
func projectsWithMinimumVersion(ctx context.Context, project string, minimumVersion time.Time) (ProjectConfigs, error) {
	var pc ProjectConfigs
	var err error
	cache := projectsCache.LRU(ctx)
	if cache == nil {
		// A fallback useful in unit tests that may not have the process cache
		// available. Production environments usually have the cache installed
		// by the framework code that initializes the root context.
		pc, err = fetchProjects(ctx)
		if err != nil {
			return ProjectConfigs{}, err
		}
	} else {
		value, _ := projectsCache.LRU(ctx).Mutate(ctx, "projects", func(it *lru.Item[ProjectConfigs]) *lru.Item[ProjectConfigs] {
			var pc ProjectConfigs
			if it != nil {
				pc = it.Value
				projectCfg := pc.Project(project)
				if project == "" || (!projectCfg.LastUpdated.AsTime().Before(minimumVersion)) {
					// Projects contains the specified project at the given minimum version.
					// There is no need to update it.
					return it
				}
			}
			if pc, err = fetchProjects(ctx); err != nil {
				// Error refreshing config. Keep existing entry (if any).
				return it
			}
			return &lru.Item[ProjectConfigs]{
				Value: pc,
				Exp:   ProjectCacheExpiry,
			}
		})
		if err != nil {
			return ProjectConfigs{}, err
		}
		pc = value
	}

	projectCfg := pc.Project(project)
	if project != "" && (projectCfg.LastUpdated.AsTime().Before(minimumVersion)) {
		return ProjectConfigs{}, fmt.Errorf("could not obtain projects configuration with project %s at minimum version (%v)", project, minimumVersion)
	}
	return pc, nil
}

// Projects returns all project configurations, in a map by project name.
// Uses in-memory cache to avoid hitting datastore all the time.
func Projects(ctx context.Context) (ProjectConfigs, error) {
	return projectsWithMinimumVersion(ctx, "", time.Time{})
}

// fetchProjects retrieves all project configurations from datastore.
func fetchProjects(ctx context.Context) (ProjectConfigs, error) {
	ctx = cleanContext(ctx)

	cachedCfgs, err := fetchProjectConfigEntities(ctx)
	if err != nil {
		return ProjectConfigs{}, errors.Fmt("fetching cached config: %w", err)
	}
	result := make(map[string]*configpb.ProjectConfig)
	for project, cached := range cachedCfgs {
		cfg := &configpb.ProjectConfig{}
		if err := proto.Unmarshal(cached.Config, cfg); err != nil {
			return ProjectConfigs{}, errors.Fmt("unmarshalling cached config: %w", err)
		}
		result[project] = cfg
	}
	return ProjectConfigs{contents: result}, nil
}

// cleanContext returns a context with datastore using the default namespace
// and not using transactions.
func cleanContext(ctx context.Context) context.Context {
	return datastore.WithoutTransaction(info.MustNamespace(ctx, ""))
}

// SetTestProjectConfig sets test project configuration in datastore.
// It should be used from unit/integration tests only.
func SetTestProjectConfig(ctx context.Context, cfg map[string]*configpb.ProjectConfig) error {
	fetchedConfigs := make(map[string]*fetchedProjectConfig)
	for project, pcfg := range cfg {
		fetchedConfigs[project] = &fetchedProjectConfig{
			Config: pcfg,
			Meta:   config.Meta{},
		}
	}
	setForTesting := true
	if err := updateStoredConfig(ctx, fetchedConfigs, setForTesting); err != nil {
		return err
	}
	testable := datastore.GetTestable(ctx)
	if testable == nil {
		return errors.New("SetTestProjectConfig should only be used with testable datastore implementations")
	}
	// An up-to-date index is required for fetch to retrieve the project
	// entities we just saved.
	testable.CatchupIndexes()
	return nil
}

// Project returns the configuration of the requested project.
func Project(ctx context.Context, project string) (*configpb.ProjectConfig, error) {
	return ProjectWithMinimumVersion(ctx, project, time.Time{})
}

// ProjectWithMinimumVersion returns the configuration of the requested
// project, which has a LastUpdated time of at least minimumVersion.
// This bypasses the in-process cache if the cached version is older
// than the specified version.
func ProjectWithMinimumVersion(ctx context.Context, project string, minimumVersion time.Time) (*configpb.ProjectConfig, error) {
	configs, err := projectsWithMinimumVersion(ctx, project, minimumVersion)
	if err != nil {
		return nil, err
	}
	return configs.Project(project), nil
}

// NewEmptyProject returnes an empty project config.
func NewEmptyProject() *configpb.ProjectConfig {
	return &configpb.ProjectConfig{
		LastUpdated: timestamppb.New(StartingEpoch),
	}
}
