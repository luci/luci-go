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
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	configInterface "go.chromium.org/luci/common/config"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/milo/api/config"
)

// Console is a datastore entity representing a single console.
type Console struct {
	// Parent is a key to the parent Project entity where this console was
	// defined in.
	Parent *datastore.Key `gae:"$parent"`
	// ID is the ID of the console.
	ID string `gae:"$id"`
	// RepoURL and Ref combined defines the commits the show up on the left
	// hand side of a Console.
	RepoURL string
	// RepoURL and Ref combined defines the commits the show up on the left
	// hand side of a Console.
	Ref string
	// ManifestName is the name of the manifest to look for when querying for
	// builds under this console.
	ManifestName string
	// URL is the URL to the luci-config definition of this console.
	URL string
	// FaviconURL is the URL to the favicon to be displayed on this console's page.
	FaviconURL string
	// Revision is the luci-config reivision from when this Console was retrieved.
	Revision string
	// Builders is a list of universal builder IDs.  This is indexed.
	Builders []string
	// BuilderMeta is an expanded set of builders containing additional metadata.
	// This should always match the Builders field.
	BuilderMetas []BuilderMeta
	// _ is a "black hole" which absorbs any extra props found during a
	// datastore Get. These props are not written back on a datastore Put.
	_ datastore.PropertyMap `gae:"-,extra"`
}

// BuilderMeta is a struct containing metadata about a builder.
type BuilderMeta struct {
	Category  string
	ShortName string
}

// ParseCategory takes a BuilderMeta's Category and parses it into a list of
// subcategories. The top-level category is listed first.
func (b *BuilderMeta) ParseCategory() []string {
	return strings.Split(b.Category, "|")
}

// GetProjectName retrieves the project name of the console out of the Console's
// parent key.
func (con *Console) GetProjectName() string {
	return con.Parent.StringID()
}

// NewConsole creates a fully populated console out of the luci-config proto
// definition of a console.
func NewConsole(project *datastore.Key, URL, revision string, con *config.Console) *Console {
	return &Console{
		Parent:       project,
		ID:           con.Id,
		RepoURL:      con.RepoUrl,
		Ref:          con.Ref,
		ManifestName: con.ManifestName,
		Revision:     revision,
		URL:          URL,
		FaviconURL:   con.FaviconUrl,
		Builders:     BuilderFromProto(con.Builders),
		BuilderMetas: BuilderRefFromProto(con.Builders),
	}
}

// BuilderFromProto tranforms a luci-config proto builder format into the datastore
// format.
func BuilderFromProto(cb []*config.Builder) []string {
	builders := make([]string, len(cb))
	for i, b := range cb {
		builders[i] = b.Name
	}
	return builders
}

