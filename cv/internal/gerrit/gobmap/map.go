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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common/lease"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
)

const (
	mapKind           = "MapPart"
	parentKind        = "MapPartParent"
	maxUpdateDuration = 60 * time.Second
)

// mapPart contains config groups for a particular LUCI project and host/repo
// combination.
//
// MapPart entities are stored with a parent key of the form (MapPartParent,
// host/repo), so that all mapPart entities with a particular host/repo can be
// fetched with an ancestor query; the goal is to have fast reads by host/repo.
//
// The MapPart entities as a whole store a mapping used to lookup which host,
// repo and ref maps to which config group; the map is updated when a project
// config is updated.
type mapPart struct {
	// TODO(tandrii): s/MapPart/gobmap.MapPart, since "MapPart" is too generic.
	_kind string `gae:"$kind,MapPart"`

	// The ID of this MapPart, which is the LUCI project name.
	ID string `gae:"$id"`

	// LUCI project name.
	//
	// This field contains the same value as ID, and is included so
	// that we can index on it, and thus filter on it in queries.
	Project string

	// MapPartParent key. This parent has an ID of the form "host/repo".
	Parent *datastore.Key `gae:"$parent"`

	// Groups keeps config groups of a LUCI project applicable to this
	// host/repo.
	Groups *cfgmatcher.Groups

	// ConfigHash is the hash of latest CV project config imported from LUCI
	// Config; this is updated based on ProjectConfig entity.
	ConfigHash string `gae:",noindex"`
}

// Update updates the gob map entities according to the given project config.
//
// This may include adding, removing and modifying entities, which is not done
// atomically.
// Changes to individual Gerrit repos are atomic.  This means that
// IF Update() is in progress from config versions 1 to 2, identified by
//
//	hashes h1 and h2, respectively,
//
// AND both h1 and h2 watch specific Gerrit repo, possibly among many others,
// THEN a concurrent Lookup(host,repo,...) is guaranteed to return either
// based on @h1 or based on @h2.
//
// However, there is no atomicity across entire project config. This means that
// IF Update() is in progress from config versions 1 to 2, identified by
//
//	hashes h1 and h2, respectively,
//
// THEN two sequential calls to Lookup with different Gerrit repos may return
// results based on @h2 at first and @h1 for the second, ie:
//
//	Lookup(host1,repoA,...) -> @h2
//	Lookup(host1,repoB,...) -> @h1
//
// Thus, a failed Update() may leave gobmap in a corrupted state, whereby some
// Gerrit repos may represent old and some new config versions. In such a
// case it's important that Update() caller retries as soon as possible.
//
// TODO(crbug.com/1179286): make Update() incorruptible.
// See TestGobMapConcurrentUpdates which reproduces corruption.
//
// Update is idempotent.
func Update(ctx context.Context, meta *prjcfg.Meta, cgs []*prjcfg.ConfigGroup) error {
	ctx, cleanup, err := leaseExclusive(ctx, meta)
	if err != nil {
		return err
	}
	defer cleanup()
	return update(ctx, meta, cgs)
}

// getAll returns the gob map entities matching given host and repo.
func getAll(ctx context.Context, host, repo string) ([]*mapPart, error) {
	hostRepo := host + "/" + repo
	parentKey := datastore.MakeKey(ctx, parentKind, hostRepo)
	q := datastore.NewQuery(mapKind).Ancestor(parentKey)
	mps := []*mapPart{}
	if err := datastore.GetAll(ctx, q, &mps); err != nil {
		return nil, errors.Annotate(err, hostRepo).Tag(transient.Tag).Err()
	}
	return mps, nil
}

// leaseExclusive obtains exclusive lease on the gob map for the LUCI project.
//
// Returns time-limited context for the duration of the lease and a cleanup
// function.
func leaseExclusive(ctx context.Context, meta *prjcfg.Meta) (context.Context, func(), error) {
	taskID := "unknown"
	if info := tq.TaskExecutionInfo(ctx); info != nil {
		taskID = info.TaskID
	}
	l, err := lease.Apply(ctx, lease.Application{
		ResourceID: lease.ResourceID("gobmap/" + meta.Project),
		ExpireTime: clock.Now(ctx).Add(maxUpdateDuration),
		Holder:     taskID, // Used for debugging, only.
	})
	if err != nil {
		var alreadyInLeaseErr *lease.AlreadyInLeaseErr
		if errors.As(err, &alreadyInLeaseErr) {
			return nil, nil, transient.Tag.Apply(errors.Fmt("gobmap for %s is already being updated: %w", meta.Project, err))
		}
		return nil, nil, err
	}
	limitedCtx, cancel := clock.WithDeadline(ctx, l.ExpireTime)
	cleanup := func() {
		cancel()
		if err := l.Terminate(ctx); err != nil {
			// Best-effort termination since lease will expire naturally.
			logging.Warningf(ctx, "failed to cancel gobmap Update lease: %s", err)
		}
	}
	return limitedCtx, cleanup, nil
}

