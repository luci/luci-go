// Copyright 2021 The LUCI Authors.
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

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/cfgclient"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// GetSettingsCfg fetches the settings.cfg from luci-config.
func GetSettingsCfg(ctx context.Context) (*pb.SettingsCfg, error) {
	lucicfg := cfgclient.Client(ctx)
	cfg, err := lucicfg.GetConfig(ctx, "services/${appid}", "settings.cfg", false)
	if err != nil {
		return nil, errors.Annotate(err, "loading settings.cfg from luci-config").Err()
	}
	settingsCfg := &pb.SettingsCfg{}
	if err := proto.UnmarshalText(cfg.Content, settingsCfg); err != nil {
		return nil, errors.Annotate(err, "unmarshalling Buildbucket SettingsCfg proto").Err()
	}
	return settingsCfg, nil
}
