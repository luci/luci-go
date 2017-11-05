// Copyright 2017 The LUCI Authors.
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
	"fmt"
	"net/http"
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
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	notifyConfig "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/server/router"
)

// clearDeadNotifiers deletes all Notifier entities from the datastore
// which are not in a given "live" set, and which are associated with a
// specific project via parent key.
func clearDeadNotifiers(c context.Context, parent *datastore.Key, liveNotifiers stringset.Set) error {
	// Delete all the notifiers that no longer exist or are part of deleted projects.
	toDelete := []*datastore.Key{}
	err := datastore.Run(c, datastore.NewQuery("Notifier").Ancestor(parent), func(key *datastore.Key) error {
		id := key.StringID()
		if !liveNotifiers.Has(id) {
			logging.Infof(
				c, "deleting %s/%s because the notifier no longer exists", parent.StringID(), id)
			toDelete = append(toDelete, key)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return datastore.Delete(c, toDelete)
}

// updateProject updates all relevant entities corresponding to a particular project in
// a single datastore transaction.
//
// Returns the set of notifiers that were updated.
func updateProject(c context.Context, projectName string, projectCfg *notifyConfig.ProjectConfig, cfg *configInterface.Config) error {
	existingProject := Project{Name: projectName}
	switch err := datastore.Get(c, &existingProject); err {
	case datastore.ErrNoSuchEntity:
		// continue
	case nil:
		if existingProject.Revision == cfg.Revision {
			logging.Debugf(c, "same revision: %s, %s", projectName, cfg.Revision)
			return nil
		}
	default:
		return errors.Annotate(err, "checking existence").Err()
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		notifiers := make([]*Notifier, len(projectCfg.Notifiers))
		liveNotifiers := stringset.New(len(projectCfg.Notifiers))
		newProject := NewProject(projectName, cfg)
		parentKey := datastore.KeyForObj(c, newProject)
		for i, cfgNotifier := range projectCfg.Notifiers {
			notifiers[i] = NewNotifier(parentKey, cfgNotifier)
			liveNotifiers.Add(cfgNotifier.Name)
		}
		if err := datastore.Put(c, newProject, notifiers); err != nil {
			return errors.Annotate(err, "saving project and notifiers").Err()
		}
		return clearDeadNotifiers(c, parentKey, liveNotifiers)
	}, nil)
}

// clearDeadProjects deletes all Notifier entities from the datastore
// associated with dead projects. It will leave the Project structure,
// however, because it doesn't take up much space.
func clearDeadProjects(c context.Context, liveProjects stringset.Set) error {
	toDelete := []*datastore.Key{}
	markForDeletion := func(key *datastore.Key) error {
		toDelete = append(toDelete, key)
		return nil
	}
	err := datastore.Run(c, datastore.NewQuery("Project"), func(key *datastore.Key) error {
		id := key.StringID()
		if !liveProjects.Has(id) {
			logging.Infof(
				c, "deleting notifiers for %s because the project no longer exists", id)
			notifierQuery := datastore.NewQuery("Notifier").Ancestor(key)
			return datastore.Run(c, notifierQuery, markForDeletion)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return datastore.Delete(c, toDelete)
}

// updateProjects updates all Projects and their Notifiers in the datastore.
func updateProjects(c context.Context) error {
	cfgName := info.AppID(c) + ".cfg"
	logging.Debugf(c, "fetching configs for %s", cfgName)
	lucicfg := backend.Get(c).GetConfigInterface(c, backend.AsService)
	configs, err := lucicfg.GetProjectConfigs(c, cfgName, false)
	if err != nil {
		return errors.Annotate(err, "while fetching project configs").Err()
	}
	logging.Infof(c, "got %d project configs", len(configs))

	// Live projects includes both valid and invalid configurations, as long as
	// they are found via luci-config. Otherwise, a minor mistake in a
	// configuration can cause projects to be deleted.
	liveProjects := stringset.New(0)

	// Iterate through each project config, extracting notifiers.
	merr := errors.MultiError{}
	for _, cfg := range configs {
		// This looks like "projects/<project name>"
		splitPath := strings.SplitN(cfg.ConfigSet, "/", 2)
		if len(splitPath) != 2 {
			merr = append(merr, fmt.Errorf("invalid config set %s", cfg.ConfigSet))
			continue
		}
		projectName := splitPath[1]

		// All found projects are considered 'alive'.
		liveProjects.Add(projectName)

		project := notifyConfig.ProjectConfig{}
		if err := proto.UnmarshalText(cfg.Content, &project); err != nil {
			merr = append(merr, errors.Annotate(err, "unmarshalling proto").Err())
			continue
		}
		if err := validateProjectConfig(cfg.ConfigSet, &project); err != nil {
			merr = append(merr, errors.Annotate(err, "validating config").Err())
			continue
		}
		if err := updateProject(c, projectName, &project, &cfg); err != nil {
			err = errors.Annotate(err, "processing %s", cfg.ConfigSet).Err()
			merr = append(merr, err)
		}
	}
	if err := clearDeadProjects(c, liveProjects); err != nil {
		err = errors.Annotate(err, "deleting projects").Err()
		merr = append(merr, err)
	}
	if len(merr) == 0 {
		return nil
	}
	return merr
}

// UpdateHandler is the HTTP router handler for handling cron-triggered
// configuration update requests.
func UpdateHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	c, _ = context.WithTimeout(c, time.Minute)
	if err := updateProjects(c); err != nil {
		logging.WithError(err).Errorf(c, "error while updating project configs")
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := updateSettings(c); err != nil {
		logging.WithError(err).Errorf(c, "error while updating settings")
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}
