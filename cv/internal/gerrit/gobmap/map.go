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
)

// UpdateProjectConfig loads the new config and updates the map accordingly.
func UpdateProjectConfig(ctx context.Context, project string) error {
	// Call ~config.LatestProjectConfig(project) and for each group, update map as
	// needed by computing the diff between the old and new versions of configs.
	return errors.New("not implemented")
}

// Temp. Must be defined in config package.
// project/content_hash/index
type ProjectConfigGroupID string

// Lookup returns ConfigGroup IDs which watch the given combination of Gerrit host,
// repo and ref.
//
// Due to ref_regexp[_exclude] options, CV can't ensure that each possible
// combination is watched by at most one ConfigGroup.
func Lookup(ctx context.Context, host, repo, ref string) ([]ProjectConfigGroupID, error) {
	// Load entity with ID=(host/repo),
	// find and return all config groups with match the ref.
	return nil, errors.New("not implemented")
}
