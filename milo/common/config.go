// Copyright 2016 The LUCI Authors.
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

package common

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/buildbucket/access"
	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	protoutil "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/retry/transient"
	configInterface "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/server/cfgclient"
	cfgclientAccess "go.chromium.org/luci/config/server/cfgclient/access"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/milo/git/gitacls"

	// Register "${appid}" placeholder for config validation rules.
	_ "go.chromium.org/luci/config/appengine/gaeconfig"
)

// Project is a datastore entity representing a single project.  Its children
// are consoles.
type Project struct {
	ID               string `gae:"$id"`
	LogoURL          string
	BuildBugTemplate config.BugTemplate
}

// Console is a datastore entity representing a single console.
type Console struct {
	// Parent is a key to the parent Project entity where this console was
	// defined in.
	Parent *datastore.Key `gae:"$parent"`

	// ID is the ID of the console.
	ID string `gae:"$id"`

	// Ordinal specifies the console's ordering in its project's consoles list.
	Ordinal int

	// The URL to the luci-config definition of this console.
	ConfigURL string

	// The luci-config reivision from when this Console was retrieved.
	ConfigRevision string `gae:",noindex"`

	// (indexed) All builder IDs mentioned by this console config.
	Builders []string

	// Def is the actual underlying proto Console definition.
	Def config.Console `gae:",noindex"`

	// _ is a "black hole" which absorbs any extra props found during a
	// datastore Get. These props are not written back on a datastore Put.
	_ datastore.PropertyMap `gae:"-,extra"`
}

func (c *Console) ConsoleID() ConsoleID {
	return ConsoleID{Project: c.ProjectID(), ID: c.ID}
}

// ProjectID retrieves the project ID string of the console out of the Console's
// parent key.
func (c *Console) ProjectID() string {
	if c.Parent == nil {
		return ""
	}
	return c.Parent.StringID()
}

// FilterBuilders uses an access.Permissions to filter out builder IDs and builders
// from the definition, and builders in the definition's header, which are not
// allowed by the permissions.
func (c *Console) FilterBuilders(perms access.Permissions) {
	okBuilderIDs := make([]string, 0, len(c.Builders))
	for _, id := range c.Builders {
		if bucket := extractBucket(id); bucket != "" && !perms.Can(bucket, access.AccessBucket) {
			continue
		}
		okBuilderIDs = append(okBuilderIDs, id)
	}
	c.Builders = okBuilderIDs
	okBuilders := make([]*config.Builder, 0, len(c.Def.Builders))
	// A single builder entry could have multiple builder names.
	for _, b := range c.Def.Builders {
		okNames := make([]string, 0, len(b.Name))
		for _, name := range b.Name {
			if bucket := extractBucket(name); bucket != "" && !perms.Can(bucket, access.AccessBucket) {
				continue
			}
			okNames = append(okNames, name)
		}
		b.Name = okNames
		if len(b.Name) > 0 {
			okBuilders = append(okBuilders, b)
		}
	}
	c.Def.Builders = okBuilders
}

// Buckets returns all buckets referenced by this Console's Builders.
func (c *Console) Buckets() stringset.Set {
	buckets := stringset.New(1)
	for _, id := range c.Builders {
		if bucket := extractBucket(id); bucket != "" {
			buckets.Add(bucket)
		}
	}
	return buckets
}

// extractBucket extracts bucket from a builder ID if possible.
//
// TODO(mknyszek): Get rid of this by either moving the logic above
// or somehow getting access to BuilderID otherwise without an import
// cycle.
func extractBucket(id string) string {
	if !strings.HasPrefix(id, "buildbucket/") {
		return ""
	}
	toks := strings.SplitN(id, "/", 3)
	if len(toks) != 3 {
		return ""
	}
	return toks[1]
}

// ConsoleID is a reference to a console.
type ConsoleID struct {
	Project string
	ID      string
}

