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

package prjmanager

import (
	"time"

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/gae/service/datastore"
)

// ProjectKind is Datastore entity kind for Project.
const ProjectKind = "Project"

// Project is an entity per LUCI Project in Datastore.
type Project struct {
	// $kind must match ProjectKind.
	_kind  string                `gae:"$kind,Project"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// ID is LUCI project name.
	ID string `gae:"$id"`

	// EVersion is entity version. Every update should increment it by 1.
	EVersion int `gae:",noindex"`
	// UpdateTime is exact time of when this entity was last updated.
	//
	// It's not indexed to avoid hot areas in the index.
	UpdateTime time.Time `gae:",noindex"`

	// Status of project manager {STARTED, STOPPING, STOPPED (disabled)}.
	Status Status `gae:",noindex"`
	// ConfigHash is the latest processed Project Config hash.
	ConfigHash string `gae:",noindex"`
	// IncompleteRuns are sorted IDs of Runs which aren't yet complete.
	// ProjectManager is responsible for notifying these Runs of config change.
	IncompleteRuns run.IDs `gae:",noindex"`
}
