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
	"errors"

	pb "go.chromium.org/luci/cv/api/config/v2"
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

// ProjectGobWatchMap contains a mapping used to look up config group IDs
// for a particular LUCI project.
type ProjectGobWatchMap struct {
	_kind string `gae:"$kind,ProjectGobWatchMap"`

	// LUCI project name.
	ID string `gae:"$id"`

	// Each config group may have a list of include ref regexps and exclude ref
	// regexps. The config group that's returned is the first one where the ref
	// matches an include ref regexp but does not match any exclude ref regexp.
	//
	// A list of Project proto messages with Gerrit project information. Each
	// contains Name (i.e. Gerrit repo name) RefRegexp, and RefRegexpExclude.
	Projects []*pb.ProjectConfig_Gerrit_Project

	// ConfigHash is the hash of latest CV config file imported from LUCI Config;
	// this is updated based on ProjectConfig entity.
	ConfigHash string `gae:",noindex"`
}

// UpdateProjectConfig loads the new config and updates the map accordingly.
func UpdateProjectConfig(ctx context.Context, project string) error {
	// 1. Get the latest config from datastore (using the config package).
	// 2. For each config group in the config, there is a host, repo, and
	//    a ref specification. Specifically, if cg is the ConfigGroup,
	//    cg.Gerrit.Url is the host (plus schema), cg.Gerrit.Projects is
	//    a list of Gerrit Project messages; within each of those messages
	//    is Name (i.e. repo), RefRegexp, and RefRegexpExclude).
	//    field, which has Url field, which is the Gerrit host plus schema.

	// For
	// - Get or create the relevant GobWatchMap

	return errors.New("not implemented")
}

// stripSchema returns a hostname given a url which may include a
// schema part (e.g. "https://")
func stripSchema(url string) string {
	return strings.TrimPrefix("https://")
}

// TODO(qyearsley): Update/remove this when this is in the config package.
// This is expected to be "hash/name".
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
	// Load the all GobWatchMapProject entities for the given host and repo;
	// then iterate through them, and decide which apply to the given ref.
	hostRepo := host + "/" + repo
	parentKey := datastore.NewKey(ctx, "GobWatchMap", id, 0, nil)
	q := datastore.NewQuery("GobWatchMapProject").Ancestor(parentKey)
	var projects []*GobWatchMapProject
	if err := datastore.GetAll(ctx, q, &all); err != nil {
		return nil, errors.Reason("Error while fetching for %q", hostRepo).Err()
	}
	// Load
	return nil, errors.New("not implemented")
}
