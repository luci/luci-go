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
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"
	notifypb "go.chromium.org/luci/luci_notify/api/config"
	"go.chromium.org/luci/luci_notify/common"
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
			Name:               cs.ProjectID,
			Revision:           cs.Revision,
			URL:                cs.ViewURL,
			TreeClosingEnabled: cs.ProjectConfig.TreeClosingEnabled,
		}
		parentKey := datastore.KeyForObj(c, project)

		toSave := make([]interface{}, 0, 1+len(cs.ProjectConfig.Notifiers)+len(cs.EmailTemplates))
		toSave = append(toSave, project)

		// Collect the list of builders and tree closers we want to update or create.
		liveBuilders := stringset.New(len(cs.ProjectConfig.Notifiers))
		builders := make([]*Builder, 0, len(cs.ProjectConfig.Notifiers))

		// 1/4 the number of notifiers is guess at how many tree closers we'll have.
		// Completely made up, but ultimately this is just a hint for the initial
		// size of the set, and hence doesn't need to be perfect (or even very good).
		sizeHint := len(cs.ProjectConfig.Notifiers) / 4
		liveTreeClosers := stringset.New(sizeHint)
		treeClosers := make([]*TreeCloser, 0, sizeHint)

		for _, cfgNotifier := range cs.ProjectConfig.Notifiers {
			for _, cfgBuilder := range cfgNotifier.Builders {
				builder := &Builder{
					ProjectKey: parentKey,
					ID:         fmt.Sprintf("%s/%s", cfgBuilder.Bucket, cfgBuilder.Name),
				}
				builders = append(builders, builder)
				liveBuilders.Add(datastore.KeyForObj(c, builder).String())

				builderKey := datastore.KeyForObj(c, builder)
				for _, cfgTreeCloser := range cfgNotifier.TreeClosers {
					tc := &TreeCloser{
						BuilderKey:     builderKey,
						TreeStatusHost: cfgTreeCloser.TreeStatusHost,
					}
					treeClosers = append(treeClosers, tc)
					liveTreeClosers.Add(datastore.KeyForObj(c, tc).String())
				}
			}
		}

		// Lookup the builders and tree closers in the datastore, if they're not
		// found that's OK since there could be new entities being initialized.
		datastore.Get(c, builders, treeClosers)

		builderIndex := 0
		treeCloserIndex := 0
		for _, cfgNotifier := range cs.ProjectConfig.Notifiers {
			for _, cfgBuilder := range cfgNotifier.Builders {
				builder := builders[builderIndex]
				builder.Repository = cfgBuilder.Repository
				builder.Notifications = notifypb.Notifications{
					Notifications: cfgNotifier.Notifications,
				}
				toSave = append(toSave, builder)
				builderIndex++

				for _, cfgTreeCloser := range cfgNotifier.TreeClosers {
					treeCloser := treeClosers[treeCloserIndex]
					treeCloser.TreeCloser = *cfgTreeCloser

					// Default the status to "Open" for new TreeClosers that we haven't
					// yet seen any builds for.
					if treeCloser.Status == "" {
						treeCloser.Status = Open
					}

					toSave = append(toSave, treeCloser)
					treeCloserIndex++
				}
			}
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
				return removeDescendants(c, "Builder", parentKey, func(key *datastore.Key) bool {
					return liveBuilders.Has(key.String())
				})
			}
			work <- func() error {
				return removeDescendants(c, "TreeCloser", parentKey, func(key *datastore.Key) bool {
					return liveTreeClosers.Has(key.String())
				})
			}
			work <- func() error {
				return removeDescendants(c, "EmailTemplate", parentKey, func(key *datastore.Key) bool {
					_, ok := cs.EmailTemplates[key.StringID()]
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
func deleteProject(c context.Context, projectID string) error {
	return datastore.RunInTransaction(c, func(c context.Context) error {
		project := &Project{Name: projectID}
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
	appID, err := common.GetAppID(c)
	if err != nil {
		return errors.Annotate(err, "failed to get app ID").Err()
	}
	cfgName := appID + ".cfg"
	logging.Debugf(c, "fetching configs for %s", cfgName)
	lucicfg := cfgclient.Client(c)
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

			logging.Infof(
				c, "upgrading config of project %q: %q => %q",
				cfg.ConfigSet.Project(), curRev, cfg.Revision)

			work <- func() error {
				projectID := cfg.ConfigSet.Project()
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

				emailTemplates, err := fetchAllEmailTemplates(c, lucicfg, projectID)
				if err != nil {
					return errors.Annotate(err, "failed to fetch email templates").Err()
				}

				parsedConfigSet := &parsedProjectConfigSet{
					ProjectID:      projectID,
					ProjectConfig:  project,
					EmailTemplates: emailTemplates,
					Revision:       cfg.Revision,
					ViewURL:        cfg.ViewURL,
				}
				if err := updateProject(c, parsedConfigSet); err != nil {
					return errors.Annotate(err, "importing project %q", projectID).Err()
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

// UpdateHandler is the HTTP router handler for handling cron-triggered
// configuration update requests.
func UpdateHandler(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()

	if err := updateProjects(ctx); err != nil {
		return transient.Tag.Apply(err)
	}

	return transient.Tag.Apply(updateSettings(ctx))
}

// removeDescedants deletes all entities of a given kind under the ancestor.
// If preserve is not nil and it returns true for a string id, the entity
// is not deleted.
func removeDescendants(c context.Context, kind string, ancestor *datastore.Key, preserve func(*datastore.Key) bool) error {
	var toDelete []*datastore.Key
	q := datastore.NewQuery(kind).Ancestor(ancestor).KeysOnly(true)
	err := datastore.Run(c, q, func(key *datastore.Key) error {
		if preserve != nil && preserve(key) {
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
