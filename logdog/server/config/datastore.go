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
	"sync"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	luciproto "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/logdog/api/config/svcconfig"
)

const (
	serviceConfigKind = "logdog.ServiceConfig"
	projectConfigKind = "logdog.ProjectConfig"

	serviceConfigPath = "services.cfg"
)

type cachedConfig struct {
	Kind   string      `gae:"$kind"`    // logdog.ServiceConfig or logdog.ProjectConfig
	ID     string      `gae:"$id"`      // e.g. "services.cfg" or "chromium"
	Config []byte      `gae:",noindex"` // binary-serialized proto
	Meta   config.Meta `gae:",noindex"` // mostly for Revision

	_extra datastore.PropertyMap `gae:"-,extra"`

	// Disable dscache to allow these entities be updated from GAEv2,
	// which doesn't work with dscache.
	_ datastore.Toggle `gae:"$dscache.enable,false"`
}

func (c *cachedConfig) update(cfg *config.Config, typ proto.Message) error {
	if err := luciproto.UnmarshalTextML(cfg.Content, typ); err != nil {
		return err
	}
	blob, err := proto.Marshal(typ)
	if err != nil {
		return err
	}
	c.Config = blob
	c.Meta = cfg.Meta
	return nil
}

// Sync makes sure configs in the datastore match what's in LUCI Config.
//
// Performs as much work as possible (even in presence of errors). Logs errors
// inside in addition to returning them as a single errors.MultiError.
func Sync(ctx context.Context) error {
	wg := sync.WaitGroup{}
	wg.Add(2)

	var prjErr errors.MultiError
	go func() {
		defer wg.Done()
		if prjErr = syncProjectConfigs(ctx); prjErr != nil {
			logging.Errorf(ctx, "Failed to sync project configs: %s", prjErr)
		}
	}()

	var svcErr error
	go func() {
		defer wg.Done()
		if svcErr = syncServiceConfig(ctx); svcErr != nil {
			logging.Errorf(ctx, "Failed to sync services.cfg: %s", svcErr)
		}
	}()

	wg.Wait()

	// Reuse projErr as our final multierror.
	if svcErr != nil {
		prjErr = append(prjErr, svcErr)
	}
	if len(prjErr) != 0 {
		return prjErr
	}
	return nil
}

func syncProjectConfigs(ctx context.Context) (merr errors.MultiError) {
	wg := sync.WaitGroup{}
	wg.Add(2)

	// Fetch freshest configs from the LUCI Config.
	var freshMap map[string]config.Config
	var freshErr error
	go func() {
		defer wg.Done()
		var fresh []config.Config
		fresh, freshErr = cfgclient.Client(ctx).GetProjectConfigs(ctx, "${appid}.cfg", false)
		if freshErr == nil {
			freshMap = make(map[string]config.Config, len(fresh))
			for _, cfg := range fresh {
				if p := cfg.ConfigSet.Project(); p != "" {
					freshMap[p] = cfg
				}
			}
		}
	}()

	// Grab existing configs in the datastore.
	var existingMap map[string]*cachedConfig
	var existingErr error
	go func() {
		defer wg.Done()
		var existing []*cachedConfig
		existingErr = datastore.GetAll(ctx, datastore.NewQuery(projectConfigKind), &existing)
		if existingErr == nil {
			existingMap = make(map[string]*cachedConfig, len(existing))
			for _, ent := range existing {
				existingMap[ent.ID] = ent
			}
		}
	}()

	wg.Wait()

	if freshErr != nil {
		merr = append(merr, errors.Fmt("failed to fetch project configs from LUCI Config: %w", freshErr))
	}
	if existingErr != nil {
		merr = append(merr, errors.Fmt("failed to fetch cached project configs from datastore: %w", existingErr))
	}
	if len(merr) != 0 {
		return merr
	}

	// Create new and update existing entities.
	var toPut []*cachedConfig
	for projectID, cfg := range freshMap {
		ent, ok := existingMap[projectID]
		if !ok {
			ent = &cachedConfig{
				Kind: projectConfigKind,
				ID:   projectID,
			}
		}

		if ent.Meta.Revision == cfg.Meta.Revision {
			logging.Infof(ctx, "Project config of %q is up-to-date at rev %s", projectID, cfg.Meta.Revision)
			continue
		}

		logging.Infof(ctx, "Updating project config of %q: %s => %s", projectID, ent.Meta.Revision, cfg.Meta.Revision)
		if err := ent.update(&cfg, &svcconfig.ProjectConfig{}); err != nil {
			logging.Errorf(ctx, "Skipping bad project config %q at rev %s: %s", projectID, cfg.Meta.Revision, err)
			merr = append(merr, errors.Fmt("bad project config %q at rev %s: %w", projectID, cfg.Meta.Revision, err))
		} else {
			toPut = append(toPut, ent)
		}
	}

	if err := datastore.Put(ctx, toPut); err != nil {
		merr = append(merr, errors.Fmt("failed to update some project configs in datastore: %w", err))
	}

	// Delete stale entities.
	var toDelete []*datastore.Key
	for projectID, ent := range existingMap {
		if _, ok := freshMap[projectID]; !ok {
			toDelete = append(toDelete, datastore.KeyForObj(ctx, ent))
		}
	}

	if err := datastore.Delete(ctx, toDelete); err != nil {
		merr = append(merr, errors.Fmt("failed to delete some stale project configs: %w", err))
	}

	return merr
}

func syncServiceConfig(ctx context.Context) error {
	cfg, err := cfgclient.Client(ctx).GetConfig(ctx, "services/${appid}", serviceConfigPath, false)
	if err != nil {
		return errors.Fmt("failed to fetch service config from LUCI Config: %w", err)
	}

	ent := cachedConfig{
		Kind: serviceConfigKind,
		ID:   serviceConfigPath,
	}
	if err := datastore.Get(ctx, &ent); err != nil && err != datastore.ErrNoSuchEntity {
		return errors.Fmt("failed to fetch service config from datastore: %w", err)
	}

	if ent.Meta.Revision == cfg.Meta.Revision {
		logging.Infof(ctx, "Service config %q is up-to-date at rev %s", serviceConfigPath, cfg.Meta.Revision)
		return nil
	}
	logging.Infof(ctx, "Updating service config %q: %s => %s", serviceConfigPath, ent.Meta.Revision, cfg.Meta.Revision)
	if err := ent.update(cfg, &svcconfig.Config{}); err != nil {
		return errors.Fmt("bad service config at rev %s: %w", cfg.Meta.Revision, err)
	}

	if err := datastore.Put(ctx, &ent); err != nil {
		return errors.Fmt("failed to store updated config: %w", err)
	}
	return nil
}

// fromDatastore fetches the config from datastore and deserializes it.
//
// Returns datastore.ErrNoSuchEntity if the config doesn't exist, a transient
// error if the operation fails or a fatal error if the proto can't be
// deserialized.
func fromDatastore(ctx context.Context, kind, id string, msg proto.Message) error {
	ctx = datastore.WithoutTransaction(info.MustNamespace(ctx, ""))
	ent := cachedConfig{Kind: kind, ID: id}
	switch err := datastore.Get(ctx, &ent); {
	case err == datastore.ErrNoSuchEntity:
		return err
	case err != nil:
		return transient.Tag.Apply(errors.Fmt("failed to access datastore: %w", err))
	default:
		return proto.Unmarshal(ent.Config, msg)
	}
}
