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

package config

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
)

// Status of LUCI Project Config from CV perspective.
type Status int

const (
	// StatusNotExists means CV doesn't have config for a LUCI prject.
	//
	// The LUCI project itself may exist in LUCI-Config service.
	StatusNotExists Status = iota
	// StatusDisabled means CV has LUCI project's config, but either CV was
	// disabled for the project or the project was deactivated LUCI-wide.
	StatusDisabled
	// StatusEnabled means CV has LUCI project's config and CV is active for the
	// project.
	StatusEnabled
)

// Meta describes LUCI project's config version.
type Meta struct {
	// Project is LUCI Project ID.
	Project string
	// Status is status of the LUCI project config.
	Status Status
	// EVersion allows to compare progression of Project's config.
	//
	// Larger values means later config.
	// If StatusNotExists, the value is 0.
	EVersion int64
	// ConfigGroupIDs are the names of all ConfigGroups in this version.
	//
	// If project doesn't exist, empty.
	// Otherwise, contains at least one group.
	ConfigGroupIDs []ConfigGroupID

	// hashLen is used for extracting hash from ConfigGroupIDs,
	// which all share the same hash prefix.
	hashLen int
}

// Exists returns whether project config exists.
func (m *Meta) Exists() bool {
	return m.Status != StatusNotExists
}

// Hash returns unique identifier of contents of the imported Project config.
//
// Panics if project config doesn't exist.
func (m *Meta) Hash() string {
	if !m.Exists() {
		panic(fmt.Errorf("project %q config doesn't exist", m.Project))
	}
	return string(m.ConfigGroupIDs[0])[:m.hashLen]
}

// GetLatestMeta returns latest metadata for a project.
func GetLatestMeta(ctx context.Context, project string) (Meta, error) {
	m := Meta{
		Project: project,
		Status:  StatusNotExists,
	}
	p := &ProjectConfig{Project: project}
	switch err := datastore.Get(ctx, p); {
	case err == datastore.ErrNoSuchEntity:
		return m, nil
	case err != nil:
		return m, errors.Annotate(err, "failed to get ProjectConfig(project=%q)", project).Tag(transient.Tag).Err()
	case p.Enabled:
		m.Status = StatusEnabled
	default:
		m.Status = StatusDisabled
	}
	m.EVersion = p.EVersion
	m.ConfigGroupIDs = make([]ConfigGroupID, len(p.ConfigGroupNames))
	for i, name := range p.ConfigGroupNames {
		m.ConfigGroupIDs[i] = makeConfigGroupID(p.Hash, name, -1 /* not required */)
	}
	m.hashLen = len(p.Hash)
	return m, nil
}

// GetHashMeta returns metadata for a project for a given config hash.
//
// Doesn't check whether a project currently exists.
// Returns error if specific (project, hash) combo doesn't exist in Datastore.
func GetHashMeta(ctx context.Context, project, hash string) (Meta, error) {
	h := ConfigHashInfo{
		Project: datastore.MakeKey(ctx, projectConfigKind, project),
		Hash:    hash,
	}
	if err := datastore.Get(ctx, &h); err != nil {
		if err != datastore.ErrNoSuchEntity {
			err = transient.Tag.Apply(err)
		}
		return Meta{}, errors.Annotate(err, "failed to get ConfigHashInfo(project=%q @ %q)", project, hash).Err()
	}
	m := Meta{
		Project:        project,
		EVersion:       h.ProjectEVersion,
		Status:         StatusEnabled,
		ConfigGroupIDs: make([]ConfigGroupID, len(h.ConfigGroupNames)),
		hashLen:        len(hash),
	}
	for i, name := range h.ConfigGroupNames {
		m.ConfigGroupIDs[i] = MakeConfigGroupID(hash, name)
	}
	return m, nil
}

// GetConfigGroups loads all ConfigGroups from datastore for this meta.
//
// Meta must correspond to existing project.
func (m Meta) GetConfigGroups(ctx context.Context) ([]*ConfigGroup, error) {
	if !m.Exists() {
		panic(fmt.Errorf("project %q config doesn't exist", m.Project))
	}
	projKey := datastore.MakeKey(ctx, projectConfigKind, m.Project)
	cs := make([]*ConfigGroup, len(m.ConfigGroupIDs))
	for i, c := range m.ConfigGroupIDs {
		cs[i] = &ConfigGroup{
			Project: projKey,
			ID:      c,
		}
	}
	err := datastore.Get(ctx, cs)
	if err == nil {
		return cs, nil
	}
	if merr, ok := err.(errors.MultiError); ok {
		for _, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				// If any ConfigGroup is not found, either all are long gone,
				// or soon will be deleted.
				return nil, errors.Annotate(err, "ConfigGroups for %s @ %s not found", m.Project, m.Hash()).Err()
			}
		}
		err = merr.First()
	}
	return nil, errors.Annotate(err, "failed to get ConfigGroups for %s @ %s", m.Project, m.Hash()).Tag(transient.Tag).Err()
}

// GetConfigGroup loads ConfigGroup from datastore if exists.
//
// To handle non-existing ConfigGroup, use datastore.IsErrNoSuchEntity.
// Doesn't check whether validity or existence of the given LUCI project.
func GetConfigGroup(ctx context.Context, project string, id ConfigGroupID) (*ConfigGroup, error) {
	c := &ConfigGroup{
		Project: datastore.MakeKey(ctx, projectConfigKind, project),
		ID:      id,
	}
	switch err := datastore.Get(ctx, c); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.Annotate(err, "ConfigGroup(%q %q) not found", project, id).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to get ConfigGroup(%q %q)", project, id).Tag(transient.Tag).Err()
	default:
		return c, nil
	}
}