func ParseConsoleID(id string) (cid ConsoleID, err error) {
	components := strings.Split(id, "/")
	if len(components) != 2 {
		err = errors.New("invalid console id: " + id)
		return
	}
	return ConsoleID{
		Project: components[0],
		ID:      components[1],
	}, nil
}

func (id *ConsoleID) String() string {
	return fmt.Sprintf("%s/%s", id.Project, id.ID)
}

// NewEntity returns an empty Console datastore entity keyed with itself.
func (id *ConsoleID) SetID(c context.Context, console *Console) *Console {
	if console == nil {
		console = &Console{}
	}
	console.Parent = datastore.MakeKey(c, "Project", id.Project)
	console.ID = id.ID
	return console
}

// ErrConsoleNotFound is returned from GetConsole if the requested console
// isn't known to exist.
var ErrConsoleNotFound = errors.New("console not found")

func init() {
	// We need to wait until error codes have generated tags before initializing this error
	ErrConsoleNotFound = errors.New("console not found", grpcutil.NotFoundTag)
}

// LuciConfigURL returns a user friendly URL that specifies where to view
// this console definition.
func LuciConfigURL(c context.Context, configSet, path, revision string) string {
	// TODO(hinoka): This shouldn't be hardcoded, instead we should get the
	// luci-config instance from the context.  But we only use this instance at
	// the moment so it is okay for now.
	// TODO(hinoka): The UI doesn't allow specifying paths and revision yet.  Add
	// that in when it is supported.
	return fmt.Sprintf("https://luci-config.appspot.com/newui#/%s", configSet)
}

// ServiceConfigID is the key for the service config entity in datastore.
const ServiceConfigID = "service_config"

// ServiceConfig is a container for the instance's service config.
type ServiceConfig struct {
	// ID is the datastore key.  This should be static, as there should only be
	// one service config.
	ID string `gae:"$id"`
	// Revision is the revision of the config, taken from luci-config.  This is used
	// to determine if the entry needs to be refreshed.
	Revision string
	// Data is the binary proto of the config.
	Data []byte `gae:",noindex"`
	// Text is the text format of the config.  For human consumption only.
	Text string `gae:",noindex"`
	// LastUpdated is the time this config was last updated.
	LastUpdated time.Time
}

// ReplaceNSEWith takes an errors.MultiError returned by a datastore.Get() on a slice
// (which is always a MultiError), filters out all datastore.ErrNoSuchEntitiy
// or replaces it with replacement instances, and returns an error generated
// by errors.LazyMultiError.
func ReplaceNSEWith(err errors.MultiError, replacement error) error {
	lme := errors.NewLazyMultiError(len(err))
	for i, ierr := range err {
		if ierr == datastore.ErrNoSuchEntity {
			ierr = replacement
		}
		lme.Assign(i, ierr)
	}
	return lme.Get()
}

// GetSettings returns the service (aka global) config for the current
// instance of Milo from the datastore.  Returns an empty config and warn heavily
// if none is found.
// TODO(hinoka): Use process cache to cache configs.
func GetSettings(c context.Context) *config.Settings {
	settings := config.Settings{
		Buildbot: &config.Settings_Buildbot{},
	}

	msg, err := GetCurrentServiceConfig(c)
	if err != nil {
		// The service config does not exist, just return an empty config
		// and complain loudly in the logs.
		logging.WithError(err).Errorf(c,
			"Encountered error while loading service config, using empty config.")
		return &settings
	}

	err = proto.Unmarshal(msg.Data, &settings)
	if err != nil {
		// The service config is broken, just return an empty config
		// and complain loudly in the logs.
		logging.WithError(err).Errorf(c,
			"Encountered error while unmarshalling service config, using empty config.")
		// Zero out the message just incase something got written in.
		settings = config.Settings{Buildbot: &config.Settings_Buildbot{}}
	}

	return &settings
}

var serviceCfgCache = caching.RegisterCacheSlot()

