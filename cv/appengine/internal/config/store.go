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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	pb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/proto"
)

const projectConfigKind string = "ProjectConfig"

// ProjectConfig is the root entity that keeps track of the latest version
// info of the CV config for a LUCI Project. It only contains high-level
// metadata about the config. The actual content of config is stored in the
// `ConfigGroup` entities which can be looked up by `ConfigGroupIDs` field.
type ProjectConfig struct {
	_kind string `gae:"$kind,ProjectConfig"`
	// Project is the name of this LUCI Project.
	Project string `gae:"$id"`
	// Enabled indicates whether CV is enabled for this LUCI Project.
	//
	// Project is disabled if it is de-registered in LUCI Config or it no longer
	// has CV config file.
	Enabled bool
	// UpdatedTime is the latest timestamp when this ProjectConfig is updated.
	UpdatedTime time.Time
	// Version is the version number of this ProjectConfig. It is an auto
	// incremented integer that increments every time a new config change is
	// imported to CV for this LUCI Project. It is primarily used for cleaning
	// up old configs.
	Version int64
	// ConfigHash is the hash of latest CV config file imported from LUCI Config.
	ConfigHash string `gae:",noindex"`
	// ConfigGroupIDs are IDs of all ConfigGroups in the current version of CV
	// Config.
	ConfigGroupIDs []ConfigGroupID `gae:",noindex"`
}

// getAllProjectIDs returns names of all projects available in datastore.
func getAllProjectIDs(ctx context.Context, enabledOnly bool) ([]string, error) {
	var projects []*ProjectConfig
	query := datastore.NewQuery(projectConfigKind).Project("Enabled")
	if err := datastore.GetAll(ctx, query, &projects); err != nil {
		return nil, err
	}
	ret := make([]string, 0, len(projects))
	for _, p := range projects {
		if enabledOnly && !p.Enabled {
			continue
		}
		ret = append(ret, p.Project)
	}
	return ret, nil
}

// ConfigGroupID is the ID for ConfigGroup Entity. It is in the format of
// "hash/name". `hash` corresponds to the `ConfigHash` field in the containing
// `ProjectConfig`. `name` is the value of `ConfigGroup.Name` if specified.
// If not provided (Name in ConfigGroup is optional as of Sep. 2020. See:
// crbug/1063508), use "index_i" as name instead where `i` is the index
// (0-based) of this ConfigGroup in the config.
type ConfigGroupID string

// MkConfigGroupID creates a new ConfigGroupID.
func MkConfigGroupID(configHash string, cg *pb.ConfigGroup, index int) ConfigGroupID {
	name := cg.Name
	if name == "" {
		name = fmt.Sprintf("index_%d", index)
	}
	return ConfigGroupID(fmt.Sprintf("%s/%s", configHash, name))
}

// ConfigGroup is an entity that represents a ConfigGroup defined in a single
// version of CV config. Its parent is `ProjectConfig` entity.
type ConfigGroup struct {
	_kind   string         `gae:"$kind,ProjectConfigGroup"`
	Project *datastore.Key `gae:"$parent"`
	ID      ConfigGroupID  `gae:"$id"`
	// Version corresponds to the `Version` field in containing `ProjectConfig`.
	Version int64
	// CreateTime is the timestamp when this ConfigGroup is created.
	CreateTime time.Time
	// DrainingStartTime represents `draining_start_time` field in the CV config.
	//
	// Note that this is a project-level field. Therefore, all ConfigGroups in a
	// single version of config should have the same value.
	DrainingStartTime string `gae:",noindex"`
	// SubmitOptions represents `submit_options` field in the CV config.
	//
	// Note that this is a project-level field. Therefore, all ConfigGroups in a
	// single version of Config should have the same value.
	SubmitOptions pb.SubmitOptions `gae:",noindex"`
	// Content represents a `pb.ConfigGroup` proto message defined in the CV
	// config
	Content pb.ConfigGroup `gae:",noindex"`
}

func writeConfigGroups(ctx context.Context, cfg *pb.Config, project, configHash string, version int64) ([]ConfigGroupID, error) {
	cgLen := len(cfg.GetConfigGroups())
	if cgLen == 0 {
		return nil, nil
	}

	err := errors.NewLazyMultiError(cgLen)
	projKey := datastore.MakeKey(ctx, projectConfigKind, project)
	ids := make([]ConfigGroupID, cgLen)
	for index, cg := range cfg.ConfigGroups {
		ids[index] = MkConfigGroupID(configHash, cg, index)
		entity := ConfigGroup{
			Project:           projKey,
			ID:                ids[index],
			CreateTime:        clock.Now(ctx),
			Version:           version,
			DrainingStartTime: cfg.DrainingStartTime,
		}
		proto.Merge(&(entity.Content), cg)
		if submitOpts := cfg.SubmitOptions; submitOpts != nil {
			proto.Merge(&(entity.SubmitOptions), submitOpts)
		}
		err.Assign(index, datastore.Put(ctx, &entity))
	}
	return ids, err.Get()
}
