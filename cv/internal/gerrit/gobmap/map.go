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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// GobWatchMap contains config groups for a particular LUCI project and
// host/repo combination.
//
// GobWatchMap entities are stored with a parent key of the form
// (GobWatchMapParent, host/repo), so that all GobWatchMap entities with a
// particular host/repo can be fetched with an ancestor query; the goal
// is to have fast reads by host/repo.
//
// The GobWatchMap entities as a whole store data used to lookup which
// host/repo/ref maps to which config group, and the map is updated when a
// project config is updated.
type GobWatchMap struct {

	// The ID of this GobWatchMap, which contains (host, repo, project).
	ID string `gae:"$id"`

	// LUCI project name.
	//
	// This is used when querying for entities to update or remove in the
	// update operation.
	Project string

	// GobWatchMapParent key. This parent has an ID of the form "host/repo".
	Parent *datastore.Key `gae:"$parent"`

	// Each config group has a name, and a list of include ref regexps and
	// exclude ref regexps. The config group that's returned is the first one
	// where the ref matches an include ref regexp but does not match any
	// exclude ref regexp.
	//
	// We need the ref regexps to determine which group matches, and we need
	// the group name to construct a config group ID.
	// TODO(qyearsley): Add RefSpecGroupMap when it's checked in.

	// ConfigHash is the hash of latest CV config file imported from LUCI Config;
	// this is updated based on ProjectConfig entity.
	ConfigHash string `gae:",noindex"`
}

// Update loads the new config and updates the gob map entities
// accordingly.
//
// This may include adding, removing and modifying entities.
func Update(ctx context.Context, project string) error {
	// TODO(qyearsley): Implement.
	// 1. Fetch the current state of the map by querying for all
	//    GobWatchMap entities for the LUCI project.
	// 2. Get the latest config from datastore (using the config package).
	// 3. Determine which GobWatchMap entities need to be modified,
	//    removed, or added.
	// Note: for each config group in the config, there is a host, repo,
	//    and a ref specification. Specifically, if cg is the ConfigGroup,
	//    cg.Gerrit.Url is the host (plus schema), cg.Gerrit.Projects is a list
	//    of Gerrit Project messages; within each of those messages is Name
	//    (i.e. repo), RefRegexp, and RefRegexpExclude). field, which has Url
	//    field, which is the Gerrit host plus schema.
	return errors.New("not implemented")
}

// TODO(qyearsley): Update when this is in the config package. This is expected
// to be "project/hash/name", where project is the LUCI project, hash is the
// config hash for the LUCI project config that contains the config group, and
// name is the config group name.
type ProjectConfigGroupID string

// Lookup returns config group IDs which watch the given combination of Gerrit
// host, repo and ref.
//
// For example: the input might be ("chromium-review.googlesource.com",
// "chromium/src", "refs/heads/main"), and the output might be 0 or 1 or
// multiple IDs which can be used to fetch config groups.
//
// Due to the ref_regexp[_exclude] options, CV can't ensure that each possible
// combination is watched by at most one ConfigGroup, which is why this may
// return multiple ProjectConfigGroupIDs.
func Lookup(ctx context.Context, host, repo, ref string) ([]ProjectConfigGroupID, error) {
	// TODO(qyearsley): Implement:
	// 1. Fetch all GobWatchMap entities for the given host and repo.
	//    This should be done with a ancestor query for a host/repo.
	// 2. For each entity, inspect the RefSpecMapGroup to determine
	//    which configs apply.
	return nil, errors.New("not implemented")
}
