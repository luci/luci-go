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
	"time"

	"github.com/golang/protobuf/proto"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"
	configInterface "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"
	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/server/router"
)

// parsedProjectConfigSet contains all configurations of a project.
type parsedProjectConfigSet struct {
	ProjectID      string
	ProjectConfig  *notifypb.ProjectConfig
	EmailTemplates map[string]*EmailTemplate
	Revision       string
	ViewURL        string
}

// updateProject updates all relevant entities corresponding to a particular project in
// a single datastore transaction.
//
// Returns the set of notifiers that were updated.
func updateProject(c context.Context, cs *parsedProjectConfigSet) error {
	return datastore.RunInTransaction(c, func(c context.Context) error {
		project := &Project{
			Name:     cs.ProjectID,
			Revision: cs.Revision,
			URL:      cs.ViewURL,
		}
		parentKey := datastore.KeyForObj(c, project)

		toSave := make([]interface{}, 0, 1+len(cs.ProjectConfig.Notifiers)+len(cs.EmailTemplates))
		toSave = append(toSave, project)

		liveBuilders := stringset.New(len(cs.ProjectConfig.Notifiers))
		oldBuilders := make([]*Builder, 0, len(cs.ProjectConfig.Notifiers))
		newBuilders := make([]*Builder, 0, len(cs.ProjectConfig.Notifiers))
		for _, cfgNotifier := range cs.ProjectConfig.Notifiers {
			for _, cfgBuilder := range cfgNotifier.Builders {
				id := fmt.Sprintf("%s/%s", cfgBuilder.Bucket, cfgBuilder.Name)
				liveBuilders.Add(id)
				oldBuilders = append(oldBuilders, &Builder{
					ProjectKey: parentKey,
					ID:         id,
				})
				newBuilder := &Builder{
					ProjectKey: parentKey,
					ID:         id,
					Repository: cfgBuilder.Repository,
					Notifications: notifypb.Notifications{
						Notifications: cfgNotifier.Notifications,
					},
				}
				newBuilders = append(newBuilders, newBuilder)
				toSave = append(toSave, newBuilder)
			}
		}

		// Populate the new builders with information from the old builders,
		// if possible and if applicable. Note that if there are transient datastore
		// errors here, that the worst that can happen is a single on_change notification
		// will be missed.
		datastore.Get(c, oldBuilders)
		for i := range oldBuilders {
			oldBuilder := oldBuilders[i]
			newBuilder := newBuilders[i]
			if oldBuilder.Repository == newBuilder.Repository {
				newBuilder.Revision = oldBuilder.Revision
			}
			newBuilder.Status = oldBuilder.Status
			newBuilder.BuildTime = oldBuilder.BuildTime
			newBuilder.GitilesCommits = oldBuilder.GitilesCommits
		}

		for _, et := range cs.EmailTemplates {
			et.ProjectKey = parentKey
			toSave = append(toSave, et)
		}

		return parallel.FanOutIn(func(work chan<- func() error) {
			work <- func() error {
				return datastore.Put(c, toSave)
			}
			work <- func() error {
				return removeDescendants(c, "Builder", parentKey, liveBuilders.Has)
			}
			work <- func() error {
				return removeDescendants(c, "EmailTemplate", parentKey, func(name string) bool {
					_, ok := cs.EmailTemplates[name]
					return ok
				})
			}
		})
	}, nil)
}

// clearDeadProjects calls deleteProject for all projects in the datastore
// that are not in liveProjects.
func clearDeadProjects(c context.Context, liveProjects stringset.Set) error {
	var allProjects []*Project
	projectQ := datastore.NewQuery("Project").KeysOnly(true)
	if err := datastore.GetAll(c, projectQ, &allProjects); err != nil {
		return err
	}
	return parallel.WorkPool(10, func(work chan<- func() error) {
		for _, p := range allProjects {
			p := p
			if !liveProjects.Has(p.Name) {
				work <- func() error {
					logging.Warningf(c, "deleting project %s", p.Name)
					return deleteProject(c, p.Name)
				}
			}
		}
	})
}

// deleteProject deletes a Project entity and all of its descendants.
func deleteProject(c context.Context, projectId string) error {
	return datastore.RunInTransaction(c, func(c context.Context) error {
		project := &Project{Name: projectId}
		ancestorKey := datastore.KeyForObj(c, project)
		return parallel.FanOutIn(func(work chan<- func() error) {
			work <- func() error {
				return removeDescendants(c, "Builder", ancestorKey, nil)
			}
			work <- func() error {
				return removeDescendants(c, "EmailTemplate", ancestorKey, nil)
			}
			work <- func() error {
				return datastore.Delete(c, project)
			}
		})
	}, nil)
}