// GetCurrentServiceConfig gets the service config for the instance from either
// process cache or datastore cache.
func GetCurrentServiceConfig(c context.Context) (*ServiceConfig, error) {
	// This maker function is used to do the actual fetch of the ServiceConfig
	// from datastore.  It is called if the ServiceConfig is not in proc cache.
	item, err := serviceCfgCache.Fetch(c, func(interface{}) (interface{}, time.Duration, error) {
		msg := ServiceConfig{ID: ServiceConfigID}
		err := datastore.Get(c, &msg)
		if err != nil {
			return nil, time.Minute, err
		}
		logging.Infof(c, "loaded service config from datastore")
		return msg, time.Minute, nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get service config: %s", err.Error())
	}
	if msg, ok := item.(ServiceConfig); ok {
		logging.Infof(c, "loaded config entry from %s", msg.LastUpdated.Format(time.RFC3339))
		return &msg, nil
	}
	return nil, fmt.Errorf("could not load service config %#v", item)
}

const globalConfigFilename = "settings.cfg"

// UpdateServiceConfig fetches the service config from luci-config
// and then stores a snapshot of the configuration in datastore.
func UpdateServiceConfig(c context.Context) (*config.Settings, error) {
	// Load the settings from luci-config.
	cs := cfgclient.CurrentServiceConfigSet(c)
	// Acquire the raw config client.
	lucicfg := backend.Get(c).GetConfigInterface(c, backend.AsService)
	cfg, err := lucicfg.GetConfig(c, cs, globalConfigFilename, false)
	if err != nil {
		return nil, fmt.Errorf("could not load %s from luci-config: %s", globalConfigFilename, err)
	}
	settings := &config.Settings{}
	err = protoutil.UnmarshalTextML(cfg.Content, settings)
	if err != nil {
		return nil, fmt.Errorf(
			"could not unmarshal proto from luci-config:\n%s", cfg.Content)
	}
	newConfig := ServiceConfig{
		ID:          ServiceConfigID,
		Text:        cfg.Content,
		Revision:    cfg.Revision,
		LastUpdated: time.Now().UTC(),
	}
	newConfig.Data, err = proto.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("could not marshal proto into binary\n%s", newConfig.Text)
	}

	// Do the revision check & swap in a datastore transaction.
	err = datastore.RunInTransaction(c, func(c context.Context) error {
		oldConfig := ServiceConfig{ID: ServiceConfigID}
		err := datastore.Get(c, &oldConfig)
		switch err {
		case datastore.ErrNoSuchEntity:
			// Might be the first time this has run.
			logging.WithError(err).Warningf(c, "No existing service config.")
		case nil:
			// Continue
		default:
			return fmt.Errorf("could not load existing config: %s", err)
		}
		// Check to see if we need to update
		if oldConfig.Revision == newConfig.Revision {
			logging.Infof(c, "revisions matched (%s), no need to update", oldConfig.Revision)
			return nil
		}
		logging.Infof(c, "revisions differ (old %s, new %s), updating",
			oldConfig.Revision, newConfig.Revision)
		return datastore.Put(c, &newConfig)
	}, nil)

	if err != nil {
		return nil, errors.Annotate(err, "failed to update config entry in transaction").Err()
	}
	logging.Infof(c, "successfully updated to new config")

	return settings, nil
}