// update updates gob map Datastore entities.
func update(ctx context.Context, meta *prjcfg.Meta, cgs []*prjcfg.ConfigGroup) error {
	var toPut, toDelete []*mapPart

	// Fetch stored GWM entities.
	mps := []*mapPart{}
	q := datastore.NewQuery(mapKind).Eq("Project", meta.Project)
	if err := datastore.GetAll(ctx, q, &mps); err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to get MapPart entities for project %q: %w", meta.Project, err))
	}

	if meta.Status != prjcfg.StatusEnabled {
		// The project was disabled or removed, delete everything.
		toDelete = mps
	} else {
		toPut, toDelete = listUpdates(ctx, mps, cgs, meta.Hash(), meta.Project)
	}

	if err := datastore.Delete(ctx, toDelete); err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to delete %d MapPart entities when updating project %q: %w",
			len(toDelete), meta.Project, err))

	}
	if err := datastore.Put(ctx, toPut); err != nil {
		return transient.Tag.Apply(errors.Fmt("failed to put %d MapPart entities when updating project %q: %w",
			len(toPut), meta.Project, err))

	}
	return nil
}

// listUpdates determines what needs to be updated.
//
// It computes which of the existing MapPart entities should be
// removed, and which MapPart entities should be put (added or updated).
func listUpdates(ctx context.Context, mps []*mapPart, latestConfigGroups []*prjcfg.ConfigGroup,
	latestHash, project string) (toPut, toDelete []*mapPart) {
	// Make a map of host/repo to config hashes for currently
	// existing MapPart entities; used below.
	existingHashes := make(map[string]string, len(mps))
	for _, mp := range mps {
		hostRepo := mp.Parent.StringID()
		existingHashes[hostRepo] = mp.ConfigHash
	}

	// List `internal.Groups` present in the latest config groups.
	hostRepoToGroups := internalGroups(latestConfigGroups)

	// List MapParts to put; these are those either have
	// no existing hash yet or a different existing hash.
	for hostRepo, groups := range hostRepoToGroups {
		if existingHashes[hostRepo] == latestHash {
			// Already up to date.
			continue
		}
		mp := &mapPart{
			ID:         project,
			Project:    project,
			Parent:     datastore.MakeKey(ctx, parentKind, hostRepo),
			Groups:     groups,
			ConfigHash: latestHash,
		}
		toPut = append(toPut, mp)
	}

	// List MapParts to delete; these are those that currently exist but
	// have no groups in the latest config.
	toDelete = []*mapPart{}
	for _, mp := range mps {
		hostRepo := mp.Parent.StringID()
		if _, ok := hostRepoToGroups[hostRepo]; !ok {
			toDelete = append(toDelete, mp)
		}
	}

	return toPut, toDelete
}

// internalGroups converts config.ConfigGroups to cfgmatcher.Groups.
//
// It returns a map of host/repo to cfgmatcher.Groups.
func internalGroups(configGroups []*prjcfg.ConfigGroup) map[string]*cfgmatcher.Groups {
	ret := make(map[string]*cfgmatcher.Groups)
	for _, g := range configGroups {
		for _, gerrit := range g.Content.Gerrit {
			host := prjcfg.GerritHost(gerrit)
			for _, p := range gerrit.Projects {
				hostRepo := host + "/" + p.Name
				group := cfgmatcher.MakeGroup(g, p)
				if groups, ok := ret[hostRepo]; ok {
					groups.Groups = append(groups.Groups, group)
				} else {
					ret[hostRepo] = &cfgmatcher.Groups{Groups: []*cfgmatcher.Group{group}}
				}
			}
		}
	}
	return ret
}

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
//
// Always returns non-nil object, even if there are no watching projects.
func Lookup(ctx context.Context, host, repo, ref string) (*changelist.ApplicableConfig, error) {
	// Fetch all MapPart entities for the given host and repo.
	mps, err := getAll(ctx, host, repo)
	if err != nil {
		return nil, errors.Fmt("failed to fetch MapParts: %w", err)
	}

	// For each MapPart entity, inspect the Groups to determine which configs
	// apply for the given ref.
	ac := &changelist.ApplicableConfig{}
	for _, mp := range mps {
		if groups := mp.Groups.Match(ref); len(groups) != 0 {
			ids := make([]string, len(groups))
			for i, g := range groups {
				ids[i] = g.GetId()
			}
			ac.Projects = append(ac.Projects, &changelist.ApplicableConfig_Project{
				Name:           mp.Project,
				ConfigGroupIds: ids,
			})
		}
	}
	return ac, nil
}

// LookupProjects returns all the LUCI projects that have at least one
// applicable config for a given host and repo.
//
// Returns a sorted slice with a unique set of the LUCI projects.
func LookupProjects(ctx context.Context, host, repo string) ([]string, error) {
	// Fetch all MapPart entities for the given host and repo.
	mps, err := getAll(ctx, host, repo)
	if err != nil {
		return nil, errors.Fmt("failed to fetch MapParts: %w", err)
	}
	prjs := stringset.New(len(mps))
	for _, mp := range mps {
		prjs.Add(mp.Project)
	}
	return prjs.ToSortedSlice(), nil
}
