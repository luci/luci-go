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

package prjcfg

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
	// ConfigGroupNames are the names part of all ConfigGroups in this version.
	//
	// If project doesn't exist, empty.
	// Otherwise, contains at least one group.
	ConfigGroupNames []string
	// ConfigGroupIDs are the standalone IDs of all ConfigGroups in this version.
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
		return m, transient.Tag.Apply(errors.Fmt("failed to get ProjectConfig(project=%q): %w", project, err))
	case p.Enabled:
		m.Status = StatusEnabled
	default:
		m.Status = StatusDisabled
	}
	m.EVersion = p.EVersion
	m.ConfigGroupNames = p.ConfigGroupNames
	m.ConfigGroupIDs = make([]ConfigGroupID, len(p.ConfigGroupNames))
	for i, name := range p.ConfigGroupNames {
		m.ConfigGroupIDs[i] = MakeConfigGroupID(p.Hash, name)
	}
	m.hashLen = len(p.Hash)
	return m, nil
}

// GetHashMeta returns metadata for a project for a given config hash.
//
// Doesn't check whether a project currently exists.
// Returns error if specific (project, hash) combo doesn't exist in Datastore.
func GetHashMeta(ctx context.Context, project, hash string) (Meta, error) {
	switch metas, err := GetHashMetas(ctx, project, hash); {
	case err != nil:
		return Meta{}, err
	default:
		return metas[0], nil
	}
}

// GetHashMetas returns a metadata for each given config hash.
//
// Doesn't check whether a project currently exists.
// Returns error if any (project, hash) combo doesn't exist in Datastore.
func GetHashMetas(ctx context.Context, project string, hashes ...string) ([]Meta, error) {
	infos := make([]ConfigHashInfo, len(hashes))
	projKey := datastore.MakeKey(ctx, projectConfigKind, project)
	for i, hash := range hashes {
		infos[i] = ConfigHashInfo{
			Project: projKey,
			Hash:    hash,
		}
	}
	if err := datastore.Get(ctx, infos); err != nil {
		if !datastore.IsErrNoSuchEntity(err) {
			err = transient.Tag.Apply(err)
		}
		return nil, errors.Fmt("failed to load ConfigHashInfo(project=%q @ %s): %w", project, hashes, err)
	}
	metas := make([]Meta, len(infos))
	for i, info := range infos {
		metas[i] = Meta{
			Project:          project,
			EVersion:         info.ProjectEVersion,
			Status:           StatusEnabled,
			ConfigGroupNames: info.ConfigGroupNames,
			ConfigGroupIDs:   make([]ConfigGroupID, len(info.ConfigGroupNames)),
			hashLen:          len(info.Hash),
		}
		for j, name := range info.ConfigGroupNames {
			metas[i].ConfigGroupIDs[j] = MakeConfigGroupID(info.Hash, name)
		}
	}
	return metas, nil
}

// GetConfigGroups loads all ConfigGroups from datastore for this meta.
//
// Meta must correspond to an existing project.
func (m Meta) GetConfigGroups(ctx context.Context) ([]*ConfigGroup, error) {
	if !m.Exists() {
		return nil, fmt.Errorf("project %q config doesn't exist", m.Project)
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
				return nil, errors.Fmt("ConfigGroups for %s @ %s not found: %w", m.Project, m.Hash(), err)
			}
		}
		err = merr.First()
	}
	return nil, transient.Tag.Apply(errors.WrapIf(err, "failed to get ConfigGroups for %s @ %s", m.Project, m.Hash()))
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
		return nil, errors.Fmt("ConfigGroup(%q %q) not found: %w", project, id, err)
	case err != nil:
		return nil, transient.Tag.Apply(errors.Fmt("failed to get ConfigGroup(%q %q): %w", project, id, err))
	default:
		return c, nil
	}
}