// updateProjectConsoles updates all of the consoles for a given project,
// and then returns a set of known console names.
func updateProjectConsoles(c context.Context, projectID string, cfg *configInterface.Config) (stringset.Set, error) {
	proj := config.Project{}
	if err := protoutil.UnmarshalTextML(cfg.Content, &proj); err != nil {
		return nil, errors.Annotate(err, "unmarshalling proto").Err()
	}

	// Extract the headers into a map for convenience.
	headers := make(map[string]*config.Header, len(proj.Headers))
	for _, header := range proj.Headers {
		headers[header.Id] = header
	}
	// Keep a list of known consoles so we can prune deleted ones later.
	knownConsoles := stringset.New(len(proj.Consoles))
	// Save the project into the datastore.
	project := Project{ID: projectID, LogoURL: proj.LogoUrl}
	if proj.BuildBugTemplate != nil {
		project.BuildBugTemplate = *proj.BuildBugTemplate
	}
	if err := datastore.Put(c, &project); err != nil {
		return nil, err
	}
	parentKey := datastore.KeyForObj(c, &project)
	// Iterate through all the proto consoles, adding and replacing the
	// known ones if needed.
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		toPut := make([]*Console, 0, len(proj.Consoles))
		for i, pc := range proj.Consoles {
			if header, ok := headers[pc.HeaderId]; pc.Header == nil && ok {
				// Inject a header if HeaderId is specified, and it doesn't already have one.
				pc.Header = header
			}
			knownConsoles.Add(pc.Id)
			con, err := GetConsole(c, projectID, pc.Id)
			switch {
			case err == ErrConsoleNotFound:
				// continue
			case err != nil:
				return errors.Annotate(err, "checking %s", pc.Id).Err()
			case con.ConfigRevision == cfg.Revision && con.Ordinal == i:
				// Check if revisions match; if so just skip it.
				// TODO(jchinlee): remove Ordinal check when Version field is added to Console.
				continue
			}
			toPut = append(toPut, &Console{
				Parent:         parentKey,
				ID:             pc.Id,
				Ordinal:        i,
				ConfigURL:      LuciConfigURL(c, string(cfg.ConfigSet), cfg.Path, cfg.Revision),
				ConfigRevision: cfg.Revision,
				Builders:       pc.AllBuilderIDs(),
				Def:            *pc,
			})
		}
		return datastore.Put(c, toPut)
	}, nil)

	if err != nil {
		logging.WithError(err).Errorf(c, "failed to save consoles of project %q at revision %q", projectID, cfg.Revision)
		return nil, err
	}

	logging.Infof(c, "saved consoles of project %q at revision %q", projectID, cfg.Revision)

	return knownConsoles, nil
}

// UpdateConsoles updates internal console definitions entities based off luci-config.
func UpdateConsoles(c context.Context) error {
	cfgName := info.AppID(c) + ".cfg"

	logging.Debugf(c, "fetching configs for %s", cfgName)
	// Acquire the raw config client.
	lucicfg := backend.Get(c).GetConfigInterface(c, backend.AsService)
	// Project configs for Milo contains console definitions.
	configs, err := lucicfg.GetProjectConfigs(c, cfgName, false)
	if err != nil {
		return errors.Annotate(err, "while fetching project configs").Err()
	}
	logging.Infof(c, "got %d project configs", len(configs))

	merr := errors.MultiError{}
	knownProjects := map[string]stringset.Set{}
	// Iterate through each project config, extracting the console definition.
	for _, cfg := range configs {
		projectName := cfg.ConfigSet.Project()
		if projectName == "" {
			return fmt.Errorf("Invalid config set path %s", cfg.ConfigSet)
		}
		knownProjects[projectName] = nil
		if kp, err := updateProjectConsoles(c, projectName, &cfg); err != nil {
			err = errors.Annotate(err, "processing project %s", projectName).Err()
			merr = append(merr, err)
		} else {
			knownProjects[projectName] = kp
		}
	}

	// Delete all the consoles that no longer exists or are part of deleted projects.
	toDelete := []*datastore.Key{}
	err = datastore.Run(c, datastore.NewQuery("Console"), func(key *datastore.Key) error {
		proj := key.Parent().StringID()
		id := key.StringID()
		// If this console is either:
		// 1. In a project that no longer exists, or
		// 2. Not in the project, then delete it.
		knownConsoles, ok := knownProjects[proj]
		if !ok {
			logging.Infof(
				c, "deleting %s/%s because the project no longer exists", proj, id)
			toDelete = append(toDelete, key)
			return nil
		}
		if knownConsoles == nil {
			// The project exists but we couldn't check it this time.  Skip it and
			// try again the next cron cycle.
			return nil
		}
		if !knownConsoles.Has(id) {
			logging.Infof(
				c, "deleting %s/%s because the console no longer exists", proj, id)
			toDelete = append(toDelete, key)
		}
		return nil
	})
	if err != nil {
		merr = append(merr, err)
	} else if err := datastore.Delete(c, toDelete); err != nil {
		merr = append(merr, err)
	}

	// Print some stats.
	processedConsoles := 0
	for _, cons := range knownProjects {
		if cons != nil {
			processedConsoles += cons.Len()
		}
	}
	logging.Infof(
		c, "processed %d consoles over %d projects", processedConsoles, len(knownProjects))

	if len(merr) == 0 {
		return nil
	}
	return merr
}

