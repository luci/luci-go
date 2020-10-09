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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

// For each host/repo combination there is a separate GobWatchMap.
//
// A GobWatchMap entity stores the updated time, and is also the parent
// of all of the GobWatchMapProject entities which include LUCI project?
type GobWatchMap struct {
	_kind string `gae:"$kind,GobWatchMap"`

	// A string "<host>/<repo>" where host is the Gerrit host and repo is the
	// Gerrit project, e.g. "chromium-review.googlesource.com/chromium/src".
	// The schema of the Gerrit host is not included; it is always assumed to
	// be https.
	ID string `gae:"$id"`

	// The time that this was last updated.
	UpdatedTime time.Time `gae:",noindex"`
}

// GobWatchMapProject contains a mapping used to look up config group IDs
// for a particular LUCI project and host/repo combination.
type GobWatchMapProject struct {
	_kind string `gae:"$kind,GobWatchMapProject"`

	// LUCI project name.
	ID string `gae:"$id"`

	// GobWatchMap key. This includes host and repo.
	Parent *datastore.Key `gae:"$parent"`

	// LUCI project name.
	//
	// This is used when querying for entities to update or remove in the
	// update operation.
	// TODO(qyearsley): Determine whether this is needed, and remove it
	// if querying with a filter on ID doesn't work.
	Project string

	// Each config group has a name, and a list of include ref regexps and
	// exclude ref regexps. The config group that's returned is the first one
	// where the ref matches an include ref regexp but does not match any
	// exclude ref regexp.
	// We need the ref regexps to determine which group matches, and we
	// need the group name to construct a config group ID.
	// TODO(qyearsley): Add RefSpecGroupMap when it's checked in

	// ConfigHash is the hash of latest CV config file imported from LUCI Config;
	// this is updated based on ProjectConfig entity.
	ConfigHash string `gae:",noindex"`
}

// UpdateProjectConfig loads the new config and updates the map accordingly.
//
// This may include adding, removing and modifying entities.
func UpdateProjectConfig(ctx context.Context, project string) error {
	// TODO(qyearsley): Implement.
	// - Fetch the current state of the map by querying for all
	//   GobWatchMapProject entities for the LUCI project.
	// - Get the latest config from datastore (using the config package).
	// - Determine which GobWatchMapProject entities need to be modified,
	//   removed, or added.
	// - Note that for each config group in the config, there is a host, repo,
	//   and a ref specification. Specifically, if cg is the ConfigGroup,
	//   cg.Gerrit.Url is the host (plus schema), cg.Gerrit.Projects is a list
	//   of Gerrit Project messages; within each of those messages is Name
	//   (i.e. repo), RefRegexp, and RefRegexpExclude). field, which has Url
	//   field, which is the Gerrit host plus schema.
	return errors.New("not implemented")
}

// TODO(qyearsley): Update/remove this when this is in the config package.
// This is expected to be "hash/name", where hash is the config hash for
// the LUCI project config that contains the config group.
type ConfigGroupID string

// Lookup returns config group IDs which watch the given combination of Gerrit
// host, repo and ref.
//
// For example: the input might be ("chromium-review.googlesource.com",
// "chromium/src", "refs/heads/main"), and the output might be 0 or 1 or
// multiple IDs which can be used to fetch config groups.
//
// Due to the ref_regexp[_exclude] options, CV can't ensure that each possible
// combination is watched by at most one ConfigGroup, which is why this may
// return multiple ConfigGroupIDs.
func Lookup(ctx context.Context, host, repo, ref string) ([]ConfigGroupID, error) {
	// TODO(qyearsley): Implement:
	// (1) Load the all GobWatchMapProject entities for the given host and
	// repo, by doing an ancestor query for GobWatchMapProject with ancestor
	// (GobWatchMap, "host/repo").
	// (2) For each entity, inspect the Projects field to decide which apply to
	// the given ref. By this means, build up a list of config group IDs.
	return nil, errors.New("not implemented")
}
