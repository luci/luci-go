// Copyright 2023 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package realmsinternals

import (
	"context"
	"fmt"
	"sync"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/common/errors"
	realmsconf "go.chromium.org/luci/common/proto/realms"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/auth/service/protocol"

	"golang.org/x/sync/errgroup"
)

// RealmsCfgRev is information about fetched or previously processed realms.cfg.
// Comes either from LUCI Config (then `config_body` is set, but `perms_rev`
// isn't) or from the datastore (then `perms_rev` is set, but `config_body`
// isn't). All other fields are always set.
type RealmsCfgRev struct {
	projectID    string
	configRev    string
	configDigest string

	// Thes two are mutually exclusive
	configBody []byte
	permsRev   string
}

const (
	Cria             = "services/chrome-infra-auth"
	CriaDev          = "services/chrome-infra-auth-dev"
	RealmsCfgPath    = "realms.cfg"
	RealmsDevCfgPath = "realms-dev.cfg"
)

type realmsMap struct {
	mu     *sync.Mutex
	cfgMap map[string]*config.Config
}

func CheckConfigChanges(permissionsDB *permissions.PermissionsDB, latest []*RealmsCfgRev, stored []*RealmsCfgRev) {
	// TODO(cjacomet): Implement
}

func UpdateRealms(revs []*RealmsCfgRev) {
	// TODO(cjacomet): Implement
}

func ExpandRealms(db *permissions.PermissionsDB, projectID string, realmsCfg *realmsconf.RealmsCfg) *protocol.Realms {
	// TODO(cjacomet): Implement
	return nil
}

// GetConfigs fetches the configs concurrently; the
// latest configs from luci-cfg, the stored config meta from datastore.
//
// Errors
//
//	ErrNoConfig -- config is not found
//	annotated error -- for all other errors
func GetConfigs(ctx context.Context) ([]*RealmsCfgRev, []*RealmsCfgRev, error) {
	projects, err := cfgclient.ProjectsWithConfig(ctx, cfgPath(ctx))
	if err != nil {
		return nil, nil, err
	}

	// client to fetch configs
	client := cfgclient.Client(ctx)
	latestRevs := make([]*RealmsCfgRev, len(projects)+1)

	eg, childCtx := errgroup.WithContext(ctx)

	latestMap := realmsMap{
		mu:     &sync.Mutex{},
		cfgMap: make(map[string]*config.Config, len(projects)+1),
	}

	storedMeta := []*model.AuthProjectRealmsMeta{}

	self := func(ctx context.Context) string {
		if cfgPath(ctx) == RealmsDevCfgPath {
			return CriaDev
		}
		return Cria
	}

	// Get Project Metadata configs stored in datastore
	eg.Go(func() error {
		storedMeta, err = model.GetAllAuthProjectRealmsMeta(ctx)
		if err != nil {
			return err
		}
		return nil
	})

	// Get self config i.e. services/chrome-infra-auth/realms.cfg
	eg.Go(func() error {
		return latestMap.getLatestConfig(childCtx, client, self(ctx))

	})
	// Get Project Configs
	for _, project := range projects {
		project := project
		eg.Go(func() error {
			return latestMap.getLatestConfig(childCtx, client, project)
		})
	}

	err = eg.Wait()
	if err != nil {
		return nil, nil, err
	}

	storedRevs := make([]*RealmsCfgRev, len(storedMeta))

	idx := 0
	for projID, cfg := range latestMap.cfgMap {
		latestRevs[idx] = &RealmsCfgRev{
			projectID:    projID,
			configRev:    cfg.Revision,
			configDigest: cfg.ContentHash,
			configBody:   []byte(cfg.Content),
		}
		idx++
	}

	for i, meta := range storedMeta {
		storedRevs[i] = &RealmsCfgRev{
			projectID:    meta.ID,
			configRev:    meta.ConfigRev,
			configDigest: meta.ConfigDigest,
			permsRev:     meta.PermsRev,
		}
	}

	if err != nil {
		return nil, nil, err
	}

	return latestRevs, storedRevs, nil
}

// getLatestConfig fetches the most up to date realms.cfg for a given project, unless
// fetching the config for self, in which case it fetches the service config. The configs are
// written to a map mapping K: project name (string) -> V: *config.Config.
func (r *realmsMap) getLatestConfig(ctx context.Context, client config.Interface, project string) error {
	project, cfgSet, err := r.cfgSet(project)
	if err != nil {
		return err
	}

	cfg, err := client.GetConfig(ctx, cfgSet, cfgPath(ctx), false)
	if err != nil {
		return errors.Annotate(err, "failed to fetch realms.cfg for %s", project).Err()
	}

	r.mu.Lock()
	r.cfgMap[project] = cfg
	r.mu.Unlock()

	return nil
}

// cfgPath is a helper function to know which cfg, depending on dev or prod env.
func cfgPath(ctx context.Context) string {
	if info.IsDevAppServer(ctx) {
		return RealmsDevCfgPath
	}
	return RealmsCfgPath
}

// cfgSet is a helper function to know which configSet to use, this is necessary for
// getting the realms cfg for CrIA or CrIADev since the realms.cfg is stored as
// a service config instead of a project config.
func (r *realmsMap) cfgSet(project string) (string, config.Set, error) {
	if project == Cria || project == CriaDev {
		r.mu.Lock()
		defer r.mu.Unlock()
		if _, ok := r.cfgMap[realms.InternalProject]; ok {
			return "", "", fmt.Errorf("unexpected LUCI Project: %s", realms.InternalProject)
		}
		return realms.InternalProject, config.Set(project), nil
	}

	ps, err := config.ProjectSet(project)
	if err != nil {
		return "", "", err
	}
	return project, ps, nil
}
