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
	"strings"
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

// updateProject updates all relevant entities corresponding to a particular
// project.
//
// To support large projects with many entities, updates occur over multiple
// transactions:
//   - In the first transaction(s), the builders and tree closers configured in
//     the provided project configset are created/updated. Old builders and
//     tree closers will be retained.
//   - In the last transaction, the stored project revision, and project-level
//     properties (e.g. TreeClosingEnabled) will be updated. Old builders and
//     tree closers are deleted.
//
// Between the first and last transactions, a configuration that is a mix of
// the old and the new configuration will be in effect. This could take the
// form of (possibly created/updated) builders with old tree closers still
// present, or old builders and all their old tree closers, all in conjunction
// with a (possibly old) tree closure enabled setting. The user impact of this
// is expected to be benign (and transient). This interim config will be valid.
//
// In case of an error before all transactions complete, config application
// will be retried by the cron job because the revision is not updated until
// all updates are applied.
//
// This method should not be called concurrently for the same project,
// especially with different config versions, as the result is undefined.
// It is assumed this will be guaranteed by the fact the config cron runs
// run sequentially and will not overrun eachother.
func updateProject(c context.Context, cs *parsedProjectConfigSet) error {
	project := &Project{
		Name:               cs.ProjectID,
		Revision:           cs.Revision,
		URL:                cs.ViewURL,
		TreeClosingEnabled: cs.ProjectConfig.TreeClosingEnabled,
	}
	parentKey := datastore.KeyForObj(c, project)

	// The set of builders and tree closers which should be kept.
	liveBuilders := stringset.New(0)
	liveTreeClosers := stringset.New(0)

	partitions := partitionNotifiers(cs.ProjectConfig.Notifiers)
	for i, partition := range partitions {
		r, err := updateProjectNotifiers(c, parentKey, partition)
		if err != nil {
			return errors.Annotate(err, "applying partition %v", i).Err()
		}
		liveBuilders = r.LiveBuilders.Union(liveBuilders)
		liveTreeClosers = r.LiveTreeClosers.Union(liveTreeClosers)
	}

	return datastore.RunInTransaction(c, func(c context.Context) error {
		toSave := make([]any, 0, 1+len(cs.ProjectConfig.Notifiers)+len(cs.EmailTemplates))
		toSave = append(toSave, project)

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

type updatedNotifiers struct {
	// BuilderKeys contains the keys of the builders which were specified in
	// the project configuration.
	LiveBuilders stringset.Set
	// TreeCloserKeys contains the keys of the tree closers which were
	// specified in the project configuration.
	LiveTreeClosers stringset.Set
}

// updateProjectNotifiers puts a partition of project notifiers in a single
// datastore transaction.
func updateProjectNotifiers(ctx context.Context, parentKey *datastore.Key, notifiers []*notifypb.Notifier) (updatedNotifiers, error) {
	var result updatedNotifiers

	f := func(ctx context.Context) error {
		// Collect the list of builders and tree closers we want to update or create.
		liveBuilders := stringset.New(len(notifiers))
		builders := make([]*Builder, 0, len(notifiers))

		// 1/4 the number of notifiers is guess at how many tree closers we'll have.
		// Completely made up, but ultimately this is just a hint for the initial
		// size of the set, and hence doesn't need to be perfect (or even very good).
		sizeHint := len(notifiers) / 4
		liveTreeClosers := stringset.New(sizeHint)
		treeClosers := make([]*TreeCloser, 0, sizeHint)

		for _, cfgNotifier := range notifiers {
			for _, cfgBuilder := range cfgNotifier.Builders {
				builder := &Builder{
					ProjectKey: parentKey,
					ID:         fmt.Sprintf("%s/%s", cfgBuilder.Bucket, cfgBuilder.Name),
				}
				builders = append(builders, builder)
				liveBuilders.Add(datastore.KeyForObj(ctx, builder).String())

				builderKey := datastore.KeyForObj(ctx, builder)
				for _, cfgTreeCloser := range cfgNotifier.TreeClosers {
					treeName := cfgTreeCloser.TreeName
					if treeName == "" {
						treeName = TreeNameFromHost(cfgTreeCloser.TreeStatusHost)
					}

					tc := &TreeCloser{
						BuilderKey: builderKey,
						TreeName:   treeName,
					}
					treeClosers = append(treeClosers, tc)
					liveTreeClosers.Add(datastore.KeyForObj(ctx, tc).String())
				}
			}
		}

		// Lookup the builders and tree closers in the datastore, if they're not
		// found that's OK since there could be new entities being initialized.
		datastore.Get(ctx, builders, treeClosers)

		toSave := make([]any, 0, 1+len(notifiers))
		builderIndex := 0
		treeCloserIndex := 0
		for _, cfgNotifier := range notifiers {
			for _, cfgBuilder := range cfgNotifier.Builders {
				builder := builders[builderIndex]
				if builder.Repository != cfgBuilder.Repository {
					// Reset the revision if repository is changed.
					builder.Revision = ""
					builder.Repository = cfgBuilder.Repository
				}
				builder.Notifications = &notifypb.Notifications{
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
		err := datastore.Put(ctx, toSave)
		if err != nil {
			return err
		}
		result = updatedNotifiers{
			LiveBuilders:    liveBuilders,
			LiveTreeClosers: liveTreeClosers,
		}
		return nil
	}

	err := datastore.RunInTransaction(ctx, f, nil)
	if err != nil {
		return updatedNotifiers{}, err
	}
	return result, err
}

func TreeNameFromHost(host string) string {
	return strings.TrimSuffix(strings.TrimSuffix(host, ".appspot.com"), "-status")
}

func partitionNotifiers(notifiers []*notifypb.Notifier) [][]*notifypb.Notifier {
	// Datastore allows max. 500 entity updates per transaction.
	// https://cloud.google.com/datastore/docs/concepts/limits#limits
	const maxPartitionSize = 500

	var result [][]*notifypb.Notifier
	var partition []*notifypb.Notifier
	partitionSize := 0
	for _, cfgNotifier := range notifiers {
		// Calculate the number of datastore entities that would be put
		// for this notifier.
		size := len(cfgNotifier.Builders) * (1 + len(cfgNotifier.TreeClosers))

		// Try to stay within the max partition size.
		if partitionSize+size > maxPartitionSize && len(partition) > 0 {
			result = append(result, partition)
			partition = nil
			partitionSize = 0
		}
		partition = append(partition, cfgNotifier)
		partitionSize += size
	}
	if len(partition) > 0 {
		result = append(result, partition)
	}
	return result
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
