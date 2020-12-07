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

package impl

import (
	"time"

	"go.chromium.org/luci/gae/service/datastore"
)

// project is an entity per LUCI Project in Datastore.
type project struct {
	// $kind must match prjmanager.ProjectEntityKind.
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
}