// BuilderRefFromProto tranforms a luci-config proto builder format into the
// BuilderMeta format.
func BuilderRefFromProto(cb []*config.Builder) []BuilderMeta {
	builders := make([]BuilderMeta, len(cb))
	for i, b := range cb {
		builders[i] = BuilderMeta{
			Category:  b.Category,
			ShortName: b.ShortName,
		}
	}
	return builders
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

// GetCurrentServiceConfig gets the service config for the instance from either
// process cache or datastore cache.
func GetCurrentServiceConfig(c context.Context) (*ServiceConfig, error) {
	// This maker function is used to do the actual fetch of the ServiceConfig
	// from datastore.  It is called if the ServiceConfig is not in proc cache.
	item, err := caching.ProcessCache(c).GetOrCreate(c, ServiceConfigID, func() (interface{}, time.Duration, error) {
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

// UpdateServiceConfig fetches the service config from luci-config
// and then stores a snapshot of the configuration in datastore.
func UpdateServiceConfig(c context.Context) (*config.Settings, error) {
	// Load the settings from luci-config.
	cs := string(cfgclient.CurrentServiceConfigSet(c))
	// Acquire the raw config client.
	lucicfg := backend.Get(c).GetConfigInterface(c, backend.AsService)
	// Our global config name is called settings.cfg.
	cfg, err := lucicfg.GetConfig(c, cs, "settings.cfg", false)
	if err != nil {
		return nil, fmt.Errorf("could not load settings.cfg from luci-config: %s", err)
	}
	settings := &config.Settings{}
	err = proto.UnmarshalText(cfg.Content, settings)
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
func updateProjectConsoles(c context.Context, projectName string, cfg *configInterface.Config) (stringset.Set, error) {
	proj := config.Project{}
	if err := proto.UnmarshalText(cfg.Content, &proj); err != nil {
		return nil, errors.Annotate(err, "unmarshalling proto").Err()
	}

	// Keep a list of known consoles so we can prune deleted ones later.
	knownConsoles := stringset.New(len(proj.Consoles))
	// Iterate through all the proto consoles, adding and replacing the
	// known ones if needed.
	parentKey := datastore.MakeKey(c, "Project", projectName)
	for _, pc := range proj.Consoles {
		knownConsoles.Add(pc.Id)
		con, err := GetConsole(c, projectName, pc.Id)
		switch err {
		case datastore.ErrNoSuchEntity:
			// continue
		case nil:
			// Check if revisions match, if so just skip it.
			if con.Revision == cfg.Revision {
				continue
			}
		default:
			return nil, errors.Annotate(err, "checking %s", pc.Id).Err()
		}
		URL := LuciConfigURL(c, cfg.ConfigSet, cfg.Path, cfg.Revision)
		con = NewConsole(parentKey, URL, cfg.Revision, pc)
		if err = datastore.Put(c, con); err != nil {
			return nil, errors.Annotate(err, "saving %s", pc.Id).Err()
		}
		logging.Infof(c, "saved a new %s / %s (revision %s)", projectName, con.ID, cfg.Revision)
	}
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
		// This looks like "projects/<project name>"
		splitPath := strings.SplitN(cfg.ConfigSet, "/", 2)
		if len(splitPath) != 2 {
			return fmt.Errorf("Invalid config set path %s", cfg.ConfigSet)
		}
		projectName := splitPath[1]
		knownProjects[projectName] = nil
		if kp, err := updateProjectConsoles(c, projectName, &cfg); err != nil {
			err = errors.Annotate(err, "processing project %s", cfg.ConfigSet).Err()
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

// GetAllConsoles returns all registered projects with the builder name.
// If builderName is empty, then this retrieves all Consoles.
//
// TODO-perf(iannucci,hinoka): This gets called in a LOT of places; we should
// profile this to see if we can rework it into something cached, ESPECIALLY
// since this data changes infrequently, and we actually get an 'update' event
// in the form of the UpdateConsoles function.
func GetAllConsoles(c context.Context, builderName string) ([]*Console, error) {
	q := datastore.NewQuery("Console")
	if builderName != "" {
		q = q.Eq("Builders", builderName)
	}
	con := []*Console{}
	err := datastore.GetAll(c, q, &con)
	return con, err
}

// GetProjectConsoles returns all consoles for the given project.
func GetProjectConsoles(c context.Context, projectName string) ([]*Console, error) {
	// Check ACLs if user is allowed to access this project.
	ok, err := IsAllowed(c, projectName)
	switch {
	case err != nil:
		return nil, err
	case !ok:
		return nil, errors.New("no access to project", CodeNoAccess)
	}
	// Query datastore for consoles related to the project.
	q := datastore.NewQuery("Console")
	parentKey := datastore.MakeKey(c, "Project", projectName)
	q = q.Ancestor(parentKey)
	con := []*Console{}
	err = datastore.GetAll(c, q, &con)
	return con, err
}

// GetConsole returns the requested console.
//
// TODO-perf(iannucci,hinoka): Memcache this.
func GetConsole(c context.Context, proj, id string) (*Console, error) {
	con := Console{
		Parent: datastore.MakeKey(c, "Project", proj),
		ID:     id,
	}
	err := datastore.Get(c, &con)
	return &con, err
}
