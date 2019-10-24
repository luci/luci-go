// Copyright 2019 The LUCI Authors.
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

package monitoring

import (
	"context"
	"net/http"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/appengine/gaeconfig"
	"go.chromium.org/luci/config/impl/remote"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/config/v1"
)

const wlID = "ClientMonitoringWhitelist"

// Datastore representation of api.ClientMonitoringConfig.
type clientMonitoringConfig struct {
	IPWhitelist string
	Label       string
}

// Datastore representation of api.ClientMonitoringWhitelist.
type clientMonitoringWhitelist struct {
	_kind    string                   `gae:"$kind,ClientMonitoringWhitelist"`
	_extra   datastore.PropertyMap    `gae:"-,extra"`
	ID       string                   `gae:"$id"`
	Entries  []clientMonitoringConfig `gae:"entries,noindex"`
	Revision string                   `gae:"revision,noindex"`
}

const cfgFile = "monitoring.cfg"

func importConfig(ctx context.Context, cli config.Interface) error {
	rev := ""
	wl := &api.ClientMonitoringWhitelist{}
	switch cfg, err := cli.GetConfig(ctx, cfgclient.CurrentServiceConfigSet(ctx), cfgFile, false); {
	case err == config.ErrNoConfig:
		logging.Debugf(ctx, "%q not found; assuming empty", cfgFile)
	case err != nil:
		return errors.Annotate(err, "failed to fetch %q", cfgFile).Err()
	default:
		rev = cfg.Revision
		if err := proto.UnmarshalText(cfg.Content, wl); err != nil {
			return errors.Annotate(err, "failed to parse %q", cfgFile).Err()
		}
	}

	ent := &clientMonitoringWhitelist{
		ID: wlID,
	}
	if err := datastore.Get(ctx, ent); err != nil && err != datastore.ErrNoSuchEntity {
		return err
	}
	if ent.Revision == rev {
		return nil
	}
	logging.Debugf(ctx, "found %q revision %q", cfgFile, rev)
	ent = &clientMonitoringWhitelist{
		ID:       wlID,
		Entries:  make([]clientMonitoringConfig, len(wl.GetClientMonitoringConfig())),
		Revision: rev,
	}
	for i, m := range wl.GetClientMonitoringConfig() {
		ent.Entries[i].IPWhitelist = m.IpWhitelist
		ent.Entries[i].Label = m.Label
	}
	return datastore.Put(ctx, ent)
}

func ImportConfig(ctx context.Context) error {
	s, err := gaeconfig.FetchCachedSettings(ctx)
	if err != nil {
		return err
	}
	cli := remote.New(s.ConfigServiceHost, false, func(ctx context.Context) (*http.Client, error) {
		t, err := auth.GetRPCTransport(ctx, auth.AsSelf)
		if err != nil {
			return nil, err
		}
		return &http.Client{Transport: t}, nil
	})
	return importConfig(ctx, cli)
}

// monitoringConfig returns the *clientMonitoringConfig which applies to the
// current auth.State, or nil if there isn't one.
func monitoringConfig(ctx context.Context) (*clientMonitoringConfig, error) {
	ent := &clientMonitoringWhitelist{
		ID: wlID,
	}
	switch err := datastore.Get(ctx, ent); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(err)
	}
	for _, cfg := range ent.Entries {
		switch ok, err := auth.IsInWhitelist(ctx, cfg.IPWhitelist); {
		case err != nil:
			return nil, err
		case ok:
			return &cfg, nil
		}
	}
	return nil, nil
}
