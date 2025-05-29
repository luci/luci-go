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
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

const (
	// ProjectKind is the Datastore entity kind for Project.
	ProjectKind = "Project"
	// ProjectLogKind is the Datastore entity kind for ProjectLog.
	ProjectLogKind = "ProjectLog"
)

// Project is an entity per LUCI Project in Datastore.
type Project struct {
	// $kind must match ProjectKind.
	_kind  string                `gae:"$kind,Project"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	// ID is LUCI project name.
	ID string `gae:"$id"`

	// EVersion is entity version. Every update should increment it by 1.
	EVersion int64 `gae:",noindex"`
	// UpdateTime is exact time of when this entity was last updated.
	//
	// It's not indexed to avoid hot areas in the index.
	UpdateTime time.Time `gae:",noindex"`

	// State serializes internal Project Manager state.
	//
	// The `LuciProject` field isn't set as it duplicates Project.ID.
	State *prjpb.PState
}

// ProjectStateOffload stores rarely-changed project state duplicated from the
// main Project entity for use in transactions creating Runs.
//
// Although this is already stored in the main Project entity, doing so would
// result in retries of Run creation transactions, since Project entity is
// frequently modified in busy projects.
//
// On the other hand, ProjectStateOffload is highly likely to remain unchanged
// by the time Run creation transaction commits, thus avoiding needless
// retries.
type ProjectStateOffload struct {
	_kind string `gae:"$kind,ProjectRarelyChanged"`

	Project *datastore.Key `gae:"$parent"`
	// ID is alaways the same, set/read only by the datastore ORM.
	ID string `gae:"$id,const"`

	// Status of the project {STARTED, STOPPING, STOPPED (disabled)}.
	Status prjpb.Status `gae:",noindex"`
	// ConfigHash is the latest processed Project Config hash.
	ConfigHash string `gae:",noindex"`

	// UpdateTime is the last time the entity was modifed.
	UpdateTime time.Time `gae:",noindex"`
}

// ProjectLog stores historic state of a project.
type ProjectLog struct {
	_kind string `gae:"$kind,ProjectLog"`

	Project *datastore.Key `gae:"$parent"`
	// EVersion and other fields are the same as the Project & ProjectStateOffload
	// entities written at the same time.
	EVersion   int64        `gae:"$id"`
	Status     prjpb.Status `gae:",noindex"`
	ConfigHash string       `gae:",noindex"`
	UpdateTime time.Time    `gae:",noindex"`
	State      *prjpb.PState

	// Reasons records why this Log entity was written.
	//
	// There can be more than one, all of which will be indexed.
	Reasons []prjpb.LogReason `gae:"reason"`
}

// IncompleteRuns are IDs of Runs which aren't yet completed.
func (p *Project) IncompleteRuns() (ids common.RunIDs) {
	return p.State.IncompleteRuns()
}

// Status returns Project Manager status.
func (p *Project) Status() prjpb.Status {
	return p.State.GetStatus()
}

// ConfigHash returns Project's Config hash.
func (p *Project) ConfigHash() string {
	return p.State.GetConfigHash()
}

// Load loads LUCI project state from Datastore.
//
// If project doesn't exist in Datastore, returns nil, nil.
func Load(ctx context.Context, luciProject string) (*Project, error) {
	p := &Project{ID: luciProject}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to load Project state: %w", err))
	default:
		return p, nil
	}
}
