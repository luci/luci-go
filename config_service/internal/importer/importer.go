// Copyright 2023 The LUCI Authors.
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

// Package importer handles all configs importing.
package importer

import (
	"context"

	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/api/gitiles"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/internal/taskpb"
)

const (
	// projRegistryFilePath is the path of luci-config self config file where it
	// stores a list of registered projects.
	projRegistryFilePath = "projects.cfg"
)

func init() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        "import-configs",
		Kind:      tq.NonTransactional,
		Prototype: (*taskpb.ImportConfigs)(nil),
		Queue:     "backend-v2",
		Handler: func(ctx context.Context, payload proto.Message) error {
			return errors.Reason("import-configs handler hasn't implemented").Err()
		},
	})
}

// ImportAllConfigs schedules a task for each service and project config set to
// import configs from Gitiles and clean up stale config sets.
func ImportAllConfigs(ctx context.Context) error {
	cfgLoc, _ := getGlobalConfigLoc()

	// Get all config sets.
	cfgSets, err := getAllServiceCfgSets(ctx, cfgLoc)
	if err != nil {
		return errors.Annotate(err, "failed to load service config sets").Err()
	}
	sets, err := getAllProjCfgSets(ctx)
	if err != nil {
		return errors.Annotate(err, "failed to load project config sets").Err()
	}
	cfgSets = append(cfgSets, sets...)

	// Enqueue tasks
	err = parallel.WorkPool(8, func(workCh chan<- func() error) {
		for _, cs := range cfgSets {
			cs := cs
			workCh <- func() error {
				err := tq.AddTask(ctx, &tq.Task{
					Payload: &taskpb.ImportConfigs{ConfigSet: cs}},
				)
				return errors.Annotate(err, "failed to enqueue ImportConfigs task for %q: %s", cs, err).Err()
			}
		}
	})
	if err != nil {
		return err
	}

	// Delete stale config sets.
	var keys []*datastore.Key
	if err := datastore.GetAll(ctx, datastore.NewQuery(model.ConfigSetKind).KeysOnly(true), &keys); err != nil {
		return errors.Annotate(err, "failed to fetch all config sets from Datastore").Err()
	}
	cfgSetsInDB := stringset.New(len(keys))
	for _, key := range keys {
		cfgSetsInDB.Add(key.StringID())
	}
	cfgSetsInDB.DelAll(cfgSets)
	if len(cfgSetsInDB) > 0 {
		logging.Infof(ctx, "deleting stale config sets: %v", cfgSetsInDB)
		var toDel []*datastore.Key
		for _, cs := range cfgSetsInDB.ToSlice() {
			toDel = append(toDel, datastore.KeyForObj(ctx, &model.ConfigSet{ID: config.Set(cs)}))
		}
		return datastore.Delete(ctx, toDel)
	}
	return nil
}

// getAllProjCfgSets fetches all "projects/*" config sets
func getAllProjCfgSets(ctx context.Context) ([]string, error) {
	file, err := model.GetLatestConfigFile(ctx, config.MustServiceSet(info.AppID(ctx)), projRegistryFilePath)
	switch {
	case errors.Contains(err, datastore.ErrNoSuchEntity):
		// May happen on the cron job first run. Just log the warning.
		logging.Warningf(ctx, "failed to compose all project configsets: %s", err)
		return nil, nil
	case err != nil:
		return nil, err
	}

	content := file.Content
	// TODO(crbug.com/1446839): if file.Content is nil, download from GCS url;
	projCfg := &cfgcommonpb.ProjectsCfg{}
	if err := prototext.Unmarshal(content, projCfg); err != nil {
		return nil, errors.Annotate(err, "failed to unmarshal %s file content", projRegistryFilePath).Err()
	}
	cfgsets := make([]string, len(projCfg.Projects))
	for i, proj := range projCfg.Projects {
		cs, err := config.ProjectSet(proj.Id)
		if err != nil {
			return nil, err
		}
		cfgsets[i] = string(cs)
	}
	return cfgsets, nil
}

// getAllServiceCfgSets returns all "service/*" config sets.
func getAllServiceCfgSets(ctx context.Context, cfgLoc *cfgcommonpb.GitilesLocation) ([]string, error) {
	if cfgLoc == nil {
		return nil, nil
	}
	host, project, err := gitiles.ParseRepoURL(cfgLoc.Repo)
	if err != nil {
		return nil, errors.Annotate(err, "invalid gitiles repo: %s", cfgLoc.Repo).Err()
	}
	gitilesClient, err := clients.NewGitilesClient(ctx, host, "")
	if err != nil {
		return nil, errors.Annotate(err, "failed to create a gitiles client").Err()
	}

	res, err := gitilesClient.ListFiles(ctx, &gitilespb.ListFilesRequest{
		Project:    project,
		Committish: cfgLoc.Ref,
		Path:       cfgLoc.Path,
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to call Gitiles to list files").Err()
	}
	var cfgSets []string
	for _, f := range res.GetFiles() {
		if f.Type != git.File_TREE {
			continue
		}
		cs, err := config.ServiceSet(f.GetPath())
		if err != nil {
			logging.Errorf(ctx, "skip importing service config: %s", err)
			continue
		}
		cfgSets = append(cfgSets, string(cs))
	}
	return cfgSets, nil
}

// getGlobalConfigLoc return the root of gitiles location where it stores all
// registered services in Luci Config.
// TODO(crbug.com/1446839): implement this func once we figure out how to port
// GlobalConfig and GlobalConfigRoot related functionality to v2.
func getGlobalConfigLoc() (*cfgcommonpb.GitilesLocation, error) {
	return &cfgcommonpb.GitilesLocation{
		Repo: "https://chrome-internal.googlesource.com/infradata/config",
		Ref:  "main",
		Path: "dev-configs",
	}, nil
}
