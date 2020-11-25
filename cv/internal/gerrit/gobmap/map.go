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
	"regexp"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	pb "go.chromium.org/luci/cv/api/config/v2"
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
	gwms := []*gobWatchMap{}
	q := datastore.NewQuery(gobWatchMapKind).Eq("Project", project)
	if err := datastore.GetAll(ctx, q, &gwms); err != nil {
		return errors.Annotate(err, "failed to get GobWatchMap entities for project %q", project).Tag(transient.Tag).Err()
	}

	toPut := []*gobWatchMap{}
	toDelete := []*gobWatchMap{}
	switch meta, err := config.GetLatestMeta(ctx, project); {
	case err != nil:
		return err
	case !meta.Exists():
		// The project was removed, delete everything.
		toDelete = gwms
	default:
		cgs, err := meta.GetConfigGroups(ctx)
		hash := meta.Hash()
		if err != nil {
			// In most cases, this is due to transient error.
			return errors.Annotate(err, "failed to get config groups for %q, hash %q",
				project, hash).Tag(transient.Tag).Err()
		}
		// Compute the entities to put or delete.
		toPut, toDelete = listUpdates(ctx, gwms, cgs, hash, project)
	}

	if err := datastore.Delete(ctx, toDelete); err != nil {
		return errors.Annotate(err, "failed to delete %d GobWatchMap entities when updating project %q",
			len(toDelete), project).Tag(transient.Tag).Err()
	}
	if err := datastore.Put(ctx, toPut); err != nil {
		return errors.Annotate(err, "failed to put %d GobWatchMap entities when updating project %q",
			len(toPut), project).Tag(transient.Tag).Err()
	}

	return nil
}

// listUpdates determines what needs to be updated.
//
// It computes which of the existing GobWatchMap entities should be
// removed, and which GobWatchMap entities should be put (added or updated).
func listUpdates(ctx context.Context, gwms []*gobWatchMap, latestConfigGroups []*config.ConfigGroup,
	latestHash, project string) (toPut, toDelete []*gobWatchMap) {
	// Make a map of host/repo to config hashes for currently
	// existing GobWatchMap entities; used below.
	existingHashes := make(map[string]string)
	for _, gwm := range gwms {
		existingHashes[gwm.Parent.StringID()] = gwm.ConfigHash
	}

	// List `internal.Groups` present in the latest config groups.
	hostRepoToGroups := internalGroups(latestConfigGroups)

	// List GobWatchMaps to put; these are those either have
	// no existing hash yet or a different existing hash.
	toPut = []*gobWatchMap{}
	for hostRepo, groups := range hostRepoToGroups {
		if hash, ok := existingHashes[hostRepo]; !ok || hash != latestHash {
			gwm := &gobWatchMap{
				ID:         project,
				Project:    project,
				Parent:     datastore.MakeKey(ctx, parentKind, hostRepo),
				Groups:     groups,
				ConfigHash: latestHash,
			}
			toPut = append(toPut, gwm)
		}
	}

	// List GobWatchMaps to delete; these are either those that
	// currently exist but have no groups in the latest config.
	toDelete = []*gobWatchMap{}
	for _, gwm := range gwms {
		hostRepo := gwm.Parent.StringID()
		if _, ok := hostRepoToGroups[hostRepo]; !ok {
			toDelete = append(toDelete, gwm)
		}
	}

	return toPut, toDelete
}

// internalGroups converts config.ConfigGroups to internal.Groups.
//
// It returns a map of host/repo to internal.Groups.
func internalGroups(configGroups []*config.ConfigGroup) map[string]*internal.Groups {
	ret := make(map[string]*internal.Groups)
	for _, g := range configGroups {
		for _, gerrit := range g.Content.Gerrit {
			// Gerrit hosts are assumed to always use https.
			host := strings.TrimPrefix(gerrit.Url, "https://")
			host = strings.TrimSuffix(host, "/")
			for _, p := range gerrit.Projects {
				hostRepo := host + "/" + p.Name
				group := &internal.Group{
					Id:       string(g.ID),
					Include:  p.RefRegexp,
					Exclude:  p.RefRegexpExclude,
					Fallback: g.Content.Fallback == pb.Toggle_YES,
				}
				if groups, ok := ret[hostRepo]; ok {
					groups.Groups = append(groups.Groups, group)
				} else {
					ret[hostRepo] = &internal.Groups{
						Groups: []*internal.Group{group},
					}
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
	ac.UpdateTime = timestamppb.New(clock.Now(ctx).UTC())
	for _, gwm := range gwms {
		fallbackIds := []string{}
		ids := []string{}
		for _, g := range gwm.Groups.Groups {
			if matchesAny(g.Include, ref) && !matchesAny(g.Exclude, ref) {
				if g.Fallback {
					fallbackIds = append(fallbackIds, g.Id)
				} else {
					ids = append(ids, g.Id)
				}
			}
		}
		// If there are two groups that match and one of them has fallback
		// set to true, then the one with Fallback set to false is the one
		// to use.
		if len(ids) == 0 && len(fallbackIds) > 0 {
			ids = fallbackIds
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

// matchesAny returns true iff s matches any of the patterns.
//
// It is assumed that all patterns have been pre-validated and
// are valid regexps.
func matchesAny(patterns []string, s string) bool {
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if re.MatchString(s) {
			return true
		}
	}
	return false

}
