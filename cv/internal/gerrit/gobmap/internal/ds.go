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

// Package internal contains GobMap storage structs.
package internal

import (
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
	RefSpecGroupMap *RefSpecGroupMap

	// ConfigHash is the hash of latest CV config file imported from LUCI Config;
	// this is updated based on ProjectConfig entity.
	ConfigHash string `gae:",noindex"`
}