type consolesCacheKey string

// GetAllConsoles returns all Consoles (across all projects) which contian the
// builder ID. If builderID is empty, then this retrieves all Consoles.
//
// TODO-perf(iannucci): Maybe memcache this too.
func GetAllConsoles(c context.Context, builderID string) ([]*Console, error) {
	itm, err := caching.RequestCache(c).GetOrCreate(c, consolesCacheKey(builderID), func() (interface{}, time.Duration, error) {
		q := datastore.NewQuery("Console")
		if builderID != "" {
			q = q.Eq("Builders", builderID)
		}
		con := []*Console{}
		err := datastore.GetAll(c, q, &con)

		return con, 0, errors.
			Annotate(err, "getting consoles for %q", builderID).
			Tag(transient.Tag).
			Err()
	})
	con, _ := itm.([]*Console)
	return con, err
}

func GetProject(c context.Context, project string) (*Project, error) {
	switch allowed, err := IsAllowed(c, project); {
	case err != nil:
		return nil, err
	case !allowed:
		return nil, errors.Annotate(cfgclientAccess.ErrNoAccess, "no access to project").Err()
	}
	proj := Project{
		ID: project,
	}
	err := datastore.Get(c, &proj)
	if err == datastore.ErrNoSuchEntity {
		err = nil
	}
	return &proj, errors.Annotate(err, "getting project %q", project).Err()
}

// GetAllProjects returns all projects the current user has access to.
func GetAllProjects(c context.Context) ([]Project, error) {
	q := datastore.NewQuery("Project")
	projs := []Project{}

	if err := datastore.GetAll(c, q, &projs); err != nil {
		return nil, errors.Annotate(err, "getting projects").Err()
	}
	result := []Project{}
	for _, proj := range projs {
		switch allowed, err := IsAllowed(c, proj.ID); {
		case err != nil:
			return nil, err
		case allowed:
			result = append(result, proj)
		}
	}
	return result, nil
}

// GetProjectConsoles returns all consoles for the given project ordered as in config.
func GetProjectConsoles(c context.Context, projectID string) ([]*Console, error) {
	// Query datastore for consoles related to the project.
	q := datastore.NewQuery("Console")
	parentKey := datastore.MakeKey(c, "Project", projectID)
	q = q.Ancestor(parentKey)
	con := []*Console{}
	err := datastore.GetAll(c, q, &con)
	sort.Slice(con, func(i, j int) bool { return con[i].Ordinal < con[j].Ordinal })
	return con, errors.Annotate(err, "getting project %q consoles", projectID).Err()
}

// GetConsole returns the requested console.
//
// TODO-perf(iannucci,hinoka): Memcache this.
func GetConsole(c context.Context, proj, id string) (*Console, error) {
	con := Console{
		Parent: datastore.MakeKey(c, "Project", proj),
		ID:     id,
	}
	switch err := datastore.Get(c, &con); err {
	case datastore.ErrNoSuchEntity:
		return nil, ErrConsoleNotFound
	case nil:
		return &con, nil
	default:
		return nil, errors.Annotate(err, "getting project %q console %q", proj, id).Err()
	}
}

