// Copyright 2023 The LUCI Authors.
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
	"regexp"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"

	configpb "go.chromium.org/luci/bisection/proto/config"
)

var projectCacheSlot = caching.RegisterCacheSlot()

const projectConfigKind = "luci.bisection.ProjectConfig"

// ProjectCacheExpiry defines how often project configuration stored
// in the in-process cache is refreshed from datastore.
const ProjectCacheExpiry = 1 * time.Minute

// ChromiumMilestoneProjectRe is the chromium milestone projects, e.g. chromium-m100.
var ChromiumMilestoneProjectRe = regexp.MustCompile(`^(chrome|chromium)-m[0-9]+$`)

var (
	importAttemptCounter = metric.NewCounter(
		"bisection/project_config/import_attempt",
		"The number of import attempts of project config",
		nil,
		// status can be "success" or "failure".
		field.String("project"), field.String("status"))
)

var (
	// Returned if configuration for a given project does not exist.
	ErrNotFoundProjectConfig = fmt.Errorf("no project config found")
)

type cachedProjectConfig struct {
	_extra datastore.PropertyMap `gae:"-,extra"`
	_kind  string                `gae:"$kind,luci.bisection.ProjectConfig"`

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

// UpdateProjects fetches fresh project-level configuration from LUCI Config
// service and stores it in datastore.
func UpdateProjects(ctx context.Context) error {
	// Fetch freshest configs from the LUCI Config.
	fetchedConfigs, err := fetchLatestProjectConfigs(ctx)
	if err != nil {
		return err
	}

	var errs []error
	parsedConfigs := make(map[string]*fetchedProjectConfig)
	for project, fetch := range fetchedConfigs {
		// We don't support milestone projects, skipping them.
		if IsMilestoneProject(project) {
			continue
		}

		valCtx := validation.Context{Context: ctx}
		valCtx.SetFile(fetch.Path)
		msg := validateProjectConfigRaw(&valCtx, project, fetch.Content)
		if err := valCtx.Finalize(); err != nil {
			blocking := err.(*validation.Error).WithSeverity(validation.Blocking)
			if blocking != nil {
				// Continue through validation errors to ensure a validation
				// error in one project does not affect other projects.
				errs = append(errs, errors.Annotate(blocking, "validation errors for %q", project).Err())
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
// contains the new configs to store, forceUpdate forces overwrite of existing
// configuration (ignoring whether the config revision is newer).
func updateStoredConfig(ctx context.Context, fetchedConfigs map[string]*fetchedProjectConfig, forceUpdate bool) error {
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
		blob, err := proto.Marshal(fetch.Config)
		if err != nil {
			// Continue through errors to ensure bad config for one project
			// does not affect others.
			errs = append(errs, errors.Annotate(err, "marshal fetched config for project %s", project).Err())
			continue
		}
		cur, ok := currentConfigs[project]
		if !ok {
			cur = &cachedProjectConfig{
				ID: project,
			}
		}
		if !forceUpdate && cur.Meta.Revision == fetch.Meta.Revision {
			logging.Infof(ctx, "Cached config %s %s is up-to-date at rev %q", project, cur.ID, cur.Meta.Revision)
			continue
		}
		logging.Infof(ctx, "Updating cached config %s %s: %q -> %q", project, cur.ID, cur.Meta.Revision, fetch.Meta.Revision)
		toPut = append(toPut, &cachedProjectConfig{
			ID:     cur.ID,
			Config: blob,
			Meta:   fetch.Meta,
		})
	}
	if err := datastore.Put(ctx, toPut); err != nil {
		errs = append(errs, errors.Annotate(err, "updating project configs").Err())
	}

	var toDelete []*datastore.Key
	for project, cur := range currentConfigs {
		if _, ok := fetchedConfigs[project]; ok {
			continue
		}
		toDelete = append(toDelete, datastore.KeyForObj(ctx, cur))
	}

	if err := datastore.Delete(ctx, toDelete); err != nil {
		errs = append(errs, errors.Annotate(err, "deleting stale project configs").Err())
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
		return nil, errors.Annotate(err, "fetching project configs from datastore").Err()
	}
	result := make(map[string]*cachedProjectConfig)
	for _, cfg := range configs {
		result[cfg.ID] = cfg
	}
	return result, nil
}

// Projects returns all project configurations, in a map by project name.
// Uses in-memory cache to avoid hitting datastore all the time.
// Note that the config may be stale by up to 1 minute.
func Projects(ctx context.Context) (map[string]*configpb.ProjectConfig, error) {
	val, err := projectCacheSlot.Fetch(ctx, func(any) (val any, exp time.Duration, err error) {
		var pc map[string]*configpb.ProjectConfig
		if pc, err = fetchProjects(ctx); err != nil {
			return nil, 0, err
		}
		return pc, time.Minute, nil
	})
	switch {
	case errors.Is(err, caching.ErrNoProcessCache):
		// A fallback useful in unit tests that may not have the process cache
		// available. Production environments usually have the cache installed
		// by the framework code that initializes the root context.
		return fetchProjects(ctx)
	case err != nil:
		return nil, err
	default:
		pc := val.(map[string]*configpb.ProjectConfig)
		return pc, nil
	}
}

// fetchProjects retrieves all project configurations from datastore.
func fetchProjects(ctx context.Context) (map[string]*configpb.ProjectConfig, error) {
	ctx = cleanContext(ctx)

	cachedCfgs, err := fetchProjectConfigEntities(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "fetching cached config").Err()
	}
	result := make(map[string]*configpb.ProjectConfig)
	for project, cached := range cachedCfgs {
		cfg := &configpb.ProjectConfig{}
		if err := proto.Unmarshal(cached.Config, cfg); err != nil {
			return nil, errors.Annotate(err, "unmarshalling cached config for project %s", project).Err()
		}
		result[project] = cfg
	}
	return result, nil
}

// cleanContext returns a context with datastore and not using transactions.
func cleanContext(ctx context.Context) context.Context {
	return datastore.WithoutTransaction(ctx)
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
	forceUpdate := true
	if err := updateStoredConfig(ctx, fetchedConfigs, forceUpdate); err != nil {
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

// Project returns the configurations of the requested project.
// Returns an ErrNotFoundProjectConfig error if config for the given project
// does not exist.
func Project(ctx context.Context, project string) (*configpb.ProjectConfig, error) {
	configs, err := Projects(ctx)
	if err != nil {
		return nil, err
	}
	if c, ok := configs[project]; ok {
		return c, nil
	}
	return nil, errors.Annotate(ErrNotFoundProjectConfig, "%s", project).Err()
}

func IsMilestoneProject(project string) bool {
	return ChromiumMilestoneProjectRe.MatchString(project)
}

// SupportedProjects returns the list of projects supported by LUCI Bisection.
func SupportedProjects(ctx context.Context) ([]string, error) {
	configs, err := Projects(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(configs))
	for project := range configs {
		result = append(result, project)
	}
	sort.Strings(result)
	return result, nil
}
