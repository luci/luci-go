// Copyright 2020 The LUCI Authors.
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

package gobmap

import (
	"context"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/gerrit/gobmap/internal"
	"go.chromium.org/luci/gae/service/datastore"
)

const (
	gobWatchMapKind = "GobWatchMap"
	parentKind      = "GobWatchMapParent"
)

// gobWatchMap contains config groups for a particular LUCI project and
// host/repo combination.
//
// GobWatchMap entities are stored with a parent key of the form
// (GobWatchMapParent, host/repo), so that all gobWatchMap entities with a
// particular host/repo can be fetched with an ancestor query; the goal
// is to have fast reads by host/repo.
//
// The GobWatchMap entities as a whole store data used to lookup which
// host/repo/ref maps to which config group, and the map is updated when a
// project config is updated.
type gobWatchMap struct {
	_kind string `gae:"$kind,GobWatchMap"`

	// The ID of this GobWatchMap, which is the LUCI project name.
	ID string `gae:"$id"`

	// LUCI project name.
	//
	// This is used when querying for entities to update or remove in the
	// update operation.
	Project string

	// GobWatchMapParent key. This parent has an ID of the form "host/repo".
	Parent *datastore.Key `gae:"$parent"`

	// Groups keeps config groups of a LUCI project applicable to this host/repo.
	Groups *internal.Groups

	// ConfigHash is the hash of latest CV project config imported from LUCI
	// Config; this is updated based on ProjectConfig entity.
	ConfigHash string `gae:",noindex"`
}

// Update loads the new config and updates the gob map entities
// accordingly.
//
// This may include adding, removing and modifying entities.
func Update(ctx context.Context, project string) error {
	// Fetch stored GobWatchMap entities for the project.
	gwms := []*gobWatchMap{}
	q := datastore.NewQuery(gobWatchMapKind).Eq("Project", project)
	if err := datastore.GetAll(ctx, q, &gwms); err != nil {
		return errors.Annotate(err, "failed to get GobWatchMap entities for project %q", project).Tag(transient.Tag).Err()
	}

	// Get the latest ProjectConfig and ConfigGroups from datastore.
	toPut := []*gobWatchMap{}
	toDelete := []*gobWatchMap{}
	switch meta, err := config.GetLatestMeta(ctx, project); {
	case err != nil:
		return err
	case !meta.Exists():
		// The project was removed, delete everything.
	default:
		cgs, err := meta.GetConfigGroups(ctx)
		hash := meta.Hash()
		if err != nil {
			// In most cases, this is due to transient error,
			// XXX: mark as transient and annotate
			return err
		}
		toPut, toDelete = listUpdates(gwms, cgs, hash)
	}
	if err := datastore.Delete(ctx, toDelete); err != nil {
		return errors.Annotate(err, "failed to delete when updating project %q", project).Tag(transient.Tag).Err()
	}
	if err := datastore.Put(ctx, toPut); err != nil {
		return errors.Annotate(err, "failed to put when updating project %q", project).Tag(transient.Tag).Err()
	}
	return nil
}

// listUpdates determines what needs to be updated.
//
// That is, it lists which of the existing GobWatchMap entities should be
// removed, and what GobWatchMap entities should be put (added or updated).
func listUpdates(gwms []*gobWatchMap, latestGroups []*config.ConfigGroup, latestHash string) (toPut, toDelete []*gobWatchMap) {
	latestHostRepos := stringset.New(0)

	for _, g := range latestGroups {
		for _, gerrit := range g.Content.Gerrit {
			host := strings.TrimPrefix(gerrit.Url, "https://")
			host = strings.TrimSuffix(host, "/")
			for _, p := range gerrit.Projects {
				hostRepo := host + "/" + p.Name
				latestHostRepos.Add(hostRepo)
			}
		}
	}

	toPut = []*gobWatchMap{}
	toDelete = []*gobWatchMap{}

	for _, gwm := range gwms {
		hostRepo := gwm.Parent.StringID()
		if !latestHostRepos.Has(hostRepo) {
			toDelete = append(toDelete, gwm)
		}
	}

	existingHashes := make(map[string]string)
	for _, gwm := range gwms {
		existingHashes[gwm.Parent.StringID()] = gwm.ConfigHash
	}

	for _, g := range latestGroups {
		for _, gerrit := range g.Content.Gerrit {
			host := strings.TrimPrefix(gerrit.Url, "https://")
			host = strings.TrimSuffix(host, "/")
			for _, p := range gerrit.Projects {
				hostRepo := host + "/" + p.Name
				switch hash, ok := existingHashes[hostRepo]; {
				case !ok:
					// Need to add a new gwm
				case hash != latestHash:
					// Need to update a gwm
				default:
					// Already up-to-date, nothing to do.
				}
				latestHostRepos.Add(hostRepo)
			}
		}
	}

	return toPut, toDelete
}

//    cg.Gerrit.Url is the host (plus schema), cg.Gerrit.Projects is a list
//    of Gerrit Project messages; within each of those messages is Name
//    (i.e. repo), RefRegexp, and RefRegexpExclude). field, which has Url
//    field, which is the Gerrit host plus schema.

// Lookup returns config group IDs which watch the given combination of Gerrit
// host, repo and ref.
//
// For example: the input might be ("chromium-review.googlesource.com",
// "chromium/src", "refs/heads/main"), and the output might be 0 or 1 or
// multiple IDs which can be used to fetch config groups.
//
// Due to the ref_regexp[_exclude] options, CV can't ensure that each possible
// combination is watched by at most one ConfigGroup, which is why this may
// return multiple ConfigGroupIDs even for the same LUCI project.
func Lookup(ctx context.Context, host, repo, ref string) (*changelist.ApplicableConfig, error) {
	// Fetch all GobWatchMap entities for the given host and repo.
	hostRepo := host + "/" + repo
	parentKey := datastore.MakeKey(ctx, parentKind, hostRepo)
	q := datastore.NewQuery(gobWatchMapKind).Ancestor(parentKey)
	gwms := []*gobWatchMap{}
	if err := datastore.GetAll(ctx, q, &gwms); err != nil {
		return nil, errors.Annotate(err, "failed to fetch GobWatchMaps for %s", hostRepo).Tag(transient.Tag).Err()
	}

	// For each GobWatchMap entity, inspect the Groups to determine which
	// configs apply for the given ref.
	ac := &changelist.ApplicableConfig{}
	// TODO(qyearsley): Set UpdateTime.
	// ApplicableConfig includes a timestamp UpdateTime
	// and Projects, each of which includes Name and
	// ConfigGroupIDs.
	for _, gwm := range gwms {
		ids := []string{}
		for _, g := range gwm.Groups.Groups {
			// TODO(qyearsley): Implement.
			// Fields to use: g.Fallback, g.Include, g.Exclude
			// compare against ref passed in.
			ids = append(ids, g.Id)
		}
		if len(ids) != 0 {
			acp := &changelist.ApplicableConfig_Project{
				Name:           gwm.Project,
				ConfigGroupIds: ids,
			}
			ac.Projects = append(ac.Projects, acp)
		}
	}
	return ac, nil
}