// GetConsoles returns the requested consoles.
//
// TODO-perf(iannucci,hinoka): Memcache this.
func GetConsoles(c context.Context, consoles []ConsoleID) ([]*Console, error) {
	result := make([]*Console, len(consoles))
	for i, con := range consoles {
		result[i] = con.SetID(c, nil)
	}
	if err := datastore.Get(c, result); err != nil {
		err = ReplaceNSEWith(err.(errors.MultiError), ErrConsoleNotFound)
		return result, errors.Annotate(err, "getting %s consoles", consoles).Err()
	}
	return result, nil
}

// Config validation rules go here.

func init() {
	// Milo is only responsible for validating the config matching the instance's
	// appID in a project config.
	validation.Rules.Add("regex:projects/.*", "${appid}.cfg", validateProjectCfg)
	validation.Rules.Add("services/${appid}", globalConfigFilename, validateServiceCfg)
}

// validateProjectCfg implements validation.Func by taking a potential Milo
// config at path, validating it, and writing the result into ctx.
//
// The validation we do include:
//
// * Make sure the config is able to be unmarshalled.
// * Make sure all consoles have either builder_view_only: true or manifest_name
func validateProjectCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	proj := config.Project{}
	if err := protoutil.UnmarshalTextML(string(content), &proj); err != nil {
		ctx.Error(err)
		return nil
	}
	knownHeaders := stringset.New(len(proj.Headers))
	for i, header := range proj.Headers {
		ctx.Enter("header #%d (%s)", i, header.Id)
		if header.Id == "" {
			ctx.Errorf("missing id")
		} else if !knownHeaders.Add(header.Id) {
			ctx.Errorf("duplicate header id")
		}
		ctx.Exit()
	}

	knownConsoles := stringset.New(len(proj.Consoles))
	for i, console := range proj.Consoles {
		ctx.Enter("console #%d (%s)", i, console.Id)
		if console.Id == "" {
			ctx.Errorf("missing id")
		} else if !knownConsoles.Add(console.Id) {
			ctx.Errorf("duplicate console")

		}
		// If this is a CI console and it's missing manifest name, the author
		// probably forgot something.
		if !console.BuilderViewOnly {
			if console.ManifestName == "" {
				ctx.Errorf("ci console missing manifest name")
			}
			if console.RepoUrl == "" {
				ctx.Errorf("ci console missing repo url")
			}
			if len(console.Refs) == 0 {
				ctx.Errorf("ci console missing refs")
			} else {
				gitiles.ValidateRefSet(ctx, console.Refs)
			}
		} else {
			if console.IncludeExperimentalBuilds {
				ctx.Errorf("builder_view_only and include_experimental_builds both set")
			}
		}

		if console.HeaderId != "" && !knownHeaders.Has(console.HeaderId) {
			ctx.Errorf("header %s not defined", console.HeaderId)
		}
		if console.HeaderId != "" && console.Header != nil {
			ctx.Errorf("cannot specify both header and header_id")
		}
		ctx.Exit()
	}
	if proj.LogoUrl != "" && !strings.HasPrefix(proj.LogoUrl, "https://storage.googleapis.com/") {
		ctx.Errorf("invalid logo url %q, must begin with https://storage.googleapis.com/", proj.LogoUrl)
	}

	return nil
}

// validateServiceCfg implements validation.Func by taking a potential Milo
// service global config, validating it, and writing the result into ctx.
//
// The validation we do include:
//
// * Make sure the config is able to be unmarshalled.
func validateServiceCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	settings := config.Settings{}
	if err := protoutil.UnmarshalTextML(string(content), &settings); err != nil {
		ctx.Error(err)
	}
	gitacls.ValidateConfig(ctx, settings.SourceAcls)
	return nil
}
