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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/appengine/gaeconfig"
	"go.chromium.org/luci/config/impl/remote"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/config/v1"
)

const cfgFile = "monitoring.cfg"

func importConfig(ctx context.Context, cli config.Interface) error {
	wl := &api.ClientMonitoringWhitelist{}
	switch cfg, err := cli.GetConfig(ctx, cfgclient.CurrentServiceConfigSet(ctx), cfgFile, false); {
	case err == config.ErrNoConfig:
		logging.Debugf(ctx, "%q not found; assuming empty", cfgFile)
	case err != nil:
		return errors.Annotate(err, "failed to fetch %q", cfgFile).Err()
	default:
		logging.Debugf(ctx, "found %q revision %q", cfgFile, cfg.Revision)
		if err := proto.UnmarshalText(cfg.Content, wl); err != nil {
			return errors.Annotate(err, "failed to parse %q", cfgFile).Err()
		}
	}

	for _, m := range wl.GetClientMonitoringConfig() {
		logging.Debugf(ctx, "ip_whitelist: %q, label: %q", m.IpWhitelist, m.Label)
	}
	return nil
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