// updateProjects updates all Projects and their Notifiers in the datastore.
func updateProjects(c context.Context) error {
	cfgName := info.AppID(c) + ".cfg"
	logging.Debugf(c, "fetching configs for %s", cfgName)
	lucicfg := GetConfigService(c)
	configs, err := lucicfg.GetProjectConfigs(c, cfgName, false)
	if err != nil {
		return errors.Annotate(err, "while fetching project configs").Err()
	}
	logging.Infof(c, "got %d project configs", len(configs))

	// Load revisions of the existing projects from Datastore.
	projectRevisions := map[string]string{} // project id -> revision
	err = datastore.Run(c, datastore.NewQuery("Project"), func(p *Project) error {
		projectRevisions[p.Name] = p.Revision
		return nil
	})
	if err != nil {
		return err
	}

	// Update each project concurrently.
	err = parallel.WorkPool(10, func(work chan<- func() error) {
		for _, cfg := range configs {
			cfg := cfg

			curRev, ok := projectRevisions[cfg.ConfigSet.Project()]
			if ok && curRev == cfg.Revision {
				// Same revision.
				continue
			}

			logging.Warningf(
				c, "upgrading config of project %q: %q => %q",
				cfg.ConfigSet.Project(), curRev, cfg.Revision)

			work <- func() error {
				projectId := cfg.ConfigSet.Project()
				project := &notifypb.ProjectConfig{}
				if err := proto.UnmarshalText(cfg.Content, project); err != nil {
					return errors.Annotate(err, "unmarshalling project config").Err()
				}

				ctx := &validation.Context{Context: c}
				ctx.SetFile(cfgName)
				validateProjectConfig(ctx, project)
				if err := ctx.Finalize(); err != nil {
					return errors.Annotate(err, "validating project config").Err()
				}

				emailTemplates, err := fetchAllEmailTemplates(c, lucicfg, projectId)
				if err != nil {
					return errors.Annotate(err, "failed to fetch email templates").Err()
				}

				parsedConfigSet := &parsedProjectConfigSet{
					ProjectID:      projectId,
					ProjectConfig:  project,
					EmailTemplates: emailTemplates,
					Revision:       cfg.Revision,
					ViewURL:        cfg.ViewURL,
				}
				if err := updateProject(c, parsedConfigSet); err != nil {
					return errors.Annotate(err, "importing project %q", projectId).Err()
				}
				return nil
			}
		}
	})
	if err != nil {
		return err
	}

	// Live projects includes both valid and invalid configurations, as long as
	// they are found via luci-config. Otherwise, a minor mistake in a
	// configuration can cause projects to be deleted.
	liveProjects := stringset.New(0)
	for _, cfg := range configs {
		liveProjects.Add(cfg.ConfigSet.Project())
	}
	return clearDeadProjects(c, liveProjects)
}

var configInterfaceKey = "configInterface"

// WithConfigService sets a luci_notify config interface to be used for all config interaction.
func WithConfigService(c context.Context, cInterface configInterface.Interface) context.Context {
	return context.WithValue(c, &configInterfaceKey, cInterface)
}

// GetConfigService returns an Inteface based on the provided context values
func GetConfigService(c context.Context) configInterface.Interface {
	if iface, ok := c.Value(&configInterfaceKey).(configInterface.Interface); ok {
		return iface
	}
	return nil
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

// removeDescedants deletes all entities of a given kind under the ancestor.
// If preserve is not nil and it returns true for a string id, the entity
// is not deleted.
func removeDescendants(c context.Context, kind string, ancestor *datastore.Key, preserve func(string) bool) error {
	var toDelete []*datastore.Key
	q := datastore.NewQuery(kind).Ancestor(ancestor).KeysOnly(true)
	err := datastore.Run(c, q, func(key *datastore.Key) error {
		id := key.StringID()
		if preserve != nil && preserve(id) {
			return nil
		}
		logging.Infof(c, "deleting entity %s", key.String())
		toDelete = append(toDelete, key)
		return nil
	})
	if err != nil {
		return err
	}
	return datastore.Delete(c, toDelete)
}
