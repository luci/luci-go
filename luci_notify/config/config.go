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

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"google.golang.org/appengine"

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

// updateProjectNotifiers updates all Notifier structures in the datastore related to a
// specific project.
func updateProjectNotifiers(c context.Context, projectName string, config *notifyConfig.ProjectConfig) (stringset.Set, error) {
	knownNotifiers := stringset.New(len(config.Notifiers))
	parentKey := datastore.MakeKey(c, "Project", projectName)
	for _, cfgNotifier := range config.Notifiers {
		err := NewNotifier(parentKey, cfgNotifier).UpdateDatastore(c)
		if err != nil {
			return nil, errors.Annotate(err, "updating %s", cfgNotifier.Name).Err()
		}
		knownNotifiers.Add(cfgNotifier.Name)
	}
	return knownNotifiers, nil
}

// updateProject updates all relevant structures corresponding to a particular project in
// a single datastore transaction.
//
// Returns the set of notifiers that were updated.
func updateProject(c context.Context, projectName string, config *notifyConfig.ProjectConfig, cfg *configInterface.Config) (stringset.Set, error) {
	var knownNotifiers stringset.Set
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		project, err := GetProject(c, projectName)
		switch err {
		case datastore.ErrNoSuchEntity:
			// continue
		case nil:
			if project.Revision == cfg.Revision {
				return nil
			}
		default:
			return errors.Annotate(err, "checking project %s", projectName).Err()
		}
		// Attempt to update notifiers first, so that if there's a failure we can try again
		// on the next update since the revision won't be updated.
		knownNotifiers, err = updateProjectNotifiers(c, projectName, config)
		if err != nil {
			return err
		}
		if err := NewProject(c, projectName, cfg).UpdateDatastore(c); err != nil {
			return err
		}
		return nil
	}, nil)
	return knownNotifiers, err
}

// clearDeadNotifiers deletes all Notifier structures from the datastore
// which are not in a given "known" set.
func clearDeadNotifiers(c context.Context, knownProjects map[string]stringset.Set) error {
	// Delete all the notifiers that no longer exist or are part of deleted projects.
	toDelete := []*datastore.Key{}
	err := datastore.Run(c, datastore.NewQuery("Notifier"), func(key *datastore.Key) error {
		project := key.Parent().StringID()
		id := key.StringID()
		// If this notifier is either:
		// 1. In a project that no longer exists, or
		// 2. Not in the project, then delete it.
		knownNotifiers, ok := knownProjects[project]
		if !ok {
			logging.Infof(
				c, "deleting %s/%s because the project no longer exists", project, id)
			toDelete = append(toDelete, key)
			return nil
		}
		if knownNotifiers == nil {
			// The project exists but we couldn't check it this time.  Skip it and
			// try again the next cron cycle.
			return nil
		}
		if !knownNotifiers.Has(id) {
			logging.Infof(
				c, "deleting %s/%s because the notifier no longer exists", project, id)
			toDelete = append(toDelete, key)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return datastore.Delete(c, toDelete)
}

// clearDeadProjects deletes all Project structures from the datastore
// which are not in a given "known" set.
func clearDeadProjects(c context.Context, knownProjects map[string]stringset.Set) error {
	// Delete all the notifiers that no longer exist or are part of deleted projects.
	toDelete := []*datastore.Key{}
	err := datastore.Run(c, datastore.NewQuery("Project"), func(key *datastore.Key) error {
		project := key.StringID()
		if _, ok := knownProjects[project]; !ok {
			logging.Infof(c, "deleting %s because it no longer exists", project)
			toDelete = append(toDelete, key)
			return nil
		}
		return nil
	})
	if err != nil {
		return err
	}
	return datastore.Delete(c, toDelete)
}

// updateNotifiers updates all Notifiers in the datastore, which are grouped
// by their respective projects.
func updateNotifiers(c context.Context) error {
	cfgName := info.AppID(c) + ".cfg"
	logging.Debugf(c, "fetching configs for %s", cfgName)
	lucicfg := backend.Get(c).GetConfigInterface(c, backend.AsService)
	configs, err := lucicfg.GetProjectConfigs(c, cfgName, false)
	if err != nil {
		return errors.Annotate(err, "while fetching project configs").Err()
	}
	logging.Infof(c, "got %d project configs", len(configs))

	merr := errors.MultiError{}
	knownProjects := map[string]stringset.Set{}
	// Iterate through each project config, extracting the notifier.
	for _, cfg := range configs {
		// This looks like "projects/<project name>"
		splitPath := strings.SplitN(cfg.ConfigSet, "/", 2)
		if len(splitPath) != 2 {
			return fmt.Errorf("Invalid config set path %s", cfg.ConfigSet)
		}
		projectName := splitPath[1]
		knownProjects[projectName] = nil
		// Unmarshal the config into types we can use.
		project := notifyConfig.ProjectConfig{}
		if err := proto.UnmarshalText(cfg.Content, &project); err != nil {
			err = errors.Annotate(err, "unmarshalling proto").Err()
			merr = append(merr, err)
			continue
		}
		if kp, err := updateProject(c, projectName, &project, &cfg); err != nil {
			err = errors.Annotate(err, "processing project %s", cfg.ConfigSet).Err()
			merr = append(merr, err)
		} else {
			knownProjects[projectName] = kp
		}
	}
	if err := clearDeadNotifiers(c, knownProjects); err != nil {
		merr = append(merr, err)
	}
	if err := clearDeadProjects(c, knownProjects); err != nil {
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
	c, h, r := ctx.Context, ctx.Writer, ctx.Request
	c = appengine.WithContext(c, r)
	if err := updateNotifiers(c); err != nil {
		logging.WithError(err).Errorf(c, "error while updating config")
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}
