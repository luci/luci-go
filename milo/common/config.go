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
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/luci_config/server/cfgclient/textproto"

	"github.com/luci/luci-go/milo/api/config"
)

// Project is a LUCI project.
type Project struct {
	// ID of the project, as per self defined.  This is the luci-config name.
	ID string `gae:"$id"`
	// Data is the Project data in protobuf binary format.
	Data []byte `gae:",noindex"`
	// Revision is the latest revision we have of this project's config.
	Revision string
}

// The key for the service config entity in datastore.
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

// GetServiceConfig returns the service (aka global) config for the current
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
	maker := func() (interface{}, time.Duration, error) {
		msg := ServiceConfig{ID: ServiceConfigID}
		err := datastore.Get(c, &msg)
		if err != nil {
			return nil, time.Minute, err
		}
		logging.Infof(c, "loaded service config from datastore")
		return msg, time.Minute, nil
	}
	item, err := proccache.GetOrMake(c, ServiceConfigID, maker)
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
		return nil, fmt.Errorf("failed to update config entry in transaction", err)
	}
	logging.Infof(c, "successfully updated to new config")

	return settings, nil
}

// UpdateProjectConfigs internal project configuration based off luci-config.
// update updates Milo's configuration based off luci config.  This includes
// scanning through all project and extract all console configs.
func UpdateProjectConfigs(c context.Context) error {
	cfgName := info.AppID(c) + ".cfg"

	var (
		configs []*config.Project
		metas   []*cfgclient.Meta
		merr    errors.MultiError
	)

	logging.Debugf(c, "fetching configs for %s", cfgName)
	if err := cfgclient.Projects(c, cfgclient.AsService, cfgName, textproto.Slice(&configs), &metas); err != nil {
		merr = err.(errors.MultiError)
		// Some configs errored out, but we can continue with the ones that work still.
	}

	// A map of project ID to project.
	projects := map[string]*Project{}
	for i, proj := range configs {
		meta := metas[i]
		projectName, _, _ := meta.ConfigSet.SplitProject()
		name := string(projectName)
		projects[name] = nil
		if merr != nil && merr[i] != nil {
			logging.WithError(merr[i]).Warningf(c, "skipping %s due to error", name)
			continue
		}

		p := &Project{
			ID:       name,
			Revision: meta.Revision,
		}

		logging.Infof(c, "prossing %s", name)

		var err error
		p.Data, err = proto.Marshal(proj)
		if err != nil {
			logging.WithError(err).Errorf(c, "Encountered error while processing project %s", name)
			// Set this to nil to signal this project exists but we don't want to update it.
			projects[name] = nil
			continue
		}
		projects[name] = p
	}

	// Now load all the data into the datastore.
	projs := make([]*Project, 0, len(projects))
	for _, proj := range projects {
		if proj != nil {
			projs = append(projs, proj)
		}
	}
	if err := datastore.Put(c, projs); err != nil {
		return err
	}

	// Delete entries that no longer exist.
	q := datastore.NewQuery("Project").KeysOnly(true)
	allProjs := []Project{}
	datastore.GetAll(c, q, &allProjs)
	toDelete := []Project{}
	for _, proj := range allProjs {
		if _, ok := projects[proj.ID]; !ok {
			toDelete = append(toDelete, proj)
		}
	}
	return datastore.Delete(c, toDelete)
}

// GetAllProjects returns all registered projects.
func GetAllProjects(c context.Context) ([]*config.Project, error) {
	q := datastore.NewQuery("Project")
	q.Order("ID")

	ps := []*Project{}
	err := datastore.GetAll(c, q, &ps)
	if err != nil {
		return nil, err
	}
	results := make([]*config.Project, len(ps))
	for i, p := range ps {
		results[i] = &config.Project{}
		if err := proto.Unmarshal(p.Data, results[i]); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// GetProject returns the requested project.
func GetProject(c context.Context, projName string) (*config.Project, error) {
	// Next, Try datastore
	p := Project{ID: projName}
	if err := datastore.Get(c, &p); err != nil {
		return nil, err
	}
	mp := config.Project{}
	if err := proto.Unmarshal(p.Data, &mp); err != nil {
		return nil, err
	}

	return &mp, nil
}

// GetConsole returns the requested console instance.
func GetConsole(c context.Context, projName, consoleName string) (*config.Console, error) {
	p, err := GetProject(c, projName)
	if err != nil {
		return nil, err
	}
	for _, cs := range p.Consoles {
		if cs.Name == consoleName {
			return cs, nil
		}
	}
	return nil, fmt.Errorf("Console %s not found in project %s", consoleName, projName)
}
