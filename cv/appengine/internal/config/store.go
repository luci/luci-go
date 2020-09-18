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
	"strconv"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/retry/transient"
	pb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/proto"
)

const (
	projectConfigKind string = "ProjectConfig"
	revisionKind      string = "Revision"
	// currentSchemaVersion is the version number of the datastore schema of
	// Change Verifier config. Bumping up this number will force CV to save
	// all configs again with the updated schema even if there's no config
	// content change.
	currentSchemaVersion int = 1
)

// ProjectConfig is the root entities that keeps track of the current revision
// info of Change Verifier config for a LUCI Project.
type ProjectConfig struct {
	_kind string `gae:"$kind,ProjectConfig"`
	// Project is the name of LUCI Project.
	Project string `gae:"$id"`
	// Enabled indicates whether Change Verifier is enabled for this LUCI Project.
	// Project is disabled if it is de-registered in LUCI Config or it removes
	// its Change Verifier config.
	Enabled bool `gae:"enabled"`
	// LatestRevision is the content hash of most recent Change Verifier config
	// CV polled from LUCI Config.
	LatestRevision string `gae:"latest_rev"`
	// UpdatedTime is the timestamp when `LatestRevision` is updated
	UpdatedTime time.Time `gae:"updated_time,noindex"`
	// GitSHA is git SHA1 of the repository where the latest config was fetched
	// from.
	GitSHA string `gae:"git_sha,noindex"`
	// SchemaVersion is the version number of the datastore schema of Change
	// Verifier config.
	SchemaVersion int `gae:"schema_version,noindex"`
	// DrainingStartTime is the timestamp when CV starts to refrain from
	// processing any CLs watched by this LUCI Project.
	//
	// Technically, this should also be version controlled. However, the history
	// value is useless for CV. CV just need the latest value and act based on
	// that. On the other hand, this option is very likely to be deprecated in
	// the future as we switch to an RPC-based start/stop of a LUCI Project.
	DrainingStartTime string `gae:"draining_start_time,noindex"`
}

// getAllProjectIDs returns names of all projects available in datastore.
func getAllProjectIDs(ctx context.Context, enabledOnly bool) ([]string, error) {
	keys := []*datastore.Key{}
	query := datastore.NewQuery(projectConfigKind).
		FirestoreMode(true).
		KeysOnly(true)
	if enabledOnly {
		query = query.Eq("enabled", true)
	}
	if err := datastore.GetAll(ctx, query, &keys); err != nil {
		return nil, transient.Tag.Apply(err)
	}
	ret := make([]string, len(keys))
	for i, key := range keys {
		ret[i] = key.StringID()
	}
	return ret, nil
}

func putProjectConfig(ctx context.Context, project, revision, gitSHA string, enabled bool, cfg *pb.Config) error {
	pc := ProjectConfig{
		Project:           project,
		Enabled:           enabled,
		UpdatedTime:       clock.Now(ctx),
		LatestRevision:    revision,
		GitSHA:            gitSHA,
		SchemaVersion:     currentSchemaVersion,
		DrainingStartTime: cfg.GetDrainingStartTime(),
	}
	if err := datastore.Put(ctx, &pc); err != nil {
		return transient.Tag.Apply(err)
	}
	return nil
}

// ConfigGroup is an entity that represents a ConfigGroup defined in a single
// revision of Change Verifier config file for a LUCI Project. Its parent
// entity is `Revision` (doesn't exists in datastore) whose key is the content
// hash of that file and its grand parent is `ProjectConfig` entity.
type ConfigGroup struct {
	_kind    string         `gae:"$kind,ConfigGroup"`
	Revision *datastore.Key `gae:"$parent"`
	// Index is 0-based index number of this ConfigGroup in the Change Verifier
	// config. It is a string value because using the value zero (integer) for
	// ID will get an automatically allocated ID assigned.
	Index string `gae:"$id"`
	// Content is the `pb.ConfigGroup` proto message in the config.
	Content pb.ConfigGroup `gae:"config_group,noindex"`
	// SubmitOptions is the `pb.SubmitOptions` proto message in the config.
	SubmitOptions pb.SubmitOptions `gae:"submit_options,noindex"`
}

func putConfigGroups(ctx context.Context, project, revision string, cfg *pb.Config) error {
	revisionKey := datastore.MakeKey(ctx, projectConfigKind, project, revisionKind, revision)
	cgs := constructConfigGroups(cfg, revisionKey)
	return datastore.Put(ctx, cgs)
}

func constructConfigGroups(cfg *pb.Config, revisionKey *datastore.Key) []ConfigGroup {
	ret := make([]ConfigGroup, len(cfg.GetConfigGroups()))
	for i, cg := range cfg.GetConfigGroups() {
		ret[i] = ConfigGroup{
			Revision: revisionKey,
			Index:    strconv.Itoa(i),
		}
		proto.Merge(&(ret[i].Content), cg)
		if submitOpts := cfg.GetSubmitOptions(); submitOpts != nil {
			proto.Merge(&(ret[i].SubmitOptions), submitOpts)
		}
	}
	return ret
}
